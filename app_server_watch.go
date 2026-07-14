// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"fmt"
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
func (a *App) startServerAndWaitReady(srv *llm.Server, ctx context.Context) error {
	if err := srv.Start(); err != nil {
		zlog.Error().Err(err).Msg("start llama-server failed")
		a.emitErrorStatus(ctx, fmt.Sprintf("启动 llama-server 失败: %v", err))
		// 配置类错误（如局域网暴露未配置 API Key）弹出对话框，确保用户立即感知
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "启动失败",
			Message: fmt.Sprintf("启动 llama-server 失败: %v", err),
		})
		return err
	}

	if err := srv.WaitForReady(60e9); err != nil {
		zlog.Error().Err(err).Msg("wait for server ready failed")
		a.emitErrorStatus(ctx, fmt.Sprintf("llama-server 未就绪: %v", err))
		return err
	}

	// 推送首次启动进度：引擎已就绪，准备加载模型
	a.emitSwitchProgressCtx(ctx, "loading", "", nil)
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
		if p.Alias == "default" {
			a.currentModelMu.Lock()
			a.currentModelName = p.Name
			a.currentModelMu.Unlock()
			if a.client != nil {
				a.client.SetCurrentModel(p.Name)
			}
			foundDefault = true
			break
		}
	}
	if !foundDefault && len(presetsSnapshot) > 0 {
		a.currentModelMu.Lock()
		a.currentModelName = presetsSnapshot[0].Name
		a.currentModelMu.Unlock()
		if a.client != nil {
			a.client.SetCurrentModel(presetsSnapshot[0].Name)
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
	if modelForDetect == "" || a.client == nil {
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

	loadErr := a.client.LoadModel(ctx, modelForDetect)
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

	if err := a.client.WaitForModelLoaded(ctx, modelForDetect, httpClientTimeout, progressCallback); err != nil {
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
func (a *App) handleModelLoadSuccess(ctx context.Context, modelForDetect string, logMsg string) {
	zlog.Info().Str("model", modelForDetect).Msg(logMsg)
	a.serverReady.Store(true)
	// 模型加载完成后重新检测架构，因为首次检测时 mmproj 可能尚未加载
	if err := a.service.DetectModelArchitectureForModel(modelForDetect); err != nil {
		zlog.Warn().Err(err).Msg("[server] re-detect model architecture after load failed")
	}
	a.emitSwitchSuccess(modelForDetect)
}

// handleModelLoadFailure 处理模型加载失败：先尝试去掉 mmproj 重试，
// 若不适用或重试失败，则启动后台 goroutine 继续等待（依赖 WatchWithCallback 检测崩溃）。
func (a *App) handleModelLoadFailure(ctx context.Context, modelForDetect string, err error, progressCallback func(int, string)) {
	// 加载失败，尝试去掉 mmproj 重试（mmproj 不兼容会导致子进程崩溃）
	if a.tryReloadWithoutMmproj(ctx, modelForDetect, progressCallback) {
		// 重试成功
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
		if bgErr := a.client.WaitForModelLoaded(bgCtx, modelForDetect, loadTimeoutMax, progressCallback); bgErr != nil {
			zlog.Error().Err(bgErr).Str("model", modelForDetect).Msg("[server] auto-load default model background wait also failed")
			// 修复：将 Running 改为 false，与 Error 字段保持语义一致
			// 此前 Running: true 会导致前端 `!status.running && status.error` 条件失效，错误被静默丢弃
			a.emitErrorStatus(ctx, fmt.Sprintf("默认模型加载失败: %v（可手动切换模型）", bgErr))
			return
		}
		a.handleModelLoadSuccess(ctx, modelForDetect, "[server] default model loaded and ready (background)")
	})
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
		a.currentModelMu.RLock()
		modelForDetect2 := a.currentModelName
		a.currentModelMu.RUnlock()
		if err := a.service.DetectModelArchitectureForModel(modelForDetect2); err != nil {
			zlog.Error().Err(err).Msg("detect model architecture after restart failed")
		}
		// 重启后重新加载当前模型，加载完成后才设置 serverReady
		if modelForDetect2 == "" || a.client == nil {
			a.serverReady.Store(true)
			return
		}
		zlog.Info().Str("model", modelForDetect2).Msg("[server] reloading model after restart")
		loadErr := a.client.LoadModel(ctx, modelForDetect2)
		if loadErr != nil && !isAlreadyRunningError(loadErr) {
			zlog.Error().Err(loadErr).Str("model", modelForDetect2).Msg("[server] reload model after restart failed")
			a.emitErrorStatus(ctx, fmt.Sprintf("重启后模型加载失败: %v", loadErr))
			return
		}
		if loadErr != nil {
			zlog.Info().Str("model", modelForDetect2).Msg("[server] model is already running/loading after restart, waiting for loaded state")
		}
		if err := a.client.WaitForModelLoaded(ctx, modelForDetect2, 120*time.Second); err != nil {
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
