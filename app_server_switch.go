package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"douya/internal/apperror"
	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/system"

	zlog "github.com/rs/zerolog/log"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// tryWatchModelLoadProgress 尝试通过 /models/sse 端点实时监听模型加载进度
// 将进度通过 wailsruntime.EventsEmit 推送到前端（事件名 modelLoadProgress）
// 如果 SSE 连接失败，静默回退到轮询方式，不影响主流程
// 返回一个 cancel 函数，调用方可提前终止 SSE 监听
func (a *App) tryWatchModelLoadProgress(ctx context.Context, modelName string) context.CancelFunc {
	// sseCtx 派生自 rootCtx（回退到入参 ctx），确保 shutdownInternal 调用 rootCancel 时
	// 能立即终止 SSE 监听 goroutine，避免 g.Wait() 等待过长。
	sseParent := a.rootCtx
	if sseParent == nil {
		sseParent = ctx
	}
	sseCtx, sseCancel := context.WithCancel(sseParent)

	// SSE 监听是长生命周期 goroutine，纳入 trackedGo 跟踪。
	// 保留原 inline recover 以输出 SSE 专属日志（trackedGo 的 recover 作为兜底）。
	a.trackedGo(func() {
		// L-2：SSE 监听 goroutine，panic 时静默回退到轮询（已有 err 处理路径）
		defer func() {
			if r := recover(); r != nil {
				zlog.Debug().Interface("panic", r).Str("model", modelName).Msg("[sse] WatchModelLoadProgress panic, fallback to polling")
			}
		}()
		defer sseCancel()

		err := a.getClient().WatchModelLoadProgress(sseCtx, modelName, func(event llm.ModelLoadEvent) {
			// 推送实时加载进度到前端
			wailsruntime.EventsEmit(ctx, EventModelLoadProgress, map[string]any{
				"model":    event.Model,
				"status":   event.Status,
				"progress": event.ProgressPercent,
			})
		})

		if err != nil {
			// SSE 连接失败，静默回退到轮询方式
			zlog.Debug().Err(err).Str("model", modelName).Msg("[sse] /models/sse unavailable, falling back to polling")
		}
	})

	return sseCancel
}

// emitSwitchingStatus emits a server status event indicating a model switch is in progress.
// P3.4 重构：委托 switchingStatus 统一构建 payload。
func (a *App) emitSwitchingStatus(modelName string) {
	wailsruntime.EventsEmit(a.ctx, EventServerStatus, switchingStatus(modelName))
}

// switchingStatus 构建"切换中"状态 payload。
// P3.4 重构：app_server.go / app_server_switch.go / app_server_watch.go 三处
// 重复的 {Running:false, ModelReady:false, Switching:true, SwitchingTo} 结构统一收敛。
func switchingStatus(modelName string) llm.ServerStatus {
	return llm.ServerStatus{
		Running:     false,
		ModelReady:  false,
		Switching:   true,
		SwitchingTo: modelName,
	}
}

// emitErrorStatus 推送错误状态事件到前端。
// 用于引擎启动失败、模型加载失败、模型崩溃等场景。
// 生活类比：就像仪表盘上的红色报警灯，无论哪个部件出问题，都通过同一个报警灯通知驾驶员。
//
// 注意：ctx 参数通常是请求级 context（如 startServerAndWatch 的入参），
// 保持与原有内联调用一致的行为；handleSwitchFailure 等无请求级 ctx 的场景传 a.ctx。
func (a *App) emitErrorStatus(ctx context.Context, errMsg string) {
	a.lastServerErrMu.Lock()
	a.lastServerError = errMsg
	a.lastServerErrMu.Unlock()
	a.serverLoadFailed.Store(true)
	wailsruntime.EventsEmit(ctx, EventServerStatus, llm.ServerStatus{
		Running:    false,
		ModelReady: false,
		Error:      errMsg,
	})
}

// clearServerLoadFailure 清除"加载彻底失败"标记和持久化错误信息。
//
// 生活类比：仪表盘上的红色报警灯熄灭了——发动机恢复正常后必须熄灭报警灯，
// 否则即使车已能正常行驶，仪表盘仍一直显示故障，驾驶员也看不到"恢复"状态。
//
// 必须在成功路径上调用（模型加载成功、切换成功、监控重载成功），
// 否则 serverLoadFailed 一旦置位就永久锁死：健康检查停止、状态推送被抑制、
// GetServerStatus 永远返回旧错误，用户无法从错误状态恢复。
func (a *App) clearServerLoadFailure() {
	a.lastServerErrMu.Lock()
	a.lastServerError = ""
	a.lastServerErrMu.Unlock()
	a.serverLoadFailed.Store(false)
}

// emitSwitchSuccess emits a server status event indicating the model switch succeeded.
func (a *App) emitSwitchSuccess(modelName string) {
	// P3.4 重构：与 runningStatus() 共用 payload 构建，避免两处结构漂移
	st := a.runningStatus()
	st.CurrentModel = modelName
	st.ModelReady = true
	wailsruntime.EventsEmit(a.ctx, EventServerStatus, st)
}

