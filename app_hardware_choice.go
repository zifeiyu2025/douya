// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"fmt"
	"time"

	"douya/internal/apperror"
	"douya/internal/llm"
	"douya/internal/system"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// GpuTypeUnknownPayload 是 EventHardwareGpuTypeUnknown 事件发送给前端的数据。
// 前端用这些信息在对话框中展示 GPU 详情，帮助用户判断该选哪个后端。
//
// 生活类比：车检员交给车主的"车况表"——写明车辆名称、排量、车检员无法判定的原因，
// 让车主根据这些信息决定开哪种驾驶模式。
type GpuTypeUnknownPayload struct {
	GPUName        string `json:"gpu_name"`        // 检测到的 GPU 名称（可能为空）
	GPUVendor      string `json:"gpu_vendor"`      // GPU 厂商：amd/intel（不会是 nvidia，NVIDIA 一律视为独显）
	GPUVRAMMB      int64  `json:"gpu_vram_mb"`     // 专用显存（MB），0 表示无数据
	GPUType        string `json:"gpu_type"`        // 当前分类结果（固定 "unknown"）
	TimeoutSeconds int    `json:"timeout_seconds"` // 超时秒数，前端可显示倒计时
}

// ResolveGpuTypeChoice 由前端在用户选择后端后调用，解除 installBackend 的阻塞等待。
//
// 触发场景：灰色地带（GPUType=unknown + BackendType=auto）时，前端弹出对话框让用户选择，
// 用户选择后调用此方法把选择结果传回后端。
//
// 参数 backend 的合法值：
//   - "cpu"      我不清楚 / 核显设备（最稳定，推荐核显/未知设备）
//   - "vulkan"   跨厂商 GPU 加速
//   - "cuda"     NVIDIA 独显
//   - "hip"      AMD 独显
//   - "sycl"     Intel 独显
//
// 设计为非阻塞写入 channel（select + default），即使 channel 已被超时关闭也不会 panic。
// 生活类比：车主填好回执后投入信箱（写 channel）——如果车检员已经下班（超时走人），
// 回执就作废（写入被丢弃），不会卡住车主。
func (a *App) ResolveGpuTypeChoice(backend string) error {
	if !llm.IsValidBackendType(backend) || backend == "auto" {
		return apperror.Newf(apperror.KindInvalidInput,
			"无效的后端选择: %q（合法值: cpu/vulkan/cuda/hip/sycl）", backend)
	}

	zlog.Info().Str("backend", backend).Msg("[hardware] 用户选择了推理后端")

	// 非阻塞写入：如果 channel 已被超时关闭（接收方已走），写入会被丢弃
	if a.gpuTypeChoiceChan != nil {
		select {
		case a.gpuTypeChoiceChan <- backend:
		default:
			zlog.Warn().Str("backend", backend).Msg("[hardware] gpuTypeChoiceChan 已关闭，用户选择被丢弃（可能已超时）")
		}
	}
	return nil
}

// gpuTypeChoiceTimeout 是灰色地带用户选择的超时时间。
// 超时后默认走 auto 流程（按厂商推断），不再等待。
const gpuTypeChoiceTimeout = 60 * time.Second

