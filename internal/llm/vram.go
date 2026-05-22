// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"os/exec"
	"strings"
	"syscall"
	"time"
	"github.com/rs/zerolog/log"
)

// waitForVRAMRelease blocks until nvidia-smi reports no llama-server GPU usage.
// Times out after vramCheckTimeout seconds.
func (s *Server) waitForVRAMRelease() {
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
