// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"errors"
	"testing"
)

// TestParseGPUArchitecture 测试从 GPU 名称解析微架构代号
//
// 生活类比：就像从车牌号前缀判断车辆品牌，从 GPU 名称中的"RTX 50/40/30"等关键字
// 可以判断它属于哪一代架构。这个测试确保解析逻辑覆盖各种常见命名格式。
func TestParseGPUArchitecture(t *testing.T) {
	tests := []struct {
		name     string
		gpuName  string
		wantArch string
	}{
		// RTX 50 系 → Blackwell
		{"RTX 5090", "NVIDIA GeForce RTX 5090", "Blackwell"},
		{"RTX 5080", "NVIDIA GeForce RTX 5080", "Blackwell"},
		{"RTX 5070 Ti", "RTX 5070 Ti", "Blackwell"},
		{"RTX 5060", "GeForce RTX 5060", "Blackwell"},
		// RTX 40 系 → Ada
		{"RTX 4090", "NVIDIA GeForce RTX 4090", "Ada"},
		{"RTX 4080", "RTX 4080", "Ada"},
		{"RTX 4060", "GeForce RTX 4060", "Ada"},
		// RTX 30 系 → Ampere
		{"RTX 3090", "NVIDIA GeForce RTX 3090", "Ampere"},
		{"RTX 3080 Ti", "RTX 3080 Ti", "Ampere"},
		{"RTX 3060", "GeForce RTX 3060", "Ampere"},
		// GTX 16 系 → Turing
		{"GTX 1660", "GeForce GTX 1660", "Turing"},
		{"GTX 1650", "GTX 1650", "Turing"},
		// 边界情况
		{"空字符串", "", "Unknown"},
		{"未知显卡", "Intel Arc A770", "Unknown"},
		{"AMD 显卡", "AMD Radeon RX 7900 XTX", "Unknown"},
		// 大小写不敏感
		{"小写 rtx 50", "rtx 5090", "Blackwell"},
		{"混合大小写", "Rtx 4090", "Ada"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseGPUArchitecture(tt.gpuName)
			if got != tt.wantArch {
				t.Errorf("parseGPUArchitecture(%q) 期望 %q，实际 %q", tt.gpuName, tt.wantArch, got)
			}
		})
	}
}

// ============================================================================
// 多厂商 GPU 检测测试（Task 1 扩展）
//
// 生活类比：这部分测试就像"模拟车检"——我们不去真的开车上路（不调用真实 wmic/powershell），
// 而是用"假车库"（mock fileExists）和"假车管所"（mock queryWMI）来验证检测逻辑是否正确。
// ============================================================================

// TestParseWMIOutput 测试 wmic /value 格式输出解析
//
// 生活类比：wmic 输出像一张"填表式登记证"，每行一个"项目=值"，
// 这个测试验证我们能正确读取登记证上的"姓名"和"排量"。
func TestParseWMIOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantVRAM int64
	}{
		{
			name:     "标准输出（4GB 显存）",
			input:    "AdapterRAM=4294967296\r\nName=AMD Radeon RX 7900 XTX\r\n",
			wantName: "AMD Radeon RX 7900 XTX",
			wantVRAM: 4096, // 4294967296 字节 / 1024 / 1024 = 4096 MB
		},
		{
			name:     "字段顺序颠倒",
			input:    "Name=Intel Arc A770\r\nAdapterRAM=8589934592\r\n",
			wantName: "Intel Arc A770",
			wantVRAM: 8192, // 8GB
		},
		{
			name:     "空输出",
			input:    "",
			wantName: "",
			wantVRAM: 0,
		},
		{
			name:     "无匹配字段",
			input:    "SomeOtherField=value\r\n",
			wantName: "",
			wantVRAM: 0,
		},
		{
			name:     "VRAM 为 0（大显存显卡 uint32 溢出场景）",
			input:    "Name=AMD Radeon RX 7900 XTX\r\nAdapterRAM=0\r\n",
			wantName: "AMD Radeon RX 7900 XTX",
			wantVRAM: 0,
		},
		{
			name:     "多显卡场景取第一个名称",
			input:    "Name=GPU1\r\nName=GPU2\r\nAdapterRAM=2097152\r\n",
			wantName: "GPU1",
			wantVRAM: 2, // 2097152 字节 = 2MB
		},
		{
			name:     "仅名称无 VRAM",
			input:    "Name=AMD Radeon Graphics\r\n",
			wantName: "AMD Radeon Graphics",
			wantVRAM: 0,
		},
		{
			name:     "带前后空格",
			input:    "  AdapterRAM = 1073741824 \r\n  Name = Intel UHD Graphics 770 \r\n",
			wantName: "Intel UHD Graphics 770",
			wantVRAM: 1024, // 1GB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVRAM := parseWMIOutput(tt.input)
			if gotName != tt.wantName {
				t.Errorf("parseWMIOutput 名称 期望 %q，实际 %q", tt.wantName, gotName)
			}
			if gotVRAM != tt.wantVRAM {
				t.Errorf("parseWMIOutput VRAM 期望 %d，实际 %d", tt.wantVRAM, gotVRAM)
			}
		})
	}
}

