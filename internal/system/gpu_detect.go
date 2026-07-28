// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/rs/zerolog/log"
)

// amdDriverDLLs 是 AMD 显卡驱动常见 DLL 文件列表（System32 固定路径）。
// 生活类比：就像通过查看车库门口停的是什么品牌的车来判断车主有什么车，
// 通过检查系统目录里有没有特定厂商的驱动 DLL，就能判断装了什么显卡。
//
// 说明：
//   - amdgx.dll   ：AMD 较新的图形驱动组件
//   - aticfx64.dll：AMD Radeon 旧版驱动组件
//   - atio6axx.dll：AMD OpenGL 驱动（装了 A 卡驱动通常存在）
var amdDriverDLLs = []string{
	`C:\Windows\System32\amdgx.dll`,
	`C:\Windows\System32\aticfx64.dll`,
	`C:\Windows\System32\atio6axx.dll`,
}

// intelDriverDLLs 是 Intel 显卡驱动常见 DLL 文件列表。
// 说明：
//   - igc64.dll        ：Intel Graphics Compiler（图形编译器）
//   - igdgmm.dll       ：Intel 图形内存管理
//   - igdml64.dll      ：Intel 媒体层
//   - igd10iumd64.dll  ：Intel D3D10/D3D11 用户模式驱动
var intelDriverDLLs = []string{
	`C:\Windows\System32\igc64.dll`,
	`C:\Windows\System32\igdgmm.dll`,
	`C:\Windows\System32\igdml64.dll`,
	`C:\Windows\System32\igd10iumd64.dll`,
}

// vulkanDLLPath 是 Vulkan 通用渲染 API 的 DLL 路径。
// Vulkan 是跨厂商 API，几乎所有现代显卡驱动都会安装它，
// 因此仅作为"实在不知道是什么显卡"时的最后兜底。
var vulkanDLLPath = `C:\Windows\System32\vulkan-1.dll`

