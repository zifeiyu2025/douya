// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"douya/internal/config"
	"douya/internal/mcp"

	zlog "github.com/rs/zerolog/log"
)

// GetMCPServers 返回配置中的 MCP 服务器列表。
func (a *App) GetMCPServers() []mcp.MCPServerConfig {
	cfg := a.getConfig()
	if cfg == nil {
		return []mcp.MCPServerConfig{}
	}
	if cfg.MCPServers == nil {
		return []mcp.MCPServerConfig{}
	}
	return cfg.MCPServers
}

// SaveMCPServers 保存 MCP 服务器配置并重新连接所有服务器。
// 生活类比：更新外卖平台对接卡，然后断开旧连接、按新配置重新连接所有平台。
func (a *App) SaveMCPServers(servers []mcp.MCPServerConfig) error {
	// 1. 更新配置（复制-修改-替换指针模式，避免破坏快照语义）
	a.configMu.Lock()
	if a.config == nil {
		a.configMu.Unlock()
		return fmt.Errorf("配置未初始化")
	}
	newCfg := *a.config
	newCfg.MCPServers = servers
	cfg := &newCfg
	a.config = cfg
	a.configMu.Unlock()

	// 2. 断开旧连接
	if a.mcpManager != nil {
		a.mcpManager.DisconnectAll()
	}

	// 3. 重新连接
	if len(servers) > 0 {
		if a.mcpManager == nil {
			a.mcpManager = mcp.NewManager()
		}
		connectCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		results := a.mcpManager.ConnectAll(connectCtx, servers)
		cancel()

		successCount := 0
		for _, r := range results {
			if r.Success {
				successCount++
			}
		}
		if a.service != nil {
			a.service.SetMCPManager(a.mcpManager)
		}
		zlog.Info().Int("total", len(servers)).Int("success", successCount).Msg("[mcp] 服务器配置已保存并重新连接")
	} else {
		a.mcpManager = nil
		if a.service != nil {
			a.service.SetMCPManager(nil)
		}
		zlog.Info().Msg("[mcp] 无 MCP 服务器配置，已清空连接")
	}

	// 4. 保存配置到磁盘
	configPath := filepath.Join(appDir(), "config.json")
	if err := config.Save(configPath, cfg); err != nil {
		return fmt.Errorf("保存配置失败: %w", err)
	}
	return nil
}

// TestMCPConnection 测试连接单个 MCP 服务器（不持久化，测试后立即断开）。
// 生活类比：先试着拨通外卖平台电话确认能联系上，再决定是否长期合作。
func (a *App) TestMCPConnection(server mcp.MCPServerConfig) (*mcp.ConnectResult, error) {
	if server.Command == "" {
		return &mcp.ConnectResult{
			Name:  server.Name,
			Error: "命令不能为空",
		}, nil
	}

	tempMgr := mcp.NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err := tempMgr.Connect(ctx, server)
	if err != nil {
		return &mcp.ConnectResult{
			Name:  server.Name,
			Error: err.Error(),
		}, nil
	}

	toolCount := 0
	status, ok := tempMgr.GetServerStatus(server.Name)
	if ok {
		toolCount = status.ToolCount
	}
	tempMgr.Disconnect(server.Name)

	return &mcp.ConnectResult{
		Name:      server.Name,
		Success:   true,
		ToolCount: toolCount,
	}, nil
}

// GetMCPStatus 返回所有已连接 MCP 服务器的状态。
func (a *App) GetMCPStatus() map[string]mcp.ServerStatus {
	if a.mcpManager == nil {
		return map[string]mcp.ServerStatus{}
	}
	return a.mcpManager.GetAllStatus()
}

// ListMCPTools 列出所有已连接 MCP 服务器的工具。
func (a *App) ListMCPTools() []mcp.ToolInfo {
	if a.mcpManager == nil {
		return []mcp.ToolInfo{}
	}
	tools := a.mcpManager.ListTools()
	if tools == nil {
		return []mcp.ToolInfo{}
	}
	return tools
}
