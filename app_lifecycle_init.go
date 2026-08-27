package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
// Store 适配：引擎可能运行在内置目录（WindowsApps）下，该目录的孤儿进程同样要清理，
// 否则商店版崩溃后重启时无法回收旧引擎进程（占用显存/内存）。
func (a *App) cleanupOrphanProcesses() {
	llm.KillOrphanLlamaServers(filepath.Join(appDir(), "runtime"))
	if br := bundledRuntimeDir(); br != "" {
		llm.KillOrphanLlamaServers(br)
	}
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
		a.emitFatalError(ctx, "配置加载失败",
			"无法读取配置文件，应用需要退出。",
			fmt.Sprintf("加载配置文件失败：\n%v\n\n"+
				"可能原因：\n"+
				"• 配置文件被其他程序占用\n"+
				"• 磁盘空间不足或无写入权限\n"+
				"• 文件系统错误\n\n"+
				"建议：\n"+
				"1. 关闭其他可能占用该文件的程序\n"+
				"2. 检查磁盘空间和写入权限\n"+
				"3. 备份后删除配置文件：%s\n"+
				"   （下次启动会自动创建默认配置）",
				err, cfgPath))
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
			a.emitFatalError(ctx, "目录创建失败",
				"无法创建应用所需的目录，应用需要退出。",
				fmt.Sprintf("创建目录失败：\n%v\n\n"+
					"目标目录：%s\n\n"+
					"可能原因：\n"+
					"• 磁盘空间不足\n"+
					"• 没有写入权限\n"+
					"• 路径包含非法字符\n\n"+
					"建议：\n"+
					"1. 检查磁盘空间是否充足\n"+
					"2. 确认应用目录有写入权限\n"+
					"3. 尝试以管理员身份运行应用",
					mkErr, dir))
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

	// 说明：不再为 AMD/Intel "灰色地带"（GPUType=unknown）弹窗询问用户。
	// ResolveBackendType 已能可靠决策：auto 模式下 NVIDIA→CUDA，AMD/Intel→Vulkan，
	// 无 GPU→CPU。既保证开箱即用（不打断启动询问），又让非 N 卡用户自动用上最稳的
	// Vulkan。原"用户手动选后端失败后询问是否回退 CPU"的弹窗也随之移除，
	// 改为下方统一的静默回退逻辑。
	// 生活类比：车检员不再犹豫反问"这是哪款车"，而是直接按车况自动选好驾驶模式，
	// 遇到装不上的发动机也悄悄换成稳妥的 CPU 模式，全程不问话。

	// P3 改进：使用多目录运行时预校验的解析函数。候选目录为
	// [内置 runtime（商店版只读、随包分发）, 数据 runtime（可写、运行期下载）]，
	// 内置目录优先保证"开箱即用"；便携版两目录合一，行为与原单目录版本完全一致。
	runtimeDirs := runtimeDirCandidates()
	resolvedBackend := llm.ResolveBackendTypeWithRuntimeDirs(a.hwInfo, cfg.BackendType, runtimeDirs)
	isAuto := cfg.BackendType == "" || cfg.BackendType == string(llm.BackendAuto)

	// P3.7 增强：能力级预检提示。
	// auto 模式已在 ResolveBackendTypeWithRuntimeDirs 内部完成剔除与回退；
	// 此处补两次面向用户的原因提示（业界 LM Studio 的 Settings→Runtime 会在
	// 用户选引擎时就提示"此引擎与显卡不匹配"，而不是静默失败再回退）：
	//   - 手动指定后端：用户手动选了 CUDA 但机器是 A 卡，或选了 Vulkan 但系统
	//     缺 vulkan-1.dll 时，提前告知，不至于装上去打不着火才回头换。
	//   - auto 模式：理论首选（纯按厂商推断）被能力预检剔除时，把"为什么没用上
	//     原生后端"告诉用户，例如"N 卡但驱动过旧 → 先用 Vulkan，升级驱动后重启
	//     即可自动回到 CUDA"。
	// 生活类比：车检员不只默默换挡，还会告诉司机"这台车油品不达标，先换通用
	// 变速箱，把油加到标号后下次启动自动换回原装发动机"。
	if !isAuto {
		if reason := llm.CheckBackendCapabilityMatch(a.hwInfo, llm.BackendType(cfg.BackendType)); reason != "" {
			zlog.Warn().Str("backend", cfg.BackendType).Str("reason", reason).
				Msg("[startup] 手动指定后端与显卡能力不匹配")
			runtime.EventsEmit(ctx, EventServerWarning, map[string]any{
				"type":    "backend_capability_mismatch",
				"message": "后端与显卡能力不匹配：" + reason + "。启动后若引擎运行失败，可切换到自动检测（auto）模式。",
			})
		}
	} else {
		preferred := llm.ResolveBackendType(a.hwInfo, cfg.BackendType)
		if reason := llm.CheckBackendCapabilityMatch(a.hwInfo, preferred); reason != "" {
			zlog.Warn().Str("preferred", preferred.String()).
				Str("resolved", resolvedBackend.String()).Str("reason", reason).
				Msg("[startup] auto 模式：理论首选后端能力不匹配，已自动切换")
			runtime.EventsEmit(ctx, EventServerWarning, map[string]any{
				"type":    "backend_capability_mismatch_auto",
				"message": "显卡能力预检：" + reason + "。已自动切换为 " + llm.GetBackendInfo(resolvedBackend).DisplayName + "。恢复对应条件（如升级显卡驱动）后重启，即可自动回到原生后端。",
			})
		}
	}

	// 幂等查找已安装后端：内置目录或数据目录任一命中即直接复用，不进入下载分支。
	// 这是商店版跳过首启"下载确认弹窗"的关键（微软认证 10.1.2.10 失败的根因正是该弹窗：
	// 测试员点「确定」前应用按设计退出，被判定为崩溃）。
	//
	// 【P3.7 商店版崩溃修复】搜索范围在商店版必须收敛到"包内内置目录"。
	// 诊断实验（diag_store）已确认：MSIX 容器内从"可写数据目录"启动引擎会触发
	// "Unknown Hard Error"（退出码 0xC0000267 = STATUS_DLL_INIT_FAILED）——
	// 该路径被文件虚拟化重定向到 LocalCache\Local，DLL 加载器无法从中加载引擎依赖；
	// 而从包内内置目录（WindowsApps，只读、非虚拟化）启动则完全正常。
	// 因此商店版绝不能使用数据目录的引擎副本，只能跑随包分发的内置引擎。
	searchDirs := runtimeDirs
	if isStoreMode() {
		if br := bundledRuntimeDir(); br != "" {
			searchDirs = []string{br}
		} else {
			searchDirs = nil
		}
	}
	serverPath := llm.FindInstalledBackend(resolvedBackend, searchDirs)

	// Store 特例：商店版已把 cuda/vulkan/cpu 三套引擎全部内置（见 make-msix.ps1），
	// N 卡 auto 模式上方即可直接命中包内 CUDA，无需再走此分支。
	// 此分支仅作安全网：万一某次发布包内缺了目标后端（如 CUDA 体积大被精简），
	// 仍从包内已内置的其他引擎兜底：优先 Vulkan（跨 N/A/I 厂商通用），其次 CPU。
	// 注意商店版绝不能回退到数据目录下载的引擎（会触发 Unknown Hard Error），
	// 因此这里的兜底只查 searchDirs（商店版已收敛为仅包内目录）。
	if isStoreMode() && serverPath == "" {
		if vkPath := llm.FindInstalledBackend(llm.BackendVulkan, searchDirs); vkPath != "" {
			zlog.Warn().Str("wanted", resolvedBackend.String()).
				Msg("[startup] 商店版：目标后端不在包内，改用包内内置 Vulkan")
			resolvedBackend = llm.BackendVulkan
			serverPath = vkPath
		} else if cpuPath := llm.FindInstalledBackend(llm.BackendCPU, searchDirs); cpuPath != "" {
			zlog.Warn().Str("wanted", resolvedBackend.String()).
				Msg("[startup] 商店版：目标后端不在包内，改用包内内置 CPU")
			resolvedBackend = llm.BackendCPU
			serverPath = cpuPath
		}
	}

	if serverPath == "" {
		// 两个目录都没有现成引擎：走原有"解压本地 zip / 提示下载"流程。
		// 统一的静默回退逻辑：auto 模式 + 非 CPU 后端安装失败时，自动降级到 CPU。
		// 同时记录错误供日志诊断。手动模式或无需回退的情况走下方原逻辑。
		// 生活类比：车检员选了驾驶模式但发动机装不上，不再回头问车主，
		// 而是安静地换装稳妥的 CPU 发动机，全程不打扰。
		var err error
		serverPath, err = llm.EnsureBackendInstalled(resolvedBackend, runtimeDir, nil)
		if err != nil {
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

	// P2.6（前端化）：便携版使用"前端风格"对话框询问是否下载后端，前端答复后解除阻塞。
	// 商店版（MSIX）例外：不弹确认框——微软商店审核 10.1.2.10 曾把
	// "测试员在下载确认框未确认前应用退出"判定为崩溃（闪退）。
	// 商店版一旦缺少引擎，直接进入后台静默下载，保证开箱即用、永不因等待确认而退出。
	// 生活类比：便携版像订餐时问顾客"要不要加辣"，商店版像连锁店按标准配方直接出品。
	proceed := true
	if !isStoreMode() {
		proceed = a.waitForBackendDownloadConfirm(ctx, BackendDownloadRequestPayload{
			GPUName:        gpuName,
			BackendName:    info.DisplayName,
			BackendType:    resolvedBackend.String(),
			MissingFiles:   missingMsg.String(),
			TimeoutSeconds: int(backendChoiceTimeout / time.Second),
			SourceURL:      "https://github.com/ggml-org/llama.cpp/releases",
		})
	} else {
		zlog.Info().Msg("[startup] 商店版：引擎缺失时静默后台下载，不弹确认框（避免审核判定闪退）")
	}

	if !proceed {
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
	// 持久化"空模型"标志：让后续 GetServerStatus（含前端轮询兜底）稳定返回
	// "模型目录为空"错误，前端据此放行引导流程，不依赖一次性事件是否被收到。
	a.modelsEmpty.Store(true)
	// 前端化：不再用 OS 级弹窗阻塞，改为推送非阻塞事件，
	// 由前端以轻量提示展示"如何下载模型"的引导文案；用户看完可正常进入界面。
	if ctx != nil {
		runtime.EventsEmit(ctx, EventStartupModelNotice, map[string]any{
			"message": msg.String(),
		})
	}
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
		// 前端化：改为发送启动错误事件，让前端在启动屏上以"错误卡"展示，
		// 用户确认后才退出（而非直接操作系统级弹窗）。
		a.emitFatalError(ctx, "加密密钥加载失败",
			"无法读取或创建加密密钥，应用需要退出。",
			fmt.Sprintf("加载加密密钥失败：\n%v\n\n"+
				"密钥文件：%s\n\n"+
				"可能原因：\n"+
				"• 密钥文件已损坏（长度不是 32 字节）\n"+
				"• 没有写入权限\n"+
				"• 磁盘空间不足\n\n"+
				"注意：请勿直接删除密钥文件，否则历史上已加密的对话数据将无法解密。\n"+
				"请按上述提示处理后重新启动应用。", err, keyPath))
		return err
	}
	a.encKey = key
	return nil
}

// initDatabase 初始化数据库，失败时以前端风格提示并返回 error。
func (a *App) initDatabase(ctx context.Context, dbPath string) error {
	db, err := store.Init(dbPath, a.encKey)
	if err != nil {
		zlog.Error().Err(err).Msg("init database failed")
		// 前端化：DB 初始化失败属致命错误（后续功能都依赖数据库），
		// 改为发送启动错误事件，由前端错误卡展示、用户确认后退出。
		a.emitFatalError(ctx, "数据库初始化失败",
			"无法初始化数据库，应用需要退出。",
			fmt.Sprintf("初始化数据库失败：\n%v\n\n"+
				"数据库文件：%s\n\n"+
				"可能原因：\n"+
				"• 数据库文件损坏（异常退出可能导致）\n"+
				"• 磁盘空间不足\n"+
				"• 没有写入权限\n\n"+
				"建议：\n"+
				"1. 检查磁盘空间是否充足\n"+
				"2. 备份后删除数据库文件（下次启动会自动重建，但历史对话记录会丢失）\n"+
				"3. 确认应用目录有写入权限",
				err, dbPath))
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
		// 前端化：RAG 失败不阻塞启动（基本对话功能仍可用），
		// 改为推送非阻塞事件，由前端以轻量提示告知用户"知识库已禁用"，
		// 不再用操作系统级弹窗打断启动。
		if ctx != nil {
			runtime.EventsEmit(ctx, EventStartupRagDisabled, map[string]any{
				"detail": fmt.Sprintf(`知识库（RAG）初始化失败，已自动禁用知识库功能。
基本对话功能不受影响，但无法使用文档问答。

错误信息：%v

可能原因：
• 知识库存储目录被占用或损坏
• 磁盘空间不足

建议：
1. 重启应用尝试自动恢复
2. 备份后删除目录：%s（下次启动会自动重建，已有知识库内容会丢失）`, err, ragDir),
			})
		}
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
