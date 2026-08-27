package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"douya/internal/apperror"
	"douya/internal/llm"
	"douya/internal/logger"

	zlog "github.com/rs/zerolog/log"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	httpClientTimeout = 300 * time.Second // 普通 HTTP 请求超时
	loadTimeoutBase   = 180 * time.Second // 模型加载基础超时
	loadTimeoutPerGB  = 30 * time.Second  // 每 GB 额外超时
	loadTimeoutMax    = 600 * time.Second // 模型加载最大超时
	apiTimeoutShort   = 10 * time.Second  // 轻量 API 超时
	apiTimeoutMedium  = 30 * time.Second  // 普通 API 超时
)

// resolveMediaPath 解析媒体路径并检查目录是否存在，不存在则返回空字符串
// 防止 llama-server 因 --media-path 指向不存在的目录而启动失败
func (a *App) resolveMediaPath(mediaPath string) string {
	if mediaPath == "" {
		return ""
	}
	resolved := resolvePath(mediaPath)
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		zlog.Warn().Str("media_path", resolved).Msg("[config] media path directory does not exist, skipping")
		return ""
	}
	return resolved
}

// watchServerHealth 监控服务器健康状态，崩溃时自动重启。
// 从 startServerAndWatch() 抽出以降低单函数复杂度。
// 作为独立 goroutine 运行，与原匿名 goroutine 逻辑等价。
func (a *App) watchServerHealth(ctx context.Context, watchCtx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	consecutiveCrashes := 0
	const maxConsecutiveCrashes = 3

	for {
		select {
		case <-watchCtx.Done():
			return
		case <-ticker.C:
		}

		// 每次循环体用 recover 保护，避免单次 panic 永久终止健康监控或崩溃整个进程（M-4）
		// 原 continue 改为 return（从闭包返回，等价于跳过本次循环）；
		// 原 return（serverLoadFailed）通过 stop 标志传出闭包
		stop := false
		func() {
			defer recoverLog("[router-monitor] health check panic, will retry next tick")

			// 启动/加载已彻底失败，停止健康监控，避免覆盖错误状态
			if a.serverLoadFailed.Load() {
				stop = true
				return
			}

			// 正在生成时跳过轮询，避免与生成请求争用 HTTP 连接池
			if a.service != nil && a.service.IsGenerating() {
				return
			}

			// 不在切换中且 serverReady 为 true 时才检查
			if a.modelSessionSnapshot().Switching || !a.serverReady.Load() {
				return
			}

			modelName := a.currentModel()
			if modelName == "" || a.getClient() == nil {
				return
			}

			status, err := a.getClient().GetModelStatus(watchCtx, modelName)
			if err != nil {
				// 查询失败可能是暂时的网络问题，跳过
				return
			}

			switch status.Status {
			case "loaded", "sleeping":
				// 模型正常运行，重置崩溃计数
				if consecutiveCrashes > 0 {
					zlog.Info().Str("model", modelName).Msg("[router-monitor] model recovered, resetting crash count")
					consecutiveCrashes = 0
				}
			case "unloaded", "failed":
				consecutiveCrashes++
				exitInfo := ""
				if status.ExitCode != 0 {
					exitInfo = fmt.Sprintf(" (exit_code=%d)", status.ExitCode)
				}
				zlog.Warn().
					Str("model", modelName).
					Str("status", status.Status).
					Bool("failed", status.Failed).
					Int("crash_count", consecutiveCrashes).
					Msg("[router-monitor] model became unloaded/failed" + exitInfo)

				// 获取 stderr 诊断信息
				stderrHint := a.getServerStderrHint()
				if stderrHint != "" {
					zlog.Warn().Str("stderr_hint", stderrHint).Msg("[router-monitor] server stderr hint")
				}

				if consecutiveCrashes > maxConsecutiveCrashes {
					zlog.Error().Str("model", modelName).Msg("[router-monitor] model keeps crashing, giving up auto-reload")
					a.serverReady.Store(false)
					a.emitErrorStatus(ctx, fmt.Sprintf("模型 %s 反复崩溃，请检查模型文件是否损坏或显存是否充足", modelName))
					return
				}

				// 自动重新加载模型
				zlog.Info().Str("model", modelName).Msg("[router-monitor] attempting to reload crashed model")
				a.serverReady.Store(false)
				wailsruntime.EventsEmit(ctx, EventServerStatus, switchingStatus(modelName))

				loadErr := a.getClient().LoadModel(watchCtx, modelName)
				if loadErr != nil && !isAlreadyRunningError(loadErr) {
					zlog.Error().Err(loadErr).Str("model", modelName).Msg("[router-monitor] reload failed")
					a.emitErrorStatus(ctx, fmt.Sprintf("模型重新加载失败: %v", loadErr))
					return
				}

				if err := a.getClient().WaitForModelLoaded(watchCtx, modelName, 120*time.Second); err != nil {
					zlog.Error().Err(err).Str("model", modelName).Msg("[router-monitor] reload wait failed")
					// 用 errors.Is 精准区分错误类型，给用户更准确的提示
					errMsg := fmt.Sprintf("模型重新加载失败: %v", err)
					if errors.Is(err, apperror.ErrTimeout) {
						errMsg = fmt.Sprintf("模型重新加载超时: %v", err)
					} else if errors.Is(err, apperror.ErrUnavailable) {
						errMsg = fmt.Sprintf("模型重新加载时服务崩溃: %v", err)
					}
					a.emitErrorStatus(ctx, errMsg)
				} else {
					zlog.Info().Str("model", modelName).Msg("[router-monitor] model reloaded successfully")
					a.serverReady.Store(true)
					// 监控重载成功后清除"加载失败"标记，恢复健康检查与状态推送
					a.clearServerLoadFailure()
					a.emitSwitchSuccess(modelName)
				}
			}
		}()
		if stop {
			return
		}
	}
}