// emitSwitchProgress emits a progress event for model switch.
func (a *App) emitSwitchProgress(stage, targetModel string) {
	a.emitSwitchProgressCtx(a.ctx, stage, targetModel, nil)
}

// emitSwitchProgressCtx 推送切换进度事件，支持自定义 context 和额外字段。
// 生活类比：像施工进度播报，除了"当前阶段"和"目标模型"，还可以附带"备注信息"（如 VRAM 警告）。
//
// 参数：
//   - ctx: 事件推送的 context（startServerAndWatch 用请求级 ctx，其他场景用 a.ctx）
//   - stage: 阶段名称（preparing/loading/waiting/detecting/done/failed/vram-warning/spec-warning 等）
//   - targetModel: 目标模型名（可为空）
//   - extras: 额外字段（如 "model"、"message"），可为 nil
func (a *App) emitSwitchProgressCtx(ctx context.Context, stage, targetModel string, extras map[string]any) {
	payload := map[string]any{
		"stage":       stage,
		"targetModel": targetModel,
	}
	maps.Copy(payload, extras)
	wailsruntime.EventsEmit(ctx, EventServerSwitchProgress, payload)
}

// tryReloadWithoutMmproj 尝试去掉 mmproj 后重新加载模型
// 当模型加载失败（通常因 mmproj 不兼容导致子进程崩溃）时调用
// 返回 true 表示重试成功，模型已加载就绪
func (a *App) tryReloadWithoutMmproj(ctx context.Context, modelName string, progressCallback func(int, string)) bool {
	// 检查该模型是否有 mmproj，如果没有则不适用此重试策略
	if !a.reloadPresetWithoutMmproj(ctx, modelName) {
		return false
	}

	// 重新加载模型（不带 mmproj）
	zlog.Info().Str("model", modelName).Msg("[server] retrying model load without mmproj")
	if err := a.getClient().LoadModel(ctx, modelName); err != nil && !isAlreadyRunningError(err) {
		zlog.Error().Err(err).Str("model", modelName).Msg("[server] retry load model (without mmproj) failed")
		return false
	}

	// 等待模型加载
	if err := a.getClient().WaitForModelLoaded(ctx, modelName, httpClientTimeout, progressCallback); err != nil {
		zlog.Error().Err(err).Str("model", modelName).Msg("[server] retry load model (without mmproj) timed out")
		return false
	}

	// 重试成功
	zlog.Info().Str("model", modelName).Msg("[server] model loaded successfully without mmproj (text-only mode)")
	a.serverReady.Store(true)
	a.emitSwitchSuccess(modelName)

	// 通知前端多模态不可用
	wailsruntime.EventsEmit(ctx, EventServerMmprojUnavailable, map[string]string{
		"model": modelName,
		"hint":  "多模态投影器不兼容，已切换为纯文本模式",
	})

	return true
}

// reloadPresetWithoutMmproj 去掉指定模型的 mmproj 并重新生成/重载 preset 文件。
// 供 tryReloadWithoutMmproj 与 SwitchModel 共用（此前两处重复"regenerate → ReloadPresets → sleep"序列）。
// 返回 true 表示已成功去掉 mmproj 并完成 preset 重载；false 表示模型无 mmproj 或重载失败。
func (a *App) reloadPresetWithoutMmproj(ctx context.Context, modelName string) bool {
	if !a.regeneratePresetWithoutMmproj(modelName) {
		return false
	}
	// 通知路由器重新加载 preset 文件
	if err := a.getClient().ReloadPresets(ctx); err != nil {
		zlog.Warn().Err(err).Msg("[server] failed to reload presets after removing mmproj")
		return false
	}
	// 等待一小段时间让路由器处理 preset 重载
	time.Sleep(2 * time.Second)
	return true
}

// regeneratePresetWithoutMmproj 重新生成不含指定模型 mmproj 的 preset 文件
// 当模型因 mmproj 不兼容导致子进程崩溃时调用，让模型以纯文本模式加载
// 返回 true 表示成功去掉了 mmproj 并重新生成了 preset 文件
func (a *App) regeneratePresetWithoutMmproj(modelName string) bool {
	a.presetsMu.Lock()
	defer a.presetsMu.Unlock()

	found := false
	for i := range a.presets {
		if a.presets[i].Name != modelName {
			continue
		}
		if a.presets[i].MmprojPath == "" {
			// 该模型本来就没有 mmproj，不需要重试
			return false
		}
		zlog.Info().Str("model", modelName).Str("mmproj", a.presets[i].MmprojPath).
			Msg("[preset] removing mmproj from preset due to loading failure, will retry in text-only mode")
		a.presets[i].MmprojPath = ""
		a.presets[i].MmprojVision = false
		a.presets[i].MmprojAudio = false
		a.presets[i].MmprojVideo = false
		found = true
		break
	}

	if !found {
		return false
	}

	// 重新生成 preset 文件
	var globalDefaults map[string]string
	if a.hwInfo != nil {
		defaultModelPath := ""
		if len(a.presets) > 0 {
			defaultModelPath = a.presets[0].ModelPath
			for _, p := range a.presets {
				if p.Alias == "default" {
					defaultModelPath = p.ModelPath
					break
				}
			}
		}
		sp := system.CalculateSmartParams(a.hwInfo, defaultModelPath, a.resolvedBackendString(), a.getConfig().PerformanceMode)
		globalDefaults = map[string]string{
			"ctx-size": fmt.Sprintf("%d", sp.ContextSize),
		}
	}

	content := llm.GeneratePreset(a.presets, globalDefaults)
	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if err := llm.WritePresetFile(presetPath, content); err != nil {
		zlog.Error().Err(err).Msg("[preset] failed to write preset file without mmproj")
		return false
	}

	zlog.Info().Str("path", presetPath).Msg("[preset] regenerated preset file without mmproj")
	return true
}

