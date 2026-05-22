package chat

import (
	"testing"
	"douya/internal/store"
)

func TestEstimateTokensByLang(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		minToken int
		maxToken int
	}{
		{"空字符串", "", 0, 0},
		{"中文短句", "你好世界", 2, 4},
		{"中文长句", "这是一个测试句子用于估算Token数量", 9, 12},
		{"英文短句", "Hello world", 2, 4},
		{"英文长句", "This is a test sentence for token estimation", 10, 14},
		{"中英文混合", "Hello 世界", 3, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokensByLang(tt.text, "en")
			if got < tt.minToken || got > tt.maxToken {
				t.Errorf("estimateTokensByLang(%q, en) = %d, want [%d, %d]", tt.text, got, tt.minToken, tt.maxToken)
			}
		})
	}
}

func TestEstimateMessageTokens(t *testing.T) {
	tests := []struct {
		name string
		msg  *store.Message
		want int
	}{
		{"纯文本-中文", &store.Message{Content: "你好世界", Role: "user"}, 3},
		{"纯文本-英文", &store.Message{Content: "Hello world", Role: "user"}, 3},
		{"含ThinkingContent", &store.Message{Content: "你好", ThinkingContent: "我在思考", Role: "assistant"}, 3},
		{"含ToolCalls", &store.Message{Content: "hi", ToolCalls: ` [{"id":"call_1"}]`, Role: "assistant"}, 6},
		{"空消息", &store.Message{Content: "", Role: "user"}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateMessageTokens(tt.msg)
			if got < tt.want-2 || got > tt.want+2 {
				t.Errorf("estimateMessageTokens() = %d, want ~%d (±2)", got, tt.want)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"你好世界", "zh"},
		{"Hello world", "en"},
		{"Hello 世界", "zh"},
		{"123456", "en"},
		{"", "en"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := detectLanguage(tt.text)
			if got != tt.want {
				t.Errorf("detectLanguage(%q) = %s, want %s", tt.text, got, tt.want)
			}
		})
	}
}
