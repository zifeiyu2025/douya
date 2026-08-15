package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"douya/internal/apperror"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/rag"
	"douya/internal/secrets"
	"douya/internal/store"
	"douya/internal/system"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ===== startup 子函数：将原超长 startup 拆分为职责单一的子函数 =====

// cleanupOrphanProcesses 清理上次进程残留的孤儿 llama-server。
// 生活类比：开店前先清理前一天遗留的垃圾，避免影响今天的运营。
// P2.5 修复：只清理本应用 runtime 目录下的 llama-server，避免误杀用户手动启动的同名进程。
func (a *App) cleanupOrphanProcesses() {
	llm.KillOrphanLlamaServers(filepath.Join(appDir(), "runtime"))
}

// initHardware 检测硬件信息（CPU/GPU/内存等），供后续选择推理后端使用。
func (a *App) initHardware() {
	a.hwInfo = system.DetectHardware()
}

// loadAndValidateConfig 加载配置文件，失败时弹窗提示并返回 error。
// 返回 cfgPath 供后续迁移使用，配置已通过 setConfig 缓存到 App。
func (a *App) loadAndValidateConfig(ctx context.Context) (string, error) {
	cfgPath := filepath.Join(appDir(), "config.json")
	loadedCfg, err := config.Load(cfgPath)
	if err != nil {
		zlog.Error().Err(err).Msg("load config failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:  runtime.ErrorDialog,
			Title: "配置加载失败",
			Message: fmt.Sprintf(
				"加载配置文件失败：\n%v\n\n"+
					"可能原因：\n"+
					"• 配置文件被其他程序占用\n"+
					"• 磁盘空间不足或无写入权限\n"+
					"• 文件系统错误\n\n"+
					"建议：\n"+
					"1. 关闭其他可能占用该文件的程序\n"+
					"2. 检查磁盘空间和写入权限\n"+
					"3. 备份后删除配置文件：%s\n"+
					"   （下次启动会自动创建默认配置）",
				err, cfgPath),
		})
		return "", err
	}
	a.setConfig(loadedCfg)
	return cfgPath, nil
}

// ensureDirectories 确保 models 和 runtime 目录存在（不存在则自动创建）。
// 返回 runtimeDir 和 modelsDir 路径，以及可能的 error。
// 生活类比：开门营业前先确保仓库（runtime）和展厅（models）建好，
// 即使是空仓也能让后续流程正常运转（后端按需下载、模型稍后放入）。
//
// 失败处理：目录创建失败属于致命错误（后续流程都依赖这两个目录），
// 内部已弹窗提示用户原因和建议，调用方收到 error 后 forceQuit 即可。
func (a *App) ensureDirectories(ctx context.Context) (runtimeDir, modelsDir string, err error) {
	runtimeDir = filepath.Join(appDir(), "runtime")
	modelsDir = filepath.Join(appDir(), "models")
	for _, dir := range []string{runtimeDir, modelsDir} {
		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
			zlog.Error().Err(mkErr).Str("dir", dir).Msg("[startup] 创建目录失败")
			_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
				Type:  runtime.ErrorDialog,
				Title: "目录创建失败",
				Message: fmt.Sprintf(
					"创建目录失败：\n%v\n\n"+
						"目标目录：%s\n\n"+
						"可能原因：\n"+
						"• 磁盘空间不足\n"+
						"• 没有写入权限\n"+
						"• 路径包含非法字符\n\n"+
						"建议：\n"+
						"1. 检查磁盘空间是否充足\n"+
						"2. 确认应用目录有写入权限\n"+
						"3. 尝试以管理员身份运行应用",
					mkErr, dir),
			})
			return "", "", mkErr
		}
	}
	return runtimeDir, modelsDir, nil
}

