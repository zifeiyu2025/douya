// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"douya/internal/system"
)

// BackendType 表示 llama.cpp 的计算后端类型。
//
// 生活类比：就像一辆车可以选择不同的"发动机"——纯油、纯电、混动，
// llama.cpp 也可以用不同的计算后端来跑推理：CUDA（NVIDIA 显卡）、
// HIP（AMD 显卡）、SYCL/OpenVINO（Intel 显卡）、Vulkan（跨厂商）、CPU（纯 CPU）。
// 选对后端，推理才能用上对应的硬件加速能力。
type BackendType string

const (
	// BackendAuto 自动检测后端：根据硬件信息推断最合适的后端
	BackendAuto BackendType = "auto"
	// BackendCUDA NVIDIA CUDA 后端（仅适用于 N 卡）
	BackendCUDA BackendType = "cuda"
	// BackendHIP AMD HIP 后端（仅适用于 A 卡）
	BackendHIP BackendType = "hip"
	// BackendSYCL Intel SYCL 后端（仅适用于 I 卡，兼容性需手动验证）
	BackendSYCL BackendType = "sycl"
	// BackendVulkan Vulkan 跨厂商后端（N/A/I 卡通用，但性能通常不如原生后端）
	BackendVulkan BackendType = "vulkan"
	// BackendOpenVINO Intel OpenVINO 后端（仅适用于 I 卡）
	BackendOpenVINO BackendType = "openvino"
	// BackendCPU 纯 CPU 后端（无 GPU 或 GPU 不支持时的兜底方案）
	BackendCPU BackendType = "cpu"
)

// BackendInfo 描述一个后端的配置信息。
//
// 生活类比：就像每种发动机都有自己的"安装手册"——用的什么型号零件、
// 装在车的哪个位置、需要哪些专属配件。BackendInfo 就是每种后端的"安装手册"，
// 后续按需解压和启动流程会根据这些信息找到正确的运行时文件。
type BackendInfo struct {
	// Type 后端类型
	Type BackendType
	// DisplayName 中文显示名，如 "CUDA (NVIDIA)"，用于前端下拉框展示
	DisplayName string
	// Subdir runtime/ 目录下的子目录名，如 "cuda"，每种后端的 DLL 解压到这里
	Subdir string
	// ZipPattern zip 包名的 glob 模式，如 "llama-b*-bin-win-cuda-1[3-9]*-x64.zip"，
	// 用于在本地 runtime 目录中匹配已下载的后端压缩包
	ZipPattern string
	// ReleaseAssetRegex 匹配 GitHub release asset 名称的正则表达式，
	// 用于从 https://github.com/ggml-org/llama.cpp/releases/latest 下载对应后端。
	// 例如 CUDA 匹配 "llama-b10167-bin-win-cuda-13.3-x64.zip"
	ReleaseAssetRegex string
	// RequiredDLLs 该后端必须存在的核心 DLL 列表（用于路径校验，缺失则启动失败）
	RequiredDLLs []string
	// VendorDLLs 厂商特定 DLL（如 CUDA 的 cublas/cudart），其他后端可为空。
	// 这些 DLL 通常随厂商驱动或运行时包分发，不一定是 llama.cpp 自带
	VendorDLLs []string
	// Description 后端描述，用于前端 tooltip 等场景
	Description string
}

// coreDLLs 是所有 GPU 后端和 CPU 后端共享的核心 DLL 列表。
// 与 app_config.go 中 validatePaths() 的 coreDLLs 保持一致。
//
// 生活类比：就像无论用什么发动机，车都得有"方向盘、刹车、底盘"这些基础件，
// llama.cpp 无论用哪个后端，都需要这些核心 DLL 才能跑起来。
//
// 注意：ggml-cpu*.dll 使用 glob 模式，同时适配两种官方/自编译布局：
//   - 自编译版：产出统一的 ggml-cpu.dll（* 匹配 0 个字符）
//   - 官方预编译包：产出架构特定的 ggml-cpu-haswell.dll、ggml-cpu-alderlake.dll
//     等 14 个 DLL（* 匹配 "-haswell" 等后缀），运行时按 CPU 架构动态加载
//   - 检查时只要匹配到至少一个即视为通过（见 validatePaths 中的 glob 分支）
var coreDLLs = []string{
	"llama.dll",
	"llama-server-impl.dll",
	"llama-common.dll",
	"ggml.dll",
	"ggml-base.dll",
	"ggml-cpu*.dll", // glob 模式：兼容自编译(单一)和官方包(架构特定)两种布局
}

// mtmdDLL 是多模态模型支持库（multimodal），所有后端都需要
const mtmdDLL = "mtmd.dll"

