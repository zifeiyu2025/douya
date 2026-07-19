// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"testing"
)

// TestBuildSidecarURL 测试为 Eagle3/DFlash 推测解码的 sidecar 模型构造 hf-mirror.com 下载链接。
//
// 豆芽不直接调用 HuggingFace API 下载文件，而是在检测到当前模型支持推测解码
// 但未配置 draft 模型时，生成一个 hf-mirror.com 链接（国内镜像加速），
// 引导用户前往浏览器手动下载对应的 eagle3-/dflash- 文件。
//
// 生活类比：像快递柜的「取件码页面」——豆芽不帮你送货上门（不下载文件），
// 但给你一张写有地址的便签（链接），让你自己去快递柜取（浏览器下载）。
func TestBuildSidecarURL(t *testing.T) {
	tests := []struct {
		name     string
		hfRepo   string // GGUF 元数据中的 general.source.huggingface.repository
		arch     string // 模型架构（用于无 HF repo 时的搜索兜底）
		sidecar  string // "eagle3" 或 "dflash"
		wantURL  string
		wantDesc string
	}{
		{
			name:     "有 HF repo + Eagle3",
			hfRepo:   "unsloth/Qwen3.5-7B-Instruct-GGUF",
			arch:     "qwen3.5",
			sidecar:  "eagle3",
			wantURL:  "https://hf-mirror.com/unsloth/Qwen3.5-7B-Instruct-GGUF/tree/main",
			wantDesc: "Eagle3",
		},
		{
			name:     "有 HF repo + DFlash",
			hfRepo:   "Qwen/Qwen3.6-UD-GGUF",
			arch:     "qwen3.6",
			sidecar:  "dflash",
			wantURL:  "https://hf-mirror.com/Qwen/Qwen3.6-UD-GGUF/tree/main",
			wantDesc: "DFlash",
		},
		{
			name:     "无 HF repo 时用搜索兜底",
			hfRepo:   "",
			arch:     "qwen3.5",
			sidecar:  "eagle3",
			wantURL:  "https://hf-mirror.com/search?search_keyword=qwen3.5+eagle3",
			wantDesc: "Eagle3",
		},
		{
			name:     "无 HF repo + DFlash 搜索",
			hfRepo:   "",
			arch:     "qwen3.6",
			sidecar:  "dflash",
			wantURL:  "https://hf-mirror.com/search?search_keyword=qwen3.6+dflash",
			wantDesc: "DFlash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL, gotDesc := BuildSidecarURL(tt.hfRepo, tt.arch, tt.sidecar)
			if gotURL != tt.wantURL {
				t.Errorf("URL 期望 %q，实际 %q", tt.wantURL, gotURL)
			}
			if gotDesc != tt.wantDesc {
				t.Errorf("Desc 期望 %q，实际 %q", tt.wantDesc, gotDesc)
			}
		})
	}
}

// TestBuildSidecarURL_InvalidSidecar 验证传入不支持的 sidecar 类型会返回空链接。
//
// 安全实践：函数只支持 "eagle3" 和 "dflash"，其他输入应返回空串而非生成错误链接，
// 避免前端误把无效链接展示给用户。
func TestBuildSidecarURL_InvalidSidecar(t *testing.T) {
	gotURL, gotDesc := BuildSidecarURL("unsloth/test", "qwen3", "unknown")
	if gotURL != "" {
		t.Errorf("不支持的 sidecar 应返回空 URL，实际 %q", gotURL)
	}
	if gotDesc != "" {
		t.Errorf("不支持的 sidecar 应返回空 Desc，实际 %q", gotDesc)
	}
}

// TestEvaluateSpecAdvice 测试推测解码建议的触发逻辑。
//
// 覆盖所有关键路径：开关关闭、模型不支持、已配置、需要提醒。
func TestEvaluateSpecAdvice(t *testing.T) {
	tests := []struct {
		name            string
		supportsEagle3  bool
		hfRepo          string
		arch            string
		specDraftModel  string
		adviceEnabled   bool
		wantAdvice      bool // 是否期望返回非空建议
		wantDownloadURL string
		wantReason      string
	}{
		{
			name:            "支持 + 未配置 + 开关开 → 应提醒",
			supportsEagle3:  true,
			hfRepo:          "unsloth/Qwen3.5-7B-Instruct-GGUF",
			arch:            "qwen3.5",
			specDraftModel:  "",
			adviceEnabled:   true,
			wantAdvice:      true,
			wantDownloadURL: "https://hf-mirror.com/unsloth/Qwen3.5-7B-Instruct-GGUF/tree/main",
			wantReason:      "模型支持 Eagle3 推测解码，但未配置 draft 模型",
		},
		{
			name:            "开关关闭 → 不提醒（尊重用户选择）",
			supportsEagle3:  true,
			hfRepo:          "unsloth/test",
			arch:            "qwen3.5",
			specDraftModel:  "",
			adviceEnabled:   false,
			wantAdvice:      false,
		},
		{
			name:            "模型不支持 → 不提醒",
			supportsEagle3:  false,
			hfRepo:          "",
			arch:            "qwen3",
			specDraftModel:  "",
			adviceEnabled:   true,
			wantAdvice:      false,
		},
		{
			name:            "已配置 draft 模型 → 不打扰已配置用户",
			supportsEagle3:  true,
			hfRepo:          "unsloth/test",
			arch:            "qwen3.5",
			specDraftModel:  "/path/to/draft.gguf",
			adviceEnabled:   true,
			wantAdvice:      false,
		},
		{
			name:            "无 HF repo → 用搜索兜底链接",
			supportsEagle3:  true,
			hfRepo:          "",
			arch:            "qwen3.5",
			specDraftModel:  "",
			adviceEnabled:   true,
			wantAdvice:      true,
			wantDownloadURL: "https://hf-mirror.com/search?search_keyword=qwen3.5+eagle3",
			wantReason:      "模型支持 Eagle3 推测解码，但未配置 draft 模型",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			advice := EvaluateSpecAdvice(tt.supportsEagle3, tt.hfRepo, tt.arch, tt.specDraftModel, tt.adviceEnabled)
			if tt.wantAdvice {
				if advice == nil {
					t.Fatal("期望返回建议，实际返回 nil")
				}
				if advice.DownloadURL != tt.wantDownloadURL {
					t.Errorf("DownloadURL 期望 %q，实际 %q", tt.wantDownloadURL, advice.DownloadURL)
				}
				if advice.Reason != tt.wantReason {
					t.Errorf("Reason 期望 %q，实际 %q", tt.wantReason, advice.Reason)
				}
			} else {
				if advice != nil {
					t.Errorf("期望返回 nil，实际返回 %+v", advice)
				}
			}
		})
	}
}
