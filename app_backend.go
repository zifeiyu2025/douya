// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"fmt"
	"path/filepath"

	"douya/internal/config"
	"douya/internal/llm"

	zlog "github.com/rs/zerolog/log"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// BackendStatus 后端状态信息（返回给前端）。
//
// 生活类比：就像车辆仪表盘上的"发动机状态"显示区——
// 当前用什么发动机（CurrentBackend）、用户选了什么模式（ConfigBackend）、
// 车上装了什么型号的发动机（GPUVendor/GPUName/GPUVRAMMB）、
// 手头有哪些发动机可用（InstalledBackends/AvailableBackends）。
type BackendStatus struct {
	// CurrentBackend 当前后端类型："cuda"/"vulkan"/"cpu" 等（已解析，不含 auto）。
	// 为空表示尚未启动或解析失败，前端可显示为 "auto"。
	CurrentBackend string `json:"current_backend"`
	// ConfigBackend 配置中的值："auto" 或具体后端（cuda/hip/sycl/vulkan/openvino/cpu）
	ConfigBackend string `json:"config_backend"`
	// GPUVendor 检测到的 GPU 厂商："nvidia"/"amd"/"intel"/"vulkan"/""（空表示未检测到）
	GPUVendor string `json:"gpu_vendor"`
	// GPUName GPU 名称（如 "NVIDIA GeForce RTX 4090"），无 GPU 时为空
	GPUName string `json:"gpu_name"`
	// GPUVRAMMB GPU 显存（MB），无 GPU 或检测失败时为 0
	GPUVRAMMB int64 `json:"gpu_vram_mb"`
	// InstalledBackends 已安装的后端列表（runtime 目录中已有 llama-server.exe 的后端）
	InstalledBackends []string `json:"installed_backends"`
	// AvailableBackends 所有可选后端列表（含 auto），供前端下拉框展示
	AvailableBackends []string `json:"available_backends"`
}

// GetBackendStatus 返回当前显卡后端状态，供前端展示和切换使用。
//
// 生活类比：就像驾驶员按下"车辆信息"按钮，仪表盘一次性返回当前发动机型号、
// 车上装的什么发动机、手头有哪些发动机可用——让驾驶员知道能否切换发动机。
func (a *App) GetBackendStatus() BackendStatus {
	cfg := a.getConfig()
	status := BackendStatus{
		ConfigBackend: cfg.BackendType,
	}

	// 解析当前后端：优先用 startup 中缓存的 resolvedBackend（已 EnsureBackendInstalled）
	// 生活类比：用车辆登记证上的"已装发动机型号"，而非用户配置的"自动/手动"选项
	if a.resolvedBackend != "" {
		status.CurrentBackend = string(a.resolvedBackend)
	} else if cfg.BackendType != "" && cfg.BackendType != "auto" {
		// 启动时未缓存但配置中明确指定了后端，直接用配置值
		status.CurrentBackend = cfg.BackendType
	} else {
		// auto 或未设置，显示为 "auto"
		status.CurrentBackend = "auto"
	}

	// GPU 信息：从启动时检测的硬件信息读取
	if a.hwInfo != nil {
		status.GPUVendor = a.hwInfo.GPUVendor
		status.GPUName = a.hwInfo.GPUName
		status.GPUVRAMMB = a.hwInfo.GPUVRAMMB
	}

	// 计算已安装后端列表：遍历所有后端类型（排除 auto），调用 IsBackendInstalled 检查
	runtimeDir := filepath.Join(appDir(), "runtime")
	allBackends := llm.AllBackendTypes()
	status.AvailableBackends = make([]string, 0, len(allBackends))
	status.InstalledBackends = make([]string, 0, len(allBackends))

	for _, bt := range allBackends {
		// AvailableBackends 包含所有后端（含 auto），用于前端下拉框
		status.AvailableBackends = append(status.AvailableBackends, string(bt))
		// InstalledBackends 仅含具体后端（排除 auto），auto 没有具体安装位置
		if bt == llm.BackendAuto {
			continue
		}
		if llm.IsBackendInstalled(bt, runtimeDir) {
			status.InstalledBackends = append(status.InstalledBackends, string(bt))
		}
	}

	return status
}

