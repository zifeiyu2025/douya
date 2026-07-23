// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
)

// 协议常量
const (
	protocolVersion  = "2024-11-05"      // MCP 协议版本
	clientName       = "douya"           // 客户端标识
	clientVersion    = "1.0.0"           // 客户端版本
	initTimeout      = 30 * time.Second  // 初始化超时
	listToolsTimeout = 15 * time.Second  // 列出工具超时
	callToolTimeout  = 120 * time.Second // 调用工具超时
	scannerMaxBuf    = 10 * 1024 * 1024  // scanner 缓冲区上限 10MB（工具结果可能较大）
)

// rpcRequest JSON-RPC 2.0 请求。
type rpcRequest struct {
	Jsonrpc string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcNotification JSON-RPC 2.0 通知（无 ID，不需要响应）。
type rpcNotification struct {
	Jsonrpc string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcResponse JSON-RPC 2.0 响应。
type rpcResponse struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// rpcError JSON-RPC 错误。
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// initializeResult MCP initialize 方法的返回结果。
type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// toolsListResult MCP tools/list 方法的返回结果。
type toolsListResult struct {
	Tools []struct {
		Name        string         `json:"name"`
		Description string         `json:"description"`
		InputSchema map[string]any `json:"inputSchema"`
	} `json:"tools"`
}

// callToolResult MCP tools/call 方法的返回结果。
type callToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

// managedServer 管理单个 MCP 服务器连接。
// 生活类比：像一个外卖平台的对讲机——通过它下单（写 stdin）、接收回执（读 stdout），
// 每次只能下一单（mutex 串行化），下单后等回执（pending map + channel）。
type managedServer struct {
	config  MCPServerConfig
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner

	mu      sync.Mutex                 // 序列化 call 请求（同一 server 串行通信）
	nextID  int64                      // JSON-RPC 请求 ID 自增
	pending map[int64]chan rpcResponse // 请求 ID -> 响应通道
	done    chan struct{}              // readLoop 结束信号

	tools  []ToolInfo   // 缓存的工具列表
	status ServerStatus // 运行状态
}

// Manager 管理所有 MCP 服务器连接，提供统一的工具列表和调用接口。
// 生活类比：餐厅前台的总调度——管理所有外卖平台对接（servers），
// 维护一张统一的菜单（toolToServer），顾客点哪道菜就知道去哪家平台下单。
type Manager struct {
	mu           sync.RWMutex
	servers      map[string]*managedServer // name -> server
	toolToServer map[string]string         // toolName -> serverName（工具路由表）
}

// NewManager 创建 MCP 管理器。
func NewManager() *Manager {
	return &Manager{
		servers:      make(map[string]*managedServer),
		toolToServer: make(map[string]string),
	}
}

// ConnectAll 批量连接所有已启用的 MCP 服务器。
// 单个服务器失败不影响其他服务器（错误隔离）。
// 返回每个服务器的连接结果。
func (m *Manager) ConnectAll(ctx context.Context, configs []MCPServerConfig) []ConnectResult {
	results := make([]ConnectResult, 0, len(configs))
	for _, cfg := range configs {
		if !cfg.Enabled {
			results = append(results, ConnectResult{
				Name:    cfg.Name,
				Success: false,
				Error:   "已禁用",
			})
			continue
		}
		err := m.Connect(ctx, cfg)
		if err != nil {
			results = append(results, ConnectResult{
				Name:    cfg.Name,
				Success: false,
				Error:   err.Error(),
			})
			log.Warn().Err(err).Str("server", cfg.Name).Msg("[mcp] 连接服务器失败")
		} else {
			toolCount := 0
			if s, ok := m.servers[cfg.Name]; ok {
				toolCount = len(s.tools)
			}
			results = append(results, ConnectResult{
				Name:      cfg.Name,
				Success:   true,
				ToolCount: toolCount,
			})
			log.Info().Str("server", cfg.Name).Int("tools", toolCount).Msg("[mcp] 连接成功")
		}
	}
	return results
}

// Connect 连接单个 MCP 服务器。
// 流程：启动子进程 -> 初始化握手 -> 发送 initialized 通知 -> 拉取工具列表。
func (m *Manager) Connect(ctx context.Context, cfg MCPServerConfig) error {
	// 如果已存在同名连接，先断开
	m.Disconnect(cfg.Name)

	// 启动子进程
	cmd := exec.CommandContext(ctx, cfg.Command, cfg.Args...)
	// 合并环境变量：父进程 env + 配置 env
	env := os.Environ()
	for k, v := range cfg.Env {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("创建 stdin 管道失败: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("创建 stdout 管道失败: %w", err)
	}
	// stderr 直接丢弃（MCP server 日志不应干扰豆芽）
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("启动进程失败: %w", err)
	}

	server := &managedServer{
		config:  cfg,
		cmd:     cmd,
		stdin:   stdin,
		scanner: bufio.NewScanner(stdout),
		pending: make(map[int64]chan rpcResponse),
		done:    make(chan struct{}),
	}
	// 增大 scanner 缓冲区（默认 64KB 可能不够）
	server.scanner.Buffer(make([]byte, 0, 1024), scannerMaxBuf)

	// 启动读取循环
	go server.readLoop()

	// 初始化握手
	initCtx, initCancel := context.WithTimeout(ctx, initTimeout)
	defer initCancel()

	var initResp initializeResult
	if err := server.callAndUnmarshal(initCtx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    clientName,
			"version": clientVersion,
		},
	}, &initResp); err != nil {
		server.kill()
		return fmt.Errorf("初始化握手失败: %w", err)
	}

	// 发送 initialized 通知（不需要响应）
	if err := server.notify("notifications/initialized", nil); err != nil {
		log.Warn().Err(err).Str("server", cfg.Name).Msg("[mcp] 发送 initialized 通知失败")
	}

	// 拉取工具列表
	listCtx, listCancel := context.WithTimeout(ctx, listToolsTimeout)
	defer listCancel()

	var toolsResp toolsListResult
	if err := server.callAndUnmarshal(listCtx, "tools/list", nil, &toolsResp); err != nil {
		server.kill()
		return fmt.Errorf("拉取工具列表失败: %w", err)
	}

	// 转换工具列表
	server.tools = make([]ToolInfo, 0, len(toolsResp.Tools))
	for _, t := range toolsResp.Tools {
		server.tools = append(server.tools, ToolInfo{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}
	server.status = ServerStatus{
		Connected: true,
		ToolCount: len(server.tools),
	}

	// 注册到 Manager
	m.mu.Lock()
	m.servers[cfg.Name] = server
	for _, t := range server.tools {
		if prev, exists := m.toolToServer[t.Name]; exists {
			log.Warn().Str("tool", t.Name).Str("prev_server", prev).Str("new_server", cfg.Name).
				Msg("[mcp] 工具名冲突，后注册者覆盖")
		}
		m.toolToServer[t.Name] = cfg.Name
	}
	m.mu.Unlock()

	log.Info().
		Str("server", cfg.Name).
		Str("server_info", initResp.ServerInfo.Name+" "+initResp.ServerInfo.Version).
		Int("tools", len(server.tools)).
		Msg("[mcp] 服务器已连接")

	return nil
}

// Disconnect 断开单个 MCP 服务器。
func (m *Manager) Disconnect(name string) {
	m.mu.Lock()
	server, ok := m.servers[name]
	if ok {
		delete(m.servers, name)
		// 清理工具路由表
		for toolName, sName := range m.toolToServer {
			if sName == name {
				delete(m.toolToServer, toolName)
			}
		}
	}
	m.mu.Unlock()

	if ok {
		server.kill()
		log.Info().Str("server", name).Msg("[mcp] 服务器已断开")
	}
}

// DisconnectAll 断开所有 MCP 服务器。
func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	names := make([]string, 0, len(m.servers))
	for name := range m.servers {
		names = append(names, name)
	}
	m.mu.Unlock()

	for _, name := range names {
		m.Disconnect(name)
	}
}

// ListTools 返回所有已连接服务器的工具列表。
func (m *Manager) ListTools() []ToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var tools []ToolInfo
	for _, server := range m.servers {
		tools = append(tools, server.tools...)
	}
	return tools
}

