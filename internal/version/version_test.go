package version

import "testing"

// TestVersion_Format 验证版本号格式符合语义化版本规范
//
// 生活类比：发版前先查身份证号格式——必须是 X.Y.Z 格式（如 0.10.7），
// 否则发版脚本、自动更新、GitHub Release tag 都会出问题。
func TestVersion_Format(t *testing.T) {
	if !IsValid(Version) {
		t.Errorf("版本号格式无效: %q（应为 X.Y.Z 格式，如 0.10.7）", Version)
	}
}

// TestIsValid 验证 IsValid 函数对不同格式版本号的判断
func TestIsValid(t *testing.T) {
	tests := []struct {
		name string
		v    string
		want bool
	}{
		{"标准格式", "0.10.7", true},
		{"简单格式", "1.0.0", true},
		{"大版本号", "2024.1.15", true},
		{"带预发布后缀", "1.0.0-beta.1", true},
		{"带预发布后缀2", "2.0.0-alpha", true},
		{"空字符串", "", false},
		{"缺少补丁号", "1.0", false},
		{"缺少次要号", "1", false},
		{"带 v 前缀", "v1.0.0", false},
		{"带构建信息", "1.0.0+build.123", false}, // 当前正则不支持 build metadata
		{"包含空格", "1.0.0 ", false},
		{"包含字母版本号", "1.0.x", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValid(tt.v); got != tt.want {
				t.Errorf("IsValid(%q) = %v, want %v", tt.v, got, tt.want)
			}
		})
	}
}

// TestGitHubURL 验证 GitHubURL 返回正确的 URL
func TestGitHubURL(t *testing.T) {
	got := GitHubURL()
	want := "https://github.com/" + GitHubOwner + "/" + GitHubRepo
	if got != want {
		t.Errorf("GitHubURL() = %q, want %q", got, want)
	}
}

// TestConstants_NonEmpty 验证所有常量非空
func TestConstants_NonEmpty(t *testing.T) {
	if Version == "" {
		t.Error("Version 常量不应为空")
	}
	if GitHubOwner == "" {
		t.Error("GitHubOwner 常量不应为空")
	}
	if GitHubRepo == "" {
		t.Error("GitHubRepo 常量不应为空")
	}
}
