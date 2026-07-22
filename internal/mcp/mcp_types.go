// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

// Package mcp 实现豆芽原生的 MCP (Model Context Protocol) 客户端。
//
// 生活类比：把豆芽想象成一家多功能餐厅，MCP 就是「外卖对接系统」。
// 每个 MCP 服务器是一家外卖平台，豆芽通过 stdio（标准输入输出）和它们对接，
// 把它们的「菜品」（工具）自动注册到自己的菜单里，顾客（LLM）点哪道菜，
// 豆芽就知道去哪家平台下单。
//
// 本包不依赖外部 MCP SDK，自行实现 JSON-RPC 2.0 over stdio 通信，
// 只支持 stdio transport（最通用、最稳定），覆盖 Initialize/ListTools/CallTool 三个核心方法。
package mcp

// MCPServerConfig 单个 MCP 服务器的配置。
// 生活类比：像一张外卖平台对接卡——平台名(name)、对接程序(command)、
// 启动参数(args)、所需凭证(env)、是否启用(enabled)。
type MCPServerConfig struct {
	Name    string            `json:"name"`    // 服务器唯一标识（用于日志和工具路由）
	Command string            `json:"command"` // 可执行文件路径（如 "npx" 或 "python"）
	Args    []string          `json:"args"`    // 命令行参数
	Env     map[string]string `json:"env"`     // 环境变量（会与父进程 env 合并）
	Enabled bool              `json:"enabled"` // 是否启用（false 则不连接）
}

// ToolInfo MCP 服务器暴露的工具信息。
// 生活类比：菜单上的一道菜——菜名(name)、介绍(description)、配方(input_schema)。
type ToolInfo struct {
	Name        string         `json:"name"`         // 工具名（全局唯一，冲突时后注册者覆盖）
	Description string         `json:"description"`  // 工具描述，供 LLM 理解何时调用
	InputSchema map[string]any `json:"input_schema"` // JSON Schema 格式的参数定义
}

// ServerStatus 单个 MCP 服务器的运行状态。
type ServerStatus struct {
	Connected bool   `json:"connected"`            // 是否已连接
	Error     string `json:"error,omitempty"`      // 错误信息（未连接时）
	ToolCount int    `json:"tool_count"`           // 已注册工具数量
}

// ConnectResult 连接单个 MCP 服务器的结果。
type ConnectResult struct {
	Name      string `json:"name"`                 // 服务器名
	Success   bool   `json:"success"`              // 是否连接成功
	Error     string `json:"error,omitempty"`      // 失败原因
	ToolCount int    `json:"tool_count"`           // 发现的工具数量
}
