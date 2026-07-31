// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/system"
)

// waitForVRAMRelease blocks until nvidia-smi reports no llama-server GPU usage.
// Times out after vramCheckTimeout seconds.
func (s *Server) WaitForVRAMRelease() {
	log.Info().Msg("[VRAM] waiting for VRAM to be released...")
	deadline := time.Now().Add(vramCheckTimeout * time.Second)
	for time.Now().Before(deadline) {
		free, err := checkVRAMFree()
		if err != nil {
			// 无 NVIDIA GPU 是预期降级场景，用 Warn 而非 Error
			log.Warn().Err(err).Msg("[VRAM] nvidia-smi not available (no NVIDIA GPU?), skip waiting")
			return
		}
		if free {
			log.Info().Msg("[VRAM] released successfully")
			return
		}
		time.Sleep(vramCheckInterval)
	}
	log.Warn().Msg("[VRAM] timeout waiting for VRAM release, proceeding anyway")
}

// checkVRAMFree returns true if no llama-server process is using GPU memory.
func checkVRAMFree() (bool, error) {
	cmd := exec.Command("nvidia-smi", "--query-compute-apps=pid,name,used_memory", "--format=csv,noheader,nounits")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		// nvidia-smi not present or no GPU — treat as VRAM free
		return true, err
	}
	lines := strings.SplitSeq(strings.TrimSpace(string(output)), "\n")
	for line := range lines {
		if strings.Contains(line, "llama-server") {
			return false, nil
		}
	}
	return true, nil
}

// EstimateModelVRAM 估算模型+mmproj 的 VRAM 需求（字节）
// 估算方式：GGUF 文件大小 × 1.15（加载开销）+ KV cache 按上下文长度动态预估
//
// 生活类比：估算一道菜需要多少食材——主料（模型权重）按重量×1.15 算损耗，
// 配料（KV cache）按盘子大小（上下文长度）来算，盘子越大配料越多。
//
// 改进说明：原实现固定加 512MB KV cache，对 8K 以上上下文严重低估，
// 导致大上下文场景 OOM。新实现按 ctxSize 动态估算：
//   - ctxSize <= 0：保守用 8K 默认值（约 512MB）
//   - ctxSize > 0：按 ctxSize 线性缩放（8K ≈ 512MB）
//
// 注意：这是粗略估算，实际 KV cache 大小还取决于模型层数、head 数、量化类型，
// 但 GGUF 文件大小已隐含了层数信息，因此按 ctxSize 线性缩放已足够安全。
func EstimateModelVRAM(modelPath, mmprojPath string, ctxSize int) uint64 {
	var total uint64

	if info, err := os.Stat(modelPath); err == nil {
		// 模型文件大小 × 1.15（包含加载时的内存开销）
		total += uint64(float64(info.Size()) * 1.15)
	}

	if mmprojPath != "" {
		if info, err := os.Stat(mmprojPath); err == nil {
			// mmproj 文件大小 × 1.1
			total += uint64(float64(info.Size()) * 1.1)
		}
	}

	// KV cache 预估：按上下文长度动态计算
	// 基准：8K context ≈ 512MB（Q4 模型典型值）
	// 线性缩放：ctxSize / 8192 × 512MB
	const baseCtxSize = 8192
	const baseKVCacheMB = 512
	effectiveCtx := ctxSize
	if effectiveCtx <= 0 {
		effectiveCtx = baseCtxSize // 默认 8K
	}
	kvCacheMB := int64(effectiveCtx) * baseKVCacheMB / baseCtxSize
	// 保底至少 256MB（极小上下文也要有基本开销）
	if kvCacheMB < 256 {
		kvCacheMB = 256
	}
	total += uint64(kvCacheMB) * 1024 * 1024

	return total
}

// GetGPUVRAMBytes 获取 GPU 总 VRAM（字节），支持多厂商。
//
// 改进说明：原实现只查 nvidia-smi，A 卡/I 卡直接报错。
// 新实现优先复用 system.HardwareInfo 已检测的 GPUVRAMMB（避免重复查询），
// 并在 hw 为 nil 时回退到 nvidia-smi（保持向后兼容）。
//
// 生活类比：原来只认识 NVIDIA 的仪表盘（nvidia-smi），其他车都读不到油量；
// 现在改成"先看车辆登记证（HardwareInfo）上写的油箱容量"，没有登记证时
// 才回退到原方案。这样 A 卡/I 卡也能读到 VRAM 了。
//
// 参数：
//   - hw: 硬件信息（可为 nil，nil 时回退到 nvidia-smi 查询）
func GetGPUVRAMBytes(hw *system.HardwareInfo) (uint64, error) {
	// 优先复用已检测的 HardwareInfo（多厂商支持）
	if hw != nil && hw.HasGPU && hw.GPUVRAMMB > 0 {
		return uint64(hw.GPUVRAMMB) * 1024 * 1024, nil
	}

	// 回退：hw 为 nil 或无 GPU 信息时，尝试 nvidia-smi（保持向后兼容）
	// 生活类比：登记证找不到，才回退到去车上看仪表盘
	cmd := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("nvidia-smi not available and no HardwareInfo provided: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return 0, fmt.Errorf("no GPU found")
	}

	// 取第一块 GPU 的 VRAM（MiB）
	firstLine := strings.TrimSpace(lines[0])
	mib, err := strconv.ParseUint(firstLine, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse VRAM value %q: %w", firstLine, err)
	}

	// MiB → Bytes
	return mib * 1024 * 1024, nil
}

// FormatBytes 将字节数格式化为人类可读的字符串
func FormatBytes(bytes uint64) string {
	const gb = 1024 * 1024 * 1024
	const mb = 1024 * 1024
	if bytes >= gb {
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	}
	if bytes >= mb {
		return fmt.Sprintf("%.0f MB", float64(bytes)/float64(mb))
	}
	return fmt.Sprintf("%d B", bytes)
}
