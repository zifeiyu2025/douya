// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package mcp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestNewManager 测试新建管理器的初始状态。
// 生活类比：刚开业的餐厅前台，还没对接任何外卖平台，
// 菜单是空的，查任何菜都查不到。
func TestNewManager(t *testing.T) {
	m := NewManager()

	if tools := m.ListTools(); len(tools) != 0 {
		t.Errorf("新管理器 ListTools 应为空，实际 %d 项", len(tools))
	}
	if m.HasTool("anything") {
		t.Error("新管理器 HasTool 应返回 false")
	}
	if status, ok := m.GetServerStatus("nope"); ok {
		t.Errorf("新管理器 GetServerStatus 不应找到服务器，got ok=%v status=%+v", ok, status)
	}
	if all := m.GetAllStatus(); len(all) != 0 {
		t.Errorf("新管理器 GetAllStatus 应为空，实际 %d 项", len(all))
	}
}

// TestDisconnectAll 空管理器调用 DisconnectAll 不应 panic。
func TestDisconnectAll_empty(t *testing.T) {
	m := NewManager()
	m.DisconnectAll() // 不应 panic
}

// TestConnectAll_disabled 测试禁用的服务器不会尝试连接。
// 生活类比：对接卡上标注了"暂停合作"的平台，前台直接跳过，不尝试对接。
func TestConnectAll_disabled(t *testing.T) {
	m := NewManager()
	configs := []MCPServerConfig{
		{Name: "disabled-server", Command: "nonexistent-cmd", Enabled: false},
	}
	results := m.ConnectAll(context.Background(), configs)
	if len(results) != 1 {
		t.Fatalf("应返回 1 个结果，实际 %d", len(results))
	}
	if results[0].Success {
		t.Error("禁用的服务器 Success 应为 false")
	}
	if results[0].Error != "已禁用" {
		t.Errorf("错误信息应为'已禁用'，实际 %q", results[0].Error)
	}
}

// TestConnect_nonexistentCmd 测试连接不存在的命令会返回错误。
// 生活类比：尝试对接一个不存在的平台（命令找不到），应立即报错。
func TestConnect_nonexistentCmd(t *testing.T) {
	m := NewManager()
	cfg := MCPServerConfig{
		Name:    "ghost",
		Command: "this-command-does-not-exist-12345",
		Args:    []string{},
		Enabled: true,
	}
	err := m.Connect(context.Background(), cfg)
	if err == nil {
		t.Error("连接不存在的命令应返回错误")
		// 清理：如果意外成功，断开连接
		m.Disconnect("ghost")
	}
}

// TestCallTool_unknownTool 测试调用未注册的工具会返回错误。
func TestCallTool_unknownTool(t *testing.T) {
	m := NewManager()
	_, err := m.CallTool(context.Background(), "nonexistent-tool", nil)
	if err == nil {
		t.Error("调用未注册的工具应返回错误")
	}
}

// ===== 以下是集成测试，使用真实的模拟 MCP server =====

// mockServerSource 模拟 MCP server 的 Go 源码。
// 它通过 stdin 读取 JSON-RPC 请求，按 MCP 协议规范响应。
const mockServerSource = `package main

import (
	"bufio"
	"encoding/json"
	"os"
)

type request struct {
	Jsonrpc string          ` + "`json:\"jsonrpc\"`" + `
	ID      int64           ` + "`json:\"id\"`" + `
	Method  string          ` + "`json:\"method\"`" + `
	Params  json.RawMessage ` + "`json:\"params,omitempty\"`" + `
}

type response struct {
	Jsonrpc string      ` + "`json:\"jsonrpc\"`" + `
	ID      int64       ` + "`json:\"id\"`" + `
	Result  interface{} ` + "`json:\"result,omitempty\"`" + `
	Error   interface{} ` + "`json:\"error,omitempty\"`" + `
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024), 10*1024*1024)
	enc := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		if req.ID == 0 {
			continue // 通知，不响应
		}
		switch req.Method {
		case "initialize":
			enc.Encode(response{
				Jsonrpc: "2.0", ID: req.ID,
				Result: map[string]interface{}{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]interface{}{},
					"serverInfo":      map[string]interface{}{"name": "mock", "version": "1.0"},
				},
			})
		case "tools/list":
			enc.Encode(response{
				Jsonrpc: "2.0", ID: req.ID,
				Result: map[string]interface{}{
					"tools": []map[string]interface{}{
						{
							"name":        "echo",
							"description": "回显输入的文本",
							"inputSchema": map[string]interface{}{
								"type": "object",
								"properties": map[string]interface{}{
									"text": map[string]interface{}{"type": "string"},
								},
							},
						},
					},
				},
			})
		case "tools/call":
			enc.Encode(response{
				Jsonrpc: "2.0", ID: req.ID,
				Result: map[string]interface{}{
					"content": []map[string]interface{}{
						{"type": "text", "text": "mock result"},
					},
					"isError": false,
				},
			})
		default:
			enc.Encode(response{
				Jsonrpc: "2.0", ID: req.ID,
				Error:   map[string]interface{}{"code": -32601, "message": "method not found"},
			})
		}
	}
}
`

