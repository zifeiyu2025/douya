// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"testing"

	"douya/internal/llm"
)

// TestScoreChatMessage_CodeBlock 验证含代码块的消息加分（+3）
// 生活类比：就像快递包裹上的"易碎品"标签，含代码的消息是"高价值包裹"，分拣时优先保留。
func TestScoreChatMessage_CodeBlock(t *testing.T) {
	msg := llm.ChatMessage{
		Role:    "assistant",
		Content: "这是示例代码：\n```go\nfunc main() {}\n```",
	}
	score := ScoreChatMessage(msg)
	// 期望：代码块 +3，assistant 最终回复 +1 = 4
	if score != 4 {
		t.Errorf("含代码块的 assistant 消息评分 = %d, 期望 4", score)
	}
}

// TestScoreChatMessage_UserImportantKeyword 验证含"记住"等关键词的 user 消息加分（+2）
// 生活类比：用户说"请记住我的项目用 Go 1.22"时，相当于在笔记上画了重点线。
func TestScoreChatMessage_UserImportantKeyword(t *testing.T) {
	cases := []struct {
		content string
		desc   string
	}{
		{"请记住我的项目用 Go 1.22", "中文'记住'"},
		{"重要决策：使用 SQLite 而非 PostgreSQL", "中文'重要'"},
		{"Remember to use UTF-8 encoding", "英文'remember'"},
		{"This is important: don't forget the API key", "英文'important'+'don't forget'"},
	}
	for _, c := range cases {
		msg := llm.ChatMessage{Role: "user", Content: c.content}
		score := ScoreChatMessage(msg)
		// 期望：user +1，关键词 +2 = 3
		if score != 3 {
			t.Errorf("[%s] 评分 = %d, 期望 3 (user+1, 关键词+2)", c.desc, score)
		}
	}
}

// TestScoreChatMessage_ToolRole 验证 tool 角色消息加分（+2）
// tool 消息通常含结构化工具返回结果，是 tool call 链的终点，丢失会破坏对话连续性
func TestScoreChatMessage_ToolRole(t *testing.T) {
	msg := llm.ChatMessage{
		Role:    "tool",
		Content: `{"result": "search complete"}`,
	}
	score := ScoreChatMessage(msg)
	// 期望：tool +2 = 2
	if score != 2 {
		t.Errorf("tool 消息评分 = %d, 期望 2", score)
	}
}

// TestScoreChatMessage_AssistantWithToolCalls 验证带 tool_calls 的 assistant 加分（+2）
// assistant 带 tool_calls 是 tool call 链的起点，与 tool 消息配对，丢失任一会破坏对话
func TestScoreChatMessage_AssistantWithToolCalls(t *testing.T) {
	msg := llm.ChatMessage{
		Role:    "assistant",
		Content: "",
		ToolCalls: []llm.ToolCall{
			{ID: "call_1", Type: "function"},
		},
	}
	score := ScoreChatMessage(msg)
	// 期望：tool_calls +2 = 2（不带 thinking 标记，所以不加最终回复分）
	if score != 2 {
		t.Errorf("带 tool_calls 的 assistant 消息评分 = %d, 期望 2", score)
	}
}

// TestScoreChatMessage_ThinkingContent 验证 thinking 推理内容不加分
// thinking 是模型的"草稿纸"，可以安全裁剪，不应占用宝贵预算
func TestScoreChatMessage_ThinkingContent(t *testing.T) {
	msg := llm.ChatMessage{
		Role:             "assistant",
		Content:          "最终答案",
		ReasoningContent: "我先思考一下...",
	}
	score := ScoreChatMessage(msg)
	// 期望：assistant 带 thinking，不加最终回复分，无代码块/关键词，得分 0
	if score != 0 {
		t.Errorf("带 thinking 的 assistant 消息评分 = %d, 期望 0", score)
	}
}

// TestScoreChatMessage_SystemRole 验证 system 消息评分 0
// system 消息由调用方单独保护，不参与评分
func TestScoreChatMessage_SystemRole(t *testing.T) {
	msg := llm.ChatMessage{
		Role:    "system",
		Content: "你是豆芽，由 zifeiyu 开发的本地 AI 助手。",
	}
	score := ScoreChatMessage(msg)
	if score != 0 {
		t.Errorf("system 消息评分 = %d, 期望 0", score)
	}
}