// installBackend 解析并安装推理后端，处理 runtime 缺失时的下载弹窗。
// 返回 shouldReturn：true 表示 startup 应直接返回（用户取消下载或已触发异步下载）。
//
// 流程：
//  1. 根据硬件和配置解析 auto 为具体后端（如 cuda/hip/cpu）
//  2. EnsureBackendInstalled 确保后端已解压到 runtime 目录（幂等：已安装则直接返回路径）
//  3. 如果安装失败，尝试回退到 CPU 后端
//  4. 将解析结果缓存到 App 结构体，供 validatePaths 和 buildServerConfig 复用
//  5. 若 runtime 缺失，弹窗询问用户是否下载；用户选「是」则异步下载并返回 true
func (a *App) installBackend(ctx context.Context, runtimeDir string) bool {
	cfg := a.getConfig()

	// fromGpuTypeChoice 标记"本次后端来自灰色地带用户选择"。
	// 这类后端如果安装/加载失败，不走默认的下载流程，而是弹询问框让用户决定是否回退 CPU。
	// 生活类比：车检员让车主自己选了驾驶模式，如果选的模式不适用（比如选了跑车模式但车不是跑车），
	// 车检员会回来问车主"要不要换成稳妥的 CPU 模式"，而不是直接让车主去买新车。
	fromGpuTypeChoice := false

	// ===== 灰色地带检测：auto 模式 + GPUType=unknown 时让用户选择后端 =====
	// 触发条件：
	//   1. 用户未手动指定后端（BackendType == "auto"）
	//   2. 检测到 AMD 或 Intel GPU 但显卡状态未知（GPUType == "unknown"）
	// 此时自动检测无法做出可靠决策，让用户在 UI 上选择，避免错误启用 GPU 导致 OOM。
	//
	// 完整逻辑链：
	//   - 检测到独显 → 加载对应厂商后端（CUDA/HIP/SYCL）
	//   - 检测到核显 → 直接走 CPU 后端（HasGPU=false）
	//   - 灰色地带 → 弹框让用户选择（本分支）
	//   - 用户选择的后端尝试失败 → 询问是否回退 CPU（下方 fromGpuTypeChoice 分支）
	if (cfg.BackendType == "" || cfg.BackendType == string(llm.BackendAuto)) &&
		a.hwInfo != nil && a.hwInfo.GPUType == system.GPUTypeUnknown &&
		(a.hwInfo.HasAMDGPU || a.hwInfo.HasIntelGPU) {
		chosen := a.waitForGpuTypeChoice(ctx)
		if chosen != "" {
			// 用户已选择后端：写入配置并持久化，下次启动不再弹窗
			zlog.Info().Str("chosen", chosen).Msg("[startup] 用户选择了推理后端，保存到配置")
			if err := a.updateConfig(func(c *config.Config) error {
				c.BackendType = chosen
				return nil
			}); err != nil {
				zlog.Warn().Err(err).Msg("[startup] 保存用户选择的后端失败，将仅本次生效")
			} else {
				cfgPath := filepath.Join(appDir(), "config.json")
				if saveErr := config.Save(cfgPath, a.getConfig()); saveErr != nil {
					zlog.Warn().Err(saveErr).Msg("[startup] 持久化配置失败，将仅本次生效")
				}
			}
			// 刷新 cfg 快照，让后续 ResolveBackendTypeWithRuntime 用新的 BackendType
			cfg = a.getConfig()
			// 标记本次后端来自灰色地带用户选择，失败时走"询问 CPU 回退"分支
			fromGpuTypeChoice = true
		}
		// chosen == "" 表示超时，继续走 auto 流程（ResolveBackendTypeWithRuntime 会按厂商推断）
	}

	// P3 改进：使用带运行时预校验的解析函数，auto 模式下优先选择已安装的后端，
	// 避免推断出未下载的后端（如 Vulkan）后走下载流程（原实现会失败再回退 CPU）
	resolvedBackend := llm.ResolveBackendTypeWithRuntime(a.hwInfo, cfg.BackendType, runtimeDir)
	serverPath, err := llm.EnsureBackendInstalled(resolvedBackend, runtimeDir, nil)
	if err != nil {
		// 灰色地带用户选择的后端失败时，区分两种情况：
		//   1. KindNotFound（zip 包未下载）→ 走下载流程（后续 validatePaths 弹下载对话框）
		//   2. 其他错误（解压失败/磁盘问题/驱动不兼容）→ 询问是否回退 CPU
		//
		// 生活类比：车检员让车主选了跑车模式，但发现跑车引擎还没进货（未下载），
		// 就告诉车主"引擎需要订购，要不要下订单？"——而不是问"要不要换 CPU 模式"。
		// 但如果引擎到了却装不上（解压失败/不兼容），才问"要不要换稳妥的 CPU 模式"。
		if fromGpuTypeChoice && resolvedBackend != llm.BackendCPU {
			if apperror.Is(err, apperror.KindNotFound) {
				// zip 包未下载：让 serverPath 保持空，后续 validatePaths 会弹下载对话框
				zlog.Warn().Err(err).Str("backend", resolvedBackend.String()).
					Msg("[startup] 灰色地带用户选择的后端未下载，走下载流程")
			} else {
				// 其他失败（解压失败/磁盘问题）：询问是否回退 CPU
				zlog.Warn().Err(err).Str("backend", resolvedBackend.String()).
					Msg("[startup] 灰色地带用户选择的后端安装失败，询问是否回退 CPU")
				if a.askUseCPUFallback(ctx, resolvedBackend.String()) {
					// 用户同意回退 CPU
					fallbackPath, cpuErr := llm.EnsureBackendInstalled(llm.BackendCPU, runtimeDir, nil)
					if cpuErr != nil {
						zlog.Error().Err(cpuErr).Msg("[startup] CPU 后端也安装失败，validatePaths 将报告缺失文件")
					} else {
						serverPath = fallbackPath
						resolvedBackend = llm.BackendCPU
						// 更新配置为 CPU，避免下次启动再弹灰色地带对话框
						if updateErr := a.updateConfig(func(c *config.Config) error {
							c.BackendType = string(llm.BackendCPU)
							return nil
						}); updateErr != nil {
							zlog.Warn().Err(updateErr).Msg("[startup] 更新配置为 CPU 失败")
						} else {
							cfgPath := filepath.Join(appDir(), "config.json")
							if saveErr := config.Save(cfgPath, a.getConfig()); saveErr != nil {
								zlog.Warn().Err(saveErr).Msg("[startup] 持久化 CPU 配置失败")
							}
						}
					}
				} else {
					// 用户拒绝回退 CPU：退出应用
					zlog.Info().Msg("[startup] 用户拒绝回退 CPU，退出应用")
					a.forceQuit()
					return true
				}
			}
		} else {
			// 原有逻辑：auto 模式静默回退 CPU，手动模式走下载流程
			isAuto := cfg.BackendType == "" || cfg.BackendType == string(llm.BackendAuto)
			zlog.Warn().Err(err).Str("backend", resolvedBackend.String()).Bool("auto", isAuto).
				Msg("[startup] 后端安装失败")
			// 自动模式 + NVIDIA 独显 + 原生 CUDA：优先下载对应 CUDA 版本，
			// 不要静默降级到已装的 CPU 兜底（否则用户删除 cuda 目录后无法重新拉取 CUDA）。
			skipSilentCPUFallback := isAuto &&
				resolvedBackend == llm.BackendCUDA &&
				a.hwInfo != nil && a.hwInfo.GPUVendor == "nvidia"
			if isAuto && resolvedBackend != llm.BackendCPU && !skipSilentCPUFallback {
				fallbackPath, cpuErr := llm.EnsureBackendInstalled(llm.BackendCPU, runtimeDir, nil)
				if cpuErr != nil {
					zlog.Error().Err(cpuErr).Msg("[startup] CPU 后端也安装失败，validatePaths 将报告缺失文件")
				} else {
					serverPath = fallbackPath
					resolvedBackend = llm.BackendCPU
				}
			}
		}
	}

	a.setResolvedBackend(resolvedBackend, serverPath)

	// CUDA 后端：确保 cudart 包也已解压（幂等）
	// 主包已安装但 cudart 包未解压时，validatePaths 会检测到厂商 DLL 缺失，
	// 导致无限提示下载。此处主动解压已有的 cudart zip 包，避免重复下载。
	// 生活类比：电脑装好了但外设配件包还没拆，开机前先把配件包装好。
	if resolvedBackend == llm.BackendCUDA && serverPath != "" {
		if cudartErr := llm.EnsureCudartInstalled(runtimeDir, nil); cudartErr != nil {
			zlog.Warn().Err(cudartErr).Msg("[startup] cudart 包未安装，validatePaths 将报告缺失")
		}
	}

	// ===== 统一 runtime 完整性检测（合并原两处弹窗为一个） =====
	// 原逻辑中存在两处弹窗：serverPath 为空时弹一次，validatePaths 报告缺失时再弹一次。
	// 现合并为单次弹窗：只要 serverPath 为空 或 validatePaths 报告 runtime 缺失，统一询问用户。
	// 生活类比：提车前只做一次全面检查，发现问题一次性告知，不会先问"发动机没装要不要装"，
	// 紧接着又问"变速箱也缺要不要买"——那样会让顾客被反复打断。
	checkResult := a.validatePaths()
	needDownload := serverPath == "" || checkResult.HasRuntimeIssues()

	if !needDownload {
		return false
	}

	info := llm.GetBackendInfo(resolvedBackend)
	gpuName := "未知"
	if a.hwInfo != nil && a.hwInfo.GPUName != "" {
		gpuName = a.hwInfo.GPUName
	}

	// 构造缺失文件清单：validatePaths 有结果时用其清单，否则用通用提示
	var missingMsg strings.Builder
	if checkResult.HasRuntimeIssues() {
		for _, p := range checkResult.RuntimeMissing {
			missingMsg.WriteString("  ❌ ")
			missingMsg.WriteString(p)
			missingMsg.WriteString("\n")
		}
	} else {
		missingMsg.WriteString("  ❌ 推理引擎（llama-server.exe）及依赖文件缺失\n")
	}

	askMsg := fmt.Sprintf(
		"检测到您的显卡：%s\n"+
			"推荐后端：%s\n\n"+
			"runtime 目录缺少以下文件：\n%s\n"+
			"是否从 GitHub 自动下载并安装？\n"+
			"（来源：https://github.com/ggml-org/llama.cpp/releases）\n\n"+
			"点击「是」将在启动界面显示下载进度，完成后自动重启应用。\n"+
			"点击「否」将直接退出应用。",
		gpuName, info.DisplayName, missingMsg.String())

	dlResult, dlErr := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.QuestionDialog,
		Title:   "缺少推理后端，是否下载？",
		Message: askMsg,
		Buttons: []string{"是", "否"},
	})
	// 对话框调用失败时 dlResult 为空，白名单退出逻辑会走默认下载路径；此处仅留痕诊断
	if dlErr != nil {
		zlog.Error().Err(dlErr).Msg("[startup] MessageDialog 调用失败，将按默认流程继续（下载）")
	}

	// 记录返回值用于调试（Wails MessageDialog 在不同 Windows 版本下返回值可能有编码差异）
	zlog.Info().Str("dlResult", dlResult).Msg("[startup] MessageDialog 返回值")

	// Windows 上 QuestionDialog 默认显示"是/否"按钮：
	//   - 点"是" → 下载（默认行为，也兼容"Yes"、"下载"等返回值）
	//   - 点"否" → 退出（明确匹配"否"、"No"、"退出"等）
	// 逻辑采用"白名单退出"：只有明确选择否定意图才退出，避免编码不匹配导致误退出
	if dlResult == "否" || dlResult == "No" || dlResult == "退出" || dlResult == "Cancel" {
		zlog.Info().Msg("[startup] 用户取消下载，退出应用")
		a.forceQuit()
		return true
	}

	// 用户选择「是」：异步下载+安装（带重试 3 次），startup 直接返回
	// 下载进度通过事件推送到前端，在启动动效中展示
	zlog.Info().Str("backend", resolvedBackend.String()).Msg("[startup] 用户选择从 GitHub 下载后端")

	// 通知前端进入下载阶段（splashScreen 将切换到 downloading 阶段并显示进度条）
	runtime.EventsEmit(ctx, EventBackendDownloadStart, map[string]any{
		"backend": resolvedBackend.String(),
		"name":    info.DisplayName,
	})

	a.setResolvedBackend(resolvedBackend, "")

	// 异步下载+安装（CUDA 额外下载 cudart 包，失败重试最多 3 次）
	backendToDownload := resolvedBackend
	go func() {
		// 防止 panic 导致整个进程崩溃（下载涉及网络和文件 IO，可能 panic）
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[startup] 下载后端 goroutine panic")
				runtime.EventsEmit(a.ctx, EventBackendDownloadProgress, llm.DownloadProgress{
					Backend: backendToDownload,
					Status:  "failed",
					Error:   fmt.Sprintf("下载后端发生内部错误：%v", r),
				})
			}
		}()
		if dlErr := a.downloadBackendWithRetry(backendToDownload, runtimeDir, 3); dlErr != nil {
			zlog.Error().Err(dlErr).Str("backend", backendToDownload.String()).Msg("[startup] 下载后端失败（已重试 3 次）")
			runtime.EventsEmit(a.ctx, EventBackendDownloadProgress, llm.DownloadProgress{
				Backend: backendToDownload,
				Status:  "failed",
				Error:   fmt.Sprintf("已重试 3 次仍失败：%v", dlErr),
			})
		}
	}()

	// startup 直接返回，跳过后续流程（数据库/llama-server 启动等）
	// 应用窗口正常显示，前端监听下载事件展示进度，完成后提示用户重启
	return true
}

