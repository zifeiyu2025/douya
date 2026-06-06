package main

import "testing"

// TestMaskAPIKey 测试 API Key 掩码函数
func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name     string // 测试用例名称
		input    string // 输入的 API Key
		expected string // 期望的掩码结果
	}{
		{
			name:     "空字符串返回空字符串",
			input:    "",
			expected: "",
		},
		{
			name:     "长度1返回星号",
			input:    "a",
			expected: "****",
		},
		{
			name:     "长度4返回星号",
			input:    "abcd",
			expected: "****",
		},
		{
			name:     "刚好5个字符返回星号加末4位",
			input:    "abcde",
			expected: "****bcde",
		},
		{
			name:     "正常长度返回星号加末4位",
			input:    "sk-1234567890abcdef",
			expected: "****cdef",
		},
		{
			name:     "长度6返回星号加末4位",
			input:    "abcdef",
			expected: "****cdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskAPIKey(tt.input)
			if result != tt.expected {
				t.Errorf("maskAPIKey(%q) = %q, 期望 %q", tt.input, result, tt.expected)
			}
		})
	}
}

// TestIsMaskedValue 测试掩码值检测函数
func TestIsMaskedValue(t *testing.T) {
	tests := []struct {
		name     string // 测试用例名称
		input    string // 输入的字符串
		expected bool   // 期望的检测结果
	}{
		{
			name:     "掩码值加后缀返回true",
			input:    "****abcd",
			expected: true,
		},
		{
			name:     "纯星号返回true",
			input:    "****",
			expected: true,
		},
		{
			name:     "普通字符串返回false",
			input:    "abc",
			expected: false,
		},
		{
			name:     "不带星号前缀的字符串返回false",
			input:    "abcd1234",
			expected: false,
		},
		{
			name:     "空字符串返回false",
			input:    "",
			expected: false,
		},
		{
			name:     "三个星号返回false",
			input:    "***",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMaskedValue(tt.input)
			if result != tt.expected {
				t.Errorf("isMaskedValue(%q) = %v, 期望 %v", tt.input, result, tt.expected)
			}
		})
	}
}
