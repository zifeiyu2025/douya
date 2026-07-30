// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"douya/internal/chat"
	"douya/internal/llm"
)

// startServerAndWatch 启动 llama-server 并监控其状态。
//
// 拆分说明：原 237 行函数按启动流程拆为调度器 + 8 子函数：
//   - startServerAndWaitReady:   启动引擎并等待就绪
//   - selectAndDetectDefaultModel: 选择默认模型并检测架构
//   - autoLoadDefaultModel:      启动后自动加载默认模型
//   - makeLoadProgressCallback:  创建加载进度回调
//   - handleModelLoadSuccess:    处理模型加载成功
//   - handleModelLoadFailure:    处理模型加载失败（mmproj 重试 + 后台等待）
//   - startServerWatcher:        启动状态监控和健康检查
//   - makeStatusCallback:        创建状态回调
//   - makeRestartCallback:       创建重启回调
//
// 生活类比：像开机流程——按下电源（startServer）→ 选择启动系统（selectModel）
// → 加载系统（autoLoad）→ 开启后台服务（watcher）。
func (a *App) startServerAndWatch(srv *llm.Server, ctx context.Context) {
	// 推送首次启动进度：准备启动引擎
	a.emitSwitchProgressCtx(ctx, "preparing", "", nil)

	// 1. 启动引擎并等待就绪
	if err := a.startServerAndWaitReady(srv, ctx); err != nil {
		return
	}

	// 2. 选择默认模型并检测架构
	modelForDetect := a.selectAndDetectDefaultModel(ctx)

	// 根据用户配置的 --image-max-tokens 设置图片 token 估算值（首次启动）
	// 后续切换模型时由 switchFinalize 更新
	if cfg := a.getConfig(); cfg != nil {
		chat.SetImageTokenEstimate(cfg.ImageMaxTokens)
	}

	// 3. 自动加载默认模型
	a.autoLoadDefaultModel(ctx, modelForDetect)

	// 4. 启动状态监控和健康检查
	a.startServerWatcher(srv, ctx)
}

// startServerAndWaitReady 启动引擎并等待就绪。
// 启动失败或等待就绪超时均返回 error，调用方根据返回值决定是否继续后续流程。
//
// 失败处理统一流程（C2+M2+M3 修复）：
//   - 回退成功：先回退配置 → 推送"已回退"状态 → 弹窗提示 → forceQuit 退出
//   - 回退失败：弹 ErrorDialog → forceQuit 退出（避免应用处于不可用状态）
//
// 为什么所有失败路径都 forceQuit：
//   服务器未启动时应用无法对话，继续运行只会让用户面对一个不可用的界面。
//   统一 forceQuit 让用户重启后走完整启动流程，避免半死不活的状态。
func (a *App) startServerAndWaitReady(srv *llm.Server, ctx context.Context) error {
	if err := srv.Start(); err != nil {
		zlog.Error().Err(err).Msg("start llama-server failed")
		a.handleBackendStartupFailure(ctx, "启动 llama-server", err)
		return err
	}

	if err := srv.WaitForReady(60e9); err != nil {
		zlog.Error().Err(err).Msg("wait for server ready failed")
		a.handleBackendStartupFailure(ctx, "llama-server 就绪", err)
		return err
	}

	// 启动成功：清空回退备份（撕掉旧照片，避免下次正常启动被误判为需要回退）
	a.clearBackendRollback()

	// 推送首次启动进度：引擎已就绪，准备加载模型
	a.emitSwitchProgressCtx(ctx, "loading", "", nil)

	// F-3 修复：preset 文件生成失败时通知前端显示警告横幅
	if a.presetGenFailed {
		runtime.EventsEmit(ctx, "server:warning", map[string]any{
			"type":    "preset_failed",
			"message": "预设文件生成失败，模型加载可能使用默认参数。如遇异常请在设置中检查预设配置。",
		})
	}

	// llama-server 就绪后异步刷新 MCP 工具缓存。
	// /tools 端点由 llama-server 提供，不依赖模型加载，可在模型加载前并行拉取。
	// 失败不影响主流程：下次 buildAvailableTools 时会再次尝试懒加载。
	go func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[startup] refresh MCP tools panic")
			}
		}()
		if a.service != nil {
			a.service.RefreshMcpToolsCache()
		}
	}()

	return nil
}

