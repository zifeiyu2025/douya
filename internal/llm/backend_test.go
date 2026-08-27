// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"douya/internal/llm"
	"douya/internal/system"
)

// TestGetBackendInfo_AllTypes 验证每种后端类型都能返回正确的配置信息。
//
// 生活类比：就像查车辆登记证——每辆车的发动机型号、安装位置、零件清单都该对得上号，
// 不能出现"登记写 V8 实际装的是电动机"这种错配。
func TestGetBackendInfo_AllTypes(t *testing.T) {
	tests := []struct {
		name          string
		bt            llm.BackendType
		wantSubdir    string // 期望的子目录名（auto 为空）
		wantHasMTMD   bool   // 是否应包含 mtmd.dll（多模态支持库）
		wantHasGGML   bool   // 是否应包含 ggml-*.dll（核心计算库）
		wantVendorCnt int    // 厂商 DLL 数量（CUDA=3，其他=0）
	}{
		{"CUDA 后端", llm.BackendCUDA, "cuda", true, true, 3},
		{"HIP 后端", llm.BackendHIP, "hip", true, true, 0},
		{"SYCL 后端", llm.BackendSYCL, "sycl", true, true, 0},
		{"Vulkan 后端", llm.BackendVulkan, "vulkan", true, true, 0},
		{"OpenVINO 后端", llm.BackendOpenVINO, "openvino", true, true, 0},
		{"CPU 后端", llm.BackendCPU, "cpu", true, true, 0},
		{"auto 后端", llm.BackendAuto, "", false, false, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := llm.GetBackendInfo(tt.bt)

			if info.Type != tt.bt {
				t.Errorf("Type = %q, want %q", info.Type, tt.bt)
			}
			if info.Subdir != tt.wantSubdir {
				t.Errorf("Subdir = %q, want %q", info.Subdir, tt.wantSubdir)
			}
			if info.DisplayName == "" {
				t.Error("DisplayName 不应为空")
			}
			if info.Description == "" {
				t.Error("Description 不应为空")
			}

			// 检查 RequiredDLLs 是否包含期望的核心库
			hasMTMD := contains(info.RequiredDLLs, "mtmd.dll")
			hasGGML := contains(info.RequiredDLLs, "ggml.dll")
			if hasMTMD != tt.wantHasMTMD {
				t.Errorf("RequiredDLLs 包含 mtmd.dll = %v, want %v", hasMTMD, tt.wantHasMTMD)
			}
			if hasGGML != tt.wantHasGGML {
				t.Errorf("RequiredDLLs 包含 ggml.dll = %v, want %v", hasGGML, tt.wantHasGGML)
			}

			// VendorDLLs 数量校验
			if len(info.VendorDLLs) != tt.wantVendorCnt {
				t.Errorf("VendorDLLs 数量 = %d, want %d", len(info.VendorDLLs), tt.wantVendorCnt)
			}

			// 关键：RequiredDLLs 和 VendorDLLs 不应为 nil（避免后续空指针）
			// 生活类比：就算没有专属配件，配件清单也得是"空清单"而不是"没有清单"
			if info.RequiredDLLs == nil {
				t.Error("RequiredDLLs 不应为 nil（应为空切片）")
			}
			if info.VendorDLLs == nil {
				t.Error("VendorDLLs 不应为 nil（应为空切片）")
			}
		})
	}
}

