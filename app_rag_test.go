// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"strings"
	"testing"

	"douya/internal/rag"
)

// newRAGTestApp 构造一个 ragVS 非 nil、serverReady 为 true 的 App，
// 使 UploadDocument 能越过前置检查到达 MIME 校验阶段。
// ragEmbedder 保持 nil，这样通过 MIME 校验的用例会在后续 embedder 检查处返回"知识库未初始化"，
// 从而让我们能区分"被 MIME 校验拦住"与"MIME 校验通过"。
func newRAGTestApp(t *testing.T) *App {
	t.Helper()
	vs, err := rag.NewVectorStore("") // dataDir 为空 → badger 内存模式，无需清理磁盘
	if err != nil {
		t.Fatalf("创建内存 VectorStore 失败: %v", err)
	}
	t.Cleanup(func() { _ = vs.Close() })
	app := NewApp()
	app.ragVS = vs
	app.serverReady.Store(true)
	return app
}

// TestUploadDocument_MIMEValidation 覆盖 4 种组合：mimeType 空/非空 × 扩展名合法/非法。
// 生活类比：就像安检口分别检查"有通行证/无通行证"和"包裹合规/不合规"四种情况。
func TestUploadDocument_MIMEValidation(t *testing.T) {
	// "aGVsbG8=" 是 "hello" 的 base64 编码；此处只测校验阶段，不会被真正解析
	const dummyData = "aGVsbG8="

	tests := []struct {
		name     string
		fileName string
		mimeType string
		wantErr  string
	}{
		{
			// 组合1：mimeType 空 + 扩展名合法 → 兜底推断为 text/plain，通过 MIME 校验，
			// 进入下一阶段因 ragEmbedder 为 nil 返回"知识库未初始化"
			name:     "mimeType空_扩展名合法",
			fileName: "test.txt",
			mimeType: "",
			wantErr:  "知识库未初始化",
		},
		{
			// 组合2：mimeType 非空且合法 + 扩展名合法 → 通过 MIME 校验，进入 embedder nil 阶段
			name:     "mimeType非空_扩展名合法",
			fileName: "test.txt",
			mimeType: "text/plain",
			wantErr:  "知识库未初始化",
		},
		{
			// 组合3：mimeType 空 + 扩展名非法 → 扩展名检查阶段直接拒绝
			name:     "mimeType空_扩展名非法",
			fileName: "test.exe",
			mimeType: "",
			wantErr:  "不支持的文件类型",
		},
		{
			// 组合4：mimeType 非空 + 扩展名非法 → 扩展名检查先于 MIME，仍被拒绝
			name:     "mimeType非空_扩展名非法",
			fileName: "test.exe",
			mimeType: "text/plain",
			wantErr:  "不支持的文件类型",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newRAGTestApp(t)
			err := app.UploadDocument("default", tt.fileName, dummyData, tt.mimeType)
			if err == nil {
				t.Fatal("期望返回错误，实际为 nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("错误信息应包含 %q，实际: %v", tt.wantErr, err)
			}
		})
	}
}

// TestUploadDocument_RejectsInvalidMIME 验证 mimeType 非空但非法时被 MIME 白名单拒绝，
// 而不是因为扩展名问题。这里用合法扩展名 .txt 配一个不在白名单的 MIME。
func TestUploadDocument_RejectsInvalidMIME(t *testing.T) {
	app := newRAGTestApp(t)
	err := app.UploadDocument("default", "test.txt", "aGVsbG8=", "application/x-malware")
	if err == nil {
		t.Fatal("期望返回错误，实际为 nil")
	}
	if !strings.Contains(err.Error(), "不支持的 MIME 类型") {
		t.Errorf("错误信息应包含'不支持的 MIME 类型'，实际: %v", err)
	}
}

// TestUploadDocument_OctetStreamFallback 验证前端在浏览器无法识别类型时传入的
// "application/octet-stream" 占位值也能触发 extToMIME 兜底推断，避免被误拒。
func TestUploadDocument_OctetStreamFallback(t *testing.T) {
	app := newRAGTestApp(t)
	// 前端 f.type 为空时会传 'application/octet-stream'，应被兜底推断为 text/plain 后通过校验
	err := app.UploadDocument("default", "test.txt", "aGVsbG8=", "application/octet-stream")
	if err == nil {
		t.Fatal("期望进入 embedder nil 分支返回错误，实际为 nil")
	}
	// 应通过 MIME 校验（而非被"不支持的 MIME 类型"拦住），最终因 embedder nil 报错
	if strings.Contains(err.Error(), "不支持的 MIME 类型") {
		t.Errorf("application/octet-stream 应触发兜底推断，不应被 MIME 校验拒绝: %v", err)
	}
	if !strings.Contains(err.Error(), "知识库未初始化") {
		t.Errorf("兜底推断通过后应进入 embedder nil 分支，实际: %v", err)
	}
}

// TestExtToMIME_CoversAllAllowedExts 验证 extToMIME 覆盖 allowedDocExts 中所有扩展名，
// 防止出现"扩展名合法但兜底推断失败"的边界情况导致用户上传被误拒。
func TestExtToMIME_CoversAllAllowedExts(t *testing.T) {
	for ext := range allowedDocExts {
		mime, ok := extToMIME[ext]
		if !ok {
			t.Errorf("extToMIME 缺少扩展名 %s 的映射，将导致 mimeType 为空时误拒", ext)
			continue
		}
		// 推断出的 MIME 必须在白名单内，否则推断后仍会被拒
		if !allowedDocMIMETypes[mime] {
			t.Errorf("extToMIME[%s] = %q 不在 allowedDocMIMETypes 中，推断后会被误拒", ext, mime)
		}
	}
}
