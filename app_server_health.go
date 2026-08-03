package main

import (
	"context"
	"runtime"
	"time"

	"douya/internal/version"
)

// ===== D2: 健康检查端点 =====
//
// 生活类比：Health() 就像汽车的仪表盘——一眼看到发动机是否启动（llm_server）、
// 油箱是否充足（database）、当前挂的什么挡（current_model）、已经跑了多久（uptime）。
// 前端或外部调试工具可以通过 Wails IPC 调用 App.Health() 获取完整运行时状态快照。
//
// 设计原则：
// 1. 只读快照，不修改任何状态
// 2. 并发安全：所有受 mutex 保护的字段都用 RLock/RUnlock
// 3. 数据库健康检查带 1 秒超时，避免阻塞
// 4. status 字段：ok=全部正常，degraded=部分组件异常，down=核心组件不可用

// HealthStatus 是健康检查端点返回的完整运行时状态
type HealthStatus struct {
	Status        string           `json:"status"`         // "ok" | "degraded" | "down"
	Timestamp     string           `json:"timestamp"`      // RFC3339 格式
	UptimeSeconds float64          `json:"uptime_seconds"` // 应用运行时长（秒）
	AppReady      bool             `json:"app_ready"`      // 应用是否完成启动
	ConfigLoaded  bool             `json:"config_loaded"`  // 配置是否已加载
	Version       HealthVersion    `json:"version"`
	Components    HealthComponents `json:"components"`
	Runtime       HealthRuntime    `json:"runtime"`
}

// HealthVersion 版本信息
type HealthVersion struct {
	App string `json:"app"` // 应用版本号（如 "0.10.7"）
	Go  string `json:"go"`  // Go 编译器版本（如 "go1.23.5"）
}

// HealthComponents 各组件健康状态
type HealthComponents struct {
	LLM      HealthLLM      `json:"llm_server"`
	Database HealthDatabase `json:"database"`
	Chat     HealthChat     `json:"chat_service"`
	RAG      HealthRAG      `json:"rag"`
	Hardware HealthHardware `json:"hardware"`
}

// HealthLLM llama-server 子进程状态
type HealthLLM struct {
	Running          bool   `json:"running"`           // 进程是否在运行
	ModelReady       bool   `json:"model_ready"`       // 模型是否就绪
	PermanentFailure bool   `json:"permanent_failure"` // 是否永久失败（不再自动重启）
	LoadFailed       bool   `json:"load_failed"`       // 模型加载是否彻底失败
	CurrentModel     string `json:"current_model"`     // 当前加载的模型名
	Switching        bool   `json:"switching"`         // 是否正在切换模型
	SwitchingTo      string `json:"switching_to"`      // 切换目标模型名
	Port             int    `json:"port"`              // llama-server 监听端口
	APIBase          string `json:"api_base"`          // API 基础 URL
	LastError        string `json:"last_error"`        // 最后一次错误信息
}

// HealthDatabase 数据库状态
type HealthDatabase struct {
	Available bool   `json:"available"` // 数据库是否可用（Ping 成功）
	Error     string `json:"error"`     // 不可用时的错误信息
}

// HealthChat 聊天服务状态
type HealthChat struct {
	Available  bool `json:"available"`  // 聊天服务是否已初始化
	Generating bool `json:"generating"` // 是否正在生成回复
}

// HealthRAG RAG 向量库状态
type HealthRAG struct {
	Available              bool `json:"available"`                // RAG 是否已初始化
	VectorStoreInitialized bool `json:"vector_store_initialized"` // 向量库是否已就绪
}

// HealthHardware 硬件信息
type HealthHardware struct {
	CPUCores       int    `json:"cpu_cores"`
	HasGPU         bool   `json:"has_gpu"`
	GPUName        string `json:"gpu_name"`
	GPUVRAMMB      int64  `json:"gpu_vram_mb"`
	HasCUDABackend bool   `json:"has_cuda_backend"`
}

// HealthRuntime Go 运行时状态
type HealthRuntime struct {
	Goroutines    int    `json:"goroutines"`      // goroutine 数量
	MemAllocBytes uint64 `json:"mem_alloc_bytes"` // 已分配内存（字节）
	MemSysBytes   uint64 `json:"mem_sys_bytes"`   // 系统分配内存（字节）
}