// TestGetBackendInfo_CUDA_VendorDLLs 单独验证 CUDA 后端的厂商 DLL 是否正确。
//
// CUDA 后端是唯一需要厂商运行时 DLL 的后端（cudart/cublas/cublasLt），
// 这些 DLL 随 NVIDIA 驱动分发，缺失会导致启动失败。
//
// VendorDLLs 使用 glob 模式（cudart64_*.dll 等），同时兼容 CUDA 12 和 CUDA 13：
//   - CUDA 12：cudart64_12.dll / cublas64_12.dll / cublasLt64_12.dll
//   - CUDA 13：cudart64_13.dll / cublas64_13.dll / cublasLt64_13.dll
//
// validatePaths 中对 VendorDLLs 仅做警告级检查（缺失不阻断启动），
// 因为这些 DLL 可能存在于系统 PATH（NVIDIA 驱动自带）而非 runtime 目录。
func TestGetBackendInfo_CUDA_VendorDLLs(t *testing.T) {
	info := llm.GetBackendInfo(llm.BackendCUDA)

	// 期望的 glob 模式条目
	expectedVendorGlobs := []string{
		"cudart64_*.dll",
		"cublas64_*.dll",
		"cublasLt64_*.dll",
	}
	for _, dll := range expectedVendorGlobs {
		if !contains(info.VendorDLLs, dll) {
			t.Errorf("CUDA VendorDLLs 缺少 glob 条目 %q", dll)
		}
	}

	// 确保不再硬编码 _13 后缀（已改为 glob）
	for _, dll := range info.VendorDLLs {
		if strings.HasSuffix(dll, "_13.dll") {
			t.Errorf("CUDA VendorDLLs 不应再硬编码 _13 后缀（应使用 glob）, got %q", dll)
		}
	}

	// CUDA 专属的 ggml-cuda.dll 必须在 RequiredDLLs 中
	if !contains(info.RequiredDLLs, "ggml-cuda.dll") {
		t.Error("CUDA RequiredDLLs 缺少 ggml-cuda.dll")
	}
}

// TestGetBackendInfo_CoreDLLs_GlobPattern 验证 coreDLLs 中 ggml-cpu 条目使用 glob 模式。
//
// 新版官方预编译包把 ggml-cpu.dll 拆分为架构特定的 ggml-cpu-haswell.dll 等 14 个 DLL，
// 自编译版仍产出统一的 ggml-cpu.dll。coreDLLs 用 "ggml-cpu*.dll" glob 模式同时兼容两种布局，
// 此测试确保该 glob 条目存在于所有具体后端的 RequiredDLLs 中。
//
// 生活类比：检查清单上写的是"任一款 CPU 轮胎"（通配符），而不是某个固定型号，
// 这样不管厂家发的是通用胎还是按车型发的专用胎，只要有一款就能通过检查。
func TestGetBackendInfo_CoreDLLs_GlobPattern(t *testing.T) {
	// 所有具体后端（非 auto）都应包含 ggml-cpu*.dll glob 条目
	concreteBackends := []llm.BackendType{
		llm.BackendCUDA, llm.BackendHIP, llm.BackendSYCL,
		llm.BackendVulkan, llm.BackendOpenVINO, llm.BackendCPU,
	}
	for _, bt := range concreteBackends {
		info := llm.GetBackendInfo(bt)
		if !contains(info.RequiredDLLs, "ggml-cpu*.dll") {
			t.Errorf("后端 %s 的 RequiredDLLs 应包含 glob 条目 ggml-cpu*.dll", bt.String())
		}
	}
}

// TestGetBackendInfo_UnknownType 验证未知后端类型回退到 CPU。
//
// 生活类比：用户填了个不存在的发动机型号"V12 柴油"，系统应回退到默认的"电动发动机"（CPU），
// 而不是返回空配置导致后续流程崩溃。
func TestGetBackendInfo_UnknownType(t *testing.T) {
	info := llm.GetBackendInfo(llm.BackendType("unknown_backend"))

	if info.Type != llm.BackendCPU {
		t.Errorf("未知后端应回退到 CPU, got Type = %q", info.Type)
	}
	if info.Subdir != "cpu" {
		t.Errorf("未知后端应回退到 cpu 子目录, got Subdir = %q", info.Subdir)
	}
}

// TestResolveBackendType_Auto 验证 auto 模式下根据 GPU 厂商自动选择原生后端。
//
// 生活类比：用户说"随便帮我选个发动机"，系统根据车库里有啥车来推荐——
// 有 NVIDIA 就用 CUDA，有 AMD/Intel 就统一走 Vulkan（借系统驱动自带的 Vulkan 运行时，
// 规避 Windows 上脆弱的 ROCm/HIP、SYCL 计算栈），啥都没有就用 CPU。
// auto 实际只会解析到 CUDA / Vulkan / CPU 三档；HIP/SYCL/OpenVINO 仅手动可选。
func TestResolveBackendType_Auto(t *testing.T) {
	tests := []struct {
		name        string
		vendor      string
		wantBackend llm.BackendType
	}{
		{"NVIDIA 显卡 → CUDA", "nvidia", llm.BackendCUDA},
		{"AMD 显卡 → Vulkan（成熟稳定，无需 ROCm 运行时）", "amd", llm.BackendVulkan},
		{"Intel 显卡 → Vulkan（非 N 卡统一走 Vulkan）", "intel", llm.BackendVulkan},
		{"Vulkan 设备 → Vulkan", "vulkan", llm.BackendVulkan},
		{"未知厂商 → CPU", "unknown_vendor", llm.BackendCPU},
		{"空厂商 → CPU", "", llm.BackendCPU},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hw := &system.HardwareInfo{GPUVendor: tt.vendor}
			got := llm.ResolveBackendType(hw, string(llm.BackendAuto))
			if got != tt.wantBackend {
				t.Errorf("ResolveBackendType(vendor=%q, auto) = %q, want %q",
					tt.vendor, got, tt.wantBackend)
			}
		})
	}
}

