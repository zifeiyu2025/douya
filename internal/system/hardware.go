// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"os/exec"

	"github.com/rs/zerolog/log"
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
		log.Info().Int("cpu_cores", hw.CPUCores).Str("gpu", hw.GPUName).Int64("vram_mb", hw.GPUVRAMMB).Msg("[system] hardware detected")
	} else {
		log.Info().Int("cpu_cores", hw.CPUCores).Msg("[system] hardware: no NVIDIA GPU detected")
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
		log.Error().Err(err).Msg("[system] nvidia-smi query failed")
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
		log.Error().Err(err).Msg("[system] parse VRAM value failed")
		return
	}

	hw.GPUVRAMMB = vramMB
	hw.GPUName = strings.TrimSpace(parts[1])
	hw.HasGPU = true
}