// TestParsePowerShellWMIOutput 测试 PowerShell "Name|AdapterRAM" 格式输出解析
//
// 生活类比：PowerShell 输出像一张"用竖线分隔的表格"，比 wmic 的键值对更紧凑，
// 这个测试验证我们能正确拆分表格单元格。
func TestParsePowerShellWMIOutput(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantName string
		wantVRAM int64
	}{
		{
			name:     "标准输出（16GB 显存）",
			input:    "AMD Radeon RX 7900 XTX|17163091968\r\n",
			wantName: "AMD Radeon RX 7900 XTX",
			wantVRAM: 16368, // 17163091968 / 1024 / 1024 = 16368 MB
		},
		{
			name:     "VRAM 为 0",
			input:    "Intel Arc A770|0\n",
			wantName: "Intel Arc A770",
			wantVRAM: 0,
		},
		{
			name:     "空输出",
			input:    "",
			wantName: "",
			wantVRAM: 0,
		},
		{
			name:     "仅空白字符",
			input:    "   \r\n  \n",
			wantName: "",
			wantVRAM: 0,
		},
		{
			name:     "多行取第一行",
			input:    "GPU1|2097152\nGPU2|4194304\n",
			wantName: "GPU1",
			wantVRAM: 2, // 2MB
		},
		{
			name:     "名称含空格",
			input:    "  AMD Radeon RX 7800 XT  |  17163091968  \n",
			wantName: "AMD Radeon RX 7800 XT",
			wantVRAM: 16368,
		},
		{
			name:     "无竖线分隔（仅名称）",
			input:    "Some GPU Name\n",
			wantName: "Some GPU Name",
			wantVRAM: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotVRAM := parsePowerShellWMIOutput(tt.input)
			if gotName != tt.wantName {
				t.Errorf("parsePowerShellWMIOutput 名称 期望 %q，实际 %q", tt.wantName, gotName)
			}
			if gotVRAM != tt.wantVRAM {
				t.Errorf("parsePowerShellWMIOutput VRAM 期望 %d，实际 %d", tt.wantVRAM, gotVRAM)
			}
		})
	}
}

// mockGPUDeps 保存并替换 GPU 检测相关包级变量，返回恢复函数。
// 生活类比：就像把车检现场的真实车库和车管所"封存"起来，换成模拟的，
// 测试结束后再"解封"恢复原状，避免影响其他测试。
func mockGPUDeps(
	existsFn func(string) bool,
	lookPathFn func(string) (string, error),
	wmiFn func(string) (string, int64),
	rocmFn func() (int64, bool),
) func() {
	origExists, origLookPath, origWMI, origROCm := fileExists, execLookPath, queryWMI, queryROCmVRAM
	fileExists = existsFn
	execLookPath = lookPathFn
	queryWMI = wmiFn
	queryROCmVRAM = rocmFn
	return func() {
		fileExists, execLookPath, queryWMI, queryROCmVRAM = origExists, origLookPath, origWMI, origROCm
	}
}

// errNotFound 是测试用的"未找到"错误
var errNotFound = errors.New("not found")