// handleMissingModels 检查 models 目录，若为空则弹窗提示用户下载模型。
// 不阻塞启动，用户点击「确定」后继续进入界面。
//
// runtime 已完整时才检查 models，避免与 runtime 弹窗叠加。
func (a *App) handleMissingModels(ctx context.Context) {
	checkResult := a.validatePaths()
	if !checkResult.HasModelIssues() {
		return
	}

	var msg strings.Builder
	msg.WriteString("⚠️ 还没有可用的 AI 模型，暂时无法对话。\n\n")
	if checkResult.ModelsDirMissing {
		fmt.Fprintf(&msg, "模型目录（将自动创建）：%s\n", checkResult.ModelsDir)
	} else {
		fmt.Fprintf(&msg, "模型目录：%s\n", checkResult.ModelsDir)
		msg.WriteString("该目录下还没有 .gguf 模型文件。\n")
	}
	msg.WriteString("\n")
	msg.WriteString("【如何下载模型】\n")
	msg.WriteString("豆芽使用 GGUF 格式的模型文件，推荐从以下站点下载（国内访问快）：\n\n")
	msg.WriteString("1. ModelScope（魔搭社区，阿里出品，中文友好）\n")
	msg.WriteString("   https://www.modelscope.cn/\n")
	msg.WriteString("   搜索关键词：GGUF\n\n")
	msg.WriteString("2. HF 镜像（HuggingFace 国内镜像站）\n")
	msg.WriteString("   https://hf-mirror.com/\n")
	msg.WriteString("   搜索关键词：gguf\n\n")
	msg.WriteString("【推荐的入门模型】（选 Q4_K_M 量化，速度与效果均衡）\n")
	msg.WriteString("   - Qwen3-8B（通义千问，中文最强入门）\n")
	msg.WriteString("   - Gemma-3-4B（轻量，适合低配机器）\n")
	msg.WriteString("   - Llama-3.1-8B（Meta 出品，英文能力强）\n\n")
	msg.WriteString("【下载后如何使用】\n")
	msg.WriteString("   1. 下载 .gguf 文件（通常 3~6 GB）\n")
	msg.WriteString("   2. 将文件放入上面的模型目录\n")
	msg.WriteString("   3. 重启豆芽，在顶部模型下拉框选择即可\n\n")
	msg.WriteString("点击「确定」先进入界面，模型文件可以稍后再放入。")

	zlog.Warn().Str("models_dir", checkResult.ModelsDir).
		Bool("dir_missing", checkResult.ModelsDirMissing).
		Bool("empty", checkResult.ModelsEmpty).
		Msg("[startup] models directory empty, continuing startup")
	_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.WarningDialog,
		Title:   "模型目录为空",
		Message: msg.String(),
	})
	runtime.EventsEmit(ctx, EventServerStatus, llm.ServerStatus{
		Running:    false,
		ModelReady: false,
		Error:      "模型目录为空，请下载 .gguf 模型文件后放入 models 目录",
	})
}