// CallTool 调用指定工具。
// 根据工具名查找所属服务器，转发调用请求。
// 生活类比：顾客点了一道菜，前台查菜单找到是哪家平台的，然后去那家下单。
func (m *Manager) CallTool(ctx context.Context, toolName string, arguments map[string]any) (string, error) {
	m.mu.RLock()
	serverName, ok := m.toolToServer[toolName]
	server := m.servers[serverName]
	m.mu.RUnlock()

	if !ok || server == nil {
		return "", fmt.Errorf("工具 %q 未注册或所属服务器已断开", toolName)
	}

	callCtx, cancel := context.WithTimeout(ctx, callToolTimeout)
	defer cancel()

	var result callToolResult
	err := server.callAndUnmarshal(callCtx, "tools/call", map[string]any{
		"name":      toolName,
		"arguments": arguments,
	}, &result)
	if err != nil {
		return "", err
	}

	// 拼接所有 text content
	var content string
	for _, c := range result.Content {
		if c.Type == "text" && c.Text != "" {
			if content != "" {
				content += "\n"
			}
			content += c.Text
		}
	}

	if result.IsError {
		return "", fmt.Errorf("工具 %q 返回错误: %s", toolName, content)
	}

	return content, nil
}

// GetServerStatus 返回指定服务器的状态。
func (m *Manager) GetServerStatus(name string) (ServerStatus, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	server, ok := m.servers[name]
	if !ok {
		return ServerStatus{Connected: false, Error: "未连接"}, false
	}
	return server.status, true
}

