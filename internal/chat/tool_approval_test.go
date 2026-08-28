// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"testing"
	"time"

	"douya/internal/config"
	"douya/internal/llm"
)

// newApprovalTestService 构造带工具权限元数据的测试 Service（不触网）。
func newApprovalTestService() *Service {
	s := NewService(nil, nil, nil, config.DefaultConfig(), nil, "")
	s.mcpToolsCacheMu.Lock()
	s.mcpToolPerms = map[string]toolPermission{
		"read_file":          {Write: false, DisplayName: "Read file", Known: true},
		"exec_shell_command": {Write: true, DisplayName: "Execute shell command", Known: true},
		"write_file":         {Write: true, DisplayName: "Write file", Known: true},
		"mcp_fs_write":       {Write: false, DisplayName: "", Known: false}, // MCP 工具未声明权限
	}
	s.mcpToolsCache = []llm.ToolDefinition{{Type: "function", Function: llm.FunctionDef{Name: "read_file"}}}
	s.mcpToolsCacheMu.Unlock()
	return s
}

// TestClassifyToolRisk 验证风险分级以引擎 permissions.write 元数据为唯一事实。
func TestClassifyToolRisk(t *testing.T) {
	s := newApprovalTestService()
	cases := map[string]ToolRisk{
		"search":             ToolRiskSafe,    // 自实现搜索
		"read_file":          ToolRiskSafe,    // 引擎声明只读
		"exec_shell_command": ToolRiskWrite,   // 引擎声明写操作
		"write_file":         ToolRiskWrite,   // 引擎声明写操作
		"mcp_fs_write":       ToolRiskUnknown, // MCP 工具未声明权限
		"totally_unknown":    ToolRiskUnknown, // 不在缓存中的工具
	}
	for name, want := range cases {
		if got := s.classifyToolRisk(name); got != want {
			t.Errorf("classifyToolRisk(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestToolNeedsApproval 验证三种审批模式的分级决策与会话级允许清单优先级。
func TestToolNeedsApproval(t *testing.T) {
	s := newApprovalTestService()

	// auto（默认）：只读放行，写/未知需确认
	for _, mode := range []string{"", "auto"} {
		if need, _ := s.toolNeedsApproval("read_file", mode); need {
			t.Errorf("auto 模式下只读工具不应需要审批")
		}
		if need, _ := s.toolNeedsApproval("exec_shell_command", mode); !need {
			t.Errorf("auto 模式下写操作工具需要审批")
		}
		if need, _ := s.toolNeedsApproval("mcp_fs_write", mode); !need {
			t.Errorf("auto 模式下未知权限的 MCP 工具需要审批")
		}
	}
	// always：所有工具都确认
	if need, risk := s.toolNeedsApproval("read_file", "always"); !need || risk != ToolRiskAll {
		t.Errorf("always 模式下所有工具都应需要审批（risk=%s）", risk)
	}
	// never：全部放行
	for _, name := range []string{"read_file", "exec_shell_command", "mcp_fs_write"} {
		if need, _ := s.toolNeedsApproval(name, "never"); need {
			t.Errorf("never 模式下 %s 不应需要审批", name)
		}
	}
	// 会话级允许清单优先于 auto 分级
	s.approvalMu.Lock()
	s.sessionAllowedTools["exec_shell_command"] = true
	s.approvalMu.Unlock()
	if need, _ := s.toolNeedsApproval("exec_shell_command", "auto"); need {
		t.Errorf("会话级允许清单应跳过审批")
	}
}

// TestResolveToolApproval 验证审批决定的回传链路与过期请求报错。
func TestResolveToolApproval(t *testing.T) {
	s := newApprovalTestService()
	tc := llm.ToolCall{ID: "call-1", Function: llm.FunctionCall{Name: "exec_shell_command", Arguments: `{"command":"dir"}`}}

	decided := make(chan toolApprovalDecision, 1)
	go func() {
		approved, reason := s.requestToolApproval(t.Context(), "conv-test", tc, ToolRiskWrite)
		decided <- toolApprovalDecision{approved: approved}
		t.Logf("reason=%q", reason)
	}()

	// 等注册表出现再回传，避免竞态
	deadline := time.Now().Add(2 * time.Second)
	for {
		s.approvalMu.Lock()
		_, ok := s.pendingApprovals["call-1"]
		s.approvalMu.Unlock()
		if ok || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := s.ResolveToolApproval("call-1", true, true); err != nil {
		t.Fatalf("ResolveToolApproval 返回错误: %v", err)
	}
	got := <-decided
	if !got.approved {
		t.Fatalf("期望批准，实际拒绝")
	}
	if !s.isToolSessionAllowed("exec_shell_command") {
		t.Errorf("remember=true 应把工具加入会话级允许清单")
	}
	// 重复回传：请求已消费，应报错
	if err := s.ResolveToolApproval("call-1", false, false); err == nil {
		t.Errorf("已解决的审批请求再次回传应返回错误")
	}
}

// TestToolCallMaxRounds 验证轮次默认值与上限钳制。
func TestToolCallMaxRounds(t *testing.T) {
	s := newApprovalTestService()
	if got := s.toolCallMaxRounds(nil); got != 8 {
		t.Errorf("nil 配置应返回默认 8，实际 %d", got)
	}
	if got := s.toolCallMaxRounds(&config.Config{}); got != 8 {
		t.Errorf("未配置应返回默认 8，实际 %d", got)
	}
	if got := s.toolCallMaxRounds(&config.Config{AgentMaxRounds: 3}); got != 3 {
		t.Errorf("显式配置应生效，实际 %d", got)
	}
	if got := s.toolCallMaxRounds(&config.Config{AgentMaxRounds: 999}); got != 25 {
		t.Errorf("超上限应钳制到 25，实际 %d", got)
	}
}

// TestToolResultOK 验证失败前缀识别（仅 UI 着色用途）。
func TestToolResultOK(t *testing.T) {
	s := newApprovalTestService()
	_ = s
	if toolResultOK("") != true {
		t.Errorf("空结果应视为成功")
	}
	if toolResultOK(`Error: invalid arguments`) != false {
		t.Errorf("Error: 前缀应视为失败")
	}
	if toolResultOK(`<file content>`) != true {
		t.Errorf("正常内容应视为成功")
	}
}