// TestDetectAMDGPUFound 测试检测到 AMD 显卡驱动 DLL 的场景
func TestDetectAMDGPUFound(t *testing.T) {
	origDLLs := amdDriverDLLs
	amdDriverDLLs = []string{"mock-amd.dll"}
	defer func() { amdDriverDLLs = origDLLs }()

	restore := mockGPUDeps(
		func(p string) bool { return p == "mock-amd.dll" }, // mock: 只有 mock-amd.dll 存在
		func(string) (string, error) { return "", errNotFound }, // mock: PATH 中找不到
		func(string) (string, int64) { return "AMD Radeon RX 7900 XTX", 8192 }, // mock WMI
		func() (int64, bool) { return 0, false }, // mock: rocm-smi 不可用
	)
	defer restore()

	hw := &HardwareInfo{}
	detectAMDGPU(hw)

	if !hw.HasAMDGPU {
		t.Error("期望 HasAMDGPU=true，实际 false")
	}
	if !hw.HasGPU {
		t.Error("期望 HasGPU=true，实际 false")
	}
	if hw.GPUVendor != "amd" {
		t.Errorf("期望 GPUVendor=amd，实际 %q", hw.GPUVendor)
	}
	if hw.GPUName != "AMD Radeon RX 7900 XTX" {
		t.Errorf("期望 GPUName=AMD Radeon RX 7900 XTX，实际 %q", hw.GPUName)
	}
	if hw.GPUVRAMMB != 8192 {
		t.Errorf("期望 GPUVRAMMB=8192，实际 %d", hw.GPUVRAMMB)
	}
}

// TestDetectAMDGPUWithROCm 测试 rocm-smi 优先于 WMI 获取 VRAM
//
// 生活类比：行驶证（rocm-smi）精度高能显示真实排量，车管所登记（WMI）只能填 4 位数，
// 两份资料都有时，以行驶证为准。
func TestDetectAMDGPUWithROCm(t *testing.T) {
	origDLLs := amdDriverDLLs
	amdDriverDLLs = []string{"mock-amd.dll"}
	defer func() { amdDriverDLLs = origDLLs }()

	restore := mockGPUDeps(
		func(p string) bool { return p == "mock-amd.dll" },
		func(string) (string, error) { return "", errNotFound },
		func(string) (string, int64) { return "AMD GPU", 4096 }, // WMI 返回 4GB（受 uint32 限制不准）
		func() (int64, bool) { return 16384, true },             // rocm-smi 返回 16GB（精确）
	)
	defer restore()

	hw := &HardwareInfo{}
	detectAMDGPU(hw)

	// rocm-smi 优先，VRAM 应为 16384 而非 4096
	if hw.GPUVRAMMB != 16384 {
		t.Errorf("期望 VRAM=16384（rocm-smi 优先），实际 %d", hw.GPUVRAMMB)
	}
	if hw.GPUName != "AMD GPU" {
		t.Errorf("期望 GPUName=AMD GPU，实际 %q", hw.GPUName)
	}
}

// TestDetectAMDGPUNotFound 测试未检测到 AMD 驱动 DLL 的场景
func TestDetectAMDGPUNotFound(t *testing.T) {
	origDLLs := amdDriverDLLs
	amdDriverDLLs = []string{"mock-amd-1.dll", "mock-amd-2.dll"}
	defer func() { amdDriverDLLs = origDLLs }()

	restore := mockGPUDeps(
		func(string) bool { return false }, // mock: 所有 DLL 都不存在
		func(string) (string, error) { return "", errNotFound },
		func(string) (string, int64) { return "should-not-be-called", 9999 },
		func() (int64, bool) { return 0, false },
	)
	defer restore()

	hw := &HardwareInfo{}
	detectAMDGPU(hw)

	if hw.HasAMDGPU {
		t.Error("期望 HasAMDGPU=false，实际 true")
	}
	if hw.HasGPU {
		t.Error("期望 HasGPU=false，实际 true")
	}
	if hw.GPUVendor != "" {
		t.Errorf("期望 GPUVendor 为空，实际 %q", hw.GPUVendor)
	}
	if hw.GPUName != "" {
		t.Errorf("期望 GPUName 为空（不应调用 WMI），实际 %q", hw.GPUName)
	}
}

