// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// TestReleaseHasBinaryAsset 验证 release 二进制资产判断。
// llama.cpp 2026-08-21 起稳定版（vX.Y.Z）release 只含 nightly-tag.txt 指针文件，
// nightly（bXXXXX）release 含 *.zip 二进制包。
func TestReleaseHasBinaryAsset(t *testing.T) {
	tests := []struct {
		name    string
		assets  []string
		hasBinary bool
	}{
		{
			name:    "nightly 含 zip 二进制",
			assets:  []string{"llama-b10567-bin-win-cuda-13.3-x64.zip", "cudart-llama-bin-win-cuda-13.3-x64.zip"},
			hasBinary: true,
		},
		{
			name:    "稳定版只有 nightly-tag.txt 指针",
			assets:  []string{"nightly-tag.txt"},
			hasBinary: false,
		},
		{
			name:    "空资产列表",
			assets:  nil,
			hasBinary: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			release := &GitHubRelease{TagName: "b10567"}
			for _, n := range tt.assets {
				release.Assets = append(release.Assets, GitHubAsset{Name: n})
			}
			if got := releaseHasBinaryAsset(release); got != tt.hasBinary {
				t.Errorf("releaseHasBinaryAsset(%v) = %v, 期望 %v", tt.assets, got, tt.hasBinary)
			}
		})
	}
}

// TestFetchLatestBinaryReleaseNewMode 验证新模式回退：
// releases/latest 是 v0.2.0 稳定版（只有 nightly-tag.txt 指针），
// 应改查 release 列表，从新到旧找到第一个含二进制的 nightly（b10567）。
func TestFetchLatestBinaryReleaseNewMode(t *testing.T) {
	// 列表按 GitHub API 顺序从新到旧：v0.2.0（指针）→ b10567（含二进制）→ b10566（含二进制）
	listJSON := `[
		{"tag_name": "v0.2.0", "assets": [{"name": "nightly-tag.txt", "browser_download_url": "https://example.com/nightly-tag.txt", "size": 8}]},
		{"tag_name": "b10567", "assets": [{"name": "llama-b10567-bin-win-cuda-13.3-x64.zip", "browser_download_url": "https://example.com/llama-b10567-bin-win-cuda-13.3-x64.zip", "size": 314572800}]},
		{"tag_name": "b10566", "assets": [{"name": "llama-b10566-bin-win-cuda-13.3-x64.zip", "browser_download_url": "https://example.com/llama-b10566-bin-win-cuda-13.3-x64.zip", "size": 314572700}]}
	]`
	latestJSON := `{"tag_name": "v0.2.0", "assets": [{"name": "nightly-tag.txt", "browser_download_url": "https://example.com/nightly-tag.txt", "size": 8}]}`

	listSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(listJSON))
	}))
	defer listSrv.Close()
	latestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(latestJSON))
	}))
	defer latestSrv.Close()

	release, err := fetchLatestBinaryRelease(latestSrv.URL, listSrv.URL)
	if err != nil {
		t.Fatalf("fetchLatestBinaryRelease 失败: %v", err)
	}
	if release.TagName != "b10567" {
		t.Errorf("回退后 tag = %q, 期望 %q（应跳过 v0.2.0 指针取最新 nightly）", release.TagName, "b10567")
	}
	if len(release.Assets) == 0 || release.Assets[0].Name != "llama-b10567-bin-win-cuda-13.3-x64.zip" {
		t.Errorf("回退后 assets 不正确: %+v", release.Assets)
	}
}

// TestFetchLatestBinaryReleaseOldMode 验证旧模式直通：
// releases/latest 直接是含二进制的 nightly，无需查列表（列表服务不应被请求）。
func TestFetchLatestBinaryReleaseOldMode(t *testing.T) {
	latestJSON := `{"tag_name": "b10567", "assets": [{"name": "llama-b10567-bin-win-cuda-13.3-x64.zip", "browser_download_url": "https://example.com/llama-b10567-bin-win-cuda-13.3-x64.zip", "size": 314572800}]}`

	latestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(latestJSON))
	}))
	defer latestSrv.Close()

	// 列表服务一旦被请求就让测试失败（旧模式不应查列表）
	listSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("旧模式下不应请求 release 列表接口")
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer listSrv.Close()

	release, err := fetchLatestBinaryRelease(latestSrv.URL, listSrv.URL)
	if err != nil {
		t.Fatalf("fetchLatestBinaryRelease 失败: %v", err)
	}
	if release.TagName != "b10567" {
		t.Errorf("tag = %q, 期望 %q", release.TagName, "b10567")
	}
}

// TestFetchLatestBinaryReleaseNoBinary 验证列表中所有 release 都无二进制时返回错误。
func TestFetchLatestBinaryReleaseNoBinary(t *testing.T) {
	latestJSON := `{"tag_name": "v0.2.0", "assets": [{"name": "nightly-tag.txt", "size": 8}]}`
	listJSON := `[
		{"tag_name": "v0.2.0", "assets": [{"name": "nightly-tag.txt", "size": 8}]},
		{"tag_name": "v0.1.2", "assets": [{"name": "nightly-tag.txt", "size": 8}]}
	]`

	latestSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(latestJSON))
	}))
	defer latestSrv.Close()
	listSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(listJSON))
	}))
	defer listSrv.Close()

	if _, err := fetchLatestBinaryRelease(latestSrv.URL, listSrv.URL); err == nil {
		t.Error("所有 release 均无二进制资产时应返回错误，实际返回 nil")
	}
}

// TestPinnedReleaseAPIConfig 验证版本固定配置的完整性：
// GitHubReleasesAPI 必须指向 PinnedReleaseTag 对应的 release 详情接口，
// 且 tag 必须是 "b+数字" 形式的 nightly tag。
// 这是版本固定策略的"看门狗"测试——防止未来有人把 API 地址误改回
// releases/latest，导致应用重新自动跟随上游最新版（违背固定策略）。
func TestPinnedReleaseAPIConfig(t *testing.T) {
	want := "https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/" + PinnedReleaseTag
	if GitHubReleasesAPI != want {
		t.Errorf("GitHubReleasesAPI = %q, 期望 %q（必须指向固定 tag 的 release 详情接口，而非 releases/latest）", GitHubReleasesAPI, want)
	}

	if !strings.HasPrefix(PinnedReleaseTag, "b") {
		t.Errorf("PinnedReleaseTag = %q, 应为 \"b+数字\" 形式的 nightly tag", PinnedReleaseTag)
	}
	buildNum := strings.TrimPrefix(PinnedReleaseTag, "b")
	if _, err := strconv.Atoi(buildNum); err != nil {
		t.Errorf("PinnedReleaseTag = %q, 构建编号部分 %q 应可解析为整数: %v", PinnedReleaseTag, buildNum, err)
	}
}