// selectAndDetectDefaultModel 从预设中选择默认模型并检测其架构。
// 优先选择 alias 为 "default" 的预设，找不到则使用第一个预设。
func (a *App) selectAndDetectDefaultModel(ctx context.Context) string {
	a.presetsMu.RLock()
	presetsSnapshot := make([]llm.ModelPreset, len(a.presets))
	copy(presetsSnapshot, a.presets)
	a.presetsMu.RUnlock()

	foundDefault := false
	for _, p := range presetsSnapshot {
		if p.Alias != "default" {
			continue
		}
		a.currentModelMu.Lock()
		a.currentModelName = p.Name
		a.currentModelMu.Unlock()
		if a.getClient() != nil {
			a.getClient().SetCurrentModel(p.Name)
		}
		foundDefault = true
		break
	}
	if !foundDefault && len(presetsSnapshot) > 0 {
		a.currentModelMu.Lock()
		a.currentModelName = presetsSnapshot[0].Name
		a.currentModelMu.Unlock()
		if a.getClient() != nil {
			a.getClient().SetCurrentModel(presetsSnapshot[0].Name)
		}
		a.currentModelMu.RLock()
		zlog.Info().Str("model", a.currentModelName).Msg("[server] no default preset found, using first model")
		a.currentModelMu.RUnlock()
	}

	a.currentModelMu.RLock()
	modelForDetect := a.currentModelName
	a.currentModelMu.RUnlock()

	// 推送首次启动进度：检测模型能力
	a.emitSwitchProgressCtx(ctx, "detecting", modelForDetect, nil)

	if err := a.service.DetectModelArchitectureForModel(modelForDetect); err != nil {
		zlog.Error().Err(err).Msg("detect model architecture failed")
	}

	return modelForDetect
}

// autoLoadDefaultModel 启动后自动加载默认模型。
// 处理流程：SSE 监听 → LoadModel → WaitForModelLoaded（含 mmproj 重试和后台等待降级）。
func (a *App) autoLoadDefaultModel(ctx context.Context, modelForDetect string) {
	if modelForDetect == "" || a.getClient() == nil {
		return
	}

	zlog.Info().Str("model", modelForDetect).Msg("[server] auto-loading default model")
	runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
		Running:     true,
		ModelReady:  false,
		Switching:   true,
		SwitchingTo: modelForDetect,
	})

	// 在 LoadModel 之前启动 SSE 监听，确保捕获完整加载进度
	sseCancel := a.tryWatchModelLoadProgress(ctx, modelForDetect)
	defer sseCancel()

	loadErr := a.getClient().LoadModel(ctx, modelForDetect)
	if loadErr != nil && !isAlreadyRunningError(loadErr) {
		// 非预期错误（非 "already running"），报告失败
		zlog.Error().Err(loadErr).Str("model", modelForDetect).Msg("[server] auto-load default model failed")
		a.emitErrorStatus(ctx, fmt.Sprintf("默认模型加载失败: %v（可手动切换模型）", loadErr))
		return
	}

	// LoadModel 成功或返回 "already running"（模型可能还在 LOADING 状态）
	// 必须等待模型状态变为 loaded 才能认为就绪
	if loadErr != nil {
		zlog.Info().Str("model", modelForDetect).Msg("[server] model is already running/loading, waiting for loaded state")
	}

	progressCallback := a.makeLoadProgressCallback(ctx, modelForDetect)

	if err := a.getClient().WaitForModelLoaded(ctx, modelForDetect, httpClientTimeout, progressCallback); err != nil {
		a.handleModelLoadFailure(ctx, modelForDetect, err, progressCallback)
		return
	}

	a.handleModelLoadSuccess(ctx, modelForDetect, "[server] default model loaded and ready")
}

// makeLoadProgressCallback 创建模型加载进度回调。
// 每 5 次轮询推送一次进度，状态变化时立即推送。
func (a *App) makeLoadProgressCallback(ctx context.Context, modelForDetect string) func(int, string) {
	lastProgressStage := ""
	return func(pollCount int, status string) {
		if pollCount%5 != 1 && status == lastProgressStage {
			return
		}
		lastProgressStage = status
		stage := "loading"
		switch status {
		case "loaded", "sleeping":
			stage = "waiting"
		case "failed":
			stage = "failed"
		case "unloaded":
			stage = "retrying"
		}
		a.emitSwitchProgressCtx(ctx, stage, modelForDetect, nil)
	}
}

