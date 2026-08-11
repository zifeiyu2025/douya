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
// 有 NVIDIA 就用 CUDA，有 AMD 就用 Vulkan（成熟稳定，无需 ROCm 运行时），有 Intel 就用 SYCL，啥都没有就用 CPU。
// 优先使用厂商原生后端（性能最佳），Vulkan 作为 AMD 默认与跨厂商兜底。
func TestResolveBackendType_Auto(t *testing.T) {
	tests := []struct {
		name        string
		vendor      string
		wantBackend llm.BackendType
	}{
		{"NVIDIA 显卡 → CUDA", "nvidia", llm.BackendCUDA},
		{"AMD 显卡 → Vulkan（成熟稳定，无需 ROCm 运行时）", "amd", llm.BackendVulkan},
		{"Intel 显卡 → SYCL（原生后端）", "intel", llm.BackendSYCL},
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
// 生活类比：想用原厂发动机（SYCL），没装；找通用发动机（Vulkan），也没有；
// 找备用发动机（CPU），有就用；全都没有就保持原选择，等安装流程处理。
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
			name:          "Intel + SYCL 已安装 → SYCL",
			vendor:        "intel",
			setupBackends: []string{"sycl"},
			wantBackend:   llm.BackendSYCL,
		},
		{
			name:          "Intel + SYCL 未安装但 Vulkan 已安装 → Vulkan",
			vendor:        "intel",
			setupBackends: []string{"vulkan"},
			wantBackend:   llm.BackendVulkan,
		},
		{
			name:          "Intel + SYCL 和 Vulkan 都未安装但 CPU 已安装 → CPU",
			vendor:        "intel",
			setupBackends: []string{"cpu"},
			wantBackend:   llm.BackendCPU,
		},
		{
			name:          "Intel + 全部未安装 → 返回原推断 SYCL（触发下载）",
			vendor:        "intel",
			setupBackends: []string{},
			wantBackend:   llm.BackendSYCL,
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
// 顺序约定：auto 在最前（默认选项），cpu 在最后（兜底选项），中间是各厂商 GPU 后端。
// 生活类比：菜单上的选项要按约定顺序排列，方便用户查找。
func TestAllBackendTypes(t *testing.T) {
	all := llm.AllBackendTypes()

	// 期望的完整顺序
	wantOrder := []llm.BackendType{
		llm.BackendAuto,
		llm.BackendCUDA,
		llm.BackendHIP,
		llm.BackendSYCL,
		llm.BackendVulkan,
		llm.BackendOpenVINO,
		llm.BackendCPU,
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