func (a *App) generatePresetFile() error {
	modelsDir := filepath.Join(appDir(), "models")
	presets, err := llm.ScanModelsDir(modelsDir)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "scan models dir", err)
	}

	if len(presets) == 0 {
		return apperror.Newf(apperror.KindNotFound, "no models found in %s", modelsDir)
	}

	defaultModelPath := a.getConfig().ModelPath
	llm.SetDefaultAlias(presets, defaultModelPath)

	presetRelPaths := make(map[string]string, len(presets))
	for i := range presets {
		presetRelPaths[presets[i].Name] = presets[i].ModelPath
	}

	for i := range presets {
		absModelPath := resolvePath(presets[i].ModelPath)
		presets[i].ModelPath = absModelPath

		if presets[i].MmprojPath != "" {
			absMmproj := resolvePath(presets[i].MmprojPath)
			// 验证 mmproj 文件实际存在，不存在则降级为纯文本模式
			if _, err := os.Stat(absMmproj); err != nil {
				zlog.Warn().Str("model", presets[i].Name).Str("mmproj", absMmproj).
					Err(err).Msg("[preset] mmproj 文件不存在，将以纯文本模式加载")
				presets[i].MmprojPath = ""
				presets[i].MmprojVision = false
				presets[i].MmprojAudio = false
				presets[i].MmprojVideo = false
			} else {
				presets[i].MmprojPath = absMmproj
				var modalities []string
				if presets[i].MmprojVision {
					modalities = append(modalities, "vision")
				}
				if presets[i].MmprojAudio {
					modalities = append(modalities, "audio")
				}
				if presets[i].MmprojVideo {
					modalities = append(modalities, "video")
				}
				zlog.Info().
					Str("model", presets[i].Name).
					Str("mmproj", presets[i].MmprojPath).
					Strs("modalities", modalities).
					Msg("[preset] mmproj detected")
			}
		}
	}

	a.presetsMu.Lock()
	a.presets = presets
	a.presetRelPaths = presetRelPaths
	a.presetsMu.Unlock()

	var globalDefaults map[string]string
	if a.hwInfo != nil {
		defaultModelPath := ""
		if len(presets) > 0 {
			defaultModelPath = presets[0].ModelPath
			for _, p := range presets {
				if p.Alias == "default" {
					defaultModelPath = p.ModelPath
					break
				}
			}
		}
		sp := system.CalculateSmartParams(a.hwInfo, defaultModelPath, a.resolvedBackendString(), a.getConfig().PerformanceMode)
		globalDefaults = map[string]string{
			"ctx-size":       fmt.Sprintf("%d", sp.ContextSize),
			"mmproj-offload": "1",
			"pooling":        "mean",
		}
		zlog.Info().Int("ctx-size", sp.ContextSize).Msg("[preset] global defaults")
	}

	content := llm.GeneratePreset(presets, globalDefaults)
	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if err := llm.WritePresetFile(presetPath, content); err != nil {
		return apperror.Wrap(apperror.KindInternal, "write preset file", err)
	}

	zlog.Info().Str("path", presetPath).Int("count", len(presets)).Msg("[preset] generated preset file")
	return nil
}