// loadSecrets 加载加密密钥，用于对话内容和 API Key 等敏感数据的加密存储。
// 若密钥文件已损坏（长度不为 32 字节），返回 error 阻止启动——
// 因为覆盖会导致所有用旧密钥加密的历史数据永久无法解密，此时必须由用户手动处理。
func (a *App) loadSecrets(ctx context.Context) error {
	keyPath := filepath.Join(appDir(), "data", ".enc_key")
	key, err := secrets.LoadOrCreateKey(keyPath)
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] load encryption key failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "加密密钥加载失败",
			Message: fmt.Sprintf("加载加密密钥失败：\n%v\n\n请按上述提示处理后重新启动应用。", err),
		})
		return err
	}
	a.encKey = key
	return nil
}

// initDatabase 初始化数据库，失败时弹窗提示并返回 error。
func (a *App) initDatabase(ctx context.Context, dbPath string) error {
	db, err := store.Init(dbPath, a.encKey)
	if err != nil {
		zlog.Error().Err(err).Msg("init database failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:  runtime.ErrorDialog,
			Title: "数据库初始化失败",
			Message: fmt.Sprintf(
				"初始化数据库失败：\n%v\n\n"+
					"可能原因：\n"+
					"• 数据库文件损坏（异常退出可能导致）\n"+
					"• 磁盘空间不足\n"+
					"• 没有写入权限\n\n"+
					"建议：\n"+
					"1. 检查磁盘空间是否充足\n"+
					"2. 备份后删除数据库文件：%s\n"+
					"   （下次启动会自动重建，但历史对话记录会丢失）\n"+
					"3. 确认应用目录有写入权限",
				err, dbPath),
		})
		return err
	}
	a.db = db
	return nil
}