// handleModelLoadSuccess 处理模型加载成功：设置就绪状态、重新检测架构、推送成功事件。
func (a *App) handleModelLoadSuccess(_ context.Context, modelForDetect string, logMsg string) {
	zlog.Info().Str("model", modelForDetect).Msg(logMsg)
	a.serverReady.Store(true)
	// 模型加载完成后重新检测架构，因为首次检测时 mmproj 可能尚未加载
	if err := a.service.DetectModelArchitectureForModel(modelForDetect); err != nil {
		zlog.Warn().Err(err).Msg("[server] re-detect model architecture after load failed")
	}
	a.emitSwitchSuccess(modelForDetect)
}

// handleModelLoadFailure 处理模型加载失败：先尝试去掉 mmproj 重试，
// 若不适用或重试失败，检测栈溢出崩溃并提示后端切换，最后启动后台 goroutine 继续等待。
//
// B-1 增强：检测 0xC0000409（栈缓冲区溢出）退出码，若当前是 Vulkan 后端，
// 提示用户切换到 CPU/CUDA 后端避免反复崩溃。
// 生活类比：新车发动机启动后冒黑烟（栈溢出），技师发现是发动机型号不匹配（Vulkan 不兼容），
// 直接建议换回旧发动机（CUDA/CPU），而不是反复尝试点火。
func (a *App) handleModelLoadFailure(ctx context.Context, modelForDetect string, err error, progressCallback func(int, string)) {
	// 加载失败，尝试去掉 mmproj 重试（mmproj 不兼容会导致子进程崩溃）
	if a.tryReloadWithoutMmproj(ctx, modelForDetect, progressCallback) {
		// 重试成功
		return
	}

	// B-1：检测栈溢出崩溃，给出后端切换建议
	// 0xC0000409 = -1073740791，Vulkan 后端加载大模型时常见
	errStr := err.Error()
	currentBackend := ""
	if a.resolvedBackend != "" {
		currentBackend = string(a.resolvedBackend)
	}
	if isStackOverflowCrash(errStr) {
		zlog.Error().Err(err).Str("model", modelForDetect).Str("backend", currentBackend).
			Msg("[server] model load failed with stack overflow crash")

		hint := "检测到模型加载时发生栈溢出崩溃（0xC0000409）。\n\n"
		if currentBackend == "vulkan" {
			hint += "Vulkan 后端对该模型的兼容性较差，建议切换后端：\n"
			hint += "  - NVIDIA 显卡：切换到 CUDA 后端（性能最佳）\n"
			hint += "  - 其他显卡：切换到 CPU 后端（兼容性最好，速度较慢）\n"
			hint += "\n可在「设置 → 显卡后端」中切换，切换后需重启应用生效。"
		} else {
			hint += "可能原因：gpu_layers 过大、ctx-size 过大、或模型架构不兼容。\n"
			hint += "建议：1) 减小 gpu_layers；2) 减小 ctx-size；3) 切换到 CPU 后端。"
		}
		a.emitErrorStatus(ctx, hint)
		return
	}

	// 重试也失败或不适用，启动后台 goroutine 继续等待
	zlog.Warn().Err(err).Str("model", modelForDetect).Msg("[server] auto-load default model timed out, continuing to wait in background")
	// bgCtx 派生自 rootCtx，确保 shutdownInternal 调用 rootCancel 时
	// 能立即终止后台等待 goroutine，避免 g.Wait() 等待过长。
	// 纳入 trackedGo 跟踪：shutdown 时 g.Wait() 会等待本 goroutine 退出。
	bgParent := a.rootCtx
	if bgParent == nil {
		bgParent = ctx
	}
	bgCtx, bgCancel := context.WithCancel(bgParent)
	a.trackedGo(func() {
		defer bgCancel()
		defer func() {
			if r := recover(); r != nil {
				zlog.Error().Interface("panic", r).Str("model", modelForDetect).Msg("model load goroutine panic")
			}
		}()
		// 后台继续等待，不设超时（依赖 WatchWithCallback 检测崩溃）
		// 使用 bgCtx（rootCtx 派生）而非 ctx，确保 shutdown 能取消等待；
		// EventsEmit 仍用原始 ctx（Wails ctx），与 SSE goroutine 保持一致。
		if bgErr := a.getClient().WaitForModelLoaded(bgCtx, modelForDetect, loadTimeoutMax, progressCallback); bgErr != nil {
			zlog.Error().Err(bgErr).Str("model", modelForDetect).Msg("[server] auto-load default model background wait also failed")

			// B-1：后台等待失败也检测栈溢出，给出同样建议
			if isStackOverflowCrash(bgErr.Error()) {
				hint := "检测到模型加载时发生栈溢出崩溃（0xC0000409）。"
				if currentBackend == "vulkan" {
					hint += "\n\nVulkan 后端对该模型兼容性较差，请在「设置 → 显卡后端」切换到 CUDA 或 CPU 后端后重启应用。"
				}
				a.emitErrorStatus(ctx, hint)
				return
			}
			// 修复：将 Running 改为 false，与 Error 字段保持语义一致
			// 此前 Running: true 会导致前端 `!status.running && status.error` 条件失效，错误被静默丢弃
			a.emitErrorStatus(ctx, fmt.Sprintf("默认模型加载失败: %v（可手动切换模型）", bgErr))
			return
		}
		a.handleModelLoadSuccess(ctx, modelForDetect, "[server] default model loaded and ready (background)")
	})
}