// waitForGpuTypeChoice 发送事件通知前端弹出选择对话框，并阻塞等待用户选择。
//
// 返回用户选择的后端字符串（如 "cpu"/"vulkan"/"cuda"），超时则返回空字符串。
// 调用方根据返回值决定是否覆盖配置中的 BackendType。
//
// 阻塞设计说明：此函数在 startup 主流程中调用，会阻塞 startup 直到用户选择或超时。
// 这是有意为之——后端选择必须确定后才能继续安装后端和启动服务。
// 前端在等待期间会显示加载状态（startup 还没完成，前端无法交互，
// 但 Wails 的 EventsEmit 可以在 startup 期间触发前端监听）。
//
// 生活类比：车检员把车况表交给车主后站在门口等回执，最多等 60 秒。
// 车主选好投入信箱（ResolveGpuTypeChoice），车检员拿到回执继续办手续。
// 60 秒到了还没拿到回执，车检员就按默认流程（auto）办。
func (a *App) waitForGpuTypeChoice(ctx context.Context) string {
	if a.hwInfo == nil {
		return ""
	}

	// 创建 channel 并保存到 App 字段，供 ResolveGpuTypeChoice 写入
	ch := make(chan string, 1)
	a.gpuTypeChoiceChan = ch
	defer func() {
		// 清理：函数返回后置 nil，避免后续误写
		a.gpuTypeChoiceChan = nil
	}()

	// 构造事件 payload 发送给前端
	payload := GpuTypeUnknownPayload{
		GPUName:        a.hwInfo.GPUName,
		GPUVendor:      a.hwInfo.GPUVendor,
		GPUVRAMMB:      a.hwInfo.GPUVRAMMB,
		GPUType:        a.hwInfo.GPUType,
		TimeoutSeconds: int(gpuTypeChoiceTimeout / time.Second),
	}

	zlog.Info().
		Str("gpu", a.hwInfo.GPUName).
		Str("vendor", a.hwInfo.GPUVendor).
		Int64("vram_mb", a.hwInfo.GPUVRAMMB).
		Msg("[startup] 检测到灰色地带 GPU，通知前端让用户选择推理后端")

	runtime.EventsEmit(ctx, EventHardwareGpuTypeUnknown, payload)

	// 阻塞等待用户选择或超时
	// 同时监听 ctx.Done()：如果应用退出（如用户关窗），不再等待
	select {
	case chosen := <-ch:
		zlog.Info().Str("backend", chosen).Msg("[startup] 收到用户的后端选择")
		return chosen
	case <-time.After(gpuTypeChoiceTimeout):
		zlog.Warn().Dur("timeout", gpuTypeChoiceTimeout).Msg("[startup] 等待用户选择超时，回退到 auto 流程")
		return ""
	case <-ctx.Done():
		zlog.Info().Msg("[startup] 应用关闭，取消等待用户选择")
		return ""
	}
}

// 兼容性占位：确保 system 包被引用（灰色地带检测用到 system.GPUTypeUnknown）
var _ = system.GPUTypeUnknown

// askUseCpuFallback 弹出原生对话框，询问用户是否回退到 CPU 后端。
//
// 触发场景：灰色地带用户选择的后端（如 CUDA/HIP/SYCL）安装或加载失败时调用。
// 文案设计原则：专业友好，不责怪用户，给出明确建议。
//
// 参数 failedBackend：失败的后端名称（如 "cuda"/"hip"/"sycl"），用于展示。
// 返回 true 表示用户同意回退 CPU，false 表示用户拒绝（调用方应退出应用）。
//
// 生活类比：车检员让车主选了驾驶模式，结果发现这辆车不支持那个模式，
// 车检员礼貌地回来告诉车主"这个模式用不了，建议换成稳妥的 CPU 模式，您看行吗？"
func (a *App) askUseCpuFallback(ctx context.Context, failedBackend string) bool {
	// 后端中文名映射，让提示更友好
	backendCN := map[string]string{
		"cuda":   "CUDA（NVIDIA）",
		"hip":    "HIP（AMD）",
		"sycl":   "SYCL（Intel）",
		"vulkan": "Vulkan",
	}
	backendLabel := backendCN[failedBackend]
	if backendLabel == "" {
		backendLabel = failedBackend
	}

	msg := fmt.Sprintf(
		"您选择的推理后端 %s 在当前设备上无法使用。\n\n"+
			"可能的原因：\n"+
			"• 该后端未下载安装\n"+
			"• GPU 驱动版本不兼容\n"+
			"• 设备实际不支持该后端\n\n"+
			"建议使用 CPU 进行推理，这是最稳定的选择。\n"+
			"（后续可在「设置 → 后端」中重新选择）\n\n"+
			"是否切换到 CPU 后端继续启动？",
		backendLabel)

	dlResult, _ := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.QuestionDialog,
		Title:   "所选后端不可用",
		Message: msg,
		Buttons: []string{"使用 CPU", "退出"},
	})

	zlog.Info().Str("result", dlResult).Msg("[startup] CPU 回退询问结果")

	// 白名单：明确匹配"使用 CPU"才回退，避免编码不匹配导致误判
	return dlResult == "使用 CPU" || dlResult == "Yes" || dlResult == "是"
}
