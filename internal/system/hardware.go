// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

type HardwareInfo struct {
	CPUCores        int
	GPUVRAMMB       int64
	GPUName         string
	HasGPU          bool   // nvidia-smi 检测成功，有完整 GPU 信息（含 VRAM）
	HasCUDABackend  bool   // 系统有 NVIDIA CUDA 驱动（nvcuda.dll），但可能无法获取 VRAM
	GPUArchitecture string // GPU 微架构代号（如 "Blackwell"/"Ada"/"Ampere"/"Unknown"），用于架构感知优化
	// 多厂商 GPU 支持字段（Task 1 扩展）
	// 生活类比：就像一辆车可能装的是宝马、奔驰或奥迪的发动机，
	// GPUVendor 记录这辆"AI 推理车"装的是哪家厂商的"显卡发动机"，
	// 后续推理参数会根据厂商选择不同的"驾驶模式"（CUDA/Vulkan/SYCL 等）。
	GPUVendor   string // GPU 厂商："nvidia"/"amd"/"intel"/"vulkan"/""（空表示未检测到）
	HasAMDGPU   bool   // 检测到 AMD GPU
	HasIntelGPU bool   // 检测到 Intel GPU
}

func DetectHardware() *HardwareInfo {
	hw := &HardwareInfo{
		CPUCores: runtime.NumCPU(),
	}

	// 先检测 CUDA 驱动核心组件（nvcuda.dll），再尝试 nvidia-smi 获取详细信息
	// 生活类比：先检查发动机在不在（nvcuda.dll），再看仪表盘能不能用（nvidia-smi）
	detectCUDABackend(hw)
	detectGPU(hw)

	// NVIDIA 检测成功（有 nvidia-smi 完整信息 或 有 CUDA 驱动）时，标记厂商为 nvidia
	// 生活类比：只要车库里有 NVIDIA 的车（无论能不能启动仪表盘），门口牌子就写 NVIDIA
	if hw.HasGPU || hw.HasCUDABackend {
		hw.GPUVendor = "nvidia"
	}

	// 多厂商兜底检测：仅当没有任何 NVIDIA 痕迹时，才尝试 AMD/Intel/Vulkan
	// 生活类比：NVIDIA 车库空着，再去 AMD、Intel、Vulkan 的展厅看看有没有车
	// 注意：保留 HasCUDABackend 判断，避免有 NVIDIA 驱动但 nvidia-smi 失败时误报其他厂商
	if !hw.HasGPU && !hw.HasCUDABackend {
		if !hw.HasGPU {
			detectAMDGPU(hw)
		}
		if !hw.HasGPU {
			detectIntelGPU(hw)
		}
		if !hw.HasGPU {
			detectVulkanDevice(hw)
		}
	}

	// 统一日志输出，支持多厂商信息
	switch {
	case hw.HasGPU:
		log.Info().
			Int("cpu_cores", hw.CPUCores).
			Str("vendor", hw.GPUVendor).
			Str("gpu", hw.GPUName).
			Int64("vram_mb", hw.GPUVRAMMB).
			Bool("cuda_backend", hw.HasCUDABackend).
			Bool("amd", hw.HasAMDGPU).
			Bool("intel", hw.HasIntelGPU).
			Msg("[system] hardware detected")
	case hw.HasCUDABackend:
		log.Warn().
			Int("cpu_cores", hw.CPUCores).
			Str("vendor", hw.GPUVendor).
			Msg("[system] NVIDIA CUDA driver detected but nvidia-smi unavailable, GPU params will use fallback (no VRAM info)")
	default:
		log.Info().
			Int("cpu_cores", hw.CPUCores).
			Str("vendor", hw.GPUVendor).
			Msg("[system] hardware: no GPU detected")
	}

	return hw
}

// detectCUDABackend 检测系统是否有 NVIDIA CUDA 驱动（nvcuda.dll）
// nvcuda.dll 是 NVIDIA 显卡驱动核心组件，位于 System32，只要装了 N 卡驱动就一定存在。
// 比 nvidia-smi 更可靠——nvidia-smi 是管理工具可能不在 PATH，nvcuda.dll 是驱动必须的。
//
// 生活类比：nvidia-smi 是"仪表盘"，nvcuda.dll 是"发动机"。
// 即使仪表盘坏了（不在 PATH），只要发动机还在（nvcuda.dll），车还是能跑的。
func detectCUDABackend(hw *HardwareInfo) {
	// nvcuda.dll 常见位置
	candidates := []string{
		`C:\Windows\System32\nvcuda.dll`,
		`C:\Windows\SysWOW64\nvcuda.dll`,
	}
	// 也检查 PATH 中的 nvcuda.dll
	if p, err := exec.LookPath("nvcuda.dll"); err == nil {
		candidates = append([]string{p}, candidates...)
	}

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			hw.HasCUDABackend = true
			log.Info().Str("path", p).Msg("[system] NVIDIA CUDA driver detected (nvcuda.dll)")
			return
		}
	}
	log.Info().Msg("[system] no NVIDIA CUDA driver detected (nvcuda.dll not found)")
}