// isStackOverflowCrash 检测错误信息是否包含栈溢出崩溃特征
// 0xC0000409 (STATUS_STACK_BUFFER_OVERRUN) = -1073740791
// 0xC00000FD (STATUS_STACK_OVERFLOW) = -1073741571
func isStackOverflowCrash(errStr string) bool {
	return strings.Contains(errStr, "exit_code=-1073740791") ||
		strings.Contains(errStr, "exit_code: 3221226507") ||
		strings.Contains(errStr, "exit_code=-1073741571") ||
		strings.Contains(errStr, "exit_code: 3221225725")
}

// startServerWatcher 启动状态监控和健康检查两个长生命周期 goroutine。
// 两者均纳入 trackedGo 跟踪，依赖 watchCtx.Done() 退出。
func (a *App) startServerWatcher(srv *llm.Server, ctx context.Context) {
	watchCtx, watchCancel := context.WithCancel(ctx)
	a.serverMu.Lock()
	a.watchCancel = watchCancel
	a.serverMu.Unlock()

	// watcher 是长生命周期 goroutine，纳入 trackedGo 跟踪；
	// 依赖 watchCtx.Done() 退出，shutdownInternal 会通过 watchCancel 触发。
	a.trackedGo(func() {
		srv.WatchWithCallback(watchCtx, a.makeStatusCallback(ctx), a.makeRestartCallback(ctx))
	})

	// health 监控是长生命周期 goroutine，纳入 trackedGo 跟踪；
	// 依赖 watchCtx.Done() 退出，shutdownInternal 会通过 watchCancel 触发。
	a.trackedGo(func() { a.watchServerHealth(ctx, watchCtx) })
}

// handleBackendStartupFailure 统一处理后端启动失败（C2+M2+M3 修复）。
//
// 生活类比：新发动机打不着火后的标准处理流程——
// 1. 如果有旧发动机照片（LastSuccessfulBackend），按照片装回去，告知用户"已回退，请重启"
// 2. 没有照片（首次启动或已回退过），直接告知用户失败原因并退出
//
// 统一行为：无论哪种失败，最终都 forceQuit 退出应用。
// 原因：服务器未启动时应用无法对话，继续运行只会让用户面对不可用的界面。
//
// 参数：
//   - phase: 失败阶段描述（"启动 llama-server" 或 "llama-server 就绪"）
//   - err: 具体错误
func (a *App) handleBackendStartupFailure(ctx context.Context, phase string, err error) {
	// 先尝试回退到上一次成功的后端配置
	if a.tryRollbackBackend() {
		// 回退成功：推送"已回退"状态（而非"启动失败"），避免前端启动屏和后端弹窗信息不一致
		// C2 修复：之前此处推送的是"启动失败"，与后端弹窗"已自动回退"矛盾
		a.emitErrorStatus(ctx, fmt.Sprintf("后端启动失败，已自动回退到上一次配置：%v（请重启应用）", err))

		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:  runtime.WarningDialog,
			Title: "后端启动失败，已自动回退",
			Message: fmt.Sprintf(
				"%s失败：%v\n\n"+
					"已自动回退到上一次成功的后端配置。\n"+
					"点击「确定」后应用将退出，请重新启动应用使回退配置生效。",
				phase, err),
		})
		a.forceQuit()
		return
	}

	// 无可回退配置：推送错误状态 + 弹窗 + forceQuit
	// M2+M3 修复：之前此路径只弹窗不退出，应用处于不可用状态；
	// 且 WaitForReady 超时此路径无弹窗，与 Start 失败不一致。
	a.emitErrorStatus(ctx, fmt.Sprintf("%s失败：%v", phase, err))

	_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.ErrorDialog,
		Title:   "启动失败",
		Message: fmt.Sprintf("%s失败：%v\n\n应用将退出，请检查配置或后端文件后重新启动。", phase, err),
	})
	a.forceQuit()
}