// SwitchModel 切换模型（主流程编排）
func (a *App) SwitchModel(modelName string) SwitchResult {
	// 预检查
	if errMsg := a.switchPreCheck(modelName); errMsg != "" {
		return SwitchResult{Error: errMsg}
	}

	// VRAM 预检查（不阻塞切换，只是提前警告）
	if vramMsg := a.vramPreCheck(modelName); vramMsg != "" {
		zlog.Warn().Str("model", modelName).Str("vram_msg", vramMsg).Msg("[router] VRAM pre-check warning")
		// 注意：VRAM 预检查只是警告，不阻止切换（估算可能不准确）
		// 但将警告信息传递给前端
		a.emitSwitchProgressCtx(a.ctx, "vram-warning", "", map[string]any{
			"model":   modelName,
			"message": vramMsg,
		})
	}

	// SpecType 兼容性检查（不阻塞切换，只是提前警告）
	if specMsg := a.specTypeCompatCheck(modelName); specMsg != "" {
		zlog.Warn().Str("model", modelName).Str("spec_msg", specMsg).Msg("[router] SpecType compatibility warning")
		a.emitSwitchProgressCtx(a.ctx, "spec-warning", "", map[string]any{
			"model":   modelName,
			"message": specMsg,
		})
	}

	// 停止当前生成，记录旧模型，设置切换状态
	previousModel := a.switchPrepare(modelName)

	// 加载新模型
	alreadyRunning, loadErr := a.switchLoadModel(modelName)
	if loadErr != "" {
		// 加载失败时，尝试去掉 mmproj 后以纯文本模式重试
		// 生活类比：打不开带密码的文件？先去掉密码保护用纯文本打开，总比打不开好。
		// P3.3 重构：与 tryReloadWithoutMmproj 共用 reloadPresetWithoutMmproj 序列
		if a.reloadPresetWithoutMmproj(a.ctx, modelName) {
			zlog.Info().Str("model", modelName).Msg("[server] retrying model load without mmproj (switch)")
			retryRunning, retryErr := a.switchLoadModel(modelName)
			if retryErr == "" {
				// 纯文本重试成功，继续完成切换流程
				if !retryRunning {
					if waitErr := a.switchWaitReady(modelName); waitErr != "" {
						return a.handleSwitchFailure(modelName, previousModel, waitErr)
					}
				}
				// 通知前端多模态不可用
				wailsruntime.EventsEmit(a.ctx, EventServerMmprojUnavailable, map[string]string{
					"model": modelName,
					"hint":  "未找到匹配的多模态投影文件，已切换为纯文本模式",
				})
				return a.switchFinalize(modelName, previousModel)
			}
			// 重试也失败，使用重试的错误信息
			loadErr = retryErr
		}
		return a.handleSwitchFailure(modelName, previousModel, loadErr)
	}

	// 等待模型就绪（已运行的模型跳过）
	if !alreadyRunning {
		if waitErr := a.switchWaitReady(modelName); waitErr != "" {
			return a.handleSwitchFailure(modelName, previousModel, waitErr)
		}
	}

	// 完成切换：更新状态、保存配置、检测架构
	return a.switchFinalize(modelName, previousModel)
}

// switchPreCheck 预检查：服务器是否启动、是否正在切换、VRAM 是否足够
func (a *App) switchPreCheck(modelName string) string {
	if a.getServer() == nil || a.getClient() == nil {
		return "服务器未启动"
	}
	if !a.beginModelSwitch(modelName) {
		return "正在切换模型中，请稍候。"
	}
	return ""
}

// vramPreCheck VRAM 预检查：估算模型 VRAM 需求，与 GPU VRAM 比较
func (a *App) vramPreCheck(modelName string) string {
	a.presetsMu.RLock()
	var preset *llm.ModelPreset
	for i := range a.presets {
		if a.presets[i].Name == modelName {
			preset = &a.presets[i]
			break
		}
	}
	a.presetsMu.RUnlock()
	if preset == nil {
		return "" // 找不到 preset，跳过预检查
	}

	modelPath := resolvePath(preset.ModelPath)
	mmprojPath := ""
	if preset.MmprojPath != "" {
		mmprojPath = resolvePath(preset.MmprojPath)
	}

	// P2 改进：传入上下文长度，动态估算 KV cache（原固定 512MB 会低估大上下文）
	ctxSize := 0
	if cfg := a.getConfig(); cfg != nil {
		ctxSize = cfg.ContextSize
	}
	estimated := llm.EstimateModelVRAM(modelPath, mmprojPath, ctxSize)
	// P1 改进：传入 HardwareInfo，支持多厂商 VRAM 查询（原只查 nvidia-smi）
	gpuVRAM, err := llm.GetGPUVRAMBytes(a.hwInfo)
	if err != nil {
		zlog.Debug().Err(err).Msg("[vram-check] cannot query GPU VRAM, skip pre-check")
		return ""
	}

	zlog.Info().
		Str("model", modelName).
		Str("estimated", llm.FormatBytes(estimated)).
		Str("gpu_vram", llm.FormatBytes(gpuVRAM)).
		Msg("[vram-check] pre-check")

	if estimated > gpuVRAM {
		return fmt.Sprintf("显存不足：模型预估需要 %s，GPU 仅有 %s。建议使用更小的量化版本或关闭视觉模型。",
			llm.FormatBytes(estimated), llm.FormatBytes(gpuVRAM))
	}
	return ""
}

// specTypeCompatCheck SpecType 兼容性检查：当 llama-server 以推测解码模式运行但目标模型不兼容时发出警告
func (a *App) specTypeCompatCheck(modelName string) string {
	// 获取目标模型的 GGUF 元数据
	a.presetsMu.RLock()
	var modelPath string
	for i := range a.presets {
		if a.presets[i].Name == modelName {
			modelPath = resolvePath(a.presets[i].ModelPath)
			break
		}
	}
	a.presetsMu.RUnlock()
	if modelPath == "" {
		return ""
	}

	meta, err := system.ParseGGUFMetadataCached(modelPath)
	if err != nil {
		return ""
	}

	// 获取当前 llama-server 的 SpecType
	currentSpecType := ""
	if srv := a.getServer(); srv != nil {
		currentSpecType = srv.GetSpecType()
	}

	// 如果当前是 MTP 模式但目标模型不支持 MTP
	if currentSpecType == "draft-mtp" && !meta.HasMTP {
		return fmt.Sprintf("当前服务器以 MTP 模式运行，但 %s 不支持 MTP。切换可能导致短暂崩溃后自动恢复，建议重启应用以获得最佳体验。", modelName)
	}

	return ""
}