func (a *App) GetServerStatus() llm.ServerStatus {
	// 空模型（models 目录为空）属正常首次使用状态：稳定返回"模型目录为空"，
	// 供前端轮询/事件兜底识别后放行引导流程，而非误报为加载失败。
	if a.modelsEmpty.Load() {
		return llm.ServerStatus{
			Running:    false,
			ModelReady: false,
			Error:      "模型目录为空，请下载 .gguf 模型文件后放入 models 目录",
		}
	}
	// 若已记录启动/加载失败，优先返回持久化错误状态，避免监控循环覆盖
	if a.serverLoadFailed.Load() {
		a.lastServerErrMu.RLock()
		errMsg := a.lastServerError
		a.lastServerErrMu.RUnlock()
		return llm.ServerStatus{
			Running:    false,
			ModelReady: false,
			Error:      errMsg,
		}
	}
	a.serverMu.RLock()
	srv := a.server
	a.serverMu.RUnlock()
	if srv != nil {
		status := srv.Status()
		if status.Running {
			if a.service != nil {
				caps := a.service.GetModelCapabilities()
				status.Capabilities = &caps
			}
			status.CurrentModel = a.currentModel()
		}
		status.ModelReady = a.serverReady.Load()
		if snapshot := a.modelSessionSnapshot(); snapshot.Switching {
			status.Switching = true
			status.SwitchingTo = snapshot.SwitchingTo
		}
		return status
	}
	return llm.ServerStatus{Running: false, Error: "server not initialized"}
}