// fileExists 检查文件是否存在。
// 设计为可替换的变量（而非函数），便于测试时注入 mock，模拟 DLL 存在/缺失场景。
// 生活类比：就像派人去车库看一眼车在不在，回来报告
var fileExists = func(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// execLookPath 是 exec.LookPath 的间接引用，便于测试时注入 mock（避免真实查找 PATH）。
var execLookPath = exec.LookPath

// detectAMDGPU 检测 AMD 显卡。
//
// 检测策略：
//  1. 检查 AMD 驱动 DLL（System32 固定路径 + PATH 中的同名 DLL）
//  2. 找到则标记 HasAMDGPU/HasGPU/GPUVendor
//  3. VRAM 优先用 rocm-smi（AMD 专用工具，精度高），回退到 WMI（精度低，4GB 上限）
//  4. GPU 名称通过 WMI 获取
//
// 生活类比：先看车库有没有 AMD 的车（DLL），有的话再查行驶证（rocm-smi）
// 拿排量（VRAM），行驶证找不到就问车管所（WMI）。
func detectAMDGPU(hw *HardwareInfo) {
	// 收集候选路径：System32 固定路径 + PATH 中的同名 DLL
	candidates := append([]string{}, amdDriverDLLs...)
	for _, dll := range amdDriverDLLs {
		if p, err := execLookPath(dll); err == nil {
			candidates = append(candidates, p)
		}
	}

	foundPath := ""
	for _, p := range candidates {
		if fileExists(p) {
			foundPath = p
			break
		}
	}
	if foundPath == "" {
		log.Info().Msg("[system] no AMD GPU driver detected")
		return
	}
	log.Info().Str("path", foundPath).Msg("[system] AMD GPU driver detected")

	hw.HasAMDGPU = true
	hw.HasGPU = true
	hw.GPUVendor = "amd"

	// GPU 名称：通过 WMI 查询（rocm-smi 在 Windows 上通常不可用，且不返回名称）
	gpuName, wmiVRAM := queryWMI("AMD")
	if gpuName != "" {
		hw.GPUName = gpuName
	}

	// VRAM：优先 rocm-smi（精度高），回退 WMI（受 uint32 限制）
	if vram, ok := queryROCmVRAM(); ok && vram > 0 {
		hw.GPUVRAMMB = vram
		log.Info().Int64("vram_mb", hw.GPUVRAMMB).Str("source", "rocm-smi").Msg("[system] AMD VRAM from rocm-smi")
	} else if wmiVRAM > 0 {
		hw.GPUVRAMMB = wmiVRAM
		log.Info().Int64("vram_mb", hw.GPUVRAMMB).Str("source", "wmi").Msg("[system] AMD VRAM from WMI (may be inaccurate for >4GB cards)")
	} else {
		log.Warn().Msg("[system] AMD GPU detected but VRAM unavailable, smart-params will use fallback")
	}

	log.Info().Str("gpu", hw.GPUName).Int64("vram_mb", hw.GPUVRAMMB).Msg("[system] AMD GPU info")
}

// detectIntelGPU 检测 Intel 显卡。
//
// 检测策略与 AMD 类似：
//  1. 检查 Intel 驱动 DLL
//  2. 找到则标记 HasIntelGPU/HasGPU/GPUVendor
//  3. VRAM 和名称通过 WMI 获取（Intel 没有类似 rocm-smi 的通用工具）
//
// 生活类比：Intel 核显就像"自带发动机的整车"（CPU 集成），检测它的驱动 DLL
// 就能知道这台车是不是 Intel 出的。但 Intel 核显显存是共享内存，
// 拿到的"显存"数字参考意义有限。
func detectIntelGPU(hw *HardwareInfo) {
	// 收集候选路径
	candidates := append([]string{}, intelDriverDLLs...)
	for _, dll := range intelDriverDLLs {
		if p, err := execLookPath(dll); err == nil {
			candidates = append(candidates, p)
		}
	}

	foundPath := ""
	for _, p := range candidates {
		if fileExists(p) {
			foundPath = p
			break
		}
	}
	if foundPath == "" {
		log.Info().Msg("[system] no Intel GPU driver detected")
		return
	}
	log.Info().Str("path", foundPath).Msg("[system] Intel GPU driver detected")

	hw.HasIntelGPU = true
	hw.HasGPU = true
	hw.GPUVendor = "intel"

	// Intel 核显无专用 VRAM 查询工具，直接用 WMI
	gpuName, wmiVRAM := queryWMI("Intel")
	if gpuName != "" {
		hw.GPUName = gpuName
	}
	if wmiVRAM > 0 {
		hw.GPUVRAMMB = wmiVRAM
	}

	log.Info().Str("gpu", hw.GPUName).Int64("vram_mb", hw.GPUVRAMMB).Msg("[system] Intel GPU info")
}

// detectVulkanDevice 检测 Vulkan 通用后端。
//
// 仅当 NVIDIA/AMD/Intel 都未检测到时才作为兜底方案启用。
// Vulkan 是跨厂商 API，找到 vulkan-1.dll 只能说明"有某个支持 Vulkan 的 GPU"，
// 但不知道具体厂商和显存，因此 VRAM 设为 0（由 smartparams 处理回退）。
//
// 生活类比：前面三个品牌展厅都没找到车，最后看停车场有没有"任意品牌"的车位
// （vulkan-1.dll），有的话至少说明这里能停车，虽然不知道具体什么车。
func detectVulkanDevice(hw *HardwareInfo) {
	// 兜底保护：如果已经检测到其他 GPU，不应触发 Vulkan 兜底
	if hw.HasGPU {
		return
	}
	if !fileExists(vulkanDLLPath) {
		log.Info().Msg("[system] no Vulkan device detected")
		return
	}
	log.Info().Str("path", vulkanDLLPath).Msg("[system] Vulkan device detected (fallback, vendor unknown)")

	hw.GPUVendor = "vulkan"
	hw.HasGPU = true
	hw.GPUName = "Vulkan Device"
	// VRAM 不可用，设为 0；smartparams 会走 HasCUDABackend 之外的回退逻辑
	hw.GPUVRAMMB = 0
}

// queryWMI 通过 Windows WMI 查询显卡信息。
//
// 参数 nameFilter 是 AdapterCompatibility（驱动厂商）的过滤关键词，
// 例如 "AMD" 或 "Intel"。返回匹配显卡的名称和 VRAM（MB）。
//
// 查询顺序：
//  1. 优先 wmic（旧版 Windows 内置，速度快）
//  2. wmic 不可用或无结果时，回退 PowerShell Get-CimInstance（新版 Windows 推荐）
//
// 注意：wmic 返回的 AdapterRAM 是 uint32 类型，最大只能表示约 4GB（4294967296 字节），
// 对于 8GB/16GB/24GB 等大显存显卡会发生整数回绕，导致返回值严重偏小甚至为 0。
// 因此此值仅作为兜底方案，精确 VRAM 必须依赖厂商专用工具（nvidia-smi/rocm-smi）。
//
// 生活类比：WMI 就像去车管所查行驶证，行驶证上的"排量"字段只填了 4 位数，
// 超过 9999 的就只能显示后 4 位，所以大排量车查出来的数字不准。
//
// 设计为可替换的变量，便于测试时注入 mock，避免真实执行 wmic/powershell。
var queryWMI = func(nameFilter string) (gpuName string, vramMB int64) {
	// 优先尝试 wmic
	cmd := exec.Command("wmic", "path", "win32_VideoController",
		"where", fmt.Sprintf("AdapterCompatibility like '%%%s%%'", nameFilter),
		"get", "Name,AdapterRAM", "/value")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err == nil {
		gpuName, vramMB = parseWMIOutput(string(output))
		if gpuName != "" || vramMB > 0 {
			return gpuName, vramMB
		}
	} else {
		log.Info().Err(err).Str("filter", nameFilter).Msg("[system] wmic unavailable, will try PowerShell")
	}

	// wmic 不可用或无结果，回退到 PowerShell Get-CimInstance
	psCmd := fmt.Sprintf(
		`Get-CimInstance Win32_VideoController -Filter "AdapterCompatibility like '%%%s%%'" | ForEach-Object { "$($_.Name)|$($_.AdapterRAM)" }`,
		nameFilter,
	)
	cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", psCmd)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err = cmd.Output()
	if err != nil {
		log.Warn().Err(err).Str("filter", nameFilter).Msg("[system] PowerShell WMI query failed")
		return "", 0
	}
	return parsePowerShellWMIOutput(string(output))
}

// queryROCmVRAM 尝试通过 rocm-smi 获取 AMD GPU 的 VRAM（MB）。
// rocm-smi 是 AMD ROCm 工具链的一部分，通常只在 Linux 上可用，
// Windows 上较少见，因此作为可选的精度提升手段。
//
// 输出格式示例：
//
//	================================= ROCm System Interface Monitor =================================
//	GPU[0]		: VRAM Total Memory (B) : 17163091968
//	GPU[0]		: VRAM Total Used Memory (B) : 1234567
//
// 设计为可替换的变量，便于测试时注入 mock。
var queryROCmVRAM = func() (vramMB int64, ok bool) {
	path, err := execLookPath("rocm-smi")
	if err != nil {
		return 0, false
	}
	cmd := exec.Command(path, "--showmeminfo", "vram")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		log.Info().Err(err).Msg("[system] rocm-smi query failed")
		return 0, false
	}

	// 解析 "VRAM Total Memory (B) : 17163091968" 这一行
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.Contains(line, "VRAM Total Memory") {
			continue
		}
		// 按冒号分割，取最后一部分作为数值
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		val := strings.TrimSpace(parts[len(parts)-1])
		bytes, parseErr := strconv.ParseInt(val, 10, 64)
		if parseErr != nil || bytes <= 0 {
			continue
		}
		// 字节转 MB
		return bytes / (1024 * 1024), true
	}
	return 0, false
}

