// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestModelCapabilities_SerializeNewFields 验证新增能力字段能正确序列化
// 前端需要读取这些字段做 UI 适配
func TestModelCapabilities_SerializeNewFields(t *testing.T) {
	caps := ModelCapabilities{
		SupportsParallelToolCalls: true,
		SupportsSystemRole:        false,
		ToolCallSupport:           true,
	}

	data, err := json.Marshal(caps)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}

	jsonStr := string(data)
	// 验证 JSON 包含新字段
	if !strings.Contains(jsonStr, "supports_parallel_tool_calls") {
		t.Errorf("JSON 缺少 supports_parallel_tool_calls 字段: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, "supports_system_role") {
		t.Errorf("JSON 缺少 supports_system_role 字段: %s", jsonStr)
	}

	// 验证字段值正确
	var decoded ModelCapabilities
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("反序列化失败: %v", err)
	}
	if !decoded.SupportsParallelToolCalls {
		t.Errorf("SupportsParallelToolCalls 应为 true")
	}
	if decoded.SupportsSystemRole {
		t.Errorf("SupportsSystemRole 应为 false")
	}
}

// TestChatCompletionRequest_ParallelToolCallsSerialize 验证 parallel_tool_calls 请求字段序列化
// 场景1：设为 true（支持并发）
// 场景2：设为 false（不支持并发）
// 场景3：nil（不设置，用服务端默认）
func TestChatCompletionRequest_ParallelToolCallsSerialize(t *testing.T) {
	// 场景1：true
	t.Run("true", func(t *testing.T) {
		b := true
		req := ChatCompletionRequest{ParallelToolCalls: &b}
		data, _ := json.Marshal(req)
		if !strings.Contains(string(data), `"parallel_tool_calls":true`) {
			t.Errorf("应为 parallel_tool_calls:true, 实际: %s", data)
		}
	})

	// 场景2：false（必须显式出现在 JSON 里，不能被 omitempty 省略）
	t.Run("false", func(t *testing.T) {
		b := false
		req := ChatCompletionRequest{ParallelToolCalls: &b}
		data, _ := json.Marshal(req)
		if !strings.Contains(string(data), `"parallel_tool_calls":false`) {
			t.Errorf("应为 parallel_tool_calls:false, 实际: %s", data)
		}
	})

	// 场景3：nil（omitempty，不应出现在 JSON 里）
	t.Run("nil", func(t *testing.T) {
		req := ChatCompletionRequest{}
		data, _ := json.Marshal(req)
		if strings.Contains(string(data), "parallel_tool_calls") {
			t.Errorf("nil 时不应出现 parallel_tool_calls, 实际: %s", data)
		}
	})
}

// TestChatCompletionRequest_ToolChoiceSerialize 验证 tool_choice 请求字段序列化
// 场景1：字符串 "required"
// 场景2：字符串 "auto"
// 场景3：不设置（nil/空）
func TestChatCompletionRequest_ToolChoiceSerialize(t *testing.T) {
	// 场景1：required
	t.Run("required", func(t *testing.T) {
		req := ChatCompletionRequest{ToolChoice: "required"}
		data, _ := json.Marshal(req)
		if !strings.Contains(string(data), `"tool_choice":"required"`) {
			t.Errorf("应为 tool_choice:required, 实际: %s", data)
		}
	})

	// 场景2：不设置
	t.Run("empty", func(t *testing.T) {
		req := ChatCompletionRequest{}
		data, _ := json.Marshal(req)
		if strings.Contains(string(data), "tool_choice") {
			t.Errorf("未设置时不应出现 tool_choice, 实际: %s", data)
		}
	})
}
