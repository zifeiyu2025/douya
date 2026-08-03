// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"encoding/json"

	"douya/internal/apperror"
)

// 通用参数校验辅助函数
// 生活类比：像大楼入口的保安——检查访客证件是否齐全、有效，
// 不齐全的直接拦在门口，不让进大楼内部（业务逻辑）。
//
// 设计说明：所有校验函数返回 error 而非 string，
// 调用方可直接 `if err := validateXxx(...); err != nil { return err }`，
// 避免 fmt.Errorf 传入非常量字符串触发 go vet 警告。

// validateNonEmpty 校验字符串参数非空，空则返回中文错误。
// 参数：
//   - name: 参数的中文名（用于错误消息，如"会话ID"）
//   - value: 待校验的字符串值
//
// 返回：nil 表示通过，非 nil 表示参数错误。
//
// 使用示例：
//
//	if err := validateNonEmpty("会话ID", id); err != nil {
//	    return err
//	}
func validateNonEmpty(name, value string) error {
	if value == "" {
		return apperror.Newf(apperror.KindInvalidInput, "%s不能为空", name)
	}
	return nil
}

// validatePositiveInt 校验整数参数为正数（>=1），否则返回中文错误。
// 生活类比：像电梯按钮——按 0 或负数楼层没意义，必须按 >=1 的楼层。
func validatePositiveInt(name string, value int) error {
	if value < 1 {
		return apperror.Newf(apperror.KindInvalidInput, "%s必须为正整数（当前: %d）", name, value)
	}
	return nil
}

// validateNonNegativeInt 校验整数参数为非负（>=0），否则返回中文错误。
func validateNonNegativeInt(name string, value int) error {
	if value < 0 {
		return apperror.Newf(apperror.KindInvalidInput, "%s不能为负数（当前: %d）", name, value)
	}
	return nil
}

// validateJSONBody 校验 JSON 请求体字符串非空且是合法 JSON。
// 生活类比：像快递员检查包裹——先看外包装是否破损（空字符串），
// 再透视检查里面是否装了合理的东西（合法 JSON 结构）。
// 避免空字符串或非法 JSON 透传到下游导致 panic。
func validateJSONBody(body string) error {
	if body == "" {
		return apperror.New(apperror.KindInvalidInput, "请求体不能为空")
	}
	if !json.Valid([]byte(body)) {
		return apperror.New(apperror.KindInvalidInput, "请求体不是合法的 JSON 格式")
	}
	return nil
}

// validateStringLength 校验字符串长度在指定范围内。
// 生活类比：像填表格——名字太短无法识别，太长超出表格格子。
func validateStringLength(name, value string, maxLen int) error {
	if len(value) > maxLen {
		return apperror.Newf(apperror.KindInvalidInput, "%s长度超过限制（最大 %d 字符，当前 %d 字符）", name, maxLen, len(value))
	}
	return nil
}
