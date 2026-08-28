// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
	"douya/internal/config"
	"douya/internal/llm"
)

// ===== Agent 工具审批门禁 =====
//
// 业界成熟方案（Claude Code / Cursor 的 permission prompt 模式）：
// 写操作类工具在执行前暂停循环，弹出审批请求等待用户决定；
// 拒绝结果以 tool 消息回传给模型，让它改用其他方式作答而非中断对话。
//
// 风险分级直接采用 llama.cpp GET /tools 返回的 permissions.write 元数据
// （read_file/grep_search/file_glob_search/get_info 为只读；write_file/
// edit_file/exec_shell_command 为写操作），不靠文件名启发式猜；
// 权限未声明的 MCP 工具一律按"未知"处理，默认需要确认。

const (
	// toolApprovalTimeout 单个审批请求的等待上限。超时自动拒绝，
	// 避免用户离开电脑时 tool call 循环永久挂起（稳定性优先）。
	toolApprovalTimeout = 120 * time.Second

	// ToolRiskSafe 只读工具：search、引擎声明 write=false 的内置工具
	ToolRiskSafe ToolRisk = "safe"
	// ToolRiskWrite 写操作工具：引擎声明 write=true（write_file/edit_file/exec_shell_command）
	ToolRiskWrite ToolRisk = "write"
	// ToolRiskUnknown 权限未声明的 MCP 工具
	ToolRiskUnknown ToolRisk = "unknown"

	// ToolRiskAll 审批模式为 always 时 UI 展示用的风险标签
	ToolRiskAll ToolRisk = "all"
)

// ToolRisk 工具风险等级。
type ToolRisk string

// toolPermission 引擎 /tools 端点返回的工具权限元数据。
type toolPermission struct {
	Write       bool   // 是否写操作
	DisplayName string // 展示名（如 "Execute shell command"）
	Known       bool   // 是否来自引擎元数据（MCP 工具可能缺失，缺失时 Known=false）
}

// toolApprovalDecision 一次审批的用户决定。
type toolApprovalDecision struct {
	approved bool
	remember bool // 是否在本会话内记住对该工具的允许
}

// pendingToolApproval 等待用户决定的审批请求。
type pendingToolApproval struct {
	tool string
	ch   chan toolApprovalDecision
}

// toolPermissionFor 查询工具的权限元数据（并发安全，未知工具返回 Known=false）。
func (s *Service) toolPermissionFor(name string) toolPermission {
	s.mcpToolsCacheMu.RLock()
	defer s.mcpToolsCacheMu.RUnlock()
	if p, ok := s.mcpToolPerms[name]; ok {
		return p
	}
	return toolPermission{}
}

// classifyToolRisk 按引擎元数据分级（不猜：以 /tools 的 permissions.write 为唯一事实）。
func (s *Service) classifyToolRisk(name string) ToolRisk {
	if name == "search" {
		return ToolRiskSafe
	}
	p := s.toolPermissionFor(name)
	switch {
	case !p.Known:
		return ToolRiskUnknown
	case p.Write:
		return ToolRiskWrite
	default:
		return ToolRiskSafe
	}
}

// toolNeedsApproval 判断工具是否需要用户审批。
// 模式（cfg.AgentApproval）：
//   - "" / "auto"（默认）：只读放行，写操作与未知 MCP 工具需确认；
//   - "always"：所有工具都确认（最严格）；
//   - "never"：全部放行（信任模式，仅供高级用户）。
//
// 会话级允许清单优先于分级：用户勾选"本会话都允许"后同名工具不再重复打扰。
func (s *Service) toolNeedsApproval(name, mode string) (bool, ToolRisk) {
	risk := s.classifyToolRisk(name)
	if s.isToolSessionAllowed(name) {
		return false, risk
	}
	switch mode {
	case "never":
		return false, risk
	case "always":
		return true, ToolRiskAll
	default: // "" / "auto"
		return risk != ToolRiskSafe, risk
	}
}

// isToolSessionAllowed 查询会话级允许清单。
func (s *Service) isToolSessionAllowed(name string) bool {
	s.approvalMu.Lock()
	defer s.approvalMu.Unlock()
	return s.sessionAllowedTools[name]
}

