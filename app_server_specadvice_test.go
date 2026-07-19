// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestSmartParamsInfo_SpecAdviceJSON 验证 SmartParamsInfo 的 JSON 序列化包含 spec_advice 字段。
//
// 前端通过 info.spec_advice 是否为 null 判断是否需要弹出推测解码提醒，
// 因此后端必须在 JSON 中输出这个字段（即使为 null）。
//
// 生活类比：像快递柜屏幕上的「取件提醒」区域——即使没有取件提醒，
// 屏幕上也要有这个区域（字段），这样用户知道这里有提醒时可以看。
func TestSmartParamsInfo_SpecAdviceJSON(t *testing.T) {
	info := SmartParamsInfo{}
	// 不设置 SpecAdvice（模拟无需提醒的情况）
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	jsonStr := string(data)
	if !strings.Contains(jsonStr, "spec_advice") {
		t.Errorf("JSON 中应包含 spec_advice 字段（即使为 null），实际: %s", jsonStr)
	}

	// 设置 SpecAdvice（模拟有提醒的情况）
	info.SpecAdvice = &SpecAdviceInfo{
		Sidecar:     "eagle3",
		Desc:        "Eagle3",
		DownloadURL: "https://hf-mirror.com/unsloth/test/tree/main",
		Reason:      "模型支持 Eagle3 推测解码，但未配置 draft 模型",
	}
	data, err = json.Marshal(info)
	if err != nil {
		t.Fatalf("序列化失败: %v", err)
	}
	jsonStr = string(data)
	for _, want := range []string{"spec_advice", "eagle3", "Eagle3", "https://hf-mirror.com", "模型支持 Eagle3"} {
		if !strings.Contains(jsonStr, want) {
			t.Errorf("JSON 中应包含 %q，实际: %s", want, jsonStr)
		}
	}
}
