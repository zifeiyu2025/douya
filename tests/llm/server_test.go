// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"douya/internal/llm"
)

func TestServer_GracefulShutdown_SendsShutdownRequest(t *testing.T) {
	var shutdownCalled atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST /shutdown, got %s", r.Method)
		}
		shutdownCalled.Add(1)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := &llm.ServerConfig{
		ModelsDir:  "/path/to/models",
		ServerPath: "/path/to/llama-server",
		Port:       8080,
		GPULayers:  "99",
		APIBase:    ts.URL,
	}

	s := llm.NewServer(cfg)
	s.SetStatus(true, "")

	s.GracefulStop(5 * time.Second)

	if shutdownCalled.Load() != 1 {
		t.Errorf("expected /shutdown to be called once, got %d", shutdownCalled.Load())
	}
}

func TestServer_GracefulStop_NoPanicWhenNotRunning(t *testing.T) {
	cfg := &llm.ServerConfig{
		ModelsDir:  "/path/to/models",
		ServerPath: "/path/to/llama-server",
		Port:       8080,
		GPULayers:  "99",
		APIBase:    "http://127.0.0.1:8080",
	}

	s := llm.NewServer(cfg)

	err := s.GracefulStop(5 * time.Second)
	if err != nil {
		t.Errorf("GracefulStop on non-running server should not error, got: %v", err)
	}
}

func TestServer_GracefulStop_SetsStatusToStopped(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := &llm.ServerConfig{
		ModelsDir:  "/path/to/models",
		ServerPath: "/path/to/llama-server",
		Port:       8080,
		GPULayers:  "99",
		APIBase:    ts.URL,
	}

	s := llm.NewServer(cfg)
	s.SetStatus(true, "")

	s.GracefulStop(5 * time.Second)

	status := s.Status()
	if status.Running {
		t.Error("server should not be running after GracefulStop")
	}
}

func TestServer_GracefulStop_HandlesShutdownEndpointUnavailable(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := &llm.ServerConfig{
		ModelsDir:  "/path/to/models",
		ServerPath: "/path/to/llama-server",
		Port:       8080,
		GPULayers:  "99",
		APIBase:    ts.URL,
	}

	s := llm.NewServer(cfg)
	s.SetStatus(true, "")

	s.GracefulStop(2 * time.Second)

	status := s.Status()
	if status.Running {
		t.Error("server should not be running after GracefulStop even when /shutdown is unavailable")
	}
}

func TestServer_GracefulStop_CancelContextOnStop(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/shutdown", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	ts := httptest.NewServer(mux)
	defer ts.Close()

	cfg := &llm.ServerConfig{
		ModelsDir:  "/path/to/models",
		ServerPath: "/path/to/llama-server",
		Port:       8080,
		GPULayers:  "99",
		APIBase:    ts.URL,
	}

	s := llm.NewServer(cfg)
	s.SetStatus(true, "")

	s.GracefulStop(5 * time.Second)

	ctx := s.Ctx()

	if ctx != nil && ctx.Err() == nil {
		t.Error("server context should be cancelled after GracefulStop")
	}
}

func TestServer_Stop_NilProcess(t *testing.T) {
	cfg := &llm.ServerConfig{
		ModelsDir:  "/path/to/models",
		ServerPath: "/path/to/llama-server",
		Port:       8080,
		GPULayers:  "99",
		APIBase:    "http://127.0.0.1:8080",
	}

	s := llm.NewServer(cfg)

	err := s.Stop()
	if err != nil {
		t.Errorf("Stop on server with nil process should not error, got: %v", err)
	}
}
