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
)

// waitForVRAMRelease blocks until nvidia-smi reports no llama-server GPU usage.
// Times out after vramCheckTimeout seconds.
func (s *Server) WaitForVRAMRelease() {
	log.Info().Msg("[VRAM] waiting for VRAM to be released...")
	deadline := time.Now().Add(vramCheckTimeout * time.Second)
	for time.Now().Before(deadline) {
		free, err := checkVRAMFree()
		if err != nil {
			log.Error().Err(err).Msg("[VRAM] nvidia-smi not available (no NVIDIA GPU?), skip waiting")
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
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		if strings.Contains(line, "llama-server") {
			return false, nil
		}
	}
	return true, nil
}

// EstimateModelVRAM 估算模型+mmproj 的 VRAM 需求（字节）
// 估算方式：GGUF 文件大小 × 1.15（加载开销）+ KV cache 预估
func EstimateModelVRAM(modelPath, mmprojPath string) uint64 {
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

	// KV cache 预估：约 512MB（8K context, Q4 模型的典型值）
	total += 512 * 1024 * 1024

	return total
}

// GetGPUVRAMBytes 获取 GPU 总 VRAM（字节），通过 nvidia-smi 查询
func GetGPUVRAMBytes() (uint64, error) {
	cmd := exec.Command("nvidia-smi", "--query-gpu=memory.total", "--format=csv,noheader,nounits")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("nvidia-smi not available: %w", err)
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