// switchPrepare 停止当前生成、记录旧模型、设置切换状态
func (a *App) switchPrepare(modelName string) string {
	if a.service != nil {
		a.service.StopGeneration()
	}

	previousModel := a.currentModel()

	a.serverReady.Store(false)

	a.emitSwitchingStatus(modelName)
	a.emitSwitchProgress("loading", modelName)

	return previousModel
}

// buildStackOverflowSuggestion 构建栈溢出崩溃后的后端切换/调参建议。
// B-1 提取：统一 app_server_switch.go 与 app_server_watch.go 两处重复且措辞不一致的提示文本。
// 生活类比：像医院统一的"用药说明"——同一病症（栈溢出）无论哪个科室（switch/watch 调用点）诊断出来，
// 给患者的建议必须完全一致，不能一个科室说 A、另一个说 B。
func buildStackOverflowSuggestion(currentBackend string) string {
	if currentBackend == "vulkan" {
		return "Vulkan 后端对该模型的兼容性较差，建议切换后端：\n" +
			"  - NVIDIA 显卡：切换到 CUDA 后端（性能最佳）\n" +
			"  - 其他显卡：切换到 CPU 后端（兼容性最好，速度较慢）\n" +
			"\n可在「设置 → 显卡后端」中切换，切换后需重启应用生效。"
	}
	return "可能原因：gpu_layers 过大、ctx-size 过大、或模型架构不兼容。\n" +
		"建议：1) 减小 gpu_layers；2) 减小 ctx-size；3) 切换到 CPU 后端。"
}

// classifyWaitError 根据等待错误内容分类"模型崩溃"还是"加载超时"，并组装用户可见的错误消息。
// RF-2 修复：抽取 switchLoadModel 中两处重复的字符串匹配 + 错误消息拼接逻辑。
// 生活类比：客服收到投诉后先分类（是产品坏了还是物流慢了），再按类别套模板回复，而不是每次重新写。
//   - waitErr: WaitForModelLoaded 返回的错误
//   - stderrHint: 从 server stderr 提取的详细错误提示（可为空）
//
// 返回非空字符串表示失败原因；空字符串表示未识别为错误（调用方不应进入此分支）。
//
// B-1 增强：检测栈溢出崩溃（0xC0000409），Vulkan 后端时给出后端切换建议。
func (a *App) classifyWaitError(waitErr error, stderrHint string) string {
	waitErrStr := waitErr.Error()

	// B-1：优先检测栈溢出崩溃，给出针对性诊断
	if isStackOverflowCrash(waitErrStr) {
		currentBackend := a.resolvedBackendString()
		hint := "模型加载失败: 栈溢出崩溃（0xC0000409）。\n" + buildStackOverflowSuggestion(currentBackend)
		if stderrHint != "" {
			hint += fmt.Sprintf("\n\n详细信息: %s", stderrHint)
		}
		return hint
	}

	// 崩溃特征：进程退出、VRAM 释放、从模型列表消失
	isCrash := strings.Contains(waitErrStr, "failed to load") ||
		strings.Contains(waitErrStr, "crashed") ||
		strings.Contains(waitErrStr, "exit_code") ||
		strings.Contains(waitErrStr, "VRAM released") ||
		strings.Contains(waitErrStr, "disappeared from model list")
	if isCrash {
		if stderrHint != "" {
			return fmt.Sprintf("模型加载失败: %v\n\n详细信息: %s", waitErr, stderrHint)
		}
		return fmt.Sprintf("模型加载失败: %v", waitErr)
	}
	// 真正的超时
	if stderrHint != "" {
		return fmt.Sprintf("模型加载超时: %v\n\n详细信息: %s", waitErr, stderrHint)
	}
	return fmt.Sprintf("模型加载超时: %v", waitErr)
}

// switchLoadModel 加载模型，返回 (是否已运行, 错误消息)
func (a *App) switchLoadModel(modelName string) (bool, string) {
	loadTimeout := a.calculateLoadTimeout(modelName)
	zlog.Info().Str("model", modelName).Dur("timeout", loadTimeout).Msg("[router] switch model with dynamic timeout")

	// 在 LoadModel 之前启动 SSE 监听，确保捕获完整加载进度
	sseCancel := a.tryWatchModelLoadProgress(a.ctx, modelName)
	defer sseCancel()

	loadErr := a.getClient().LoadModel(a.ctx, modelName)
	if loadErr == nil {
		// LoadModel 返回 200 仅表示开始加载，需要等待模型真正就绪
		a.emitSwitchProgress("waiting", modelName)

		if waitErr := a.getClient().WaitForModelLoaded(a.ctx, modelName, loadTimeout); waitErr != nil {
			// 优先检测 OOM/显存/内存不足，返回明确提示
			if oomMsg := a.detectOOMError(); oomMsg != "" {
				return false, oomMsg
			}
			// RF-2 修复：复用 classifyWaitError 统一分类"崩溃 vs 超时"
			return false, a.classifyWaitError(waitErr, a.getServerStderrHint())
		}
		return false, ""
	}

	if isAlreadyRunningError(loadErr) {
		// 模型可能还在 LOADING 状态，必须等待状态变为 loaded
		zlog.Info().Str("model", modelName).Msg("[router] model is already running/loading, waiting for loaded state")
		a.emitSwitchProgress("waiting", modelName)

		if waitErr := a.getClient().WaitForModelLoaded(a.ctx, modelName, loadTimeout); waitErr != nil {
			if oomMsg := a.detectOOMError(); oomMsg != "" {
				return false, oomMsg
			}
			// RF-2 修复：复用 classifyWaitError 统一分类"崩溃 vs 超时"
			return false, a.classifyWaitError(waitErr, a.getServerStderrHint())
		}
		return true, ""
	}

	// LoadModel 本身失败，也检测 OOM
	if oomMsg := a.detectOOMError(); oomMsg != "" {
		return false, oomMsg
	}
	// 从 server stderr 获取详细错误信息
	stderrHint := a.getServerStderrHint()
	if stderrHint != "" {
		return false, fmt.Sprintf("模型加载失败: %v\n\n详细信息: %s", loadErr, stderrHint)
	}
	return false, fmt.Sprintf("模型加载失败: %v", loadErr)
}