// parseWMIOutput 解析 wmic 的 /value 格式输出。
//
// wmic ... get Name,AdapterRAM /value 输出形如：
//
//	AdapterRAM=4294967296
//	Name=AMD Radeon RX 7900 XTX
//
// 字段顺序不固定，可能有多行（多显卡场景），这里取第一个有效值。
// 生活类比：就像从一张填表式的登记证上逐行读取信息，
// 看到"姓名="就记下名字，看到"排量="就记下数字。
func parseWMIOutput(output string) (gpuName string, vramMB int64) {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// 按第一个等号分割
		eqIdx := strings.Index(line, "=")
		if eqIdx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])
		switch key {
		case "Name":
			// 多显卡场景取第一个非空名称
			if gpuName == "" && val != "" {
				gpuName = val
			}
		case "AdapterRAM":
			// wmic AdapterRAM 单位是字节，且是 uint32（最大约 4GB，大显存会溢出）
			if vramMB == 0 && val != "" {
				if n, err := strconv.ParseInt(val, 10, 64); err == nil && n > 0 {
					vramMB = n / (1024 * 1024)
				}
			}
		}
	}
	return gpuName, vramMB
}

// parsePowerShellWMIOutput 解析 PowerShell ForEach-Object 输出的 "Name|AdapterRAM" 格式。
//
// 输出形如（单行，多显卡时多行）：
//
//	AMD Radeon RX 7900 XTX|17163091968
//
// 生活类比：PowerShell 输出像一张"用竖线分隔的表格"，比 wmic 的键值对更紧凑。
func parsePowerShellWMIOutput(output string) (gpuName string, vramMB int64) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", 0
	}
	// 多显卡场景取第一行
	firstLine := strings.Split(output, "\n")[0]
	firstLine = strings.TrimSpace(firstLine)

	parts := strings.SplitN(firstLine, "|", 2)
	gpuName = strings.TrimSpace(parts[0])
	if gpuName == "" {
		return "", 0
	}
	if len(parts) == 2 {
		val := strings.TrimSpace(parts[1])
		if val != "" {
			if n, err := strconv.ParseInt(val, 10, 64); err == nil && n > 0 {
				vramMB = n / (1024 * 1024)
			}
		}
	}
	return gpuName, vramMB
}
