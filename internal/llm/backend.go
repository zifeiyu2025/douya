// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"douya/internal/system"
)

// BackendType 表示 llama.cpp 的计算后端类型。
//
// 生活类比：就像一辆车可以选择不同的"发动机"——纯油、纯电、混动，
// llama.cpp 也可以用不同的计算后端来跑推理：CUDA（NVIDIA 显卡）、
// ROCm/HIP（AMD 显卡）、SYCL/OpenVINO（Intel 显卡）、Vulkan（跨厂商）、CPU（纯 CPU）。
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
	// ZipPattern zip 包名的 glob 模式，如 "llama-*-bin-win-cuda-1[23]*-x64.zip"（匹配 CUDA 12.x/13.x），
	// 用于在本地 runtime 目录中匹配已下载的后端压缩包。
	// glob 前缀用宽松的 "llama-*" 以同时兼容历史 "b\d+" 与语义版本 "v\d+\.\d+\.\d+" 两种命名。
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
	// BackendDLL 后端专属 DLL 名（如 "ggml-cuda.dll"），用于幂等检查。
	// 空字符串表示该后端的官方包是完整的（如 CPU/OpenVINO），包含所有核心文件；
	// 非空表示该后端的官方包是模块化的（如 CUDA/Vulkan/SYCL/HIP），
	// 只含后端专属 DLL，需要先下载 CPU 包作为基础。
	BackendDLL string
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
// 分组：
//   - 自动档（auto 默认）：auto、CUDA、Vulkan、CPU —— auto 只会解析到这三者之一
//   - 手动高级选项（auto 永不自动选）：HIP、SYCL、OpenVINO —— 仅设置页手动切换可用
//
// 顺序：auto 在最前（默认），CUDA/Vulkan 为常用 GPU 后端，CPU 兜底，
// 其后为手动高级后端。
var allBackendTypesOrdered = []BackendType{
	BackendAuto,
	BackendCUDA,
	BackendVulkan,
	BackendCPU,
	BackendHIP,
	BackendSYCL,
	BackendOpenVINO,
}

