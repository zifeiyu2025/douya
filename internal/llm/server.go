// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"github.com/rs/zerolog/log"
)

const vramCheckInterval = 500 * time.Millisecond
const vramCheckTimeout = 15

type ServerConfig struct {
	ModelsDir        string
	MmprojAuto       bool
	MmprojOffload    bool
	ServerPath       string
	Port             int
	GPULayers        string
	Threads          int
	FlashAttn        bool
	CacheTypeK       string
	CacheTypeV       string
	Mlock            bool
	Repack           bool
	OpOffload        bool
	KVUnified        bool
	CacheIdleSlots   bool
	CacheRAM         int
	ImageMinTokens   int
	ImageMaxTokens   int
	FitTarget        int
	FitCtx           int
	Reasoning        string
	ReasoningBudget  int
	ReasoningFormat  string
	APIBase          string
	AppDir           string
	ModelsPreset     string
	ModelsMax        int
	SleepIdleSeconds int
}

type Server struct {
	cmd        *exec.Cmd
	config     *ServerConfig
	status     ServerStatus
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	job        *JobObject
	stderrBuf  *RingBuffer
}

func NewServer(cfg *ServerConfig) *Server {
	return &Server{
		config: cfg,
		status: ServerStatus{Running: false},
	}
}

func (s *Server) Start() error {
	s.mu.Lock()

	// ensure old model is fully stopped and VRAM released before starting new one
	if s.status.Running && s.isAlive() {
		log.Info().Msg("stopping existing model server before starting new one...")
		// unlock before stopInternal to avoid deadlock (stopInternal also locks s.mu)
		s.mu.Unlock()
		if err := s.stopInternal(); err != nil {
			log.Error().Err(err).Msg("stop existing server before restart")
		}
		// wait for VRAM to be fully released before loading new model
		s.WaitForVRAMRelease()
		log.Info().Msg("old model VRAM released, now starting new model...")
		s.mu.Lock()  // re-acquire lock
	}

	args := []string{
		"--models-dir", s.config.ModelsDir,
		"--port", fmt.Sprintf("%d", s.config.Port),
		"--host", "127.0.0.1",
		"--jinja",
		"--fit", "on",
	}

	if s.config.ModelsPreset != "" {
		args = append(args, "--models-preset", s.config.ModelsPreset)
	}
	if s.config.ModelsMax > 0 {
		args = append(args, "--models-max", fmt.Sprintf("%d", s.config.ModelsMax))
	}
	if s.config.SleepIdleSeconds > 0 {
		args = append(args, "--sleep-idle-seconds", fmt.Sprintf("%d", s.config.SleepIdleSeconds))
	}
	if s.config.GPULayers != "" {
		args = append(args, "--gpu-layers", s.config.GPULayers)
	}
	if s.config.FlashAttn {
		args = append(args, "--flash-attn", "on")
	}
	if s.config.CacheTypeK != "" {
		args = append(args, "--cache-type-k", s.config.CacheTypeK)
	}
	if s.config.CacheTypeV != "" {
		args = append(args, "--cache-type-v", s.config.CacheTypeV)
	}
	if s.config.Mlock {
		args = append(args, "--mlock")
	}
	if s.config.Threads > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", s.config.Threads))
	}
	if s.config.MmprojAuto {
		args = append(args, "--mmproj-auto")
	}
	if s.config.MmprojOffload {
		args = append(args, "--mmproj-offload")
	}
	if s.config.Reasoning != "" {
		args = append(args, "--reasoning", s.config.Reasoning)
	}
	if s.config.ReasoningBudget > 0 {
		args = append(args, "--reasoning-budget", fmt.Sprintf("%d", s.config.ReasoningBudget))
	}
	if s.config.ReasoningFormat != "" {
		args = append(args, "--reasoning-format", s.config.ReasoningFormat)
	}
	if s.config.Repack {
		args = append(args, "--repack")
	}
	if s.config.OpOffload {
		args = append(args, "--op-offload")
	}
	if s.config.KVUnified {
		args = append(args, "--kv-unified")
	}
	if s.config.CacheIdleSlots {
		args = append(args, "--cache-idle-slots")
	}
	if s.config.CacheRAM > 0 {
		args = append(args, "--cache-ram", fmt.Sprintf("%d", s.config.CacheRAM))
	}
	if s.config.ImageMinTokens > 0 {
		args = append(args, "--image-min-tokens", fmt.Sprintf("%d", s.config.ImageMinTokens))
	}
	if s.config.ImageMaxTokens > 0 {
		args = append(args, "--image-max-tokens", fmt.Sprintf("%d", s.config.ImageMaxTokens))
	}
	if s.config.FitTarget > 0 {
		args = append(args, "--fit-target", fmt.Sprintf("%d", s.config.FitTarget))
	}
	if s.config.FitCtx > 0 {
		args = append(args, "--fit-ctx", fmt.Sprintf("%d", s.config.FitCtx))
	}

	s.cmd = exec.Command(s.config.ServerPath, args...)
	var runtimeDir string
	if s.config.AppDir != "" {
		runtimeDir = filepath.Join(s.config.AppDir, "runtime")
	} else {
		runtimeDir = filepath.Join(filepath.Dir(filepath.Dir(s.config.ServerPath)), "runtime")
	}
	s.cmd.Dir = runtimeDir

	s.stderrBuf = NewRingBuffer(20)
	s.cmd.Stdout = s.stderrBuf.TeeWriter(os.Stderr)
	s.cmd.Stderr = s.stderrBuf.TeeWriter(os.Stderr)

	currentPath := os.Getenv("PATH")
	newPath := runtimeDir
	if currentPath != "" {
		newPath = runtimeDir + ";" + currentPath
	}
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "PATH=") {
			filtered = append(filtered, e)
		}
	}
	filtered = append(filtered, "PATH="+newPath)
	s.cmd.Env = filtered

	s.cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}

	if err := s.cmd.Start(); err != nil {
		s.status = ServerStatus{Running: false, Error: fmt.Sprintf("failed to start server: %v", err)}
		s.mu.Unlock()
		return fmt.Errorf("failed to start server: %w", err)
	}

	if s.job == nil {
		job, err := CreateJobObject()
		if err != nil {
			log.Error().Err(err).Msg("create job object failed (child process not bound)")
		} else {
			s.job = job
		}
	}

	if s.job != nil {
		if err := s.job.AssignProcess(s.cmd.Process.Pid); err != nil {
			log.Error().Err(err).Msg("assign process to job object failed (child process not bound)")
		} else {
			log.Info().Int("pid", s.cmd.Process.Pid).Msg("llama-server bound to job object (will auto-kill on parent exit)")
		}
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.status = ServerStatus{Running: true}

	go func() {
		err := s.cmd.Wait()
		s.mu.Lock()
		s.status = ServerStatus{Running: false}
		if err != nil && s.ctx.Err() == nil {
			errMsg := fmt.Sprintf("server exited with error: %v", err)
			if s.stderrBuf != nil {
				if tail := s.stderrBuf.String(); tail != "" {
					errMsg += "\n" + tail
				}
			}
			s.status.Error = errMsg
		}
		s.mu.Unlock()
	}()

	s.mu.Unlock()
	return nil
}