// getServerStderrHint 从 llama-server 的 stderr 缓冲区提取关键错误信息
func (a *App) getServerStderrHint() string {
	srv := a.getServer()
	if srv == nil {
		return ""
	}
	stderr := srv.LastOutput()
	if stderr == "" {
		return ""
	}
	// 提取关键错误行
	lines := strings.Split(stderr, "\n")
	var hints []string
	keywords := []string{"error", "failed", "OOM", "CUDA", "VRAM", "out of memory", "cannot", "invalid", "unsupported"}
	for _, line := range lines {
		lower := strings.ToLower(line)
		for _, kw := range keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				hints = append(hints, strings.TrimSpace(line))
				break
			}
		}
	}
	if len(hints) == 0 {
		return ""
	}
	// 最多返回 3 条关键信息
	if len(hints) > 3 {
		hints = hints[len(hints)-3:]
	}
	return strings.Join(hints, "\n")
}

// detectOOMError 检测 stderr 中是否包含 OOM/显存/内存不足错误
// 返回明确的中文错误提示，若非 OOM 则返回空字符串
// P3.1 修复：OOM 判定委托给 llm.DetectOOMInStderr（单一关键词表），
// 此处仅负责区分"显存不足"与"内存不足"并组装中文提示。
func (a *App) detectOOMError() string {
	srv := a.getServer()
	if srv == nil {
		return ""
	}
	stderr := srv.LastOutput()
	if stderr == "" {
		return ""
	}
	// P3.1 修复：OOM 判定统一委托给 llm.DetectOOMInStderr（单一关键词表），
	// 此处不再自维护一套关键词，避免与崩溃降级链行为不一致。
	if !llm.DetectOOMInStderr(stderr) {
		return ""
	}
	lower := strings.ToLower(stderr)

	// CUDA 显存不足模式
	cudaOOMPatterns := []string{
		"cuda error", "cuda_error_out_of_memory", "out of memory",
		"failed to allocate cuda", "failed to allocate gpu",
		"gpu memory", "vram", "not enough gpu memory",
	}
	for _, p := range cudaOOMPatterns {
		if strings.Contains(lower, p) {
			hint := a.getServerStderrHint()
			return fmt.Sprintf("显存不足：模型加载需要的显存超过了 GPU 可用显存。\n详细信息: %s", hint)
		}
	}

	// 系统内存不足模式
	ramOOMPatterns := []string{
		"bad allocation", "cannot allocate memory", "mmap failed",
		"std::bad_alloc", "memory allocation failed",
	}
	for _, p := range ramOOMPatterns {
		if strings.Contains(lower, p) {
			hint := a.getServerStderrHint()
			return fmt.Sprintf("内存不足：系统内存不足以加载模型，可能是物理内存不足或交换空间不够。\n详细信息: %s", hint)
		}
	}

	// 已判定为 OOM 但未细分到 CUDA/内存类别，给出通用提示
	hint := a.getServerStderrHint()
	return fmt.Sprintf("内存或显存不足：模型加载需要的资源超出当前机器可用容量。\n详细信息: %s", hint)
}

// calculateLoadTimeout 根据模型文件大小动态计算加载超时
// 基础 180 秒 + 每GB 30秒，上限 600 秒（10分钟）
func (a *App) calculateLoadTimeout(modelName string) time.Duration {
	fileSize := a.getModelFileSize(modelName)
	if fileSize <= 0 {
		// 无法获取大小时，使用保守的 300 秒（与首次加载一致）
		return httpClientTimeout
	}

	fileSizeGB := float64(fileSize) / (1024 * 1024 * 1024)
	timeout := min(loadTimeoutBase+time.Duration(fileSizeGB*float64(loadTimeoutPerGB)), loadTimeoutMax)
	return timeout
}