// migrateSearchEngines 将 config.json 中的搜索引擎 API Key 迁移到数据库。
// 幂等：仅在数据库中不存在对应 key 时迁移。
func (a *App) migrateSearchEngines(cfgPath string) {
	raw, rawErr := config.LoadRaw(cfgPath)
	if rawErr != nil {
		return
	}
	se, ok := raw["search_engines"]
	if !ok {
		return
	}
	seMap, ok := se.(map[string]any)
	if !ok {
		return
	}
	migrated := false
	setFn := func(key, value string) error {
		return a.service.SetEncryptedSetting(key, value)
	}
	getFn := func(key string) string {
		v, _ := a.service.GetEncryptedSetting(key)
		return v
	}
	if v, ok := seMap["ollama_api_key"]; ok && v != "" {
		if existing := getFn("search_ollama_api_key"); existing == "" {
			if err := setFn("search_ollama_api_key", fmt.Sprintf("%v", v)); err != nil {
				zlog.Warn().Err(err).Msg("[startup] 迁移 ollama_api_key 失败")
			} else {
				migrated = true
			}
		}
	}
	if v, ok := seMap["tavily_api_key"]; ok && v != "" {
		if existing := getFn("search_tavily_api_key"); existing == "" {
			if err := setFn("search_tavily_api_key", fmt.Sprintf("%v", v)); err != nil {
				zlog.Warn().Err(err).Msg("[startup] 迁移 tavily_api_key 失败")
			} else {
				migrated = true
			}
		}
	}
	if migrated {
		zlog.Info().Msg("[startup] migrated search API keys from config.json to database")
		// 保存前校验，失败记录日志但不阻塞保存（避免阻塞搜索引擎迁移功能）
		if err := a.getConfig().Validate(); err != nil {
			zlog.Warn().Err(err).Msg("[startup] 配置校验失败（搜索引擎迁移），仍保存")
		}
		if err := config.Save(cfgPath, a.getConfig()); err != nil {
			zlog.Warn().Err(err).Msg("[startup] 保存配置失败（搜索引擎迁移）")
		}
	}
}