// buildMockServer 编译模拟 MCP server 到临时可执行文件，返回路径。
// 测试结束后调用 cleanup 清理。
func buildMockServer(t *testing.T) (path string, cleanup func()) {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "mock_mcp_server.go")
	if err := os.WriteFile(src, []byte(mockServerSource), 0644); err != nil {
		t.Fatalf("写入模拟 server 源码失败: %v", err)
	}
	bin := filepath.Join(dir, "mock_mcp_server")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	// 编译模拟 server
	cmd := exec.Command("go", "build", "-o", bin, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("编译模拟 server 失败: %v\n%s", err, out)
	}
	return bin, func() { _ = os.Remove(bin) }
}

// TestConnectAll_integration 集成测试：连接模拟 server，验证工具发现。
// 生活类比：真的对接一家外卖平台，确认它的菜单（工具）能正确同步到前台。
func TestConnectAll_integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（-short 模式）")
	}
	bin, cleanup := buildMockServer(t)
	defer cleanup()

	m := NewManager()
	configs := []MCPServerConfig{
		{Name: "mock", Command: bin, Enabled: true},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := m.ConnectAll(ctx, configs)
	if len(results) != 1 {
		t.Fatalf("应返回 1 个结果，实际 %d", len(results))
	}
	if !results[0].Success {
		t.Fatalf("连接应成功，错误: %s", results[0].Error)
	}
	if results[0].ToolCount != 1 {
		t.Errorf("应发现 1 个工具，实际 %d", results[0].ToolCount)
	}

	// 验证 ListTools
	tools := m.ListTools()
	if len(tools) != 1 {
		t.Fatalf("ListTools 应返回 1 个工具，实际 %d", len(tools))
	}
	if tools[0].Name != "echo" {
		t.Errorf("工具名应为 echo，实际 %s", tools[0].Name)
	}

	// 验证 HasTool
	if !m.HasTool("echo") {
		t.Error("HasTool(echo) 应返回 true")
	}
	if m.HasTool("nonexistent") {
		t.Error("HasTool(nonexistent) 应返回 false")
	}

	// 验证 GetServerStatus
	status, ok := m.GetServerStatus("mock")
	if !ok {
		t.Fatal("GetServerStatus 应找到 mock 服务器")
	}
	if !status.Connected {
		t.Error("服务器应已连接")
	}
	if status.ToolCount != 1 {
		t.Errorf("状态中工具数应为 1，实际 %d", status.ToolCount)
	}

	// 验证 CallTool
	result, err := m.CallTool(ctx, "echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("CallTool 失败: %v", err)
	}
	if result != "mock result" {
		t.Errorf("工具结果应为 'mock result'，实际 %q", result)
	}

	// 清理
	m.DisconnectAll()
}

// TestDisconnect 集成测试：断开连接后工具和状态应清理。
func TestDisconnect(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（-short 模式）")
	}
	bin, cleanup := buildMockServer(t)
	defer cleanup()

	m := NewManager()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.Connect(ctx, MCPServerConfig{Name: "mock", Command: bin, Enabled: true}); err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 确认工具已注册
	if !m.HasTool("echo") {
		t.Fatal("连接后应能找到 echo 工具")
	}

	// 断开
	m.Disconnect("mock")

	// 工具应不再可用
	if m.HasTool("echo") {
		t.Error("断开后 HasTool(echo) 应返回 false")
	}
	if tools := m.ListTools(); len(tools) != 0 {
		t.Errorf("断开后 ListTools 应为空，实际 %d 项", len(tools))
	}
	if _, ok := m.GetServerStatus("mock"); ok {
		t.Error("断开后 GetServerStatus 不应找到服务器")
	}
}

// TestConnectAll_partialFailure 测试部分服务器失败不影响其他服务器。
// 生活类比：一家平台对接失败不应影响其他平台正常对接。
func TestConnectAll_partialFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试（-short 模式）")
	}
	bin, cleanup := buildMockServer(t)
	defer cleanup()

	m := NewManager()
	configs := []MCPServerConfig{
		{Name: "good", Command: bin, Enabled: true},
		{Name: "bad", Command: "this-command-does-not-exist-12345", Enabled: true},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	results := m.ConnectAll(ctx, configs)
	if len(results) != 2 {
		t.Fatalf("应返回 2 个结果，实际 %d", len(results))
	}

	// 找到两个结果
	var goodResult, badResult *ConnectResult
	for i := range results {
		switch results[i].Name {
		case "good":
			goodResult = &results[i]
		case "bad":
			badResult = &results[i]
		}
	}
	if goodResult == nil || !goodResult.Success {
		t.Error("good 服务器应连接成功")
	}
	if badResult == nil || badResult.Success {
		t.Error("bad 服务器应连接失败")
	}

	// good 的工具应可用
	if !m.HasTool("echo") {
		t.Error("good 服务器的工具应可用")
	}

	m.DisconnectAll()
}
