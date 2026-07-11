// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestMergeSearchJSON_BothEmpty 验证两个空输入返回空数组 "[]"
//
// 生活类比：就像合并两个空文件夹，结果还是一个空文件夹，但必须存在（返回 "[]"）。
func TestMergeSearchJSON_BothEmpty(t *testing.T) {
	got := MergeSearchJSON("", "")
	if got != "[]" {
		t.Errorf("两个空输入应返回 '[]'，实际: %q", got)
	}
}

// TestMergeSearchJSON_ExistingEmpty 验证 existing 为空时直接返回 new
func TestMergeSearchJSON_ExistingEmpty(t *testing.T) {
	newResults := `[{"title":"新结果","url":"http://example.com","snippet":"内容"}]`
	got := MergeSearchJSON("", newResults)
	if got != newResults {
		t.Errorf("existing 为空应直接返回 new，实际: %q", got)
	}
}

// TestMergeSearchJSON_NewEmpty 验证 new 为空时保持 existing 不变
func TestMergeSearchJSON_NewEmpty(t *testing.T) {
	existing := `[{"title":"已有结果","url":"http://old.com","snippet":"旧内容"}]`
	got := MergeSearchJSON(existing, "")
	if got != existing {
		t.Errorf("new 为空应保持 existing 不变，实际: %q", got)
	}
}

// TestMergeSearchJSON_BothValid 验证两个有效 JSON 数组合并
func TestMergeSearchJSON_BothValid(t *testing.T) {
	existing := `[{"title":"结果1","url":"http://1.com","snippet":"内容1"}]`
	newResults := `[{"title":"结果2","url":"http://2.com","snippet":"内容2"}]`
	got := MergeSearchJSON(existing, newResults)

	// 应能反序列化为包含 2 个结果的数组
	var merged []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Snippet string `json:"snippet"`
	}
	if err := json.Unmarshal([]byte(got), &merged); err != nil {
		t.Fatalf("合并结果应为有效 JSON，反序列化失败: %v", err)
	}
	if len(merged) != 2 {
		t.Errorf("合并后应有 2 个结果，实际: %d", len(merged))
	}
}

// TestMergeSearchJSON_BothInvalid 验证两个无效 JSON 返回 "[]"
func TestMergeSearchJSON_BothInvalid(t *testing.T) {
	got := MergeSearchJSON("invalid json", "also invalid")
	if got != "[]" {
		t.Errorf("两个无效 JSON 应返回 '[]'，实际: %q", got)
	}
}

// TestMergeSearchJSON_ExistingInvalid 验证 existing 无效时返回 new
func TestMergeSearchJSON_ExistingInvalid(t *testing.T) {
	newResults := `[{"title":"有效结果","url":"http://new.com","snippet":"新内容"}]`
	got := MergeSearchJSON("invalid existing", newResults)
	if got != newResults {
		t.Errorf("existing 无效应返回 new，实际: %q", got)
	}
}

// TestMergeSearchJSON_NewInvalid 验证 new 无效时返回 existing
func TestMergeSearchJSON_NewInvalid(t *testing.T) {
	existing := `[{"title":"已有结果","url":"http://old.com","snippet":"旧内容"}]`
	got := MergeSearchJSON(existing, "invalid new")
	if got != existing {
		t.Errorf("new 无效应返回 existing，实际: %q", got)
	}
}

// TestTruncateAttachmentText_ShortText 验证短文本不截断
func TestTruncateAttachmentText_ShortText(t *testing.T) {
	text := "这是一段短文本，不需要截断。"
	got := truncateAttachmentText(text, "test.txt")
	if got != text {
		t.Errorf("短文本不应被截断，期望 %q，实际: %q", text, got)
	}
}

// TestTruncateAttachmentText_LongText 验证长文本被截断
func TestTruncateAttachmentText_LongText(t *testing.T) {
	// 构造超过 maxAttachmentTextRunes (24000) 的文本
	longText := strings.Repeat("a", 30000)
	got := truncateAttachmentText(longText, "big.txt")

	// 截断后应比原文短
	if len(got) >= len(longText) {
		t.Errorf("长文本应被截断，截断后长度 %d >= 原文长度 %d", len(got), len(longText))
	}

	// 应包含截断提示
	if !strings.Contains(got, "已截断") {
		t.Errorf("截断文本应包含 '已截断' 提示，实际: %q", got)
	}
	if !strings.Contains(got, "big.txt") {
		t.Errorf("截断文本应包含文件名 'big.txt'，实际: %q", got)
	}
}

// TestTruncateAttachmentText_Boundary 验证刚好等于阈值的文本不截断
func TestTruncateAttachmentText_Boundary(t *testing.T) {
	// 刚好 24000 字符，不应截断
	text := strings.Repeat("a", 24000)
	got := truncateAttachmentText(text, "boundary.txt")
	if got != text {
		t.Errorf("刚好等于阈值的文本不应被截断")
	}

	// 超过 1 字符，应截断
	text = strings.Repeat("a", 24001)
	got = truncateAttachmentText(text, "over.txt")
	if got == text {
		t.Errorf("超过阈值的文本应被截断")
	}
}

// TestTruncateAttachmentText_PreservesHeadAndTail 验证截断后保留开头和结尾
func TestTruncateAttachmentText_PreservesHeadAndTail(t *testing.T) {
	// 构造有标识的开头和结尾的长文本
	head := "这是开头的标识内容"
	tail := "这是结尾的标识内容"
	middle := strings.Repeat("中", 30000)
	longText := head + middle + tail

	got := truncateAttachmentText(longText, "test.txt")

	if !strings.Contains(got, head) {
		t.Errorf("截断后应保留开头内容 %q", head)
	}
	if !strings.Contains(got, tail) {
		t.Errorf("截断后应保留结尾内容 %q", tail)
	}
}
