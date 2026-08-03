package main

import (
	"fmt"
	"time"

	"douya/internal/apperror"
	"douya/internal/llm"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// downloadBackendWithRetry 带重试的后端下载安装，仅用于启动阶段。
// 每次失败后推送"重试中"进度事件到前端，全部失败后返回最后一次错误。
//
// 生活类比：网购发货时快递可能中途丢失，签收失败就联系卖家重发，
// 最多重发 maxRetries 次，全部失败才放弃。
func (a *App) downloadBackendWithRetry(bt llm.BackendType, runtimeDir string, maxRetries int) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		zlog.Info().Str("backend", bt.String()).Int("attempt", attempt).Int("max", maxRetries).
			Msg("[startup] 下载后端尝试")
		// 推送重试进度到前端
		if attempt > 1 {
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: bt,
				Status:  "retrying",
				Error:   fmt.Sprintf("第 %d/%d 次重试中...", attempt, maxRetries),
			})
		}
		if err := a.downloadAndInstallBackend(bt, runtimeDir); err != nil {
			lastErr = err
			zlog.Warn().Err(err).Int("attempt", attempt).Int("max", maxRetries).
				Msg("[startup] 下载后端失败")
			continue
		}
		return nil
	}
	// 全部重试失败：补推 downloadComplete 事件（success=false），确保前端能收到完成通知
	runtime.EventsEmit(a.ctx, "backend:downloadComplete", map[string]any{
		"backend": bt.String(),
		"success": false,
		"error":   fmt.Sprintf("已重试 %d 次仍失败: %v", maxRetries, lastErr),
	})
	return apperror.Wrapf(apperror.KindInternal, "下载后端失败（已重试 %d 次）", lastErr, maxRetries)
}

// downloadAndInstallBackend 下载并安装后端，CUDA 后端会额外下载并解压 cudart 包。
// 下载和安装过程通过事件推送进度到前端，完成后自动重启应用。
//
// 生活类比：买发动机时，CUDA 发动机需要额外配一套"管线配件包"（cudart），
// 两包货都到齐后才能装车。其他发动机（CPU/Vulkan 等）一包就够了。
func (a *App) downloadAndInstallBackend(bt llm.BackendType, runtimeDir string) error {
	// 步骤 1：下载后端主包（推理引擎 + 核心 DLL）
	zlog.Info().Str("backend", bt.String()).Msg("[startup] 开始下载后端主包")
	_, dlErr := llm.DownloadBackendZip(bt, runtimeDir, func(p llm.DownloadProgress) {
		p.Label = "推理后端"
		runtime.EventsEmit(a.ctx, "backend:downloadProgress", p)
	})
	if dlErr != nil {
		return apperror.Wrap(apperror.KindInternal, "下载后端主包失败", dlErr)
	}

	// 步骤 2：CUDA 后端额外下载 cudart 包
	// cudart 包提供 cudart64_*.dll、cublas64_*.dll 等厂商运行时 DLL，
	// 需解压到与主包相同的目录（runtime/cuda/）才能被 llama-server 找到。
	if bt == llm.BackendCUDA {
		zlog.Info().Msg("[startup] CUDA 后端检测到，开始下载 cudart 包")
		// 通知前端切换到 cudart 下载阶段
		runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
			Backend: bt,
			Status:  "downloading",
			Label:   "cudart 依赖包",
			Percent: 0,
		})
		_, cudartErr := llm.DownloadCudartZip(runtimeDir, func(p llm.DownloadProgress) {
			p.Label = "cudart 依赖包"
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", p)
		})
		if cudartErr != nil {
			// cudart 下载失败直接返回错误，交由上层 downloadBackendWithRetry 重试。
			// 如果继续解压主包并重启，重启后会因厂商 DLL 缺失再次提示下载，形成无限循环。
			// 生活类比：配件包裹没到，整车就装不完整，与其装一半重启后又说缺配件，
			// 不如直接让上层重试，等配件包裹也到齐了再装车。
			zlog.Warn().Err(cudartErr).Msg("[startup] cudart 包下载失败，交由重试逻辑处理")
			return apperror.Wrap(apperror.KindInternal, "下载 cudart 包失败", cudartErr)
		}
	}

	// 步骤 3：解压安装（推送按文件数的解压进度）
	zlog.Info().Str("backend", bt.String()).Msg("[startup] 下载完成，开始解压安装")
	runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
		Backend: bt,
		Status:  "installing",
		Label:   "解压安装中",
		Percent: 0,
	})

	_, installErr := llm.EnsureBackendInstalled(bt, runtimeDir, func(current, total int) {
		percent := 0.0
		if total > 0 {
			percent = float64(current) / float64(total) * 100
		}
		runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
			Backend: bt,
			Status:  "installing",
			Label:   "解压安装中",
			Percent: percent,
		})
	})

	// complete 事件推送规则（避免重复推送）：
	// - 成功时：此处直接推送 complete {success: true}
	// - 失败时：此处不推送，由上层 downloadBackendWithRetry 在重试耗尽后统一推送
	//   原因：若此处推送失败事件，重试循环中前端会收到多次失败弹窗（C1 修复）
	if installErr != nil {
		return apperror.Wrap(apperror.KindInternal, "解压安装失败", installErr)
	}

	// 步骤 3.5：CUDA 后端额外解压 cudart 包到同一目录
	// cudart 包提供 cudart64_*.dll、cublas64_*.dll 等厂商运行时 DLL，
	// 必须解压到与主包相同的目录（runtime/cuda/）才能被 llama-server 找到。
	// 如果不解压，validatePaths 会检测到厂商 DLL 缺失，导致下次启动时又提示下载（无限循环）。
	if bt == llm.BackendCUDA {
		zlog.Info().Msg("[startup] CUDA 后端检测到，开始解压 cudart 包")
		runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
			Backend: bt,
			Status:  "installing",
			Label:   "安装 cudart 依赖包",
			Percent: 0,
		})
		if cudartInstallErr := llm.EnsureCudartInstalled(runtimeDir, func(current, total int) {
			percent := 0.0
			if total > 0 {
				percent = float64(current) / float64(total) * 100
			}
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: bt,
				Status:  "installing",
				Label:   "安装 cudart 依赖包",
				Percent: percent,
			})
		}); cudartInstallErr != nil {
			// cudart 解压失败不阻止重启：主包已安装，用户系统 PATH 中可能有 cudart
			// 但 validatePaths 会检测到缺失，下次启动可能又提示下载
			zlog.Warn().Err(cudartInstallErr).Msg("[startup] cudart 包解压失败")
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: bt,
				Status:  "cudart_failed",
				Label:   "cudart 依赖包",
				Error:   fmt.Sprintf("cudart 依赖包解压失败：%v", cudartInstallErr),
			})
		}
	}

	// 成功：推送 complete 事件
	runtime.EventsEmit(a.ctx, "backend:downloadComplete", map[string]any{
		"backend": bt.String(),
		"success": true,
	})

	zlog.Info().Str("backend", bt.String()).Msg("[startup] 下载并安装完成，准备自动重启应用")

	// 推送"重启中"状态，前端据此显示"重启中"文字
	runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
		Backend: bt,
		Status:  "completed",
		Label:   "重启中",
		Percent: 100,
	})

	// 延迟 1 秒后自动重启应用，给前端时间显示"重启中"状态
	go func() {
		// 防止 panic 导致整个进程崩溃
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[startup] 自动重启 goroutine panic")
			}
		}()
		time.Sleep(1 * time.Second)
		a.RestartApp()
	}()
	return nil
}
