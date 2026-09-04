// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"douya/internal/version"
)

// GetDiagnostics 生成一份「一键诊断信息」文本，供用户反馈问题时直接复制粘贴。
//
// 业界成熟方案参考：VS Code 的「复制诊断信息」、Chrome 的版本页、微信的「帮助与反馈」——
// 让用户在遇到问题时，不必手动逐项查找版本号 / 后端类型 / 硬件信息 / 日志位置，
// 一键复制完整环境快照，既减少用户的来回操作，也让开发者拿到可复现问题所需的全部上下文。
//
// 输出为纯文本多行键值对（等宽对齐），方便直接粘贴到群聊 / Issue / 邮件。
//
// 脱敏原则（重要，与安全审查口径一致）：
//   - 不输出任何 API Key / 搜索 Key / MCP 凭据等敏感值，只输出「是否已启用」
//   - 模型路径只显示文件名（完整路径可能包含用户名等个人信息）
//   - 不输出 MCP 服务器配置内容
func (a *App) GetDiagnostics() string {
	var b strings.Builder

	h := a.Health()

	// ===== 基础信息 =====
	fmt.Fprintln(&b, "=== 豆芽诊断信息 ===")
	fmt.Fprintf(&b, "应用版本   : %s\n", version.Version)
	fmt.Fprintf(&b, "Go 版本    : %s\n", runtime.Version())
	fmt.Fprintf(&b, "运行平台   : %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "运行时长   : %s\n", formatDiagnosticUptime(h.UptimeSeconds))
	fmt.Fprintf(&b, "健康状态   : %s\n", h.Status)
	fmt.Fprintf(&b, "数据目录   : %s\n", appDir())
	fmt.Fprintf(&b, "日志目录   : %s\n", filepath.Join(appDir(), "data", "logs"))
	fmt.Fprintln(&b)

	// ===== 服务状态 =====
	fmt.Fprintln(&b, "--- 服务状态 ---")
	stateText := "未运行"
	switch {
	case h.Components.LLM.ModelReady:
		stateText = "模型已就绪"
	case h.Components.LLM.Switching:
		stateText = "切换中 → " + h.Components.LLM.SwitchingTo
	case h.Components.LLM.PermanentFailure:
		stateText = "启动失败（不再重试）"
	case h.Components.LLM.LoadFailed:
		stateText = "模型加载失败"
	case h.Components.LLM.Running:
		stateText = "服务运行中，模型加载中"
	}
	fmt.Fprintf(&b, "服务状态   : %s\n", stateText)
	fmt.Fprintf(&b, "后端类型   : %s\n", backendDisplayName(a.resolvedBackendString()))
	if cfg := a.getConfig(); cfg != nil {
		fmt.Fprintf(&b, "端口       : %d\n", cfg.Port)
		fmt.Fprintf(&b, "上下文大小 : %d\n", cfg.ContextSize)
	}
	if name := a.currentModel(); name != "" {
		fmt.Fprintf(&b, "当前模型   : %s\n", filepath.Base(name))
	}
	if h.Components.LLM.LastError != "" {
		fmt.Fprintf(&b, "最近错误   : %s\n", truncateDiagnostic(h.Components.LLM.LastError, 200))
	}
	fmt.Fprintln(&b)

	// ===== 硬件 =====
	fmt.Fprintln(&b, "--- 硬件 ---")
	hw := h.Components.Hardware
	fmt.Fprintf(&b, "CPU 核心   : %d\n", hw.CPUCores)
	if hw.HasGPU {
		fmt.Fprintf(&b, "GPU        : %s\n", hw.GPUName)
		fmt.Fprintf(&b, "显存       : %d MB\n", hw.GPUVRAMMB)
	} else {
		fmt.Fprintln(&b, "GPU        : 未检测到独立显卡（CPU 推理）")
	}
	fmt.Fprintf(&b, "CUDA 后端  : %v\n", hw.HasCUDABackend)
	fmt.Fprintln(&b)

	// ===== 关键配置（脱敏摘要） =====
	fmt.Fprintln(&b, "--- 关键配置（已脱敏）---")
	if cfg := a.getConfig(); cfg != nil {
		fmt.Fprintf(&b, "搜索模式   : %s\n", cfg.SearchMode)
		fmt.Fprintf(&b, "RAG 知识库 : %s\n", map[bool]string{true: "开启", false: "关闭"}[cfg.RAGEnabled])
		exposeText := "仅本机 (127.0.0.1)"
		if cfg.ExposeServer {
			exposeText = "局域网 (0.0.0.0)"
		}
		keyText := "未启用"
		if cfg.ServerAPIKeyEnabled {
			keyText = "已启用"
		}
		fmt.Fprintf(&b, "API 服务   : %s，API Key %s\n", exposeText, keyText)
		fmt.Fprintf(&b, "TTS 朗读   : %s\n", map[bool]string{true: "开启", false: "关闭"}[cfg.TtsEnabled])
		fmt.Fprintf(&b, "离线模式   : %s\n", map[bool]string{true: "开启", false: "关闭"}[cfg.Offline])
		fmt.Fprintf(&b, "自定义背景 : %s\n", map[bool]string{true: "已设置", false: "未设置"}[cfg.ChatBackground != ""])
	}
	fmt.Fprintln(&b)

	// ===== 运行环境 =====
	fmt.Fprintln(&b, "--- 运行环境 ---")
	fmt.Fprintf(&b, "goroutine  : %d\n", h.Runtime.Goroutines)
	fmt.Fprintf(&b, "内存占用   : %s / %s\n",
		formatBytesDiagnostic(h.Runtime.MemAllocBytes), formatBytesDiagnostic(h.Runtime.MemSysBytes))
	fmt.Fprintf(&b, "数据库     : %s\n", map[bool]string{true: "正常", false: "异常: " + h.Components.Database.Error}[h.Components.Database.Available])
	fmt.Fprintf(&b, "RAG 向量库 : %s\n", map[bool]string{true: "已初始化", false: "未初始化"}[h.Components.RAG.VectorStoreInitialized])

	return b.String()
}

// backendDisplayName 将内部后端类型字符串转成用户友好的展示名。
// auto 已被启动流程解析为具体后端（resolvedBackend），此处一般不会出现。
func backendDisplayName(bt string) string {
	if bt == "" {
		return "auto（自动检测）"
	}
	return bt
}

// formatDiagnosticUptime 将秒数格式化为人类可读的时长（如 "12分34秒"）。
func formatDiagnosticUptime(seconds float64) string {
	d := time.Duration(seconds * float64(time.Second))
	if d < time.Minute {
		return fmt.Sprintf("%d秒", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d分%d秒", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%d小时%d分", int(d.Hours()), int(d.Minutes())%60)
}

// formatBytesDiagnostic 将字节数格式化为人类可读的大小（如 "1.2 GB"）。
func formatBytesDiagnostic(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// truncateDiagnostic 截断过长文本（防止日志错误信息撑爆诊断文本）。
func truncateDiagnostic(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