// TestDetectIntelGPUFound 测试检测到 Intel 显卡驱动 DLL 的场景
func TestDetectIntelGPUFound(t *testing.T) {
	origDLLs := intelDriverDLLs
	intelDriverDLLs = []string{"mock-intel.dll"}
	defer func() { intelDriverDLLs = origDLLs }()

	restore := mockGPUDeps(
		func(p string) bool { return p == "mock-intel.dll" },
		func(string) (string, error) { return "", errNotFound },
		func(string) (string, int64) { return "Intel UHD Graphics 770", 1024 },
		func() (int64, bool) { return 0, false },
	)
	defer restore()

	hw := &HardwareInfo{}
	detectIntelGPU(hw)

	if !hw.HasIntelGPU {
		t.Error("期望 HasIntelGPU=true，实际 false")
	}
	if !hw.HasGPU {
		t.Error("期望 HasGPU=true，实际 false")
	}
	if hw.GPUVendor != "intel" {
		t.Errorf("期望 GPUVendor=intel，实际 %q", hw.GPUVendor)
	}
	if hw.GPUName != "Intel UHD Graphics 770" {
		t.Errorf("期望 GPUName=Intel UHD Graphics 770，实际 %q", hw.GPUName)
	}
	if hw.GPUVRAMMB != 1024 {
		t.Errorf("期望 GPUVRAMMB=1024，实际 %d", hw.GPUVRAMMB)
	}
}

// TestDetectIntelGPUNotFound 测试未检测到 Intel 驱动 DLL 的场景
func TestDetectIntelGPUNotFound(t *testing.T) {
	origDLLs := intelDriverDLLs
	intelDriverDLLs = []string{"mock-intel-1.dll", "mock-intel-2.dll"}
	defer func() { intelDriverDLLs = origDLLs }()

	restore := mockGPUDeps(
		func(string) bool { return false },
		func(string) (string, error) { return "", errNotFound },
		func(string) (string, int64) { return "should-not-be-called", 9999 },
		func() (int64, bool) { return 0, false },
	)
	defer restore()

	hw := &HardwareInfo{}
	detectIntelGPU(hw)

	if hw.HasIntelGPU {
		t.Error("期望 HasIntelGPU=false，实际 true")
	}
	if hw.HasGPU {
		t.Error("期望 HasGPU=false，实际 true")
	}
	if hw.GPUVendor != "" {
		t.Errorf("期望 GPUVendor 为空，实际 %q", hw.GPUVendor)
	}
}

// TestDetectVulkanDeviceFallback 测试 Vulkan 兜底检测逻辑
//
// 生活类比：前面三个品牌展厅（NVIDIA/AMD/Intel）都没找到车，
// 最后看停车场有没有"任意品牌"的车位（vulkan-1.dll）。
// 这个测试验证：只有无其他 GPU 时才触发兜底。
func TestDetectVulkanDeviceFallback(t *testing.T) {
	origVulkan := vulkanDLLPath
	vulkanDLLPath = "mock-vulkan.dll"
	defer func() { vulkanDLLPath = origVulkan }()

	t.Run("无GPU且Vulkan存在_触发兜底", func(t *testing.T) {
		restore := mockGPUDeps(
			func(p string) bool { return p == "mock-vulkan.dll" },
			func(string) (string, error) { return "", errNotFound },
			nil, nil,
		)
		defer restore()

		hw := &HardwareInfo{}
		detectVulkanDevice(hw)

		if !hw.HasGPU {
			t.Error("期望 HasGPU=true（兜底触发）")
		}
		if hw.GPUVendor != "vulkan" {
			t.Errorf("期望 GPUVendor=vulkan，实际 %q", hw.GPUVendor)
		}
		if hw.GPUName != "Vulkan Device" {
			t.Errorf("期望 GPUName=Vulkan Device，实际 %q", hw.GPUName)
		}
		if hw.GPUVRAMMB != 0 {
			t.Errorf("期望 GPUVRAMMB=0（Vulkan 无显存信息），实际 %d", hw.GPUVRAMMB)
		}
	})

	t.Run("已有NVIDIA_GPU_不触发兜底", func(t *testing.T) {
		restore := mockGPUDeps(
			func(p string) bool { return true }, // vulkan DLL 也存在
			func(string) (string, error) { return "", errNotFound },
			nil, nil,
		)
		defer restore()

		hw := &HardwareInfo{
			HasGPU:    true,
			GPUVendor: "nvidia",
			GPUName:   "RTX 4090",
			GPUVRAMMB: 24576,
		}
		detectVulkanDevice(hw)

		// 不应覆盖已有 NVIDIA 信息
		if hw.GPUVendor != "nvidia" {
			t.Errorf("期望 GPUVendor 保持 nvidia，实际 %q", hw.GPUVendor)
		}
		if hw.GPUName != "RTX 4090" {
			t.Errorf("期望 GPUName 保持 RTX 4090，实际 %q", hw.GPUName)
		}
		if hw.GPUVRAMMB != 24576 {
			t.Errorf("期望 GPUVRAMMB 保持 24576，实际 %d", hw.GPUVRAMMB)
		}
	})

	t.Run("已有AMD_GPU_不触发兜底", func(t *testing.T) {
		restore := mockGPUDeps(
			func(p string) bool { return true },
			func(string) (string, error) { return "", errNotFound },
			nil, nil,
		)
		defer restore()

		hw := &HardwareInfo{
			HasGPU:    true,
			GPUVendor: "amd",
			HasAMDGPU: true,
		}
		detectVulkanDevice(hw)

		if hw.GPUVendor != "amd" {
			t.Errorf("期望 GPUVendor 保持 amd，实际 %q", hw.GPUVendor)
		}
	})

	t.Run("无GPU且Vulkan不存在_不触发", func(t *testing.T) {
		restore := mockGPUDeps(
			func(string) bool { return false }, // vulkan DLL 不存在
			func(string) (string, error) { return "", errNotFound },
			nil, nil,
		)
		defer restore()

		hw := &HardwareInfo{}
		detectVulkanDevice(hw)

		if hw.HasGPU {
			t.Error("期望 HasGPU=false（Vulkan DLL 不存在）")
		}
		if hw.GPUVendor != "" {
			t.Errorf("期望 GPUVendor 为空，实际 %q", hw.GPUVendor)
		}
	})
}