// initRAG 初始化 RAG（Badger-backed 向量存储 + LLM embedder）。
// 初始化失败时禁用 RAG 但不阻止启动。
func (a *App) initRAG(ctx context.Context, cfg *config.Config) {
	ragDir := filepath.Join(appDir(), "data", "rag")
	ragVS, err := rag.NewVectorStore(ragDir)
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] RAG vector store init failed (RAG disabled)")
		// RAG 失败不阻塞启动（基本对话功能仍可用），但用户应知道 RAG 不可用
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:  runtime.WarningDialog,
			Title: "知识库功能不可用",
			Message: fmt.Sprintf(
				"知识库（RAG）初始化失败，已自动禁用知识库功能。\n"+
					"基本对话功能不受影响，但无法使用文档问答。\n\n"+
					"错误信息：%v\n\n"+
					"可能原因：\n"+
					"• 知识库存储目录被占用或损坏\n"+
					"• 磁盘空间不足\n\n"+
					"建议：\n"+
					"1. 重启应用尝试自动恢复\n"+
					"2. 备份后删除目录：%s\n"+
					"   （下次启动会自动重建，已有知识库内容会丢失）",
				err, ragDir),
		})
		return
	}
	a.ragVS = ragVS
	a.ragDS = rag.NewDocumentStore(ragVS)
	// 嵌入模型：优先使用专用嵌入模型，否则使用当前聊天模型
	embedModel := cfg.EmbeddingModel
	if embedModel != "" {
		embedModel = resolvePath(embedModel)
	}
	if embedModel == "" {
		embedModel = a.currentModel()
	}
	embedder := &rag.ClientEmbedder{Client: a.getClient()}
	embedder.SetModel(embedModel)
	// 当专用嵌入模型为空时，动态获取当前聊天模型名
	embedder.SetCurrentModelFn(func() string {
		return a.currentModel()
	})
	a.ragEmbedder = embedder
	collection := cfg.RAGActiveKB
	if collection == "" {
		collection = "default"
	}
	ragEnabled := cfg.RAGEnabled
	a.service.SetRAG(ragVS, a.ragDS, embedder, collection, ragEnabled)
	zlog.Info().Str("dir", ragDir).Str("collection", collection).Str("embed_model", embedModel).Bool("enabled", ragEnabled).Msg("[startup] RAG initialized")
}