// GetMetrics 获取 llama-server 的 Prometheus 指标摘要。
// 前提：配置中 Metrics=true（启动参数 --metrics 已启用 /metrics 端点）。
// 生活类比：去医院体检拿到原始报告后，挑出关键指标整理成一页摘要给用户看。
//
// 豆芽使用 router 模式，/metrics 端点走 proxy_get，必须传 model 参数
// 路由到对应子进程。这里自动获取当前已加载的模型名传递。
// 返回解析后的结构化指标；若 /metrics 端点未启用或请求失败，返回错误。
func (a *App) GetMetrics() (llm.MetricsSummary, error) {
	if a.getClient() == nil {
		return llm.MetricsSummary{}, apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	// router 模式下必须传 model 参数，否则返回 400 "model name is missing from the request"
	modelName := a.currentModel()
	if modelName == "" {
		return llm.MetricsSummary{}, apperror.New(apperror.KindUnavailable, "当前无已加载模型，无法获取指标（router 模式需要 model 参数）")
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutShort)
	defer cancel()
	text, err := a.getClient().GetMetrics(ctx, modelName)
	if err != nil {
		return llm.MetricsSummary{}, apperror.Wrap(apperror.KindInternal, "获取指标失败", err)
	}
	return llm.ParseMetrics(text), nil
}

func (a *App) runningStatus() llm.ServerStatus {
	caps := a.service.GetModelCapabilities()
	modelName := a.currentModel()
	return llm.ServerStatus{
		Running:      true,
		ModelReady:   a.serverReady.Load(),
		CurrentModel: modelName,
		Capabilities: &caps,
	}
}

func (a *App) StopGeneration() {
	if a.service != nil {
		a.service.StopGeneration()
	}
}

// StopThinking 发送推理控制请求，强制结束当前思考块。
// 用户在流式推理过程中点击"直接回答"按钮时调用。
// 生活类比：就像你在考试时监考老师说"思考时间到，开始答题"，模型会立即结束思考开始输出答案。
func (a *App) StopThinking() error {
	if a.getClient() == nil {
		return apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if a.service == nil {
		return apperror.New(apperror.KindUnavailable, "服务未初始化")
	}
	completionID := a.service.GetCurrentCompletionID()
	if completionID == "" {
		return apperror.New(apperror.KindConflict, "当前没有正在进行的推理")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return a.getClient().StopThinking(ctx, completionID)
}

// GetServerLogs 获取 llama-server 控制台的最近日志
func (a *App) GetServerLogs() string {
	srv := a.getServer()
	if srv == nil {
		return ""
	}
	return srv.LastOutput()
}

// GetTerminalHistory 获取终端历史日志（纯文本，用于 xterm.js 初始化时回显）
// B-2.1：与 GetServerLogs 实现完全相同，委托调用避免重复
func (a *App) GetTerminalHistory() string {
	return a.GetServerLogs()
}

// ResizeTerminal 调整 ConPTY 终端尺寸（前端 xterm.js 尺寸变化时调用）
func (a *App) ResizeTerminal(cols, rows int) error {
	srv := a.getServer()
	if srv == nil {
		return nil
	}
	if err := validatePositiveInt("终端列数", cols); err != nil {
		return err
	}
	if err := validatePositiveInt("终端行数", rows); err != nil {
		return err
	}
	return srv.ResizeTerminal(cols, rows)
}

// IsConPTYMode 返回当前是否使用 ConPTY 模式（前端据此决定用 xterm.js 还是文本日志）
func (a *App) IsConPTYMode() bool {
	srv := a.getServer()
	if srv == nil {
		return false
	}
	return srv.IsConPTYMode()
}

// ===== D3: 日志级别动态调整 =====
//
// 生活类比：像电视机的音量按钮——不用关机重启，运行中随时可以调大调小。
// 排查问题时调成 "debug" 看详细信息，日常使用调成 "warn" 减少噪音。

// SetLogLevel 动态调整全局日志级别。
// 前端可通过 Wails IPC 调用：await window.go.main.App.SetLogLevel("debug")
// 支持的级别（不区分大小写）：trace / debug / info / warn / error / fatal / panic / disabled
func (a *App) SetLogLevel(level string) error {
	return logger.SetLevel(level)
}

// GetLogLevel 返回当前全局日志级别字符串
// 前端可通过 Wails IPC 调用：const level = await window.go.main.App.GetLogLevel()
func (a *App) GetLogLevel() string {
	return logger.GetLevel()
}