// makeStatusCallback 创建服务器状态变化回调。
// 跳过启动失败和切换中状态，避免覆盖错误状态或干扰切换流程。
func (a *App) makeStatusCallback(ctx context.Context) func(llm.ServerStatus) {
	return func(status llm.ServerStatus) {
		// 启动/加载已彻底失败，不再让监控循环覆盖错误状态
		if a.serverLoadFailed.Load() {
			return
		}
		if a.isSwitching.Load() {
			return
		}
		a.currentModelMu.RLock()
		curModel := a.currentModelName
		a.currentModelMu.RUnlock()
		if status.Running {
			var caps llm.ModelCapabilities
			if a.service != nil {
				caps = a.service.GetModelCapabilities()
			} else {
				caps = llm.ModelCapabilities{TextInput: true}
			}
			status.Capabilities = &caps
			status.CurrentModel = curModel
		} else {
			a.serverReady.Store(false)
		}
		status.ModelReady = a.serverReady.Load()
		runtime.EventsEmit(ctx, "server:status", status)
	}
}

// makeRestartCallback 创建服务器重启后的回调。
// 重启后重新检测架构、重新加载模型，加载完成后才设置 serverReady。
func (a *App) makeRestartCallback(ctx context.Context) func() {
	return func() {
		// llama-server 重启后异步刷新 MCP 工具缓存（不依赖模型加载）。
		// 重启意味着新进程已就绪，可立即拉取最新工具列表。
		// 失败不影响主流程，下次 buildAvailableTools 时会再次尝试懒加载。
		go func() {
			defer func() {
				if r := recover(); r != nil {
					zlog.Warn().Interface("panic", r).Msg("[restart] refresh MCP tools panic")
				}
			}()
			if a.service != nil {
				a.service.RefreshMcpToolsCache()
			}
		}()

		a.currentModelMu.RLock()
		modelForDetect2 := a.currentModelName
		a.currentModelMu.RUnlock()
		if err := a.service.DetectModelArchitectureForModel(modelForDetect2); err != nil {
			zlog.Error().Err(err).Msg("detect model architecture after restart failed")
		}
		// 重启后重新加载当前模型，加载完成后才设置 serverReady
		if modelForDetect2 == "" || a.getClient() == nil {
			a.serverReady.Store(true)
			return
		}
		zlog.Info().Str("model", modelForDetect2).Msg("[server] reloading model after restart")
		loadErr := a.getClient().LoadModel(ctx, modelForDetect2)
		if loadErr != nil && !isAlreadyRunningError(loadErr) {
			zlog.Error().Err(loadErr).Str("model", modelForDetect2).Msg("[server] reload model after restart failed")
			a.emitErrorStatus(ctx, fmt.Sprintf("重启后模型加载失败: %v", loadErr))
			return
		}
		if loadErr != nil {
			zlog.Info().Str("model", modelForDetect2).Msg("[server] model is already running/loading after restart, waiting for loaded state")
		}
		if err := a.getClient().WaitForModelLoaded(ctx, modelForDetect2, 120*time.Second); err != nil {
			zlog.Error().Err(err).Str("model", modelForDetect2).Msg("[server] reload model wait after restart failed")
			a.emitErrorStatus(ctx, fmt.Sprintf("重启后模型加载超时: %v", err))
			return
		}
		zlog.Info().Str("model", modelForDetect2).Msg("[server] model reloaded and ready after restart")
		a.serverReady.Store(true)
		// 模型加载完成后重新检测架构，因为首次检测时 mmproj 可能尚未加载
		if err := a.service.DetectModelArchitectureForModel(modelForDetect2); err != nil {
			zlog.Warn().Err(err).Msg("[server] re-detect model architecture after restart load failed")
		}
		runtime.EventsEmit(ctx, "server:status", a.runningStatus())
	}
}