// ResolveToolApproval 解决待审批请求（Wails 绑定，前端审批弹窗回传）。
// toolCallID 关联待决请求；remember=true 时把该工具加入会话级允许清单。
// 请求不存在（已超时/已取消）时返回错误，前端据此刷新弹窗状态。
func (s *Service) ResolveToolApproval(toolCallID string, approved, remember bool) error {
	s.approvalMu.Lock()
	pending, ok := s.pendingApprovals[toolCallID]
	if ok {
		delete(s.pendingApprovals, toolCallID)
	}
	if ok && approved && remember {
		s.sessionAllowedTools[pending.tool] = true
	}
	s.approvalMu.Unlock()

	if !ok {
		return apperror.Newf(apperror.KindInvalidInput, "审批请求 %s 不存在或已过期", toolCallID)
	}
	// 非阻塞投递：channel 缓冲为 1，仅可能出现一个决定
	pending.ch <- toolApprovalDecision{approved: approved, remember: remember}
	log.Info().Str("tool", pending.tool).Str("call_id", toolCallID).
		Bool("approved", approved).Bool("remember", remember).
		Msg("[approval] 工具审批已决定")
	return nil
}

// requestToolApproval 发出审批请求并阻塞等待用户决定。
// 返回 (是否允许, 拒绝原因)。拒绝原因会作为 tool 消息内容回传给模型。
func (s *Service) requestToolApproval(ctx context.Context, convID string, tc llm.ToolCall, risk ToolRisk) (bool, string) {
	p := s.toolPermissionFor(tc.Function.Name)

	s.approvalMu.Lock()
	if s.pendingApprovals == nil {
		s.pendingApprovals = make(map[string]*pendingToolApproval)
	}
	ch := make(chan toolApprovalDecision, 1)
	s.pendingApprovals[tc.ID] = &pendingToolApproval{tool: tc.Function.Name, ch: ch}
	s.approvalMu.Unlock()

	s.emitForConv(convID, EventToolApprovalRequest, ToolApprovalRequestContent{
		ToolCallID:  tc.ID,
		Tool:        tc.Function.Name,
		DisplayName: p.DisplayName,
		Risk:        string(risk),
		Arguments:   tc.Function.Arguments,
	})
	log.Info().Str("tool", tc.Function.Name).Str("call_id", tc.ID).
		Str("risk", string(risk)).Msg("[approval] 等待用户审批工具调用")

	defer func() {
		s.approvalMu.Lock()
		delete(s.pendingApprovals, tc.ID)
		s.approvalMu.Unlock()
	}()

	select {
	case d := <-ch:
		if d.approved {
			return true, ""
		}
		return false, "用户拒绝了本次工具调用。请勿再次调用该工具，改用其他方式回答或直接向用户说明无法完成。"
	case <-time.After(toolApprovalTimeout):
		return false, fmt.Sprintf("工具审批超时（%s 内未响应），已自动拒绝。请直接向用户说明情况。", toolApprovalTimeout)
	case <-ctx.Done():
		return false, "生成已被用户取消"
	}
}

// gateToolCall 执行前的审批门禁：返回 false 时 denyReason 作为 tool 消息内容回传模型。
func (s *Service) gateToolCall(ctx context.Context, convID string, tc llm.ToolCall, cfg *config.Config) (bool, string) {
	mode := ""
	if cfg != nil {
		mode = cfg.AgentApproval
	}
	need, risk := s.toolNeedsApproval(tc.Function.Name, mode)
	if !need {
		return true, ""
	}
	return s.requestToolApproval(ctx, convID, tc, risk)
}

// toolResultOK 依据工具返回内容前缀粗判执行是否成功（仅用于 UI 状态着色）。
func toolResultOK(content string) bool {
	if content == "" {
		return true
	}
	for _, prefix := range []string{"Error:", "搜索超时", "用户拒绝", "工具审批超时"} {
		if strings.HasPrefix(content, prefix) {
			return false
		}
	}
	return true
}

// toolCallMaxRounds 返回 tool call 循环的最大轮次（cfg.AgentMaxRounds 可调，缺省 8，上限 25）。
// 业界 agent 循环以上下文为界或设 10+ 轮；旧值 3 无法完成多步任务。
// 每轮结束仍有上下文 80% 预防性压缩兜底，轮次放宽不会导致溢出失控。
func (s *Service) toolCallMaxRounds(cfg *config.Config) int {
	const (
		defaultRounds = 8
		maxRoundsCap  = 25
	)
	if cfg == nil || cfg.AgentMaxRounds <= 0 {
		return defaultRounds
	}
	return min(cfg.AgentMaxRounds, maxRoundsCap)
}