// TestResolveBackendType_EmptyConfig 验证空配置等同于 auto 模式。
//
// 生活类比：用户还没选过发动机（配置为空），等同于说"随便"（auto）。
func TestResolveBackendType_EmptyConfig(t *testing.T) {
	hw := &system.HardwareInfo{GPUVendor: "nvidia"}
	got := llm.ResolveBackendType(hw, "")
	if got != llm.BackendCUDA {
		t.Errorf("空配置 + NVIDIA 应解析为 CUDA, got %q", got)
	}
}

// TestResolveBackendType_Explicit 验证显式指定的后端直接返回。
//
// 生活类比：用户明确说"我要 V8 涡轮增压"（cuda），就给他装 V8，不管车库里有啥车。
func TestResolveBackendType_Explicit(t *testing.T) {
	tests := []struct {
		name        string
		cfgBackend  string
		wantBackend llm.BackendType
	}{
		{"显式 cuda", "cuda", llm.BackendCUDA},
		{"显式 hip", "hip", llm.BackendHIP},
		{"显式 sycl", "sycl", llm.BackendSYCL},
		{"显式 vulkan", "vulkan", llm.BackendVulkan},
		{"显式 openvino", "openvino", llm.BackendOpenVINO},
		{"显式 cpu", "cpu", llm.BackendCPU},
	}

	// 即使硬件是 NVIDIA，显式指定其他后端也应尊重用户选择
	hw := &system.HardwareInfo{GPUVendor: "nvidia", HasGPU: true}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llm.ResolveBackendType(hw, tt.cfgBackend)
			if got != tt.wantBackend {
				t.Errorf("ResolveBackendType(explicit=%q) = %q, want %q",
					tt.cfgBackend, got, tt.wantBackend)
			}
		})
	}
}

// TestResolveBackendType_Invalid 验证无效配置值回退到 CPU。
//
// 生活类比：用户填了个不存在的"核动力发动机"，系统回退到安全的 CPU 模式。
func TestResolveBackendType_Invalid(t *testing.T) {
	hw := &system.HardwareInfo{GPUVendor: "nvidia"}

	tests := []string{"invalid", "CUDA", "Cuda", "123", "null"}
	for _, cfg := range tests {
		t.Run("无效值_"+cfg, func(t *testing.T) {
			got := llm.ResolveBackendType(hw, cfg)
			if got != llm.BackendCPU {
				t.Errorf("无效配置 %q 应回退到 CPU, got %q", cfg, got)
			}
		})
	}
}

// TestResolveBackendType_NilHardware 验证 hw 为 nil 时安全回退。
//
// 生活类比：连车库都查不了（硬件检测失败），只能用最保守的 CPU 模式。
func TestResolveBackendType_NilHardware(t *testing.T) {
	got := llm.ResolveBackendType(nil, string(llm.BackendAuto))
	if got != llm.BackendCPU {
		t.Errorf("nil hardware + auto 应回退到 CPU, got %q", got)
	}
}

