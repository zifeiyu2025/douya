// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"douya/internal/llm"
)

// TestOperateSlot_RejectsInvalidAction 验证非法 action 被白名单拦截。
// 生活类比：电梯只认"上/下/停"，按了"飞"这个键应该被拒绝，而不是真去执行飞。
func TestOperateSlot_RejectsInvalidAction(t *testing.T) {
	app := NewApp()
	// 白名单校验在 client nil 检查之前，即便 client 未初始化也能拦住非法 action
	err := app.operateSlot(0, "delete", "")
	if err == nil {
		t.Fatal("期望非法 action 返回错误，实际为 nil")
	}
	if !strings.Contains(err.Error(), "非法操作") {
		t.Errorf("错误信息应包含'非法操作'，实际: %v", err)
	}
	if !strings.Contains(err.Error(), "delete") {
		t.Errorf("错误信息应包含传入的 action 'delete'，实际: %v", err)
	}
}

// TestOperateSlot_RejectsEmptyAction 验证空 action 被拒绝
func TestOperateSlot_RejectsEmptyAction(t *testing.T) {
	app := NewApp()
	if err := app.operateSlot(0, "", ""); err == nil {
		t.Fatal("期望空 action 返回错误，实际为 nil")
	}
}

// TestOperateSlot_RejectsInjectionLikeAction 验证含 URL 特殊字符的 action 被白名单拦截，
// 不会进入 URL 拼接逻辑造成参数注入
func TestOperateSlot_RejectsInjectionLikeAction(t *testing.T) {
	app := NewApp()
	injections := []string{
		"save&extra=1",
		"save#fragment",
		"save%20evil",
		"erase/../../",
	}
	for _, action := range injections {
		if err := app.operateSlot(0, action, ""); err == nil {
			t.Errorf("期望注入式 action %q 被拒绝，实际放行", action)
		}
	}
}

// TestOperateSlot_AcceptsLegalActions 验证合法 action 通过白名单后能正常发起请求，
// 并通过 mock HTTP 服务器验证 URL 路径与 action 参数被正确传递
func TestOperateSlot_AcceptsLegalActions(t *testing.T) {
	var capturedPath string
	var capturedAction string
	var capturedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedAction = r.URL.Query().Get("action")
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			capturedBody = string(b)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	app := NewApp()
	app.client = llm.NewClient(srv.URL, "")

	for _, action := range []string{"save", "restore", "erase"} {
		capturedPath = ""
		capturedAction = ""
		capturedBody = ""
		if err := app.operateSlot(1, action, "conv-1"); err != nil {
			t.Errorf("合法 action %s 不应报错，实际: %v", action, err)
			continue
		}
		if capturedPath != "/slots/1" {
			t.Errorf("action=%s 请求路径应为 /slots/1，实际: %s", action, capturedPath)
		}
		if capturedAction != action {
			t.Errorf("action=%s 服务端收到的 action 参数为 %q", action, capturedAction)
		}
		// save/restore 必须携带 filename 请求体，否则新版 llama-server 会因解析空输入返回 500
		if action == "save" || action == "restore" {
			if !strings.Contains(capturedBody, `"filename"`) || !strings.Contains(capturedBody, "conv-1") {
				t.Errorf("action=%s 请求体应包含 filename=conv-1，实际: %q", action, capturedBody)
			}
		} else if capturedBody != "" {
			t.Errorf("action=%s 不应携带请求体，实际: %q", action, capturedBody)
		}
	}
}

// TestOperateSlot_URLParameterEscaped 验证 URL 查询参数使用 url.Values.Encode 进行转义。
// 通过检查 RawQuery 确认参数经过标准编码（合法 action 也会被编码为 action=xxx 形式）
func TestOperateSlot_URLParameterEscaped(t *testing.T) {
	var capturedRawQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedRawQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	app := NewApp()
	app.client = llm.NewClient(srv.URL, "")
	if err := app.operateSlot(2, "save", "conv-2"); err != nil {
		t.Fatalf("save 不应报错: %v", err)
	}
	// url.Values.Encode 产出 "action=save" 形式（key 按字典序，值经过转义）
	if capturedRawQuery != "action=save" {
		t.Errorf("RawQuery 应为 'action=save'（url.Values.Encode 产出），实际: %q", capturedRawQuery)
	}
}