// GetAllStatus 返回所有服务器的状态。
func (m *Manager) GetAllStatus() map[string]ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]ServerStatus, len(m.servers))
	for name, server := range m.servers {
		status[name] = server.status
	}
	return status
}

// HasTool 检查工具是否已注册。
func (m *Manager) HasTool(toolName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.toolToServer[toolName]
	return ok
}

// readLoop 持续从 stdout 读取 JSON-RPC 消息并分发到对应等待者。
// 生活类比：像对讲机的接收员，听到回执（response）就按编号送到对应下单人手里，
// 听到广播（notification）就忽略。
func (s *managedServer) readLoop() {
	defer close(s.done)

	for s.scanner.Scan() {
		line := s.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// 非 JSON 行（可能是 server 日志误写到 stdout），跳过
			continue
		}

		// 只处理有 ID 的响应（notification 没有 ID 或 ID=0，忽略）
		if resp.ID == 0 {
			continue
		}

		s.mu.Lock()
		ch, ok := s.pending[resp.ID]
		if ok {
			delete(s.pending, resp.ID)
		}
		s.mu.Unlock()

		if ok {
			ch <- resp
		}
	}

	// readLoop 结束说明 stdout 已关闭（server 进程退出或崩溃）
	s.mu.Lock()
	s.status = ServerStatus{
		Connected: false,
		Error:     "连接已断开",
	}
	// 通知所有等待者
	for id, ch := range s.pending {
		ch <- rpcResponse{
			ID:    id,
			Error: &rpcError{Code: -32000, Message: "连接已断开"},
		}
		delete(s.pending, id)
	}
	s.mu.Unlock()

	log.Warn().Str("server", s.config.Name).Msg("[mcp] readLoop 结束，连接已断开")
}

// call 发送 JSON-RPC 请求并等待响应（返回原始 Result）。
func (s *managedServer) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	s.mu.Lock()
	id := atomic.AddInt64(&s.nextID, 1)
	req := rpcRequest{
		Jsonrpc: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(req)
	if err != nil {
		s.mu.Unlock()
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}
	data = append(data, '\n')

	respCh := make(chan rpcResponse, 1)
	s.pending[id] = respCh
	s.mu.Unlock()

	// 写入请求
	if _, err := s.stdin.Write(data); err != nil {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, fmt.Errorf("写入请求失败: %w", err)
	}

	// 等待响应或超时
	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, fmt.Errorf("rpc 错误 [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		return resp.Result, nil
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return nil, ctx.Err()
	case <-s.done:
		return nil, fmt.Errorf("连接已断开")
	}
}

// callAndUnmarshal 发送请求并将响应反序列化到 target。
func (s *managedServer) callAndUnmarshal(ctx context.Context, method string, params any, target any) error {
	result, err := s.call(ctx, method, params)
	if err != nil {
		return err
	}
	if len(result) == 0 {
		return nil
	}
	if err := json.Unmarshal(result, target); err != nil {
		return fmt.Errorf("解析响应失败: %w", err)
	}
	return nil
}

// notify 发送 JSON-RPC 通知（无 ID，不需要响应）。
func (s *managedServer) notify(method string, params any) error {
	notif := rpcNotification{
		Jsonrpc: "2.0",
		Method:  method,
		Params:  params,
	}
	data, err := json.Marshal(notif)
	if err != nil {
		return fmt.Errorf("序列化通知失败: %w", err)
	}
	data = append(data, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	_, err = s.stdin.Write(data)
	return err
}

// kill 终止服务器进程并关闭管道。
func (s *managedServer) kill() {
	s.mu.Lock()
	s.status = ServerStatus{
		Connected: false,
		Error:     "已断开",
	}
	s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	_ = s.stdin.Close()
}