// nvidiaSMIPaths 是 nvidia-smi.exe 的常见安装路径（当不在 PATH 时回退查找）
var nvidiaSMIPaths = []string{
	`C:\Windows\System32\nvidia-smi.exe`,
	`C:\Program Files\NVIDIA Corporation\NVSMI\nvidia-smi.exe`,
	`C:\Program Files (x86)\NVIDIA Corporation\NVSMI\nvidia-smi.exe`,
}

func detectGPU(hw *HardwareInfo) {
	// 优先从 PATH 查找 nvidia-smi
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		// PATH 中找不到，遍历常见安装路径
		path = ""
		for _, p := range nvidiaSMIPaths {
			if _, statErr := os.Stat(p); statErr == nil {
				path = p
				break
			}
		}
		if path == "" {
			log.Info().Msg("[system] nvidia-smi not found in PATH or common locations")
			return
		}
		log.Info().Str("path", path).Msg("[system] nvidia-smi found in fallback location (not in PATH)")
	}

	cmd := exec.Command(path, "--query-gpu=memory.total,name", "--format=csv,noheader,nounits")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		log.Error().Err(err).Str("path", path).Msg("[system] nvidia-smi query failed")
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		log.Warn().Msg("[system] nvidia-smi returned empty output")
		return
	}

	parts := strings.SplitN(lines[0], ",", 2)
	if len(parts) < 2 {
		log.Warn().Str("output", strings.TrimSpace(string(output))).Msg("[system] nvidia-smi output format unexpected")
		return
	}

	vramMB, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		log.Error().Err(err).Str("value", strings.TrimSpace(parts[0])).Msg("[system] parse VRAM value failed")
		return
	}

	hw.GPUVRAMMB = vramMB
	hw.GPUName = strings.TrimSpace(parts[1])
	hw.HasGPU = true
	// 解析 GPU 微架构代号，用于后续架构感知优化（如 Blackwell 启用 NVFP4 量化）
	hw.GPUArchitecture = parseGPUArchitecture(hw.GPUName)
	if hw.GPUArchitecture != "Unknown" {
		log.Info().Str("gpu", hw.GPUName).Str("arch", hw.GPUArchitecture).Msg("[system] GPU architecture detected")
	}
}

// parseGPUArchitecture 从 GPU 名称解析微架构代号。
// 生活类比：就像从车牌号前缀判断车辆品牌，从 GPU 名称中的"RTX 50/40/30"等关键字
// 可以判断它属于哪一代架构，从而知道支持哪些特性（如 NVFP4 量化仅 Blackwell 支持）。
//
// 已知对应关系（NVIDIA 消费级显卡）：
//   - RTX 50 系列 → Blackwell（支持 NVFP4 4-bit 浮点量化）
//   - RTX 40 系列 → Ada Lovelace
//   - RTX 30 系列 → Ampere
//   - GTX 16 系列 → Turing
//
// 注意：nvidia-smi 返回的名称格式可能为 "NVIDIA GeForce RTX 5090" 或 "RTX 5090" 等，
// 这里用 strings.Contains 做宽松匹配，避免被前缀差异绕过。
func parseGPUArchitecture(gpuName string) string {
	if gpuName == "" {
		return "Unknown"
	}
	lowerName := strings.ToLower(gpuName)
	switch {
	case strings.Contains(lowerName, "rtx 50"):
		// RTX 50 系：Blackwell 架构，原生支持 NVFP4 4-bit 浮点量化
		return "Blackwell"
	case strings.Contains(lowerName, "rtx 40"):
		// RTX 40 系：Ada Lovelace 架构
		return "Ada"
	case strings.Contains(lowerName, "rtx 30"):
		// RTX 30 系：Ampere 架构
		return "Ampere"
	case strings.Contains(lowerName, "gtx 16"):
		// GTX 16 系：Turing 架构（无 RT 核心）
		return "Turing"
	default:
		return "Unknown"
	}
}