func (s *Server) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", s.config.Port)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		if !s.IsRunning() {
			errMsg := "server process exited while waiting for ready"
			s.mu.RLock()
			if s.stderrBuf != nil {
				if tail := s.stderrBuf.String(); tail != "" {
					errMsg += "\n" + tail
				}
			}
			s.mu.RUnlock()
			return fmt.Errorf("%s", errMsg)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("server did not become ready within %v", timeout)
}

func (s *Server) GracefulStop(timeout time.Duration) error {
	s.mu.RLock()
	running := s.status.Running
	apiBase := s.config.APIBase
	s.mu.RUnlock()

	if !running {
		return nil
	}

	shutdownURL := apiBase + "/shutdown"
	client := &http.Client{Timeout: 3 * time.Second}

	resp, err := client.Post(shutdownURL, "application/json", nil)
	if err != nil {
		log.Error().Err(err).Msg("graceful shutdown request failed (will force stop)")
	} else {
		resp.Body.Close()
		log.Info().Msg("graceful shutdown request sent, waiting for server to exit...")
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		if !s.IsRunning() {
			s.SetStatus(false, "")
			if s.cancel != nil {
				s.cancel()
			}
			log.Info().Msg("server exited gracefully, VRAM released")
			return nil
		}
		<-ticker.C
	}

	log.Warn().Dur("timeout", timeout).Msg("server did not exit within timeout, forcing stop")
	return s.Stop()
}

