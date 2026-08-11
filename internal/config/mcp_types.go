// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

// MCPServerConfig 单个 MCP 服务器的配置。
// 生活类比：像一张外卖平台对接卡——平台名(name)、对接程序(command)、
// 启动参数(args)、所需凭证(env)、是否启用(enabled)。
//
// 注意：豆芽不再自行管理 MCP 子进程，配置仅用于生成 mcp_servers.json
// 交给 llama-server 通过 --mcp-servers-config 参数加载。
// JSON 字段名与 Cursor / Claude Desktop 的 mcpServers 格式保持一致。
type MCPServerConfig struct {
	Name    string            `json:"name"`    // 服务器唯一标识（用于日志和工具路由）
	Command string            `json:"command"` // 可执行文件路径（如 "npx" 或 "python"）
	Args    []string          `json:"args"`    // 命令行参数
	Env     map[string]string `json:"env"`     // 环境变量（会与父进程 env 合并）
	Enabled bool              `json:"enabled"` // 是否启用（false 则不写入 mcp_servers.json）
}

// MCPServerStatus MCP 服务器的运行时状态。
// 由 llama-server 通过 GET /tools 端点暴露的工具列表反推得到：
// 若工具列表中存在以 "<server>_" 为前缀的工具，则认为该 server 已连接。
type MCPServerStatus struct {
	Connected bool   `json:"connected"`       // 是否已连接（llama-server 是否已加载该 server 的工具）
	Error     string `json:"error,omitempty"` // 错误信息（未连接时的提示）
	ToolCount int    `json:"tool_count"`      // 该 server 暴露的工具数量
}

// MCPConnectResult 测试连接的结果。
// 新架构下豆芽不直接启动 MCP 子进程，"测试连接"语义变为：
// 提示用户需重启 llama-server 让新配置生效。
type MCPConnectResult struct {
	Name      string `json:"name"`            // 服务器名
	Success   bool   `json:"success"`         // 是否成功（新架构下始终为 false，需重启）
	Error     string `json:"error,omitempty"` // 错误/提示信息
	ToolCount int    `json:"tool_count"`      // 工具数量（重启后可通过 ListMCPTools 查看）
}

// MCPToolInfo MCP 工具信息（供前端展示）。
// 由 llama-server /tools 端点返回的 ToolDefinition 简化而来。
type MCPToolInfo struct {
	Name        string         `json:"name"`         // 工具名（形如 "echo_echo"，含 server 前缀）
	Description string         `json:"description"`  // 工具描述
	InputSchema map[string]any `json:"input_schema"` // 工具参数 schema（JSON Schema 格式）
}
