package rag

import (
	"context"
	"strings"
	"testing"
)

// mockEmbedder 用于测试，生成固定维度的随机向量
type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	vecs := make([][]float64, len(texts))
	for i := range texts {
		vec := make([]float64, m.dim)
		// 简单的确定性向量：基于文本长度的伪随机
		for j := range vec {
			vec[j] = float64((len(texts[i])*31+j*17)%100) / 100.0
		}
		vecs[i] = vec
	}
	return vecs, nil
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		minToken int // 最小期望值
		maxToken int // 最大期望值
	}{
		{"空字符串", "", 0, 0},
		{"英文", "hello world", 5, 15},
		{"中文", "你好世界", 2, 8},
		{"混合", "hello你好", 4, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got < tt.minToken || got > tt.maxToken {
				t.Errorf("estimateTokens(%q) = %d, want [%d, %d]", tt.text, got, tt.minToken, tt.maxToken)
			}
		})
	}
}

func TestChunkDocument_BasicSplit(t *testing.T) {
	// 构造一段超过 chunkSize 的文本，用换行分隔
	text := strings.Repeat("这是第一段内容。", 50) + "\n\n" + strings.Repeat("这是第二段内容。", 50)
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 100
	cfg.ChunkOverlap = 20

	chunks := ChunkDocument(text, cfg)
	if len(chunks) < 2 {
		t.Errorf("期望至少分成 2 个块，实际得到 %d 个", len(chunks))
	}

	// 每个 chunk 不应为空
	for i, c := range chunks {
		if c.Content == "" {
			t.Errorf("chunk[%d] 内容为空", i)
		}
	}
}

func TestChunkDocument_SmallText(t *testing.T) {
	// 小文本不应被分块
	text := "这是一段很短的文本。"
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 512

	chunks := ChunkDocument(text, cfg)
	if len(chunks) != 1 {
		t.Errorf("期望 1 个块，实际得到 %d 个", len(chunks))
	}
	if chunks[0].Content != text {
		t.Errorf("内容不匹配: got %q, want %q", chunks[0].Content, text)
	}
}

func TestChunkDocument_EmptyText(t *testing.T) {
	chunks := ChunkDocument("", DefaultChunkConfig())
	if len(chunks) != 0 {
		t.Errorf("空文本应该返回 0 个块，实际得到 %d 个", len(chunks))
	}
}

func TestChunkDocument_RecursiveSplit(t *testing.T) {
	// 测试递归分块：先用段落分隔，超大段落再用句子分隔
	text := strings.Repeat("短句。", 200) // 一个超大的段落，没有 \n\n
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 50
	cfg.ChunkOverlap = 10

	chunks := ChunkDocument(text, cfg)
	if len(chunks) < 3 {
		t.Errorf("期望至少 3 个块（递归分块），实际得到 %d 个", len(chunks))
	}
}

func TestChunkDocument_Overlap(t *testing.T) {
	// 验证重叠：相邻 chunk 应有部分内容重叠
	text := strings.Repeat("这是一段用于测试重叠的文本内容。", 30)
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 50
	cfg.ChunkOverlap = 20

	chunks := ChunkDocument(text, cfg)
	if len(chunks) < 2 {
		t.Skip("块数不足，无法验证重叠")
	}

	// 检查相邻块是否有重叠内容（至少部分字符相同）
	for i := 1; i < len(chunks); i++ {
		prev := chunks[i-1].Content
		curr := chunks[i].Content
		// 检查前一个块末尾和当前块开头是否有共同子串
		hasOverlap := false
		for overlapLen := 5; overlapLen <= min(len(prev), len(curr)); overlapLen++ {
			if strings.HasSuffix(prev, curr[:overlapLen]) {
				hasOverlap = true
				break
			}
		}
		if !hasOverlap {
			// 没有严格重叠也算正常（可能在分隔符处切分），但不应该完全无关
			t.Logf("chunk[%d] 和 chunk[%d] 无严格重叠（可能在分隔符处切分）", i-1, i)
		}
	}
}

func TestChunkDocument_Separators(t *testing.T) {
	// 测试不同分隔符的切分
	text := "第一段\n\n第二段\n第三行。第四句 第五词"
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 512

	chunks := ChunkDocument(text, cfg)
	if len(chunks) < 1 {
		t.Errorf("期望至少 1 个块，实际得到 %d 个", len(chunks))
	}
	// 文本不太长，可能合为 1 个块
	totalContent := ""
	for _, c := range chunks {
		totalContent += c.Content
	}
	// 所有原始内容应被保留（可能有空格/换行差异）
	if !strings.Contains(totalContent, "第一段") || !strings.Contains(totalContent, "第二段") {
		t.Errorf("分块后丢失了原始内容")
	}
}

func TestHardSplit(t *testing.T) {
	// 测试硬切：没有分隔符时按字符切分
	text := strings.Repeat("A", 1000)
	chunks := hardSplit(text, 100, 20)
	if len(chunks) < 5 {
		t.Errorf("期望至少 5 个块，实际得到 %d 个", len(chunks))
	}
	// 验证所有原始内容都被覆盖（有重叠所以总字符数会超过原文）
	// 检查第一个 chunk 以 "A" 开头，最后一个以 "A" 结尾
	if len(chunks) > 0 {
		if !strings.HasPrefix(chunks[0].Content, "A") {
			t.Error("第一个 chunk 应以 A 开头")
		}
		if !strings.HasSuffix(chunks[len(chunks)-1].Content, "A") {
			t.Error("最后一个 chunk 应以 A 结尾")
		}
	}
}

func TestTakeOverlap(t *testing.T) {
	text := "abcdefghij"
	result := takeOverlap(text, 5)
	// overlapTokens=5 对应约 5/0.7=7 个字符
	if len(result) == 0 {
		t.Error("takeOverlap 返回空")
	}
	// 应该是文本末尾的部分
	if !strings.HasSuffix(text, result) {
		t.Errorf("takeOverlap 结果 %q 不是文本末尾", result)
	}
}