// initLogChannel 创建日志 channel 和消费者 goroutine（trackedGo 跟踪）。
// 生活类比：就像一个邮筒（logChan），邮递员（llama-server）把每封信（日志行）投进邮筒，
// 后台有一个邮局职员（消费者 goroutine）负责把信件转交给收件人（前端）。
// 用单个职员而不是每封信都派一个职员（原 go func 实现），避免 goroutine 泛滥。
func (a *App) initLogChannel() {
	a.logChan = make(chan string, 1024)
	a.trackedGo(func() {
		for {
			select {
			case <-a.rootCtx.Done():
				// shutdown 信号：排空剩余日志后退出
				// 不显式 close(logChan)：SetOnLog 可能在 srv.Stop 之前仍被调用，
				// 关闭 channel 会导致 send on closed channel panic。
				// rootCtx.Done() 已足够让消费者退出，channel 随 App 一起被 GC。
				for {
					select {
					case l := <-a.logChan:
						if a.ctx != nil {
							runtime.EventsEmit(a.ctx, EventServerLog, l)
						}
					default:
						return
					}
				}
			case l := <-a.logChan:
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, EventServerLog, l)
				}
			}
		}
	})
}

// initServer 创建 llama-server 实例并配置日志/终端数据回调。
func (a *App) initServer() {
	a.serverMu.Lock()
	defer a.serverMu.Unlock()
	a.server = llm.NewServer(a.buildServerConfig())
	// 设置 llama-server 日志实时推送到前端控制台（exec.Cmd 回调模式使用）
	// 日志通过 logChan 投递给消费者 goroutine，由其统一 EventsEmit，避免每行日志创建 goroutine
	a.server.SetOnLog(func(line string) {
		// 非阻塞投递到 logChan：缓冲区满时丢弃，避免阻塞 llama-server 输出
		select {
		case a.logChan <- line:
		default:
			// 缓冲区满，丢弃日志（极端情况：日志产生速度远超消费速度）
		}
	})
	// 设置终端原始字节流推送到前端 xterm.js（ConPTY 模式使用）
	// 数据已批量合并（50ms 窗口），直接同步发送即可
	a.server.SetOnTerminalData(func(data []byte) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, EventServerTerminal, data)
		}
	})
}