// TestNoGPUScenario 测试所有 GPU 检测都失败的场景
//
// 生活类比：车库空空如也，三个品牌展厅都没车，停车场也没车位，
// 最终结论是"这台机器没有可用的 GPU"。
func TestNoGPUScenario(t *testing.T) {
	origAMD := amdDriverDLLs
	origIntel := intelDriverDLLs
	origVulkan := vulkanDLLPath
	amdDriverDLLs = []string{"mock-amd.dll"}
	intelDriverDLLs = []string{"mock-intel.dll"}
	vulkanDLLPath = "mock-vulkan.dll"
	defer func() {
		amdDriverDLLs = origAMD
		intelDriverDLLs = origIntel
		vulkanDLLPath = origVulkan
	}()

	// 所有文件都不存在
	restore := mockGPUDeps(
		func(string) bool { return false },
		func(string) (string, error) { return "", errNotFound },
		func(string) (string, int64) { return "", 0 },
		func() (int64, bool) { return 0, false },
	)
	defer restore()

	hw := &HardwareInfo{}
	detectAMDGPU(hw)
	detectIntelGPU(hw)
	detectVulkanDevice(hw)

	if hw.HasGPU {
		t.Error("期望 HasGPU=false（无任何 GPU）")
	}
	if hw.HasAMDGPU {
		t.Error("期望 HasAMDGPU=false")
	}
	if hw.HasIntelGPU {
		t.Error("期望 HasIntelGPU=false")
	}
	if hw.GPUVendor != "" {
		t.Errorf("期望 GPUVendor 为空，实际 %q", hw.GPUVendor)
	}
	if hw.GPUName != "" {
		t.Errorf("期望 GPUName 为空，实际 %q", hw.GPUName)
	}
	if hw.GPUVRAMMB != 0 {
		t.Errorf("期望 GPUVRAMMB=0，实际 %d", hw.GPUVRAMMB)
	}
}