// TestResolveBackendTypeWithRuntime_Fallback 验证 auto 模式下的回退策略。
//
// 回退顺序：原生后端 → Vulkan → CPU → 原推断（触发下载）
// 生活类比：auto 为 Intel 选通用发动机（Vulkan）；没装 Vulkan 就找备用发动机（CPU）；
// 全都没有就保持原选择（Vulkan），等安装流程处理。SYCL 等仅手动可选，auto 不会自动选。
func TestResolveBackendTypeWithRuntime_Fallback(t *testing.T) {
	// 创建临时 runtime 目录
	tempDir := t.TempDir()

	// 辅助函数：在 runtime 目录下创建指定后端的 llama-server.exe
	setupBackend := func(subdir string) {
		backendDir := filepath.Join(tempDir, subdir)
		if err := os.MkdirAll(backendDir, 0755); err != nil {
			t.Fatalf("创建目录失败: %v", err)
		}
		serverPath := filepath.Join(backendDir, "llama-server.exe")
		if err := os.WriteFile(serverPath, []byte("fake"), 0644); err != nil {
			t.Fatalf("创建文件失败: %v", err)
		}
	}

	tests := []struct {
		name          string
		vendor        string
		setupBackends []string // 在 runtime 目录中预装的后端子目录
		wantBackend   llm.BackendType
	}{
		{
			name:          "Intel + SYCL 已安装但 Vulkan 未安装 → Vulkan（auto 不自动选 SYCL）",
			vendor:        "intel",
			setupBackends: []string{"sycl"},
			wantBackend:   llm.BackendVulkan,
		},
		{
			name:          "Intel + Vulkan 已安装 → Vulkan",
			vendor:        "intel",
			setupBackends: []string{"vulkan"},
			wantBackend:   llm.BackendVulkan,
		},
		{
			name:          "Intel + Vulkan 和 SYCL 都未安装但 CPU 已安装 → CPU",
			vendor:        "intel",
			setupBackends: []string{"cpu"},
			wantBackend:   llm.BackendCPU,
		},
		{
			name:          "Intel + 全部未安装 → 返回原推断 Vulkan（触发下载）",
			vendor:        "intel",
			setupBackends: []string{},
			wantBackend:   llm.BackendVulkan,
		},
		{
			name:          "AMD + Vulkan 已安装 → Vulkan",
			vendor:        "amd",
			setupBackends: []string{"vulkan"},
			wantBackend:   llm.BackendVulkan,
		},
		{
			name:          "AMD + Vulkan 未安装但 CPU 已安装 → CPU",
			vendor:        "amd",
			setupBackends: []string{"cpu"},
			wantBackend:   llm.BackendCPU,
		},
		{
			name:          "NVIDIA + CUDA 已安装 → CUDA",
			vendor:        "nvidia",
			setupBackends: []string{"cuda"},
			wantBackend:   llm.BackendCUDA,
		},
		{
			// 回归场景：用户删除 runtime/cuda/ 想重新拉取 CUDA 时，
			// 不应被预装的 Vulkan 兜底接管，auto 应返回原生 CUDA（触发下载）。
			name:          "NVIDIA + CUDA 未安装但 Vulkan 已安装 → CUDA",
			vendor:        "nvidia",
			setupBackends: []string{"vulkan"},
			wantBackend:   llm.BackendCUDA,
		},
		{
			name:          "NVIDIA + CUDA 未安装但 CPU 已安装 → CUDA",
			vendor:        "nvidia",
			setupBackends: []string{"cpu"},
			wantBackend:   llm.BackendCUDA,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 每个测试用例使用独立的临时目录
			testDir := filepath.Join(tempDir, tt.name)
			_ = os.MkdirAll(testDir, 0755)

			// 清理并重建后端目录
			for _, subdir := range tt.setupBackends {
				setupBackend(filepath.Join(tt.name, subdir))
			}

			hw := &system.HardwareInfo{GPUVendor: tt.vendor, HasGPU: true}
			got := llm.ResolveBackendTypeWithRuntime(hw, string(llm.BackendAuto), testDir)
			if got != tt.wantBackend {
				t.Errorf("ResolveBackendTypeWithRuntime(vendor=%q) = %q, want %q",
					tt.vendor, got, tt.wantBackend)
			}
		})
	}
}

// TestResolveBackendTypeWithRuntime_ManualNoFallback 验证手动指定后端时不回退。
//
// 生活类比：用户明确说"我要 SYCL"，即使没装也不擅自换成别的，尊重用户选择。
func TestResolveBackendTypeWithRuntime_ManualNoFallback(t *testing.T) {
	tempDir := t.TempDir()
	// 不安装任何后端
	hw := &system.HardwareInfo{GPUVendor: "intel", HasGPU: true}

	// 手动指定 SYCL，即使未安装也不回退
	got := llm.ResolveBackendTypeWithRuntime(hw, "sycl", tempDir)
	if got != llm.BackendSYCL {
		t.Errorf("手动指定 sycl 时不应回退, got %q", got)
	}
}