// SwitchBackend 切换显卡后端。
//
// 生活类比：就像驾驶员在仪表盘上选了个新发动机型号——
// 1. 先校验型号对不对（IsValidBackendType）
// 2. 把选择记到配置文件（UpdateConfig + Save）
// 3. 如果选了具体发动机（非 auto），确保它已经装到车上（EnsureBackendInstalled）
// 4. 推送状态更新到前端（EventsEmit）
//
// 注意：切换后端需要重启 llama-server 才能真正生效，因为后端类型在启动时确定。
// 此方法只更新配置并推送事件，不直接重启服务（重启逻辑复杂，且现有架构无独立 StopServer/StartServer）。
// 前端收到事件后提示用户"需要重启应用生效"。
func (a *App) SwitchBackend(bt string) error {
	// 步骤 1：校验后端类型合法性
	if !llm.IsValidBackendType(bt) {
		return fmt.Errorf("无效的后端类型: %q（合法值: auto/cuda/hip/sycl/vulkan/openvino/cpu）", bt)
	}

	zlog.Info().Str("backend", bt).Msg("[backend] 开始切换显卡后端")

	// 步骤 2：更新配置中的 backend_type
	cfg := a.getConfig()
	// 采用"复制→修改副本→替换指针"模式，避免直接修改 a.config 字段破坏快照语义
	newCfg := *cfg
	// 回退机制：把当前的后端类型存入 LastSuccessfulBackend（拍照片）。
	// 下次启动时若新后端启动失败，会根据该字段恢复旧后端（按照片装回去）。
	// 仅当新旧后端不同时才记录，避免重复切换写入相同值导致回退失效。
	if cfg.BackendType != bt {
		newCfg.LastSuccessfulBackend = cfg.BackendType
	}
	newCfg.BackendType = bt
	if err := newCfg.Validate(); err != nil {
		return fmt.Errorf("配置校验失败: %w", err)
	}
	a.setConfig(&newCfg)

	// 持久化配置到磁盘
	cfgPath := filepath.Join(appDir(), "config.json")
	if err := config.Save(cfgPath, &newCfg); err != nil {
		zlog.Error().Err(err).Msg("[backend] 保存配置失败")
		return fmt.Errorf("保存配置失败: %w", err)
	}

	if newCfg.LastSuccessfulBackend != "" {
		zlog.Info().Str("from", newCfg.LastSuccessfulBackend).Str("to", bt).
			Msg("[backend] 已记录旧后端，新后端启动失败时将自动回退")
	}

	// 步骤 3：如果选了具体后端（非 auto），确保后端已安装
	if bt != string(llm.BackendAuto) {
		runtimeDir := filepath.Join(appDir(), "runtime")
		if _, err := llm.EnsureBackendInstalled(llm.BackendType(bt), runtimeDir, nil); err != nil {
			// M6 修复：后端安装失败时不推送 backend:switched 事件。
			// 之前推送了 switched 事件，但此时 ConfigBackend 已改为新值，
			// 前端会误以为切换成功。改为只返回 error，由前端 catch 处理失败提示，
			// 并在前端刷新状态时通过 GetBackendStatus 重新获取真实状态。
			zlog.Error().Err(err).Str("backend", bt).Msg("[backend] 后端安装失败")
			return fmt.Errorf("后端 %s 安装失败: %w", bt, err)
		}
	}

	// 步骤 4：推送状态更新到前端
	// 前端收到事件后应刷新状态显示，并提示用户"切换已保存，重启应用后生效"
	status := a.GetBackendStatus()
	wailsruntime.EventsEmit(a.ctx, "backend:switched", status)

	zlog.Info().Str("backend", bt).Msg("[backend] 显卡后端切换成功（需重启应用生效）")
	return nil
}

// DownloadBackend 从 GitHub 下载指定后端的 zip 包并自动解压安装。
//
// 生活类比：驾驶员发现车上没装某款发动机，直接在仪表盘上点"下单购买"——
// 应用自动从 GitHub 仓库下载对应后端，运到本地车库（runtime 目录），
// 然后自动拆箱安装（解压）。全程通过事件推送进度，驾驶员只需等待。
//
// 该方法是异步的：立即返回 nil，下载和安装过程在后台 goroutine 中执行。
// 下载进度通过 "backend:downloadProgress" 事件推送到前端，
// 下载和安装完成后通过 "backend:downloadComplete" 事件通知前端。
//
// 参数：
//   - bt: 后端类型字符串（cuda/hip/sycl/vulkan/openvino/cpu，不能是 auto）
func (a *App) DownloadBackend(bt string) error {
	// 步骤 1：校验后端类型合法性（auto 不可下载，需先解析成具体后端）
	if !llm.IsValidBackendType(bt) || bt == string(llm.BackendAuto) {
		return fmt.Errorf("无效的后端类型: %q（合法值: cuda/hip/sycl/vulkan/openvino/cpu）", bt)
	}

	zlog.Info().Str("backend", bt).Msg("[backend] 开始从 GitHub 下载后端")

	// 步骤 2：异步下载+安装（goroutine），通过事件推送进度
	go func() {
		runtimeDir := filepath.Join(appDir(), "runtime")
		backendType := llm.BackendType(bt)

		// 下载 zip 包，进度通过事件推送
		zipPath, err := llm.DownloadBackendZip(backendType, runtimeDir, func(p llm.DownloadProgress) {
			wailsruntime.EventsEmit(a.ctx, "backend:downloadProgress", p)
		})
		if err != nil {
			zlog.Error().Err(err).Str("backend", bt).Msg("[backend] 下载后端 zip 包失败")
			// 推送失败进度
			wailsruntime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: backendType,
				Status:  "failed",
				Error:   err.Error(),
			})
			return
		}

		// 解压安装（推送按文件数的解压进度）
		zlog.Info().Str("backend", bt).Str("zip", zipPath).Msg("[backend] 下载完成，开始解压安装")
		wailsruntime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
			Backend:   backendType,
			Status:    "installing",
			Label:     "解压安装中",
			Percent:   0,
			AssetName: filepath.Base(zipPath),
		})

		serverPath, installErr := llm.EnsureBackendInstalled(backendType, runtimeDir, func(current, total int) {
			percent := 0.0
			if total > 0 {
				percent = float64(current) / float64(total) * 100
			}
			wailsruntime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: backendType,
				Status:  "installing",
				Label:   "解压安装中",
				Percent: percent,
			})
		})

		// 推送最终完成事件
		completePayload := map[string]any{
			"backend": bt,
			"success": installErr == nil,
		}
		if installErr != nil {
			completePayload["error"] = installErr.Error()
		} else {
			completePayload["server_path"] = serverPath
		}
		wailsruntime.EventsEmit(a.ctx, "backend:downloadComplete", completePayload)

		// 推送 completed 状态的进度事件，让前端进度条更新到100%
		if installErr == nil {
			wailsruntime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: backendType,
				Status:  "completed",
				Label:   "下载完成",
				Percent: 100,
			})
		}

		// 刷新后端状态并推送（更新已安装后端列表）
		status := a.GetBackendStatus()
		wailsruntime.EventsEmit(a.ctx, "backend:switched", status)

		if installErr != nil {
			zlog.Error().Err(installErr).Str("backend", bt).Msg("[backend] 解压安装失败")
		} else {
			zlog.Info().Str("backend", bt).Str("path", serverPath).Msg("[backend] 下载并安装完成")
		}
	}()

	return nil
}