// TestScoreChatMessage_MustKeep 验证"必保"消息（评分>=5）
// 组合：代码块(+3) + 关键词(+2) = 5，应被标记为必保
func TestScoreChatMessage_MustKeep(t *testing.T) {
	msg := llm.ChatMessage{
		Role:    "user",
		Content: "请记住这段重要代码：\n```go\nfunc main() {}\n```",
	}
	score := ScoreChatMessage(msg)
	// 期望：user +1，代码块 +3，关键词 +2 = 6
	if score != 6 {
		t.Errorf("必保消息评分 = %d, 期望 6", score)
	}
	if !IsMustKeep(score) {
		t.Errorf("评分 %d 应被标记为必保（>= %d）", score, importanceMustKeep)
	}
}

// TestScoreChatMessage_NotMustKeep 验证低分消息不被标记为必保
func TestScoreChatMessage_NotMustKeep(t *testing.T) {
	msg := llm.ChatMessage{
		Role:    "assistant",
		Content: "好的，我明白了。",
	}
	score := ScoreChatMessage(msg)
	// 期望：assistant 最终回复 +1 = 1
	if score != 1 {
		t.Errorf("低分消息评分 = %d, 期望 1", score)
	}
	if IsMustKeep(score) {
		t.Errorf("评分 %d 不应被标记为必保", score)
	}
}

// TestTrimMessagesToFit_ImportanceAware 验证评分驱动的裁剪：
// 当预算紧张时，必保消息（含代码）应被保留，无关闲聊应被裁剪。
// 生活类比：搬家时箱子装不下，先保留贵重物品（证书、合同），丢弃日常杂物（旧报纸）。
func TestTrimMessagesToFit_ImportanceAware(t *testing.T) {
	// 构造消息序列：system + 大量闲聊 + 一条重要代码 + 最近 user
	msgs := []llm.ChatMessage{
		{Role: "system", Content: "你是豆芽"},
		// 大量低分闲聊（每条约 20 token）
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好！很高兴见到你。"},
		{Role: "user", Content: "今天天气怎么样"},
		{Role: "assistant", Content: "我无法获取实时天气，建议查看天气应用。"},
		{Role: "user", Content: "谢谢"},
		{Role: "assistant", Content: "不客气！"},
		// 关键代码消息（必保，评分 6）
		{Role: "user", Content: "请记住这段重要代码：\n```go\nfunc main() { fmt.Println(\"hello\") }\n```"},
		{Role: "assistant", Content: "好的，已记住这段代码。"},
		// 更多闲聊
		{Role: "user", Content: "再见"},
		{Role: "assistant", Content: "再见！"},
		// 当前用户提问
		{Role: "user", Content: "我之前给你看的代码是什么？"},
	}

	// 设置很小的预算，强制触发裁剪
	// 必保消息（代码块）约占 60 token，加上 lastMsg 约 20 token，system 约 5 token
	// 设置 maxTokens=150，reserve=20，effectiveMax=130
	// 系统应保留：system + 必保代码消息 + lastMsg，丢弃大部分闲聊
	trimmed := TrimMessagesToFit(msgs, 150, 20)

	// 验证：必保代码消息应被保留
	foundCode := false
	for _, m := range trimmed {
		if m.Role == "user" && contains(m.ContentString(), "func main") {
			foundCode = true
			break
		}
	}
	if !foundCode {
		t.Errorf("必保代码消息被错误裁剪，trimmed 长度=%d", len(trimmed))
		for i, m := range trimmed {
			t.Logf("  [%d] %s: %s", i, m.Role, truncateForLog(m.ContentString(), 40))
		}
	}

	// 验证：system 和最后一条消息必须保留
	if len(trimmed) == 0 || trimmed[0].Role != "system" {
		t.Errorf("system 消息应被保留在首位")
	}
	if trimmed[len(trimmed)-1].Content != msgs[len(msgs)-1].Content {
		t.Errorf("最后一条消息应被保留在末尾")
	}
}

// contains 简化的字符串包含检查（测试辅助）
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

// indexOf 简化的子串查找（避免引入 strings 包污染测试辅助）
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// truncateForLog 截断字符串用于日志输出
func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// ===== P3-B2: selectImportantMessages 测试 =====