// GetBackendInfo 根据后端类型返回对应的配置信息。
//
// 生活类比：给出发动机型号（如 "V8 涡轮增压"），返回这台发动机的完整安装手册——
// 装在哪、用什么零件、需要什么专属配件。
//
// backendVersionPrefix 匹配 llama.cpp release 的版本前缀。
//
// 历史上官方使用提交计数格式 "b10380"（如 llama-b10380-bin-win-cuda-13.3-x64.zip）；
// 上游引入语义化版本（#26839，MAJOR.MINOR.PATCH）后新增 "v0.1.0" 格式
// （如 llama-v0.1.0-bin-win-cuda-13.3-x64.zip）。两者都必须匹配，否则上游切换
// 语义版本后，releases/latest 指向 vX.Y.Z，所有后端下载将全部失效。
//
// 注意：ZipPattern 使用 glob（filepath.Glob 不支持多选/分组语法），
// 故 glob 侧统一用宽松的 "llama-*" 前缀匹配任意版本前缀；
// ReleaseAssetRegex 用本正则精确匹配 b\d+ 与 v\d+\.\d+\.\d+ 两种前缀。
const backendVersionPrefix = `(?:b\d+|v\d+\.\d+\.\d+)`

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
			ZipPattern:  "llama-*-bin-win-cuda-1[23]*-x64.zip",
			// 全量适配：同时匹配 CUDA 12.x 和 13.x（官方 release 同时提供两个版本）
			// 豆芽优先使用 13.x（cudart64_13.dll），12.x 作为回退（cudart64_12.dll）
			// 选择策略见 FindReleaseAsset 的 CUDA 优先逻辑
			// 版本前缀兼容历史 "b\d+"（llama-b10380-...）与语义版本 "v\d+\.\d+\.\d+"（llama-v0.1.0-...）
			ReleaseAssetRegex: `^llama-` + backendVersionPrefix + `-bin-win-cuda-1[23]\.\d+-x64\.zip$`,
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
			BackendDLL:  "ggml-cuda.dll", // 模块化后端：官方包只含此 DLL，需 CPU 包作基础
			Description: "NVIDIA CUDA 后端，性能最佳，仅支持 N 卡",
		}
	case BackendHIP:
		return BackendInfo{
			Type:        BackendHIP,
			DisplayName: "ROCm (AMD)",
			Subdir:      "hip",
			// 上游自 b10xxx 起将 AMD 包由 "win-hip-radeon" 更名为 "win-rocm-<ver>"，
			// ZipPattern 用 glob（不支持多选）指向当前命名；ReleaseAssetRegex 同时兼容旧命名。
			// 版本前缀兼容历史 "b\d+" 与语义版本 "v\d+\.\d+\.\d+"。
			ZipPattern:        "llama-*-bin-win-rocm-*-x64.zip",
			ReleaseAssetRegex: `^llama-` + backendVersionPrefix + `-bin-win-(rocm-[\d.]+|hip-radeon)-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-hip.dll", mtmdDLL),
			VendorDLLs:  []string{},     // HIP 通常静态链接，无额外厂商 DLL
			BackendDLL:  "ggml-hip.dll", // 模块化后端：官方包只含此 DLL，需 CPU 包作基础
			Description: "AMD ROCm/HIP 后端，仅支持 A 卡",
		}
	case BackendSYCL:
		return BackendInfo{
			Type:              BackendSYCL,
			DisplayName:       "SYCL (Intel)",
			Subdir:            "sycl",
			ZipPattern:        "llama-*-bin-win-sycl-x64.zip",
			ReleaseAssetRegex: `^llama-` + backendVersionPrefix + `-bin-win-sycl-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-sycl.dll", mtmdDLL),
			VendorDLLs:  []string{},
			BackendDLL:  "ggml-sycl.dll", // 模块化后端：官方包只含此 DLL，需 CPU 包作基础
			Description: "Intel SYCL 后端，仅支持 I 卡，兼容性需手动验证",
		}
	case BackendVulkan:
		return BackendInfo{
			Type:              BackendVulkan,
			DisplayName:       "Vulkan (跨厂商)",
			Subdir:            "vulkan",
			ZipPattern:        "llama-*-bin-win-vulkan-x64.zip",
			ReleaseAssetRegex: `^llama-` + backendVersionPrefix + `-bin-win-vulkan-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-vulkan.dll", mtmdDLL),
			VendorDLLs:  []string{},
			BackendDLL:  "ggml-vulkan.dll", // 模块化后端：官方包只含此 DLL，需 CPU 包作基础
			Description: "Vulkan 跨厂商后端，N/A/I 卡通用，性能通常不如原生后端",
		}
	case BackendOpenVINO:
		return BackendInfo{
			Type:              BackendOpenVINO,
			DisplayName:       "OpenVINO (Intel)",
			Subdir:            "openvino",
			ZipPattern:        "llama-*-bin-win-openvino-*-x64.zip",
			ReleaseAssetRegex: `^llama-` + backendVersionPrefix + `-bin-win-openvino-[\d.]+-x64\.zip$`,
			RequiredDLLs: append(append([]string{}, coreDLLs...),
				"ggml-openvino.dll", mtmdDLL),
			VendorDLLs:  []string{},
			Description: "Intel OpenVINO 后端，仅支持 I 卡",
		}
	case BackendCPU:
		return BackendInfo{
			Type:              BackendCPU,
			DisplayName:       "CPU (纯CPU)",
			Subdir:            "cpu",
			ZipPattern:        "llama-*-bin-win-cpu-x64.zip",
			ReleaseAssetRegex: `^llama-` + backendVersionPrefix + `-bin-win-cpu-x64\.zip$`,
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
//   - cfgBackend 为 "auto" 或空时：按 GPUVendor 自动匹配原生后端
//     nvidia → CUDA, amd → Vulkan, intel → Vulkan, vulkan → Vulkan, 无 GPU → CPU
//     （auto 实际只会解析到 CUDA / Vulkan / CPU 三档；HIP/SYCL/OpenVINO 仅为手动高级选项）
//   - cfgBackend 为有效后端值（cuda/hip/sycl/vulkan/openvino/cpu）：直接返回
//   - cfgBackend 为无效值：返回 CPU（安全回退）
//
// 后端选择策略：N 卡用厂商原生 CUDA（性能最佳）；AMD/Intel 统一走 Vulkan，
// 借系统驱动自带的 Vulkan 运行时规避 Windows 上脆弱的 ROCm/HIP、SYCL 计算栈，加载成功率最高。
// 原生后端未安装时，由 ResolveBackendTypeWithRuntime 负责回退到 Vulkan 或 CPU。
func ResolveBackendType(hw *system.HardwareInfo, cfgBackend string) BackendType {
	// 先处理手动指定的情况
	if cfgBackend != "" && cfgBackend != string(BackendAuto) {
		if IsValidBackendType(cfgBackend) {
			return BackendType(cfgBackend)
		}
		// 无效配置值，安全回退到 CPU
		return BackendCPU
	}

	// auto 模式：根据硬件厂商推断原生后端
	if hw == nil {
		return BackendCPU
	}
	switch hw.GPUVendor {
	case "nvidia":
		return BackendCUDA
	case "amd":
		// AMD 在 Windows 上的成熟选择是 Vulkan：它使用系统自带的 AMD 显卡驱动（Vulkan
		// 运行时随驱动分发），不依赖脆弱的 ROCm/HIP 运行时栈，最大化加载成功率。
		// 追求极限性能且已正确配置 ROCm 环境的用户可在设置中手动选择 HIP 后端。
		return BackendVulkan
	case "intel":
		// Intel 独显与 AMD 一样统一走 Vulkan：借用系统驱动自带的 Intel Vulkan 运行时
		// （vulkan-1.dll），规避 Windows 上仍不成熟的 SYCL 计算栈，最大化加载成功率。
		// 追求极限性能且已正确配置 SYCL 环境的用户可在设置中手动选择 SYCL 后端。
		return BackendVulkan
	case "vulkan":
		return BackendVulkan
	default:
		// 无 GPU 或未识别厂商，回退到 CPU
		return BackendCPU
	}
}

// ResolveBackendTypeWithRuntime 在 ResolveBackendType 基础上增加运行时文件预校验。
//
// 回退策略（auto 模式下，原生后端未安装时）：
//  1. 先尝试 Vulkan（跨厂商 GPU 后端，仍有 GPU 加速）
//  2. 再尝试 CPU（纯 CPU，兜底方案）
//  3. 都没安装则返回原推断后端，交给 installBackend 触发下载流程
//
// 生活类比：想用原厂发动机（SYCL/HIP），但没装；先看看有没有通用发动机（Vulkan），
// 有的话先用着，至少还能跑；通用发动机也没有，才用纯 CPU 发动机。
// 这样保证非 N 卡用户也能优先享受 GPU 加速，而不是无脑退到 CPU。
//
// 参数：
//   - hw: 硬件信息（可为 nil，nil 时按 CPU 处理）
//   - cfgBackend: 用户配置的后端字符串（"auto" 或具体后端名）
//   - runtimeDir: runtime 目录的绝对路径（用于检查后端是否已安装）
//
// 返回：解析后的后端类型（保证运行时文件存在，或回退到 CPU）
func ResolveBackendTypeWithRuntime(hw *system.HardwareInfo, cfgBackend string, runtimeDir string) BackendType {
	resolved := ResolveBackendType(hw, cfgBackend)

	// 手动指定的后端：不主动回退（尊重用户选择，下载失败由 installBackend 处理）
	isAuto := cfgBackend == "" || cfgBackend == string(BackendAuto)
	if !isAuto {
		return resolved
	}

	// auto 模式：检查推断的后端是否已安装
	if resolved == BackendCPU {
		return resolved // CPU 本身就是兜底
	}

	// NVIDIA 独显 + auto：优先下载原生 CUDA，而非被已装的 Vulkan/CPU 兜底顶替。
	// auto 的语义是"为我的显卡选最优后端"，对 N 卡用户 Vulkan/CPU 只是运行时崩溃兜底
	// （ensureVulkanFallback）的替补，不应在启动选择阶段抢占原生 CUDA。
	// 否则用户删除 runtime/cuda/ 想重新拉取 CUDA 时，会被预装的 Vulkan 接管。
	// AMD/Intel 现已统一走 Vulkan（auto 即解析为 Vulkan），其回退行为同样不受影响。
	if resolved == BackendCUDA && hw != nil && hw.GPUVendor == "nvidia" {
		return resolved
	}

	if isBackendInstalled(resolved, runtimeDir) {
		return resolved
	}

	// 原生后端未安装，先尝试 Vulkan（跨厂商 GPU 加速）
	if resolved != BackendVulkan && isBackendInstalled(BackendVulkan, runtimeDir) {
		log.Warn().
			Str("preferred", resolved.String()).
			Str("fallback", string(BackendVulkan)).
			Msg("[backend] 原生后端未安装但 Vulkan 已安装，回退到 Vulkan（auto 模式）")
		return BackendVulkan
	}

	// Vulkan 也没安装，检查 CPU 是否已安装
	if isBackendInstalled(BackendCPU, runtimeDir) {
		log.Warn().
			Str("preferred", resolved.String()).
			Str("fallback", string(BackendCPU)).
			Msg("[backend] 原生后端和 Vulkan 均未安装，回退到 CPU（auto 模式）")
		return BackendCPU
	}

	// 都没安装：返回原推断，交给 installBackend 触发下载流程
	return resolved
}

// isBackendInstalled 检查指定后端是否已安装到 runtime 目录（llama-server.exe 存在）。
//
// 生活类比：派人去车库看一眼，指定型号的发动机装好了没。
func isBackendInstalled(bt BackendType, runtimeDir string) bool {
	if bt == BackendAuto {
		return false
	}
	info := GetBackendInfo(bt)
	if info.Subdir == "" {
		return false
	}
	serverPath := filepath.Join(runtimeDir, info.Subdir, llamaServerExe)
	_, err := os.Stat(serverPath)
	return err == nil
}

// backendRuntimeDeps 记录各后端在 Windows 上运行所需的外部运行时 DLL（缺失时启动会失败）。
// CUDA/Vulkan/CPU 的运行时由显卡驱动或安装包自带，无需预检；
// HIP 需要 ROCm 运行时，SYCL 需要 Intel oneAPI 运行时（安装包不附带）。
var backendRuntimeDeps = map[BackendType][]string{
	BackendHIP:  {"amdhip64.dll"},
	BackendSYCL: {"sycl8.dll", "sycl7.dll"},
}

// lookupPath 可测试性钩子：生产指向 exec.LookPath，测试中可替换
var lookupPath = exec.LookPath

// CheckBackendRuntimeReady 预检后端运行时依赖是否可见。
//
// 检查范围（任一 DLL 命中即视为就绪）：
//  1. 系统 PATH（exec.LookPath）——用户已安装 ROCm/oneAPI 并加入 PATH
//  2. runtime/<subdir>/ 目录——用户手工把运行时 DLL 放到 llama-server 旁边
//
// 返回值：就绪返回空串；缺失时返回面向用户的中文提示。
// 仅作事前预警，不阻断切换——高级用户可能有自定义环境，最终以实际启动结果为准
// （启动失败仍走既有的弹窗引导 + LastSuccessfulBackend 回退机制）。
//
// 生活类比：换特种发动机前，先看一眼车库有没有它专用的燃料——
// 没有就提前告诉司机"去搞燃料"，而不是装上发动机打不着火才发现。
func CheckBackendRuntimeReady(bt BackendType, runtimeDir string) string {
	deps, ok := backendRuntimeDeps[bt]
	if !ok || len(deps) == 0 {
		return ""
	}
	info := GetBackendInfo(bt)
	for _, dll := range deps {
		if _, err := lookupPath(dll); err == nil {
			return ""
		}
		if _, err := os.Stat(filepath.Join(runtimeDir, info.Subdir, dll)); err == nil {
			return ""
		}
	}
	switch bt {
	case BackendHIP:
		return "HIP 后端缺少 ROCm 运行时（amdhip64.dll）。请安装 AMD ROCm 并加入 PATH，或将运行时 DLL 放入 runtime/hip/ 目录，否则 llama-server 将无法启动。"
	case BackendSYCL:
		return "SYCL 后端缺少 Intel oneAPI 运行时（sycl8.dll）。请安装 Intel oneAPI 并加入 PATH，或将运行时 DLL 放入 runtime/sycl/ 目录，否则 llama-server 将无法启动。"
	}
	return ""
}

// IsValidBackendType 校验字符串是否为有效的后端类型（含 "auto"）。
//
// 生活类比：检查用户填的"发动机型号"是不是我们店里有的型号。
//
// 同步提醒：config 包有本地副本 config.isValidBackendType（未导出），
// 使用字符串字面量以避免导入 llm 包导致循环依赖。
// 新增后端类型时需两处同步更新：
//   - 此处新增 BackendType 常量
//   - config/config.go 的 isValidBackendType switch 中新增对应字面量
func IsValidBackendType(s string) bool {
	switch BackendType(s) {
	case BackendAuto, BackendCUDA, BackendHIP, BackendSYCL,
		BackendVulkan, BackendOpenVINO, BackendCPU:
		return true
	}
	return false
}

// IsModularBackend 判断后端的官方预编译包是否为模块化（只含后端 DLL，不含核心文件）。
//
// 生活类比：有些发动机是"总成"（自带底盘、线束，买来就能用），有些只是"裸机"
// （只有发动机本体，需要另外配底盘和线束）。模块化后端就像裸机，需要先有 CPU
// 包作为"底盘"（提供 llama-server.exe + 核心 DLL），再装上后端 DLL 才能运行。
//
// 官方预编译包结构（通过分析 llama.cpp release.yml 构建脚本确认）：
//   - CPU / OpenVINO：完整包（含 llama-server.exe + 所有核心 DLL）→ false
//   - CUDA / Vulkan / SYCL / HIP：模块化包（仅含后端 DLL）→ true
//
// 调用方在下载安装流程中，对模块化后端需要先下载 CPU 包作为基础。
func IsModularBackend(bt BackendType) bool {
	info := GetBackendInfo(bt)
	return info.BackendDLL != ""
}

// AllBackendTypes 返回所有后端类型列表，用于前端下拉框展示。
// 顺序：auto, cuda, vulkan, cpu（自动档），其后 hip, sycl, openvino（手动高级选项）。
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
