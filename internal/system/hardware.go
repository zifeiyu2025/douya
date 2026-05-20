// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

type HardwareInfo struct {
	CPUCores  int
	GPUVRAMMB int64
	GPUName   string
	HasGPU    bool
}

func DetectHardware() *HardwareInfo {
	hw := &HardwareInfo{
		CPUCores: runtime.NumCPU(),
	}

	detectGPU(hw)

	if hw.HasGPU {
		log.Printf("[system] hardware: CPU cores=%d, GPU=%s, VRAM=%dMB", hw.CPUCores, hw.GPUName, hw.GPUVRAMMB)
	} else {
		log.Printf("[system] hardware: CPU cores=%d, no NVIDIA GPU detected", hw.CPUCores)
	}

	return hw
}

func detectGPU(hw *HardwareInfo) {
	path, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return
	}

	cmd := exec.Command(path, "--query-gpu=memory.total,name", "--format=csv,noheader,nounits")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[system] nvidia-smi query failed: %v", err)
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return
	}

	parts := strings.SplitN(lines[0], ",", 2)
	if len(parts) < 2 {
		return
	}

	vramMB, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil {
		log.Printf("[system] parse VRAM value failed: %v", err)
		return
	}

	hw.GPUVRAMMB = vramMB
	hw.GPUName = strings.TrimSpace(parts[1])
	hw.HasGPU = true
}