// buildService 构建聊天服务：生成预设文件、同步 MCP 配置、创建 LLM client 和 service、
// 初始化 RAG、创建日志 channel 和 llama-server 实例。
func (a *App) buildService(ctx context.Context) {
	// F-3 修复：preset 文件生成失败不再静默处理，记录错误并通过事件通知前端
	// 生活类比：菜谱生成器坏了，厨师长（前端）需要知道，否则上菜时才发现没菜谱
	if err := a.generatePresetFile(); err != nil {
		zlog.Error().Err(err).Msg("[startup] generate preset file failed")
		a.presetGenFailed = true
	}

	// 同步 mcp_servers.json：让 llama-server 启动时通过 --mcp-servers-config 加载此文件，
	// 启用 /tools 端点并管理所有 MCP 子进程。
	a.ensureMcpServersFileExists()

	cfg := a.getConfig()
	a.setClient(llm.NewClient(cfg.APIBase, a.getServerAPIKey()))

	searchChain := a.buildSearchChain()

	// 填充提前创建的 service 的真实 client 和 searchChain
	a.service.UpdateClient(a.getClient())
	a.service.UpdateSearchChain(searchChain)

	// Initialize RAG (Badger-backed vector store + LLM embedder)
	a.initRAG(ctx, cfg)

	// MCP 服务器：豆芽不再自行启动 MCP 子进程，而是将配置写入 mcp_servers.json，
	// 由 llama-server 通过 --mcp-servers-config 参数加载并管理。
	// mcp_servers.json 在 startup 时通过 ensureMcpServersFileExists() 自动同步，
	// 用户在「设置 → MCP」修改配置时通过 SaveMCPServers() 重新生成。

	// 创建日志 channel 和消费者 goroutine
	a.initLogChannel()

	// 创建 llama-server 实例
	a.initServer()

	a.ready.Store(true)
}

// cleanupOrphanSessions 清理异常会话（如上次崩溃残留的对话）。
// 异步执行，不阻塞 startup；panic 会被 recover 保护，避免影响主进程。
//
// P2.4 修复：改为 trackedGo 跟踪。此前是裸 goroutine，shutdown 时 g.Wait() 不等它，
// 可能 db.Close() 执行时该 goroutine 仍在查询数据库（use-after-close）。
// 该 goroutine 是短生命周期的（一次 DB 清理即退出），纳入跟踪后 shutdown 会等待其完成，
// 消除与 db.Close 的竞争。
func (a *App) cleanupOrphanSessions(ctx context.Context) {
	// bgCtx 派生自 rootCtx，确保 shutdown 时 rootCancel 能尽早中断清理流程
	bgCtx := ctx
	if a.rootCtx != nil {
		bgCtx = a.rootCtx
	}
	a.trackedGo(func() {
		// 防止 panic 导致整个进程崩溃（启动清理涉及 DB 操作和消息解密，可能 panic）
		defer recoverLog("[startup] CleanupAbnormalConversations panic")
		if err := bgCtx.Err(); err != nil {
			zlog.Info().Msg("[startup] skip orphan session cleanup (shutting down)")
			return
		}
		removed := a.service.CleanupAbnormalConversations()
		if len(removed) > 0 {
			titles := make([]string, 0, len(removed))
			for _, ac := range removed {
				titles = append(titles, ac.Title)
			}
			zlog.Info().Int("count", len(removed)).Interface("titles", titles).Msg("[startup] removed abnormal conversations")

			a.cleanupResultMu.Lock()
			a.cleanupResult = removed
			a.cleanupResultMu.Unlock()

			runtime.EventsEmit(ctx, EventChatAbnormalCleanup, map[string]any{
				"count":   len(removed),
				"removed": removed,
			})
		}
	})
}