// allBackendTypesOrdered 按固定顺序排列的所有后端类型，供 AllBackendTypes() 使用。
// 顺序：auto 在最前（默认选项），cpu 在最后（兜底选项），中间是各厂商 GPU 后端。
var allBackendTypesOrdered = []BackendType{
	BackendAuto,
	BackendCUDA,
	BackendHIP,
	BackendSYCL,
	BackendVulkan,
	BackendOpenVINO,
	BackendCPU,
}

// GetBackendInfo 根据后端类型返回对应的配置信息。
//
// 生活类比：给出发动机型号（如 "V8 涡轮增压"），返回这台发动机的完整安装手册——
// 装在哪、用什么零件、需要什么专属配件。
//
// 注意：BackendAuto 返回的 BackendInfo 中 Subdir/ZipPattern 为空，
// RequiredDLLs/VendorDLLs 为空切片（非 nil），因为 auto 需要先解析成具体后端才有意义。
// 调用方应先通过 ResolveBackendType() 把 auto 解析成具体后端，再调用本函数。
func GetBackendInfo(bt BackendType) BackendInfo {
	switch bt {
	case BackendCUDA:
		return BackendInfo{
			Type:        BackendCUDA,
			DisplayName: "CUDA (NVIDIA)",
			Subdir:      "cuda",
			ZipPattern:  "llama-b*-bin-win-cuda-1[3-9]*-x64.zip",
			// 匹配 CUDA 13.x（豆芽用 cudart64_13.dll），如 llama-b10167-bin-win-cuda-13.3-x64.zip
			ReleaseAssetRegex: `^llama-b\d+-bin-win-cuda-1[3-9]\.\d+-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-cuda.dll", mtmdDLL),
			// VendorDLLs 使用 glob 模式，同时兼容 CUDA 12 和 CUDA 13：
			//   - CUDA 12：cudart64_12.dll / cublas64_12.dll / cublasLt64_12.dll
			//   - CUDA 13：cudart64_13.dll / cublas64_13.dll / cublasLt64_13.dll
			// 这些厂商 DLL 可能来自三处：
			//   1. 官方预编译包附带的 cudart-llama-bin-win-cuda-*.zip（需用户手动合并到 cuda/）
			//   2. 用户系统安装的 NVIDIA 驱动（在系统 PATH 中，llama-server 启动时自动搜索）
			//   3. 自编译 llama.cpp 时链接的 CUDA Runtime
			// 因此 validatePaths 中对 VendorDLLs 仅做警告级检查（缺失不阻断启动），
			// 交由 llama-server 运行时自行解析依赖。
			VendorDLLs: []string{
				"cudart64_*.dll",
				"cublas64_*.dll",
				"cublasLt64_*.dll",
			},
			Description: "NVIDIA CUDA 后端，性能最佳，仅支持 N 卡",
		}
	case BackendHIP:
		return BackendInfo{
			Type:             BackendHIP,
			DisplayName:      "HIP (AMD)",
			Subdir:           "hip",
			ZipPattern:       "llama-b*-bin-win-hip-radeon-x64.zip",
			ReleaseAssetRegex: `^llama-b\d+-bin-win-hip-radeon-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-hip.dll", mtmdDLL),
			VendorDLLs: []string{}, // HIP 通常静态链接，无额外厂商 DLL
			Description: "AMD HIP 后端，仅支持 A 卡",
		}
	case BackendSYCL:
		return BackendInfo{
			Type:             BackendSYCL,
			DisplayName:      "SYCL (Intel)",
			Subdir:           "sycl",
			ZipPattern:       "llama-b*-bin-win-sycl-x64.zip",
			ReleaseAssetRegex: `^llama-b\d+-bin-win-sycl-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-sycl.dll", mtmdDLL),
			VendorDLLs: []string{},
			Description: "Intel SYCL 后端，仅支持 I 卡，兼容性需手动验证",
		}
	case BackendVulkan:
		return BackendInfo{
			Type:             BackendVulkan,
			DisplayName:      "Vulkan (跨厂商)",
			Subdir:           "vulkan",
			ZipPattern:       "llama-b*-bin-win-vulkan-x64.zip",
			ReleaseAssetRegex: `^llama-b\d+-bin-win-vulkan-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-vulkan.dll", mtmdDLL),
			VendorDLLs: []string{},
			Description: "Vulkan 跨厂商后端，N/A/I 卡通用，性能通常不如原生后端",
		}
	case BackendOpenVINO:
		return BackendInfo{
			Type:             BackendOpenVINO,
			DisplayName:      "OpenVINO (Intel)",
			Subdir:           "openvino",
			ZipPattern:       "llama-b*-bin-win-openvino-*-x64.zip",
			ReleaseAssetRegex: `^llama-b\d+-bin-win-openvino-[\d.]+-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-openvino.dll", mtmdDLL),
			VendorDLLs: []string{},
			Description: "Intel OpenVINO 后端，仅支持 I 卡",
		}
	case BackendCPU:
		return BackendInfo{
			Type:              BackendCPU,
			DisplayName:       "CPU (纯CPU)",
			Subdir:            "cpu",
			ZipPattern:        "llama-b*-bin-win-cpu-x64.zip",
			ReleaseAssetRegex: `^llama-b\d+-bin-win-cpu-x64\.zip$`,
			RequiredDLLs:      append(append([]string{}, coreDLLs...), mtmdDLL),
			VendorDLLs:        []string{},
			Description:       "纯 CPU 后端，无 GPU 或 GPU 不支持时的兜底方案",
		}
	case BackendAuto:
		// auto 后端未解析前没有具体路径信息，字段留空，
		// 但 RequiredDLLs/VendorDLLs 用空切片而非 nil，避免后续空指针
		return BackendInfo{
			Type:         BackendAuto,
			DisplayName:  "自动检测",
			Subdir:       "",
			ZipPattern:   "",
			RequiredDLLs: []string{},
			VendorDLLs:   []string{},
			Description:  "根据硬件自动选择最合适的后端",
		}
	}
	// 未知后端类型，回退到 CPU 配置（安全默认）
	return GetBackendInfo(BackendCPU)
}

// ResolveBackendType 根据硬件信息和用户配置的后端字符串，解析出实际使用的后端类型。
//
// 生活类比：用户可能说"随便帮我选个发动机"（auto），也可能明确指定"我要 V8 涡轮"（cuda）。
// - 如果用户指定了具体型号，直接用他选的；
// - 如果用户说"随便"，就根据车库里有啥车（GPUVendor）来推荐最合适的发动机。
//
// 解析规则：
//   - cfgBackend 为 "auto" 或空时：按 GPUVendor 自动匹配
//     nvidia → CUDA, amd → HIP, intel → Vulkan（保守默认）, vulkan → Vulkan, 无 GPU → CPU
//   - cfgBackend 为有效后端值（cuda/hip/sycl/vulkan/openvino/cpu）：直接返回
//   - cfgBackend 为无效值：返回 CPU（安全回退）
//
// 注意：Intel 显卡默认走 Vulkan 而非 SYCL，因为 SYCL 兼容性未充分验证，
// 用户若确信 SYCL 可用，可手动在配置中选择 sycl。
func ResolveBackendType(hw *system.HardwareInfo, cfgBackend string) BackendType {
	// 先处理手动指定的情况
	if cfgBackend != "" && cfgBackend != string(BackendAuto) {
		if IsValidBackendType(cfgBackend) && cfgBackend != string(BackendAuto) {
			return BackendType(cfgBackend)
		}
		// 无效配置值，安全回退到 CPU
		return BackendCPU
	}

	// auto 模式：根据硬件厂商推断
	if hw == nil {
		return BackendCPU
	}
	switch hw.GPUVendor {
	case "nvidia":
		return BackendCUDA
	case "amd":
		return BackendHIP
	case "intel":
		// 保守默认：Intel 显卡走 Vulkan，SYCL 兼容性未验证
		// 用户若需用 SYCL，可手动在配置中选择 sycl
		return BackendVulkan
	case "vulkan":
		return BackendVulkan
	default:
		// 无 GPU 或未识别厂商，回退到 CPU
		return BackendCPU
	}
}

// IsValidBackendType 校验字符串是否为有效的后端类型（含 "auto"）。
//
// 生活类比：检查用户填的"发动机型号"是不是我们店里有的型号。
func IsValidBackendType(s string) bool {
	switch BackendType(s) {
	case BackendAuto, BackendCUDA, BackendHIP, BackendSYCL,
		BackendVulkan, BackendOpenVINO, BackendCPU:
		return true
	}
	return false
}

// AllBackendTypes 返回所有后端类型列表，用于前端下拉框展示。
// 顺序：auto, cuda, hip, sycl, vulkan, openvino, cpu。
func AllBackendTypes() []BackendType {
	// 返回切片副本，避免调用方修改内部切片
	out := make([]BackendType, len(allBackendTypesOrdered))
	copy(out, allBackendTypesOrdered)
	return out
}

// String 返回 BackendType 的字符串表示，满足 fmt.Stringer 接口。
//
// 生活类比：发动机铭牌上印的型号文字。这样 BackendType 可以直接用作 map key、
// 配置字符串、日志输出等场景。
func (bt BackendType) String() string {
	return string(bt)
}
