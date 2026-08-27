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
	CPUCores         int
	GPUVRAMMB        int64
	GPUName          string
	GPUDriverVersion string // NVIDIA 驱动版本（如 "585.00"），仅 NVIDIA 显卡非空；用于 CUDA 后端能力预检
	HasGPU           bool   // 检测到独立显卡（discrete GPU），可用于 GPU 加速推理
	HasCUDABackend   bool   // 系统有 NVIDIA CUDA 驱动（nvcuda.dll），但可能无法获取 VRAM
	GPUArchitecture  string // GPU 微架构代号（如 "Blackwell"/"Ada"/"Ampere"/"Unknown"），用于架构感知优化
	// 多厂商 GPU 支持字段（Task 1 扩展）
	// 生活类比：就像一辆车可能装的是宝马、奔驰或奥迪的发动机，
	// GPUVendor 记录这辆"AI 推理车"装的是哪家厂商的"显卡发动机"，
	// 后续推理参数会根据厂商选择不同的"驾驶模式"（CUDA/Vulkan/SYCL 等）。
	//
	// GPUType 进一步记录 GPU 类型（Task P5 扩展）：
	//   - "discrete"   独立显卡（独显插槽，有专用显存，适合 GPU 推理）
	//   - "integrated" 核显/集显（与 CPU 共享内存，显存小，不适合大模型推理）
	//   - "unknown"    未知显卡状态（保守起见按独显处理，由 HasGPU 实际值决定）
	// 生活类比：知道车是什么牌子（GPUVendor）还不够，还要知道是"跑车"还是"小电瓶车"
	// （GPUType）——小电瓶车跑不了长途（大模型推理）。
	GPUVendor   string // GPU 厂商："nvidia"/"amd"/"intel"/"vulkan"/""（空表示未检测到）
	GPUType     string // GPU 类型："discrete"/"integrated"/"unknown"（仅当 HasAMDGPU/HasIntelGPU=true 时有意义）
	HasAMDGPU   bool   // 检测到 AMD GPU（可能是独显或 APU 核显，由 GPUType 区分）
	HasIntelGPU bool   // 检测到 Intel GPU（可能是独显或核显，由 GPUType 区分）
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
			Str("gpu_type", hw.GPUType).
			Int64("vram_mb", hw.GPUVRAMMB).
			Str("driver_version", hw.GPUDriverVersion).
			Bool("cuda_backend", hw.HasCUDABackend).
			Bool("amd", hw.HasAMDGPU).
			Bool("intel", hw.HasIntelGPU).
			Msg("[system] hardware detected")
	case hw.HasCUDABackend:
		log.Warn().
			Int("cpu_cores", hw.CPUCores).
			Str("vendor", hw.GPUVendor).
			Msg("[system] NVIDIA CUDA driver detected but nvidia-smi unavailable, GPU params will use fallback (no VRAM info)")
	case hw.HasAMDGPU || hw.HasIntelGPU:
		// 检测到 AMD/Intel 驱动但被判为核显 → 提示用户不会被启用为 GPU 加速
		log.Info().
			Int("cpu_cores", hw.CPUCores).
			Str("vendor", hw.GPUVendor).
			Str("gpu", hw.GPUName).
			Str("gpu_type", hw.GPUType).
			Bool("amd", hw.HasAMDGPU).
			Bool("intel", hw.HasIntelGPU).
			Msg("[system] hardware: integrated GPU detected, will use CPU for LLM inference")
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

	// P3.7 增强：顺带查询驱动版本，供 CUDA 后端能力预检使用。
	// 驱动版本是判断 CUDA 能否启动的关键证据：NVIDIA 驱动 ≥531 才支持 CUDA 12，
	// Blackwell 架构（RTX 50 系）还需 ≥580。查询失败不阻断（版本未知时按保守可用处理）。
	// 生活类比：仪表盘能跑了，顺手再记一下"发动机软件版本"，
	// 版本不对后面选发动机时就有据可依，不至于选完才发现打不着火。
	if ver, err := NvidiaDriverVersion(); err == nil && ver != "" {
		hw.GPUDriverVersion = ver
		log.Info().Str("gpu", hw.GPUName).Str("driver", ver).Msg("[system] NVIDIA driver version detected")
	} else if err != nil {
		log.Warn().Err(err).Msg("[system] query NVIDIA driver version failed (CUDA capability precheck will be conservative)")
	}
}

// GPU 类型常量
const (
	GPUTypeDiscrete   = "discrete"   // 独立显卡
	GPUTypeIntegrated = "integrated" // 核显/集显
	GPUTypeUnknown    = "unknown"    // 未知
)