// TestDetectPriority 测试多厂商检测优先级逻辑
//
// 生活类比：NVIDIA 是"优先客户"，只要它有任何痕迹（nvcuda.dll），
// 就不会再去 AMD/Intel/Vulkan 展厅看车。这个测试验证 DetectHardware
// 中的优先级判断逻辑（通过模拟各厂商 DLL 存在情况验证 detect 函数不互相覆盖）。
func TestDetectPriority(t *testing.T) {
	// 子测试 1：AMD 检测成功后，Vulkan 不应覆盖
	t.Run("AMD优先于Vulkan", func(t *testing.T) {
		origAMD := amdDriverDLLs
		origVulkan := vulkanDLLPath
		amdDriverDLLs = []string{"mock-amd.dll"}
		vulkanDLLPath = "mock-vulkan.dll"
		defer func() {
			amdDriverDLLs = origAMD
			vulkanDLLPath = origVulkan
		}()

		// AMD DLL 和 Vulkan DLL 都"存在"
		restore := mockGPUDeps(
			func(p string) bool { return p == "mock-amd.dll" || p == "mock-vulkan.dll" },
			func(string) (string, error) { return "", errNotFound },
			func(string) (string, int64) { return "AMD GPU", 8192 },
			func() (int64, bool) { return 0, false },
		)
		defer restore()

		hw := &HardwareInfo{}
		// 模拟 DetectHardware 中的优先级调用顺序
		if !hw.HasGPU {
			detectAMDGPU(hw)
		}
		if !hw.HasGPU {
			detectIntelGPU(hw)
		}
		if !hw.HasGPU {
			detectVulkanDevice(hw)
		}

		if hw.GPUVendor != "amd" {
			t.Errorf("期望最终 vendor=amd，实际 %q", hw.GPUVendor)
		}
		if !hw.HasAMDGPU {
			t.Error("期望 HasAMDGPU=true")
		}
	})

	// 子测试 2：Intel 检测成功后，Vulkan 不应覆盖
	t.Run("Intel优先于Vulkan", func(t *testing.T) {
		origIntel := intelDriverDLLs
		origVulkan := vulkanDLLPath
		intelDriverDLLs = []string{"mock-intel.dll"}
		vulkanDLLPath = "mock-vulkan.dll"
		defer func() {
			intelDriverDLLs = origIntel
			vulkanDLLPath = origVulkan
		}()

		restore := mockGPUDeps(
			func(p string) bool { return p == "mock-intel.dll" || p == "mock-vulkan.dll" },
			func(string) (string, error) { return "", errNotFound },
			func(string) (string, int64) { return "Intel GPU", 2048 },
			func() (int64, bool) { return 0, false },
		)
		defer restore()

		hw := &HardwareInfo{}
		if !hw.HasGPU {
			detectAMDGPU(hw) // AMD 不存在，跳过
		}
		if !hw.HasGPU {
			detectIntelGPU(hw) // Intel 存在，标记
		}
		if !hw.HasGPU {
			detectVulkanDevice(hw) // 已有 GPU，跳过
		}

		if hw.GPUVendor != "intel" {
			t.Errorf("期望最终 vendor=intel，实际 %q", hw.GPUVendor)
		}
		if !hw.HasIntelGPU {
			t.Error("期望 HasIntelGPU=true")
		}
	})

	// 子测试 3：仅 Vulkan 存在时才用 Vulkan 兜底
	t.Run("仅Vulkan存在时使用兜底", func(t *testing.T) {
		origAMD := amdDriverDLLs
		origIntel := intelDriverDLLs
		origVulkan := vulkanDLLPath
		amdDriverDLLs = []string{"mock-amd.dll"}
		intelDriverDLLs = []string{"mock-intel.dll"}
		vulkanDLLPath = "mock-vulkan.dll"
		defer func() {
			amdDriverDLLs = origAMD
			intelDriverDLLs = origIntel
			vulkanDLLPath = origVulkan
		}()

		// 只有 vulkan DLL 存在
		restore := mockGPUDeps(
			func(p string) bool { return p == "mock-vulkan.dll" },
			func(string) (string, error) { return "", errNotFound },
			func(string) (string, int64) { return "", 0 },
			func() (int64, bool) { return 0, false },
		)
		defer restore()

		hw := &HardwareInfo{}
		if !hw.HasGPU {
			detectAMDGPU(hw) // AMD DLL 不存在，跳过
		}
		if !hw.HasGPU {
			detectIntelGPU(hw) // Intel DLL 不存在，跳过
		}
		if !hw.HasGPU {
			detectVulkanDevice(hw) // Vulkan 兜底触发
		}

		if hw.GPUVendor != "vulkan" {
			t.Errorf("期望最终 vendor=vulkan，实际 %q", hw.GPUVendor)
		}
		if !hw.HasGPU {
			t.Error("期望 HasGPU=true（Vulkan 兜底）")
		}
	})
}