func (s *Server) stopInternal() error {
	s.mu.Lock()
	if s.cmd == nil || s.cmd.Process == nil {
		s.mu.Unlock()
		return nil
	}
	apiBase := s.config.APIBase
	pid := s.cmd.Process.Pid
	s.mu.Unlock()

	// Try graceful shutdown first (like Ollama does)
	gracefulTimeout := 5 * time.Second
	deadline := time.Now().Add(gracefulTimeout)
	shutdownURL := apiBase + "/shutdown"
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Post(shutdownURL, "application/json", nil)
	if err == nil {
		resp.Body.Close()
		log.Info().Int("pid", pid).Str("graceful_timeout", gracefulTimeout.String()).Msg("[server] graceful shutdown request sent, waiting...")

		// Wait for process to exit gracefully
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for time.Now().Before(deadline) {
			if !s.isAlive() {
				s.mu.Lock()
				s.status = ServerStatus{Running: false}
				if s.cancel != nil {
					s.cancel()
				}
				s.mu.Unlock()
				log.Info().Int("pid", pid).Msg("[server] exited gracefully")
				return nil
			}
			<-ticker.C
		}
		log.Warn().Int("pid", pid).Msg("[server] graceful shutdown timed out, falling back to taskkill")
	} else {
		log.Error().Err(err).Msg("[server] graceful shutdown request failed (server may not be running)")
	}

	// Fallback: force kill process tree
	terminateCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T")
	terminateCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	_ = terminateCmd.Run()

	cmd := s.cmd
	waitDone := make(chan struct{})
	go func() {
		cmd.Wait()
		close(waitDone)
	}()

	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()

	select {
	case <-waitDone:
		s.mu.Lock()
		s.status = ServerStatus{Running: false}
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		return nil
	case <-timer.C:
		killCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/F", "/T")
		killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		_ = killCmd.Run()
		<-waitDone
		s.mu.Lock()
		s.status = ServerStatus{Running: false}
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		return fmt.Errorf("server did not terminate gracefully, force killed")
	}
}

func (s *Server) Stop() error {
	return s.stopInternal()
}

func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status.Running && s.isAlive()
}

func (s *Server) Status() ServerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Running && !s.isAlive() {
		s.status = ServerStatus{Running: false}
	}
	return s.status
}

func (s *Server) Watch(ctx context.Context, onStatusChange func(ServerStatus)) {
	s.WatchWithCallback(ctx, onStatusChange, nil)
}

func (s *Server) WatchWithCallback(ctx context.Context, onStatusChange func(ServerStatus), onRestartSuccess func()) {
	restartCount := 0
	currentBackoff := 2 * time.Second
	const maxBackoff = 60 * time.Second
	const maxRestartAttempts = 10

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !s.IsRunning() {
			if restartCount >= maxRestartAttempts {
				s.SetStatus(false, fmt.Sprintf("server crashed repeatedly (%d times), waiting %v before next attempt", restartCount, maxBackoff))
				if onStatusChange != nil {
					onStatusChange(s.Status())
				}

				select {
				case <-ctx.Done():
					return
				case <-time.After(maxBackoff):
				}

				restartCount = 0
				currentBackoff = 2 * time.Second
				continue
			}

			backoff := currentBackoff
			restartCount++
			currentBackoff = currentBackoff * 2
			if currentBackoff > maxBackoff {
				currentBackoff = maxBackoff
			}

			s.SetStatus(false, fmt.Sprintf("server crashed, restarting in %v (attempt %d/%d)", backoff, restartCount, maxRestartAttempts))
			if onStatusChange != nil {
				onStatusChange(s.Status())
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			if err := s.Start(); err != nil {
				s.SetStatus(false, fmt.Sprintf("restart failed: %v", err))
				if onStatusChange != nil {
					onStatusChange(s.Status())
				}
				continue
			}

			if err := s.WaitForReady(60 * time.Second); err != nil {
				s.SetStatus(false, fmt.Sprintf("server not ready after restart: %v", err))
				if onStatusChange != nil {
					onStatusChange(s.Status())
				}
				continue
			}

			s.SetStatus(true, "")
			restartCount = 0
			currentBackoff = 2 * time.Second
			if onRestartSuccess != nil {
				onRestartSuccess()
			}
			if onStatusChange != nil {
				onStatusChange(s.Status())
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func (s *Server) isAlive() bool {
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	return s.cmd.ProcessState == nil || !s.cmd.ProcessState.Exited()
}

func (s *Server) SetStatus(running bool, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = ServerStatus{Running: running, Error: errMsg}
}

func (s *Server) Ctx() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ctx
}

func (s *Server) CloseJob() {
	if s.job != nil {
		s.job.Close()
	}
}

func (s *Server) LastOutput() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.stderrBuf == nil {
		return ""
	}
	return s.stderrBuf.String()
}