// Health 返回应用完整运行时状态快照，用于健康检查和调试。
//
// 生活类比：就像去医院体检，医生一次性给你出一份全身检查报告——
// 心跳（llm_server 是否运行）、血压（database 是否正常）、体温（model 是否就绪）等。
//
// 前端可通过 Wails IPC 调用：await window.go.main.App.Health()
// 返回的 status 字段：
//   - "ok"：所有核心组件正常
//   - "degraded"：部分组件异常，但应用仍可运行
//   - "down"：核心组件（数据库或配置）不可用
func (a *App) Health() HealthStatus {
	// 1. 收集 LLM 状态
	llmStatus := HealthLLM{
		Port:    8080,
		APIBase: "http://127.0.0.1:8080",
	}
	if cfg := a.getConfig(); cfg != nil {
		llmStatus.Port = cfg.Port
		llmStatus.APIBase = cfg.APIBase
	}
	a.serverMu.RLock()
	srv := a.server
	a.serverMu.RUnlock()
	if srv != nil {
		llmStatus.Running = srv.IsRunning()
		llmStatus.PermanentFailure = srv.IsPermanentFailure()
	}
	llmStatus.ModelReady = a.serverReady.Load()
	llmStatus.LoadFailed = a.serverLoadFailed.Load()
	session := a.modelSessionSnapshot()
	llmStatus.CurrentModel = session.CurrentModel
	llmStatus.Switching = session.Switching
	llmStatus.SwitchingTo = session.SwitchingTo
	a.lastServerErrMu.RLock()
	llmStatus.LastError = a.lastServerError
	a.lastServerErrMu.RUnlock()

	// 2. 收集数据库状态（带 1 秒超时，避免阻塞）
	dbStatus := HealthDatabase{Available: a.db != nil}
	if a.db != nil {
		pingCtx, pingCancel := context.WithTimeout(context.Background(), time.Second)
		defer pingCancel()
		if err := a.db.PingContext(pingCtx); err != nil {
			dbStatus.Available = false
			dbStatus.Error = err.Error()
		}
	} else {
		dbStatus.Error = "database not initialized"
	}

	// 3. 收集聊天服务状态
	chatStatus := HealthChat{Available: a.service != nil}
	if a.service != nil {
		chatStatus.Generating = a.service.IsGenerating()
	}

	// 4. 收集 RAG 状态
	ragStatus := HealthRAG{
		Available:              a.ragVS != nil,
		VectorStoreInitialized: a.ragVS != nil,
	}

	// 5. 收集硬件信息
	hwStatus := HealthHardware{}
	if a.hwInfo != nil {
		hwStatus.CPUCores = a.hwInfo.CPUCores
		hwStatus.HasGPU = a.hwInfo.HasGPU
		hwStatus.GPUName = a.hwInfo.GPUName
		hwStatus.GPUVRAMMB = a.hwInfo.GPUVRAMMB
		hwStatus.HasCUDABackend = a.hwInfo.HasCUDABackend
	}

	// 6. 收集 Go 运行时状态
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	rtStatus := HealthRuntime{
		Goroutines:    runtime.NumGoroutine(),
		MemAllocBytes: memStats.Alloc,
		MemSysBytes:   memStats.Sys,
	}

	// 7. 计算整体状态
	// 核心组件：数据库 + 配置。任一不可用 → "down"
	// 重要组件：LLM 服务。异常但应用仍可运行 → "degraded"
	configLoaded := a.getConfig() != nil
	status := "ok"
	if !dbStatus.Available || !configLoaded {
		status = "down"
	} else if !llmStatus.Running || llmStatus.LoadFailed || llmStatus.PermanentFailure {
		status = "degraded"
	}

	return HealthStatus{
		Status:        status,
		Timestamp:     time.Now().Format(time.RFC3339),
		UptimeSeconds: time.Since(appStartTime).Seconds(),
		AppReady:      a.ready.Load(),
		ConfigLoaded:  configLoaded,
		Version: HealthVersion{
			App: version.Version,
			Go:  runtime.Version(),
		},
		Components: HealthComponents{
			LLM:      llmStatus,
			Database: dbStatus,
			Chat:     chatStatus,
			RAG:      ragStatus,
			Hardware: hwStatus,
		},
		Runtime: rtStatus,
	}
}