// getModelFileSize 获取模型文件大小（字节）
func (a *App) getModelFileSize(modelName string) int64 {
	a.presetsMu.RLock()
	defer a.presetsMu.RUnlock()
	for _, p := range a.presets {
		if p.Name == modelName {
			if info, err := os.Stat(p.ModelPath); err == nil {
				return info.Size()
			}
		}
	}
	return 0
}

// switchWaitReady 等待模型就绪（含 mmproj 回退检测）
// 注意：不需要在此调用 WaitForModelLoaded，因为调用方 switchLoadModel 已确保模型加载完成
func (a *App) switchWaitReady(modelName string) string {
	// 等待 mmproj 等后加载初始化完成
	// 使用指数退避：200ms → 300ms → 500ms → 800ms
	propsCtx, propsCancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer propsCancel()
	backoffs := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 600 * time.Millisecond, 800 * time.Millisecond, time.Second}
	var lastProps *llm.ServerProps
	for i := range 10 {
		props, propsErr := a.getClient().GetServerProps(propsCtx, modelName)
		if propsErr == nil {
			lastProps = props
			// 不需要 mmproj — 立即退出
			if !props.Modalities.Vision && !props.Modalities.Audio {
				break
			}
			// mmproj 已加载 — 退出
			if i > 0 {
				break
			}
			// 首次成功但有 mmproj — 可能仍在加载，再检查一次
			continue
		}
		select {
		case <-propsCtx.Done():
			return ""
		case <-time.After(backoffs[min(i, len(backoffs)-1)]):
		}
	}
	// 缓存 props 结果，供 DetectModelArchitectureForModel 复用
	if lastProps != nil {
		a.service.SetCachedProps(lastProps)
	}

	return ""
}

// switchFinalize 完成切换：更新模型名、保存配置、检测架构、发射事件
func (a *App) switchFinalize(modelName, previousModel string) SwitchResult {
	// 更新当前模型名
	a.setCurrentModel(modelName)
	// 同步更新 client 的当前模型（v9744+ API 需要）
	if a.getClient() != nil {
		a.getClient().SetCurrentModel(modelName)
	}

	// 清除旧模型的 slot 缓存：模型切换后旧 KV 缓存对新模型毫无价值，
	// 磁盘上的旧缓存文件必须清除，否则下次 restore 会把错误模型的 KV 塞回去
	if a.service != nil {
		clearCtx, clearCancel := context.WithTimeout(a.ctx, 35*time.Second)
		a.service.ClearSavedSlot(clearCtx)
		clearCancel()
	}

	// 根据用户配置的 --image-max-tokens 更新图片 token 估算值，
	// 让 MaxTokens 计算与 llama-server 实际图片 token 消耗一致
	if cfg := a.getConfig(); cfg != nil {
		chat.SetImageTokenEstimate(cfg.ImageMaxTokens)
	}

	// 更新嵌入模型名（仅在未配置专用嵌入模型时跟随聊天模型切换）
	if a.ragEmbedder != nil && a.getConfig().EmbeddingModel == "" {
		a.ragEmbedder.SetModel(modelName)
	}

	// 保存配置
	a.presetsMu.RLock()
	relPath, hasRelPath := a.presetRelPaths[modelName]
	a.presetsMu.RUnlock()

	// 读取该模型的专属生成参数（如有），切换模型时自动恢复用户保存的参数习惯
	// 生活类比：换工位前先从档案柜取出该员工的"偏好卡片"，待会儿一起设置
	var modelParams *chat.ModelParams
	if a.service != nil && hasRelPath {
		if mp, err := a.service.GetModelParams(modelName); err != nil {
			zlog.Warn().Err(err).Str("model", modelName).Msg("[switchFinalize] 读取模型参数失败，使用全局默认")
		} else {
			modelParams = mp
		}
	}

	// paramsRestored 记录是否成功应用了模型专属参数，供 SwitchResult 返回给前端显示提示
	paramsRestored := false
	if hasRelPath {
		// P3.5 重构：updateConfig 统一"复制→修改副本→替换指针"模式
		var cfg *config.Config
		if err := a.updateConfig(func(c *config.Config) error {
			c.ModelPath = relPath
			// 应用模型专属生成参数（如有），覆盖全局 Config 中的对应字段
			if modelParams != nil {
				modelParams.ApplyToConfig(c)
			}
			cfg = c
			return nil
		}); err != nil {
			zlog.Error().Err(err).Msg("[switchFinalize] 配置更新失败")
		} else {
			// 保存前校验，失败记录日志但不阻塞保存（避免阻塞模型切换功能）
			if err := cfg.Validate(); err != nil {
				zlog.Warn().Err(err).Msg("[switchFinalize] 配置校验失败，仍保存")
			}
			if err := config.Save(filepath.Join(appDir(), "config.json"), cfg); err != nil {
				zlog.Error().Err(err).Msg("[router] save config after model switch failed")
				wailsruntime.EventsEmit(a.ctx, EventServerStatus, llm.ServerStatus{
					Running:      true,
					ModelReady:   true,
					CurrentModel: modelName,
					Error:        fmt.Sprintf("config save failed, model may revert on restart: %v", err),
				})
			}
			// 同步更新 chat service 的配置引用，让生成参数立即生效
			if a.service != nil {
				a.service.UpdateConfig(cfg)
			}
			// 如果应用了模型专属参数，记录日志（前端通过 SwitchResult.ParamsRestored 显示提示）
			if modelParams != nil {
				paramsRestored = true
				zlog.Info().Str("model", modelName).Msg("[switchFinalize] 已恢复模型专属生成参数")
			}
		}
	}

	// 检测模型架构
	a.service.SetDetectedModelName(modelName)
	if err := a.service.DetectModelArchitectureForModel(modelName); err != nil {
		zlog.Error().Err(err).Msg("[router] detect model architecture after switch failed")
		wailsruntime.EventsEmit(a.ctx, EventServerStatus, llm.ServerStatus{
			Running:      true,
			ModelReady:   true,
			CurrentModel: modelName,
			Error:        fmt.Sprintf("模型架构检测失败: %v", err),
		})
	}

	// 模型切换后重置 LoRA 适配器为未应用状态（scale=0）
	// 用户可在设置界面重新启用需要的适配器
	// 仅在配置了 LoRA 路径时才调用，否则 llama-server 未加载任何适配器，
	// 调用 /lora-adapters 端点会返回 400 错误（model name is missing）
	if a.getClient() != nil && a.getConfig().LoraPaths != "" {
		loraCtx, loraCancel := context.WithTimeout(a.ctx, 5*time.Second)
		if adapters, err := a.getClient().GetLoraAdapters(loraCtx); err == nil && len(adapters) > 0 {
			// 将所有适配器的 scale 设为 0（保留列表，不删除）
			for i := range adapters {
				adapters[i].Scale = 0
			}
			if err := a.getClient().SetLoraAdapters(loraCtx, adapters); err != nil {
				zlog.Warn().Err(err).Msg("[router] failed to reset lora adapters after model switch")
			} else {
				zlog.Info().Int("count", len(adapters)).Msg("[router] lora adapters reset to scale=0 after model switch")
			}
		}
		loraCancel()
	}

	// 发射完成事件
	a.emitSwitchProgress("done", modelName)

	// 在清除切换状态之前发射成功事件
	// 确保前端在 WatchWithCallback 发出过时状态之前收到成功事件
	// 切换成功后清除"加载失败"标记，避免旧错误状态粘滞阻塞健康检查
	a.clearServerLoadFailure()
	a.emitSwitchSuccess(modelName)
	a.serverReady.Store(true)

	// 清除切换状态
	a.endModelSwitch()

	zlog.Info().Str("model", modelName).Str("previous", previousModel).Msg("[router] model switched")

	resultModel := a.currentModel()
	caps := a.service.GetModelCapabilities()
	return SwitchResult{
		Success:        true,
		CurrentModel:   resultModel,
		Capabilities:   &caps,
		PreviousModel:  previousModel,
		ParamsRestored: paramsRestored,
	}
}