// classifyGPUType 按业界成熟方案判别 GPU 是独显还是核显。
//
// 业界共识方案（双因子法，参照 Ollama gpud、Chromium GPU info、DirectX 适配器枚举）：
//  1. GPU 名称启发式：基于业界共识的独显/核显关键字库判别
//  2. 专用显存阈值兜底：名称无法判定时用专用 VRAM 兜底
//
// 判别优先级：
//  1. 名称明确命中独显关键字 → discrete
//  2. 名称明确命中核显关键字 → integrated
//  3. 名称无明确命中，看 VRAM：
//     - VRAM ≥ 1024MB → discrete（业界最低独显是 GTX 650 1GB）
//     - VRAM < 512MB → integrated（核显共享内存下专用显存通常 0-128MB）
//     - 512-1024MB 灰色地带，或 VRAM=0 无信息 → unknown（保守按独显处理，由调用方决定）
//
// NVIDIA 全部视为独显（业界共识，NVIDIA 不做主流 x86 核显）。
//
// 生活类比：判断一辆车是"跑车"还是"小电瓶车"，先看车牌关键字
// （如 "RTX"/"Arc" 明显是跑车，"UHD"/"Radeon Graphics" 明显是电瓶车），
// 车牌看不出名堂再看排量（VRAM ≥1L 肯定是跑车，<0.5L 大概率是电瓶车）。
//
// 参数：
//   - vendor: GPU 厂商（"nvidia"/"amd"/"intel"/"vulkan"/""）
//   - gpuName: GPU 名称（来自 nvidia-smi/WMI/注册表）
//   - dedicatedVRAMMB: 专用显存（MB），仅独显专用部分，不含共享内存。0 表示无数据。
func classifyGPUType(vendor, gpuName string, dedicatedVRAMMB int64) string {
	// NVIDIA 一律视为独显（业界共识：NVIDIA 不做主流 x86 核显）
	if vendor == "nvidia" {
		return GPUTypeDiscrete
	}

	// Vulkan 兜底：厂商未知，VRAM 也未知，保守视为独显
	if vendor == "vulkan" {
		return GPUTypeDiscrete
	}

	lowerName := strings.ToLower(gpuName)

	// 各厂商的独显/核显关键字库（业界共识，源自 Ollama/Chromium/DirectX 适配器枚举）
	type kwEntry struct {
		keywords []string
		gpuType  string
	}
	keywordTable := map[string][]kwEntry{
		"intel": {
			// 独显关键字
			{[]string{"arc"}, GPUTypeDiscrete},    // Intel Arc A770/A750
			{[]string{"dg1"}, GPUTypeDiscrete},    // Intel DG1（Iris Xe MAX 独显）
			{[]string{"xe max"}, GPUTypeDiscrete}, // Iris Xe MAX 独显
			// 核显关键字（注意顺序：先匹配独显，后匹配核显）
			{[]string{"uhd"}, GPUTypeIntegrated},  // Intel UHD Graphics 核显
			{[]string{"iris"}, GPUTypeIntegrated}, // Iris Xe 核显（不含 MAX）
			{[]string{"hd graphics"}, GPUTypeIntegrated},
			{[]string{"gma"}, GPUTypeIntegrated},      // 老旧 GMA 核显
			{[]string{"graphics"}, GPUTypeIntegrated}, // 兜底：Intel xxx Graphics 默认核显
		},
		"amd": {
			// 独显关键字（注意：Radeon RX Vega 是独显，Vega N Graphics 是核显）
			{[]string{"radeon rx"}, GPUTypeDiscrete},      // Radeon RX 7900/6800 等
			{[]string{"radeon pro w"}, GPUTypeDiscrete},   // Radeon Pro W系列工作站独显
			{[]string{"radeon vii"}, GPUTypeDiscrete},     // Radeon VII
			{[]string{"firepro"}, GPUTypeDiscrete},        // FirePro 工作站独显
			{[]string{"radeon hd 7"}, GPUTypeDiscrete},    // Radeon HD 7xxx 系列
			{[]string{"radeon hd 8"}, GPUTypeDiscrete},    // Radeon HD 8xxx 系列
			{[]string{"radeon rx vega"}, GPUTypeDiscrete}, // Vega 64/56 独显
			// 核显关键字（AMD APU 核显）
			{[]string{"radeon graphics"}, GPUTypeIntegrated}, // Ryzen APU 核显
			{[]string{"radeon(tm) graphics"}, GPUTypeIntegrated},
			{[]string{"vega "}, GPUTypeIntegrated}, // "Vega 8 Graphics"/"Vega 11 Graphics"
			{[]string{"radeon r3 graphics"}, GPUTypeIntegrated},
			{[]string{"radeon r4 graphics"}, GPUTypeIntegrated},
			{[]string{"radeon r5 graphics"}, GPUTypeIntegrated},
			{[]string{"radeon r6 graphics"}, GPUTypeIntegrated},
			{[]string{"radeon r7 graphics"}, GPUTypeIntegrated},
			{[]string{"radeon r8 graphics"}, GPUTypeIntegrated},
		},
	}

	// 第一因子：名称关键字判别
	if entries, ok := keywordTable[vendor]; ok {
		for _, e := range entries {
			for _, kw := range e.keywords {
				if strings.Contains(lowerName, kw) {
					return e.gpuType
				}
			}
		}
	}

	// 第二因子：专用显存阈值兜底（名称无明确命中时）
	// 业界经验阈值：独显最低 GTX 650 1GB；核显共享内存下专用显存通常 0-128MB
	switch {
	case dedicatedVRAMMB >= 1024:
		return GPUTypeDiscrete
	case dedicatedVRAMMB > 0 && dedicatedVRAMMB < 512:
		return GPUTypeIntegrated
	default:
		// 512-1024MB 灰色地带，或 VRAM=0 无信息，无法判定
		return GPUTypeUnknown
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
