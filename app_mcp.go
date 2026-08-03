// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"douya/internal/apperror"
	"douya/internal/config"

	zlog "github.com/rs/zerolog/log"
)

// cursorMcpServersFile Cursor 兼容的 mcpServers 配置文件结构。
// llama-server 通过 --mcp-servers-config 加载此文件，启动并管理所有 MCP 子进程。
// 字段名与 Cursor / Claude Desktop 的 mcpServers 格式保持一致，便于跨工具复用。
type cursorMcpServersFile struct {
	McpServers map[string]cursorMcpServer `json:"mcpServers"`
}

// cursorMcpServer 单个 MCP server 的 Cursor 格式配置。
// 字段名采用 snake_case 对齐 llama.cpp 的 parse_cursor_format 实现。
type cursorMcpServer struct {
	Command   string            `json:"command"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	Cwd       string            `json:"cwd,omitempty"`
	TimeoutMs int               `json:"timeout_ms,omitempty"`
}

// GetMCPServers 返回配置中的 MCP 服务器列表。
func (a *App) GetMCPServers() []config.MCPServerConfig {
	cfg := a.getConfig()
	if cfg == nil {
		return []config.MCPServerConfig{}
	}
	if cfg.MCPServers == nil {
		return []config.MCPServerConfig{}
	}
	return cfg.MCPServers
}

// SaveMCPServers 保存 MCP 服务器配置并写入 mcp_servers.json 文件。
// 生活类比：更新外卖平台对接卡，然后重新生成给外卖调度中心的指令清单。
//
// 注意：llama-server 启动时通过 --mcp-servers-config 加载 mcp_servers.json，
// 因此修改配置后需要重启 llama-server 才能让新配置生效（豆芽无热重载能力）。
func (a *App) SaveMCPServers(servers []config.MCPServerConfig) error {
	// 1. 更新配置（复制-修改-替换指针模式，避免破坏快照语义）
	a.configMu.Lock()
	if a.config == nil {
		a.configMu.Unlock()
		return apperror.New(apperror.KindInvalidConfig, "配置未初始化")
	}
	newCfg := *a.config
	newCfg.MCPServers = servers
	cfg := &newCfg
	a.config = cfg
	a.configMu.Unlock()

	// 2. 持久化 config.json
	configPath := filepath.Join(appDir(), "config.json")
	if err := config.Save(configPath, cfg); err != nil {
		return apperror.Wrap(apperror.KindInvalidConfig, "保存配置失败", err)
	}

	// 3. 生成 mcp_servers.json（仅包含已启用的 server）
	mcpConfigPath := filepath.Join(appDir(), "mcp_servers.json")
	if err := writeMcpServersFile(mcpConfigPath, servers); err != nil {
		return apperror.Wrap(apperror.KindInternal, "写入 mcp_servers.json 失败", err)
	}

	enabledCount := 0
	for _, s := range servers {
		if s.Enabled {
			enabledCount++
		}
	}
	zlog.Info().Int("total", len(servers)).Int("enabled", enabledCount).
		Str("path", mcpConfigPath).Msg("[mcp] 配置已保存，需重启 llama-server 才能生效")
	return nil
}

// writeMcpServersFile 将豆芽内部的 MCPServerConfig 列表转换为 Cursor 兼容格式并写入文件。
// 仅写入 Enabled=true 的 server；command 为空的 server 被跳过（与 llama-server 解析逻辑一致）。
func writeMcpServersFile(path string, servers []config.MCPServerConfig) error {
	file := cursorMcpServersFile{McpServers: make(map[string]cursorMcpServer)}
	for _, s := range servers {
		if !s.Enabled || s.Command == "" {
			continue
		}
		// server name 重复时后者覆盖前者，与 llama-server 的 duplicate skip 行为不同但可接受
		// （豆芽前端已做唯一性校验，正常情况下不会重名）
		file.McpServers[s.Name] = cursorMcpServer{
			Command:   s.Command,
			Args:      s.Args,
			Env:       s.Env,
			TimeoutMs: 30000, // 默认 30 秒，与 llama-server 默认值对齐
		}
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "序列化 mcp_servers.json 失败", err)
	}
	// RF-4 修复：MCP 配置的 Env 字段可能含 API token 等敏感信息，收紧文件权限到 0o600
	// 仅文件所有者可读写，避免同机其他用户读取明文环境变量
	return os.WriteFile(path, data, 0o600)
}

// ensureMcpServersFileExists 启动时确保 mcp_servers.json 与 config.json 同步。
// 每次 startup 都重新生成，确保配置一致（与 generatePresetFile 风格一致）。
//
// 作用：让 llama-server 启动时通过 --mcp-servers-config 加载此文件，启用 /tools 端点
// 并管理所有 MCP 子进程。若文件不存在，llama-server 不会传 --mcp-servers-config，
// /tools 端点会被禁用（返回 403 feature_disabled），MCP 工具列表与调用均不可用。
//
// 生活类比：餐厅开门前先确认菜单是否就位——根据数据库（config.json）重新打印一份
// 给调度中心（llama-server）使用，确保菜单与数据库一致。
func (a *App) ensureMcpServersFileExists() {
	cfg := a.getConfig()
	if cfg == nil {
		return
	}
	mcpConfigPath := filepath.Join(appDir(), "mcp_servers.json")
	if err := writeMcpServersFile(mcpConfigPath, cfg.MCPServers); err != nil {
		zlog.Warn().Err(err).Msg("[startup] 生成 mcp_servers.json 失败")
		return
	}
	enabledCount := 0
	for _, s := range cfg.MCPServers {
		if s.Enabled && s.Command != "" {
			enabledCount++
		}
	}
	zlog.Info().Str("path", mcpConfigPath).Int("enabled", enabledCount).
		Msg("[startup] mcp_servers.json 已同步")
}

// GetMCPStatus 返回每个 MCP 服务器的运行时状态。
// 状态由 llama-server /tools 端点暴露的工具列表反推得到：
// 若工具列表中存在以 "<server>_" 为前缀的工具，则认为该 server 已连接。
//
// 生活类比：不去问每个外卖平台"你连上了吗"，而是看调度中心（llama-server）
// 的菜单上有没有该平台的菜品——有菜品说明已对接成功。
func (a *App) GetMCPStatus() map[string]config.MCPServerStatus {
	result := make(map[string]config.MCPServerStatus)
	cfg := a.getConfig()
	if cfg == nil {
		return result
	}

	// 收集缓存中各 server 的工具数量（通过工具名前缀 "<server>_" 匹配）
	toolCountByServer := make(map[string]int)
	if a.service != nil {
		tools := a.service.GetCachedMcpTools()
		for _, t := range tools {
			// 工具名形如 "echo_echo"，下划线前是 server 名
			name := t.Function.Name
			idx := -1
			for i := 0; i < len(name); i++ {
				if name[i] == '_' {
					idx = i
					break
				}
			}
			if idx > 0 {
				serverName := name[:idx]
				toolCountByServer[serverName]++
			}
		}
	}

	// 为每个已配置的 server 生成状态
	for _, s := range cfg.MCPServers {
		if !s.Enabled {
			result[s.Name] = config.MCPServerStatus{
				Connected: false,
				Error:     "已禁用",
			}
			continue
		}
		count := toolCountByServer[s.Name]
		if count > 0 {
			result[s.Name] = config.MCPServerStatus{
				Connected:  true,
				ToolCount:  count,
			}
		} else {
			result[s.Name] = config.MCPServerStatus{
				Connected: false,
				Error:     "未连接（重启 llama-server 后生效）",
			}
		}
	}
	return result
}

// TestMCPConnection 新架构下不再支持热测试连接（豆芽不管理 MCP 子进程）。
// 返回提示信息告知用户需重启 llama-server 让新配置生效。
//
// 生活类比：以前豆芽自己开外卖平台，可以随时测试对接；
// 现在豆芽只生成配置交给调度中心，要测试得重启调度中心。
func (a *App) TestMCPConnection(server config.MCPServerConfig) config.MCPConnectResult {
	return config.MCPConnectResult{
		Name:    server.Name,
		Success: false,
		Error:   "新架构下需重启 llama-server 让配置生效。重启后点击「刷新状态」查看连接情况。",
	}
}

// ListMCPTools 返回当前缓存的 MCP 工具列表（来自 llama-server /tools 端点）。
// 生活类比：把调度中心菜单上所有外卖平台的菜品列出来给前台看。
func (a *App) ListMCPTools() []config.MCPToolInfo {
	if a.service == nil {
		return []config.MCPToolInfo{}
	}
	tools := a.service.GetCachedMcpTools()
	result := make([]config.MCPToolInfo, 0, len(tools))
	for _, t := range tools {
		result = append(result, config.MCPToolInfo{
			Name:        t.Function.Name,
			Description: t.Function.Description,
			InputSchema: t.Function.Parameters,
		})
	}
	return result
}

// RefreshMcpTools 刷新 MCP 工具缓存（供前端在 llama-server 重启后调用）。
// 生活类比：让前台去调度中心重新拿一份最新菜单。
func (a *App) RefreshMcpTools() {
	if a.service != nil {
		a.service.RefreshMcpToolsCache()
	}
}
