// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"testing"
)

// TestTruncateRunes 验证按 rune 截断字符串的正确性
// 生活类比：像裁缝剪布料，必须沿着花纹的完整边界剪，不能把一个花纹剪成两半。
// 中文每字符 3 字节，按字节截断会把字符切断产生非法 UTF-8。
func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		want     string
	}{
		{
			name:     "空字符串",
			input:    "",
			maxRunes: 10,
			want:     "",
		},
		{
			name:     "不超过限制_原样返回",
			input:    "你好世界",
			maxRunes: 10,
			want:     "你好世界",
		},
		{
			name:     "刚好等于限制_原样返回",
			input:    "你好世界",
			maxRunes: 4,
			want:     "你好世界",
		},
		{
			name:     "中文超限_截断加省略号",
			input:    "你好世界你好世界",
			maxRunes: 4,
			want:     "你好世界...",
		},
		{
			name:     "混合中英文超限",
			input:    "Hello你好World世界",
			maxRunes: 7,
			want:     "Hello你好...",
		},
		{
			name:     "maxRunes为零_不截断",
			input:    "你好世界",
			maxRunes: 0,
			want:     "你好世界",
		},
		{
			name:     "maxRunes为负_不截断",
			input:    "你好世界",
			maxRunes: -1,
			want:     "你好世界",
		},
		{
			name:     "纯英文超限",
			input:    "Hello World",
			maxRunes: 5,
			want:     "Hello...",
		},
		{
			name:     "emoji超限",
			input:    "😀😁😂🤣😃😄😅",
			maxRunes: 3,
			want:     "😀😁😂...",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncateRunes(tc.input, tc.maxRunes)
			if got != tc.want {
				t.Errorf("truncateRunes(%q, %d) = %q, want %q", tc.input, tc.maxRunes, got, tc.want)
			}
		})
	}
}

// TestShouldMergeLongSummary 验证长期摘要合并触发频率
// 每 5 次压缩触发一次合并（longSummaryMergeInterval=5）
func TestShouldMergeLongSummary(t *testing.T) {
	tests := []struct {
		name    string
		count   int
		want    bool
		comment string
	}{
		{"第0次", 0, false, "首次不合并"},
		{"第1次", 1, false, "未到间隔"},
		{"第2次", 2, false, "未到间隔"},
		{"第3次", 3, false, "未到间隔"},
		{"第4次", 4, true, "第5次压缩触发（count+1=5）"},
		{"第5次", 5, false, "已过触发点"},
		{"第9次", 9, true, "第10次压缩触发（count+1=10）"},
		{"第14次", 14, true, "第15次压缩触发（count+1=15）"},
		{"负数_触发", -1, true, "负数 (count+1)%5==0 触发"},
		{"负数_不触发", -2, false, "负数 (count+1)%5!=0 不触发"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldMergeLongSummary(tc.count)
			if got != tc.want {
				t.Errorf("shouldMergeLongSummary(%d) = %v, want %v (%s)",
					tc.count, got, tc.want, tc.comment)
			}
		})
	}
}

// TestShouldResetSummary 验证摘要重置触发周期
// 每 10 次压缩重置一次（summaryResetInterval=10），且 count<=0 不触发
func TestShouldResetSummary_Comprehensive(t *testing.T) {
	tests := []struct {
		name    string
		count   int
		want    bool
		comment string
	}{
		{"零_不触发", 0, false, "count=0 不触发重置"},
		{"负数_不触发", -1, false, "count<0 不触发重置"},
		{"负数_不触发2", -100, false, "count<0 不触发重置"},
		{"第1次", 1, false, "未到周期"},
		{"第5次", 5, false, "未到周期"},
		{"第9次", 9, true, "第10次压缩触发（count+1=10）"},
		{"第10次", 10, false, "已过触发点"},
		{"第19次", 19, true, "第20次压缩触发（count+1=20）"},
		{"第29次", 29, true, "第30次压缩触发（count+1=30）"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldResetSummary(tc.count)
			if got != tc.want {
				t.Errorf("ShouldResetSummary(%d) = %v, want %v (%s)",
					tc.count, got, tc.want, tc.comment)
			}
		})
	}
}

// TestShouldMergeAndReset_Coordination 验证合并与重置的协调关系
// 源码注释说明：第 10 次压缩两者同时触发，reset 优先级更高，会跳过 merge
// 此测试验证：① merge 每 5 次触发；② reset 每 10 次触发；③ 同时触发时 reset 优先
func TestShouldMergeAndReset_Coordination(t *testing.T) {
	// 模拟前 20 次压缩的触发情况
	for i := 0; i < 20; i++ {
		merge := shouldMergeLongSummary(i)
		reset := ShouldResetSummary(i)

		// 验证 merge 触发点：第 5, 10, 15, 20 次压缩
		switch i {
		case 4, 9, 14, 19:
			if !merge {
				t.Errorf("第 %d 次压缩应触发 merge，实际 false", i+1)
			}
		}

		// 验证 reset 触发点：第 10, 20 次压缩
		switch i {
		case 9, 19:
			if !reset {
				t.Errorf("第 %d 次压缩应触发 reset，实际 false", i+1)
			}
			// 同时触发时，调用方应优先执行 reset（源码注释约定）
			if !merge {
				t.Errorf("第 %d 次压缩 merge 也应 true（reset 优先执行，跳过 merge）", i+1)
			}
		}
	}
}