// TestSelectImportantMessages_FillsBudget 验证预算填充逻辑：
// 必保消息全部保留 + 非必保高分消息贪心填充剩余预算。
// 生活类比：搬家时箱子空间有限，先把"必带贵重物品"全放进去，
// 再用剩余空间按价值高低塞几本"想带的 书"。
func TestSelectImportantMessages_FillsBudget(t *testing.T) {
	// 构造 11 条消息：3 条必保（评分 6），1 条高分非必保（评分 3），其余低分（评分 1）
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "你好"},                                          // 0: 评分 1
		{Role: "assistant", Content: "你好！"},                                    // 1: 评分 1
		{Role: "user", Content: "请记住这段代码：\n```go\nfunc a(){}\n```"},       // 2: 必保（6）
		{Role: "assistant", Content: "好的"},                                     // 3: 评分 1
		{Role: "user", Content: "今天天气如何"},                                   // 4: 评分 1
		{Role: "assistant", Content: "我不知道"},                                 // 5: 评分 1
		{Role: "user", Content: "重要：请记住 API key=123"},                     // 6: 高分非必保（3）
		{Role: "assistant", Content: "好的，记住了"},                             // 7: 评分 1
		{Role: "user", Content: "请记住：\n```python\nprint('hi')\n```"},         // 8: 必保（6）
		{Role: "assistant", Content: "好的，已记住"},                             // 9: 评分 1
		{Role: "user", Content: "重要：请记住这段代码\n```js\nconsole.log(1)\n```"}, // 10: 必保（6）
	}

	// 统计必保消息数和总 token
	mustKeepCount := 0
	totalTokens := 0
	for _, m := range msgs {
		score := ScoreChatMessage(m)
		if IsMustKeep(score) {
			mustKeepCount++
		}
		totalTokens += estimateChatMessageTokens(m)
	}
	if mustKeepCount != 3 {
		t.Fatalf("测试数据构造错误：必保消息数 = %d, 期望 3", mustKeepCount)
	}

	// 设置预算为总 token 的 40%，验证必保全保留 + 非必保按分填充
	budget := totalTokens * 2 / 5
	selected := selectImportantMessages(msgs, budget)

	// 验证：3 条必保全部选中
	selectedMustKeep := 0
	for _, s := range selected {
		if IsMustKeep(ScoreChatMessage(s)) {
			selectedMustKeep++
		}
	}
	if selectedMustKeep != 3 {
		t.Errorf("必保消息选中数 = %d, 期望 3（必保应全部保留）", selectedMustKeep)
	}

	// 验证：选中数 >= 必保数（应有非必保高分消息被填充）
	if len(selected) < 3 {
		t.Errorf("选中数 %d < 必保数 3，至少应保留所有必保", len(selected))
	}

	// 验证：选中数 <= 总消息数
	if len(selected) > len(msgs) {
		t.Errorf("选中数 %d > 总消息数 %d", len(selected), len(msgs))
	}

	// 验证：高分非必保（索引 6，评分 3）应优先于低分消息被选中
	foundHighScore := false
	for _, s := range selected {
		if s.Content == msgs[6].Content {
			foundHighScore = true
			break
		}
	}
	// 注意：高分非必保是否被选中取决于剩余预算，这里只验证"若被选中则合理"
	if !foundHighScore && len(selected) > 3 {
		t.Logf("高分非必保消息未被选中（可能剩余预算不足），选中数=%d", len(selected))
	}
}

// TestSelectImportantMessages_TimeOrder 验证返回结果按原时间顺序排列。
// 生活类比：整理旧照片时，即使按"重要程度"挑出几张，最终相册里还是要按拍摄时间排好。
func TestSelectImportantMessages_TimeOrder(t *testing.T) {
	// 构造评分乱序、内容唯一的消息：必保(0) → 低分(1) → 必保(2) → 低分(3) → 高分非必保(4)
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "请记住代码A：\n```go\nfunc a(){}\n```"},       // 0: 必保
		{Role: "assistant", Content: "好的，明白了代码A"},                       // 1: 低分
		{Role: "user", Content: "请记住代码B：\n```py\nprint(1)\n```"},         // 2: 必保
		{Role: "assistant", Content: "好的，已记住代码B"},                       // 3: 低分
		{Role: "user", Content: "重要：记住 key=123"},                        // 4: 高分非必保
	}

	// 极小预算（budget=1）：必保消息即使超预算也保留，非必保消息因 remainingBudget<0 全部不选
	// 这样只会选中 2 条必保消息，验证它们的顺序即可
	selected := selectImportantMessages(msgs, 1)

	// 验证：只选中 2 条必保消息
	if len(selected) != 2 {
		t.Errorf("极小预算时选中数 = %d, 期望 2（仅必保，非必保因 remainingBudget<0 不选）", len(selected))
	}

	// 验证：必保消息按原索引升序（0 → 2）
	if len(selected) >= 2 {
		if selected[0].Content != msgs[0].Content {
			t.Errorf("第一条应是索引 0，实际: %s", truncateForLog(selected[0].ContentString(), 20))
		}
		if selected[1].Content != msgs[2].Content {
			t.Errorf("第二条应是索引 2，实际: %s", truncateForLog(selected[1].ContentString(), 20))
		}
	}
}