// TestIsValidBackendType 验证后端类型合法性校验。
func TestIsValidBackendType(t *testing.T) {
	validCases := []string{"auto", "cuda", "hip", "sycl", "vulkan", "openvino", "cpu"}
	for _, bt := range validCases {
		if !llm.IsValidBackendType(bt) {
			t.Errorf("IsValidBackendType(%q) = false, want true", bt)
		}
	}

	invalidCases := []string{"", "CUDA", "Cuda", "invalid", "123", "null", "metal"}
	for _, bt := range invalidCases {
		if llm.IsValidBackendType(bt) {
			t.Errorf("IsValidBackendType(%q) = true, want false", bt)
		}
	}
}

// TestAllBackendTypes 验证后端类型列表的完整性和顺序。
//
// 顺序约定：auto 在最前（默认选项），其后为常用自动档 cuda/vulkan/cpu，
// 手动高级选项 hip/sycl/openvino 置后（auto 永不自动选）。
// 生活类比：菜单上的选项要按约定顺序排列，方便用户查找。
func TestAllBackendTypes(t *testing.T) {
	all := llm.AllBackendTypes()

	// 期望的完整顺序：auto、CUDA/Vulkan/CPU 为常用自动档，HIP/SYCL/OpenVINO 为手动高级选项置后
	wantOrder := []llm.BackendType{
		llm.BackendAuto,
		llm.BackendCUDA,
		llm.BackendVulkan,
		llm.BackendCPU,
		llm.BackendHIP,
		llm.BackendSYCL,
		llm.BackendOpenVINO,
	}

	if len(all) != len(wantOrder) {
		t.Fatalf("AllBackendTypes 返回 %d 项, want %d", len(all), len(wantOrder))
	}

	for i, bt := range all {
		if bt != wantOrder[i] {
			t.Errorf("AllBackendTypes[%d] = %q, want %q", i, bt, wantOrder[i])
		}
	}
}

// TestAllBackendTypes_Immutable 验证返回的切片是副本，修改不影响内部状态。
//
// 生活类比：菜单给顾客看的是复印件，顾客在复印件上乱画不影响原版菜单。
func TestAllBackendTypes_Immutable(t *testing.T) {
	all1 := llm.AllBackendTypes()
	all1[0] = llm.BackendCPU // 篡改第一个元素

	all2 := llm.AllBackendTypes()
	if all2[0] != llm.BackendAuto {
		t.Error("AllBackendTypes 返回的应是副本，篡改第一个切片不应影响第二次调用")
	}
}

// TestBackendType_String 验证 String 方法。
func TestBackendType_String(t *testing.T) {
	tests := []struct {
		bt   llm.BackendType
		want string
	}{
		{llm.BackendAuto, "auto"},
		{llm.BackendCUDA, "cuda"},
		{llm.BackendHIP, "hip"},
		{llm.BackendCPU, "cpu"},
		{llm.BackendType("custom"), "custom"},
	}

	for _, tt := range tests {
		if got := tt.bt.String(); got != tt.want {
			t.Errorf("BackendType(%q).String() = %q, want %q", tt.bt, got, tt.want)
		}
	}
}

// contains 是测试辅助函数，检查字符串切片是否包含某值。
func contains(slice []string, target string) bool {
	for _, s := range slice {
		if s == target {
			return true
		}
	}
	return false
}

// ============================================================================
// P3.7 后端×显卡能力匹配预检测试
//
// 生活类比：这套测试像"4S 店选装审核"——在动手装发动机（后端）之前，
// 先用"选装清单审核表"（CheckBackendCapabilityMatch）验证车型（显卡）与
// 发动机是否匹配，避免装上去打不着火（启动即失败）再返工。
// ============================================================================

