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

	// 步骤 3：如果选了具体后端（非 auto），确保后端已安装
	if bt != string(llm.BackendAuto) {
		runtimeDir := filepath.Join(appDir(), "runtime")
		if _, err := llm.EnsureBackendInstalled(llm.BackendType(bt), runtimeDir); err != nil {
			// 后端安装失败：不回滚配置（用户可手动再切换或重启应用）
			// 但要推送错误状态让前端知道
			zlog.Error().Err(err).Str("backend", bt).Msg("[backend] 后端安装失败")
			status := a.GetBackendStatus()
			wailsruntime.EventsEmit(a.ctx, "backend:switched", status)
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