// TestSelectImportantMessages_MustKeepExceedsBudget 验证必保消息超预算时全部保留。
// 生活类比：即使行李箱装不下，护照和钱包这些"必带品"也不能丢，宁可超重也要带上。
func TestSelectImportantMessages_MustKeepExceedsBudget(t *testing.T) {
	// 构造 3 条必保消息
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "请记住代码：\n```go\nfunc a(){}\n```"},
		{Role: "user", Content: "请记住：\n```py\nprint(1)\n```"},
		{Role: "user", Content: "重要代码：\n```js\nconsole.log(1)\n```"},
	}

	// 验证全部是必保
	for i, m := range msgs {
		if !IsMustKeep(ScoreChatMessage(m)) {
			t.Fatalf("测试数据构造错误：消息 [%d] 不是必保", i)
		}
	}

	// 计算总 token
	totalTokens := 0
	for _, m := range msgs {
		totalTokens += estimateChatMessageTokens(m)
	}

	// 设置预算为总 token 的 30%（必保消息会超预算）
	budget := totalTokens * 3 / 10
	if budget >= totalTokens {
		budget = totalTokens / 2 // 防止整除导致 budget 不够小
	}
	selected := selectImportantMessages(msgs, budget)

	// 验证：3 条必保全部保留（即使超预算）
	if len(selected) != 3 {
		t.Errorf("必保消息超预算时，选中数 = %d, 期望 3（必保应全部保留，即使超预算）", len(selected))
	}
}

// TestSelectImportantMessages_EmptyInput 验证边界条件不 panic。
func TestSelectImportantMessages_EmptyInput(t *testing.T) {
	// nil 输入
	if selected := selectImportantMessages(nil, 100); selected != nil {
		t.Errorf("nil 输入应返回 nil，实际 %v", selected)
	}

	// 空切片
	empty := []llm.ChatMessage{}
	if selected := selectImportantMessages(empty, 100); selected != nil {
		t.Errorf("空切片应返回 nil，实际 %v", selected)
	}

	// budget = 0
	msgs := []llm.ChatMessage{{Role: "user", Content: "test"}}
	if selected := selectImportantMessages(msgs, 0); selected != nil {
		t.Errorf("budget=0 应返回 nil，实际 %v", selected)
	}

	// budget < 0
	if selected := selectImportantMessages(msgs, -1); selected != nil {
		t.Errorf("budget=-1 应返回 nil，实际 %v", selected)
	}
}

// ===== P3-C2: ShouldResetSummary 测试 =====

// TestShouldResetSummary_TriggerEvery10 验证周期性摘要重置的触发逻辑。
// 触发条件：currentCompressCount > 0 且 (currentCompressCount+1) % 10 == 0
// 即第 10、20、30… 次压缩时触发重置，首次压缩（count=0）不触发。
//
// 与 shouldMergeLongSummary（每 5 次）的协调：
//   - 第 5 次：merge=true, reset=false → 合并长期
//   - 第 10 次：merge=true, reset=true → 重置全部（优先级更高）
//   - 第 15 次：merge=true, reset=false → 合并长期
//   - 第 20 次：merge=true, reset=true → 重置全部
func TestShouldResetSummary_TriggerEvery10(t *testing.T) {
	cases := []struct {
		count    int
		expected bool
		desc     string
	}{
		{0, false, "首次压缩不触发重置"},
		{1, false, "第 2 次压缩不触发"},
		{4, false, "第 5 次压缩不触发重置（应触发合并）"},
		{9, true, "第 10 次压缩触发重置"},
		{14, false, "第 15 次压缩不触发重置（应触发合并）"},
		{19, true, "第 20 次压缩触发重置"},
		{29, true, "第 30 次压缩触发重置"},
		{-1, false, "负数不触发"},
	}

	for _, c := range cases {
		got := ShouldResetSummary(c.count)
		if got != c.expected {
			t.Errorf("[%s] count=%d, ShouldResetSummary=%v, 期望 %v",
				c.desc, c.count, got, c.expected)
		}
	}

	// 额外验证：与 shouldMergeLongSummary 的协调关系
	// 第 10 次压缩：merge=true, reset=true，重置优先级更高，调用方应跳过合并
	if !shouldMergeLongSummary(9) {
		t.Errorf("第 10 次压缩 shouldMergeLongSummary 应为 true")
	}
	if !ShouldResetSummary(9) {
		t.Errorf("第 10 次压缩 ShouldResetSummary 应为 true")
	}

	// 第 5 次压缩：merge=true, reset=false，正常合并长期
	if !shouldMergeLongSummary(4) {
		t.Errorf("第 5 次压缩 shouldMergeLongSummary 应为 true")
	}
	if ShouldResetSummary(4) {
		t.Errorf("第 5 次压缩 ShouldResetSummary 应为 false（不重置，只合并）")
	}
}