// TestCheckBackendCapabilityMatch_CUDA 验证 CUDA 后端的能力匹配判定。
// 核心规则（业界 Ollama/LM Studio 共识）：
//   - 必须有 NVIDIA GPU（或 CUDA 驱动核心组件）
//   - 驱动版本 ≥531（CUDA 12 门槛）；Blackwell 还需 ≥580
func TestCheckBackendCapabilityMatch_CUDA(t *testing.T) {
	tests := []struct {
		name string
		hw   *system.HardwareInfo
		want string // 空=匹配；非空=不匹配（含原因说明）
	}{
		{"N 卡且驱动达标", &system.HardwareInfo{GPUVendor: "nvidia", GPUDriverVersion: "585.00"}, ""},
		{"N 卡且驱动 531 门槛", &system.HardwareInfo{GPUVendor: "nvidia", GPUDriverVersion: "531.41"}, ""},
		{"N 卡但驱动过旧", &system.HardwareInfo{GPUVendor: "nvidia", GPUDriverVersion: "470.00"}, "NVIDIA 驱动版本过旧"},
		{"Blackwell 但驱动不足 580", &system.HardwareInfo{GPUVendor: "nvidia", GPUDriverVersion: "566.03", GPUArchitecture: "Blackwell"}, "Blackwell 显卡需要 NVIDIA 驱动 ≥580"},
		{"Blackwell 且驱动 580+", &system.HardwareInfo{GPUVendor: "nvidia", GPUDriverVersion: "580.88", GPUArchitecture: "Blackwell"}, ""},
		{"驱动版本未知（保守通过）", &system.HardwareInfo{GPUVendor: "nvidia"}, ""},
		{"A 卡选 CUDA", &system.HardwareInfo{GPUVendor: "amd"}, "CUDA 后端需要 NVIDIA 显卡"},
		{"仅在 CUDA 驱动（无 nvidia-smi）", &system.HardwareInfo{HasCUDABackend: true}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := llm.CheckBackendCapabilityMatch(tt.hw, llm.BackendCUDA)
			if tt.want == "" {
				if got != "" {
					t.Errorf("期望匹配（空提示），实际不匹配：%q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("期望提示包含 %q，实际 %q", tt.want, got)
			}
		})
	}

	// hw 为 nil：不做能力校验（视为通过，避免误导决策）
	if got := llm.CheckBackendCapabilityMatch(nil, llm.BackendCUDA); got != "" {
		t.Errorf("hw 为 nil 时应视为匹配，实际 %q", got)
	}
}

// TestCheckBackendCapabilityMatch_Vulkan 验证 Vulkan 后端能力匹配。
// Vulkan 是跨厂商通用后端，只需系统带 vulkan-1.dll 运行时。
func TestCheckBackendCapabilityMatch_Vulkan(t *testing.T) {
	orig := system.HasVulkanRuntime
	defer func() { system.HasVulkanRuntime = orig }()

	// 有运行时 → 匹配
	system.HasVulkanRuntime = func() bool { return true }
	if got := llm.CheckBackendCapabilityMatch(&system.HardwareInfo{GPUVendor: "amd"}, llm.BackendVulkan); got != "" {
		t.Errorf("AMD + 有 Vulkan 运行时应匹配，实际 %q", got)
	}

	// 无运行时 → 不匹配（提示缺失运行时）
	system.HasVulkanRuntime = func() bool { return false }
	got := llm.CheckBackendCapabilityMatch(&system.HardwareInfo{GPUVendor: "amd"}, llm.BackendVulkan)
	if !strings.Contains(got, "vulkan-1.dll") {
		t.Errorf("缺少 Vulkan 运行时应给出提示（含 vulkan-1.dll），实际 %q", got)
	}
}

// TestCheckBackendCapabilityMatch_HipSyclOpenvino 验证厂商专属后端的匹配规则：
//   - HIP/ROCm 需要 AMD；SYCL/OpenVINO 需要 Intel
//   - 厂商不符时返回明确原因
func TestCheckBackendCapabilityMatch_HipSyclOpenvino(t *testing.T) {
	tests := []struct {
		bt   llm.BackendType
		hw   *system.HardwareInfo
		want string // 空=匹配
	}{
		{llm.BackendHIP, &system.HardwareInfo{GPUVendor: "amd"}, ""},
		{llm.BackendHIP, &system.HardwareInfo{GPUVendor: "nvidia"}, "HIP/ROCm 后端需要 AMD 显卡"},
		{llm.BackendSYCL, &system.HardwareInfo{GPUVendor: "intel"}, ""},
		{llm.BackendSYCL, &system.HardwareInfo{GPUVendor: "amd"}, "SYCL 后端需要 Intel 显卡"},
		{llm.BackendOpenVINO, &system.HardwareInfo{GPUVendor: "intel"}, ""},
		{llm.BackendOpenVINO, &system.HardwareInfo{GPUVendor: "nvidia"}, "OpenVINO 后端需要 Intel 显卡"},
	}
	for _, tt := range tests {
		t.Run(string(tt.bt)+"@"+tt.hw.GPUVendor, func(t *testing.T) {
			got := llm.CheckBackendCapabilityMatch(tt.hw, tt.bt)
			if tt.want == "" {
				if got != "" {
					t.Errorf("期望匹配（空提示），实际不匹配：%q", got)
				}
				return
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("期望提示包含 %q，实际 %q", tt.want, got)
			}
		})
	}
}

// TestCheckBackendCapabilityMatch_CPU 验证 CPU 后端恒可用。
func TestCheckBackendCapabilityMatch_CPU(t *testing.T) {
	hws := []*system.HardwareInfo{
		nil,
		{GPUVendor: ""},
		{GPUVendor: "nvidia"},
		{GPUVendor: "amd"},
	}
	for _, hw := range hws {
		if got := llm.CheckBackendCapabilityMatch(hw, llm.BackendCPU); got != "" {
			t.Errorf("CPU 后端应恒匹配（hw=%+v），实际 %q", hw, got)
		}
	}
	if got := llm.CheckBackendCapabilityMatch(nil, llm.BackendAuto); got != "" {
		t.Errorf("auto 后端应恒匹配，实际 %q", got)
	}
}

// TestResolveBackendTypeWithRuntimeDirs_CapabilityFallback 验证能力预检集成到解析链路：
//   - N 卡 + 驱动过旧 + CUDA 未装 → 不选 CUDA（能力剔除）
//   - N 卡 + 驱动过旧 + Vulkan 已装 → 回退 Vulkan
//   - N 卡 + 驱动达标 → 保持 CUDA 原生优先（即使未安装，也交给下载流程）
func TestResolveBackendTypeWithRuntimeDirs_CapabilityFallback(t *testing.T) {
	origHasVulkan := system.HasVulkanRuntime
	defer func() { system.HasVulkanRuntime = origHasVulkan }()
	system.HasVulkanRuntime = func() bool { return true }

	dirs := []string{t.TempDir(), t.TempDir()}

	// 场景 1：N 卡 + 老驱动。不应解析回 CUDA；无任何已装后端时回退 CPU。
	oldNv := &system.HardwareInfo{GPUVendor: "nvidia", GPUDriverVersion: "470.00"}
	if got := llm.ResolveBackendTypeWithRuntimeDirs(oldNv, "auto", dirs); got != llm.BackendCPU {
		t.Errorf("N 卡+老驱动+无已装后端：期望回退 CPU，实际 %v", got)
	}

	// 场景 2：N 卡 + 老驱动 + Vulkan 已装 → 回退 Vulkan
	installFakeBackend(t, dirs[0], "vulkan")
	if got := llm.ResolveBackendTypeWithRuntimeDirs(oldNv, "auto", dirs); got != llm.BackendVulkan {
		t.Errorf("N 卡+老驱动+Vulkan 已装：期望回退 Vulkan，实际 %v", got)
	}

	// 场景 3：N 卡 + 驱动达标（585）→ 保持 CUDA 原生优先（即使未安装，也返回 CUDA 交给下载）
	newNv := &system.HardwareInfo{GPUVendor: "nvidia", GPUDriverVersion: "585.00", GPUArchitecture: "Blackwell"}
	if got := llm.ResolveBackendTypeWithRuntimeDirs(newNv, "auto", dirs); got != llm.BackendCUDA {
		t.Errorf("N 卡+驱动达标：期望保持 CUDA（原生优先），实际 %v", got)
	}
}

// installFakeBackend 在指定 runtime 目录下创建一个假后端（只放 llama-server.exe）。
// 生活类比：往"假车库"里停一辆"假车"（空壳 exe），让"已安装"检查能命中。
func installFakeBackend(t *testing.T, runtimeDir, subdir string) {
	t.Helper()
	dir := filepath.Join(runtimeDir, subdir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("创建后端目录失败: %v", err)
	}
	exePath := filepath.Join(dir, "llama-server.exe")
	if err := os.WriteFile(exePath, []byte("fake exe"), 0o644); err != nil {
		t.Fatalf("创建假 llama-server 失败: %v", err)
	}
}