// handleSwitchFailure 处理模型切换失败：尝试恢复旧模型，清理状态，返回错误结果
func (a *App) handleSwitchFailure(modelName, previousModel, errMsg string) SwitchResult {
	zlog.Error().Str("error", errMsg).Msg("[router] model switch failed")
	a.emitSwitchProgress("failed", modelName)

	// 注意：isSwitching 在回滚完成后再清除，防止回滚期间用户发起新切换
	a.clearSwitchTarget()

	rollbackSuccess := false
	if previousModel != "" && previousModel != modelName {
		zlog.Info().Str("model", previousModel).Msg("[router] attempting to restore model")
		restoreCtx, restoreCancel := context.WithTimeout(a.ctx, apiTimeoutMedium)
		if restoreErr := a.getClient().LoadModel(restoreCtx, previousModel); restoreErr == nil || isAlreadyRunningError(restoreErr) {
			// LoadModel 返回 "already running"/"already loaded" 时，旧模型实际仍在运行，
			// 视为回滚成功，避免误报失败导致 UI 状态错误
			// M1 修复：等待结果不再静默忽略——等待失败说明旧模型未真正就绪，
			// 记录日志便于排查，但继续按回滚成功处理（模型已在加载中，后续 watch 会更新状态）
			if waitErr := a.getClient().WaitForModelLoaded(restoreCtx, previousModel, apiTimeoutMedium); waitErr != nil {
				zlog.Warn().Err(waitErr).Str("model", previousModel).Msg("[router] 等待旧模型就绪超时，将依赖 watch 更新状态")
			}
			a.setCurrentModel(previousModel)
			a.emitSwitchSuccess(previousModel)
			a.serverReady.Store(true)
			rollbackSuccess = true
		} else {
			zlog.Error().Err(restoreErr).Str("model", previousModel).Msg("[router] failed to restore model")
			a.emitErrorStatus(a.ctx, fmt.Sprintf("%s，恢复旧模型也失败", errMsg))
		}
		restoreCancel()
	} else {
		a.emitErrorStatus(a.ctx, errMsg)
	}

	// 回滚完成后再清除 isSwitching
	a.endModelSwitch()

	return SwitchResult{
		Error:           errMsg,
		PreviousModel:   previousModel,
		RolledBack:      previousModel != "" && previousModel != modelName,
		RollbackSuccess: rollbackSuccess,
	}
}
