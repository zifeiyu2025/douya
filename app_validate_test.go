// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"testing"
)

// TestValidateNonEmpty 验证非空字符串校验
func TestValidateNonEmpty(t *testing.T) {
	cases := []struct {
		name        string
		param       string
		value       string
		expectError bool
		expectMsg   string
	}{
		{"空字符串应返回错误", "会话ID", "", true, "InvalidInput: 会话ID不能为空"},
		{"非空字符串应通过", "会话ID", "abc-123", false, ""},
		{"空白字符串也算非空（不在此校验 trim）", "标题", " ", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateNonEmpty(c.param, c.value)
			if c.expectError {
				if err == nil {
					t.Errorf("validateNonEmpty(%q, %q) 期望返回错误，实际返回 nil", c.param, c.value)
					return
				}
				if err.Error() != c.expectMsg {
					t.Errorf("validateNonEmpty(%q, %q) 错误消息 = %q, 期望 %q", c.param, c.value, err.Error(), c.expectMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateNonEmpty(%q, %q) 期望返回 nil，实际返回 %v", c.param, c.value, err)
				}
			}
		})
	}
}

// TestValidatePositiveInt 验证正整数校验
func TestValidatePositiveInt(t *testing.T) {
	cases := []struct {
		name        string
		param       string
		value       int
		expectError bool
		expectMsg   string
	}{
		{"负数应返回错误", "终端列数", -1, true, "InvalidInput: 终端列数必须为正整数（当前: -1）"},
		{"零应返回错误", "终端行数", 0, true, "InvalidInput: 终端行数必须为正整数（当前: 0）"},
		{"正数应通过", "终端列数", 80, false, ""},
		{"大正数应通过", "终端行数", 1000, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validatePositiveInt(c.param, c.value)
			if c.expectError {
				if err == nil {
					t.Errorf("validatePositiveInt(%q, %d) 期望返回错误，实际返回 nil", c.param, c.value)
					return
				}
				if err.Error() != c.expectMsg {
					t.Errorf("validatePositiveInt(%q, %d) 错误消息 = %q, 期望 %q", c.param, c.value, err.Error(), c.expectMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validatePositiveInt(%q, %d) 期望返回 nil，实际返回 %v", c.param, c.value, err)
				}
			}
		})
	}
}

// TestValidateNonNegativeInt 验证非负整数校验
func TestValidateNonNegativeInt(t *testing.T) {
	cases := []struct {
		name        string
		param       string
		value       int
		expectError bool
		expectMsg   string
	}{
		{"负数应返回错误", "slot ID", -1, true, "InvalidInput: slot ID不能为负数（当前: -1）"},
		{"零应通过", "slot ID", 0, false, ""},
		{"正数应通过", "slot ID", 5, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateNonNegativeInt(c.param, c.value)
			if c.expectError {
				if err == nil {
					t.Errorf("validateNonNegativeInt(%q, %d) 期望返回错误，实际返回 nil", c.param, c.value)
					return
				}
				if err.Error() != c.expectMsg {
					t.Errorf("validateNonNegativeInt(%q, %d) 错误消息 = %q, 期望 %q", c.param, c.value, err.Error(), c.expectMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateNonNegativeInt(%q, %d) 期望返回 nil，实际返回 %v", c.param, c.value, err)
				}
			}
		})
	}
}

// TestValidateJSONBody 验证 JSON 请求体校验
func TestValidateJSONBody(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		expectError bool
		expectMsg   string
	}{
		{"空字符串应返回错误", "", true, "InvalidInput: 请求体不能为空"},
		{"非法 JSON 应返回错误", "{not json", true, "InvalidInput: 请求体不是合法的 JSON 格式"},
		{"合法 JSON 对象应通过", `{"key":"value"}`, false, ""},
		{"合法 JSON 数组应通过", `[1,2,3]`, false, ""},
		{"合法 JSON 字符串应通过", `"hello"`, false, ""},
		{"合法 JSON 数字应通过", `42`, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateJSONBody(c.body)
			if c.expectError {
				if err == nil {
					t.Errorf("validateJSONBody(%q) 期望返回错误，实际返回 nil", c.body)
					return
				}
				if err.Error() != c.expectMsg {
					t.Errorf("validateJSONBody(%q) 错误消息 = %q, 期望 %q", c.body, err.Error(), c.expectMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateJSONBody(%q) 期望返回 nil，实际返回 %v", c.body, err)
				}
			}
		})
	}
}

// TestValidateStringLength 验证字符串长度校验
func TestValidateStringLength(t *testing.T) {
	cases := []struct {
		name        string
		param       string
		value       string
		maxLen      int
		expectError bool
		expectMsg   string
	}{
		{"短字符串应通过", "API Key", "abc123", 256, false, ""},
		{"恰好达到上限应通过", "API Key", "abc", 3, false, ""},
		{"超过上限应返回错误", "API Key", "abcd", 3, true, "InvalidInput: API Key长度超过限制（最大 3 字符，当前 4 字符）"},
		{"空字符串应通过", "API Key", "", 10, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateStringLength(c.param, c.value, c.maxLen)
			if c.expectError {
				if err == nil {
					t.Errorf("validateStringLength(%q, %q, %d) 期望返回错误，实际返回 nil", c.param, c.value, c.maxLen)
					return
				}
				if err.Error() != c.expectMsg {
					t.Errorf("validateStringLength(%q, %q, %d) 错误消息 = %q, 期望 %q", c.param, c.value, c.maxLen, err.Error(), c.expectMsg)
				}
			} else {
				if err != nil {
					t.Errorf("validateStringLength(%q, %q, %d) 期望返回 nil，实际返回 %v", c.param, c.value, c.maxLen, err)
				}
			}
		})
	}
}