// tryRollbackBackend 在后端启动失败时尝试回退到上一次成功的后端配置。
//
// 生活类比：新发动机打不着火，按照之前拍的照片把旧发动机装回去。
//
// 触发条件（同时满足才回退）：
//  1. LastSuccessfulBackend 非空（说明用户切换过后端，备份存在）
//  2. LastSuccessfulBackend 是合法后端类型
//  3. LastSuccessfulBackend 与当前 BackendType 不同（否则回退无意义）
//
// 回退行为：
//  - 将 BackendType 恢复为 LastSuccessfulBackend
//  - 清空 LastSuccessfulBackend（避免下次启动再次回退，形成死循环）
//  - 持久化到磁盘
//  - 返回 true 表示已执行回退，调用方应退出应用让用户重启
//
// 返回 false 表示未执行回退（无备份或不满足条件），调用方按原错误流程处理。
func (a *App) tryRollbackBackend() bool {
	cfg := a.getConfig()
	prev := cfg.LastSuccessfulBackend
	if prev == "" {
		return false
	}
	if !llm.IsValidBackendType(prev) {
		zlog.Warn().Str("prev", prev).Msg("[backend-rollback] 备份后端类型非法，放弃回退")
		return false
	}
	if prev == cfg.BackendType {
		// 新旧相同说明已回退过一次，清空备份避免循环
		zlog.Warn().Msg("[backend-rollback] 备份与当前后端相同，清空备份")
		a.clearBackendRollback()
		return false
	}

	zlog.Warn().Str("from", cfg.BackendType).Str("to", prev).
		Msg("[backend-rollback] 检测到启动失败，回退到上一次成功的后端")

	// 恢复旧后端配置，并清空 LastSuccessfulBackend（避免下次启动再次回退）
	newCfg := *cfg
	newCfg.BackendType = prev
	newCfg.LastSuccessfulBackend = ""
	if err := newCfg.Validate(); err != nil {
		zlog.Error().Err(err).Msg("[backend-rollback] 配置校验失败，放弃回退")
		return false
	}
	a.setConfig(&newCfg)

	cfgPath := filepath.Join(appDir(), "config.json")
	if err := config.Save(cfgPath, &newCfg); err != nil {
		zlog.Error().Err(err).Msg("[backend-rollback] 保存回退配置失败")
		return false
	}

	zlog.Info().Str("backend", prev).Msg("[backend-rollback] 配置已回退，需重启应用生效")
	return true
}

// clearBackendRollback 清空 LastSuccessfulBackend 字段（启动成功后调用）。
//
// 生活类比：新发动机成功打着火了，之前拍的照片就可以撕掉了，
// 避免下次正常启动时被误判为"需要回退"。
func (a *App) clearBackendRollback() {
	cfg := a.getConfig()
	if cfg.LastSuccessfulBackend == "" {
		return
	}
	newCfg := *cfg
	newCfg.LastSuccessfulBackend = ""
	a.setConfig(&newCfg)

	cfgPath := filepath.Join(appDir(), "config.json")
	if err := config.Save(cfgPath, &newCfg); err != nil {
		zlog.Warn().Err(err).Msg("[backend] 清空回退备份失败（不影响运行）")
	} else {
		zlog.Info().Msg("[backend] 新后端启动成功，已清空回退备份")
	}
}
