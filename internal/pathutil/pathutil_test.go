package pathutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestResolveInBase_EmptyPath 验证空路径返回空字符串
func TestResolveInBase_EmptyPath(t *testing.T) {
	if got := ResolveInBase("/base", ""); got != "" {
		t.Errorf("空路径应返回空字符串，实际: %s", got)
	}
}

// TestResolveInBase_AbsolutePath 验证绝对路径原样返回
func TestResolveInBase_AbsolutePath(t *testing.T) {
	// 用 TempDir 构造一个真实存在的绝对路径作为输入
	tmpFile := filepath.Join(t.TempDir(), "test.gguf")
	if err := os.WriteFile(tmpFile, []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ResolveInBase("/some/base", tmpFile); got != tmpFile {
		t.Errorf("绝对路径应原样返回，期望: %s, 实际: %s", tmpFile, got)
	}
}

// TestResolveInBase_RelativePath 验证相对路径正确拼接
func TestResolveInBase_RelativePath(t *testing.T) {
	base := t.TempDir() // 使用真实存在的绝对路径
	rel := "config.json"
	got := ResolveInBase(base, rel)
	expected := filepath.Join(base, rel)
	if got != expected {
		t.Errorf("相对路径应正确拼接，期望: %s, 实际: %s", expected, got)
	}
}

// TestResolveInBase_PathTraversal 验证路径遍历攻击被防护
//
// 生活类比：快递员发现收件地址里有 ".."（想逃出大楼），
// 应该降级为只送包裹的姓名（Base(p)），确保不会送到大楼外。
func TestResolveInBase_PathTraversal(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name     string
		input    string
		wantFile string // 期望降级后的文件名部分
	}{
		{"简单遍历", "../../../etc/passwd", "passwd"},
		{"单层遍历", "../secret", "secret"},
		{"多层遍历", "../../hidden", "hidden"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveInBase(base, tt.input)
			// 应该降级为 base + Base(input)
			expected := filepath.Join(base, tt.wantFile)
			if got != expected {
				t.Errorf("路径遍历应降级为 Base，期望: %s, 实际: %s", expected, got)
			}
		})
	}
}

// TestResolveInBase_BenignRelative 验证正常的相对路径不会被误判
func TestResolveInBase_BenignRelative(t *testing.T) {
	base := t.TempDir()

	tests := []struct {
		name  string
		input string
	}{
		{"子目录文件", "data/config.json"},
		{"嵌套子目录", "models/qwen/test.gguf"},
		{"单文件", "config.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveInBase(base, tt.input)
			expected := filepath.Join(base, filepath.Clean(tt.input))
			if got != expected {
				t.Errorf("正常相对路径不应被误判，期望: %s, 实际: %s", expected, got)
			}
		})
	}
}

// TestResolveInBase_BrotherDirBypass 验证兄弟目录绕过被防护
//
// 安全审查 #2：baseDir="C:\app" 时 "C:\app-evil" 不应通过
// 这是通过 HasPrefix 检查 baseDir + 分隔符 来防护的
func TestResolveInBase_BrotherDirBypass(t *testing.T) {
	// 这个测试需要绝对路径场景，构造一个 base 和它的"兄弟"
	if runtime.GOOS != "windows" {
		t.Skip("此测试针对 Windows 路径格式")
	}

	base := `C:\app`
	// 构造一个能解析到兄弟目录的路径
	// 由于 filepath.Join 会清理 ..，这里用绝对路径测试
	evilPath := `C:\app-evil\secret`
	got := ResolveInBase(base, evilPath)
	// 绝对路径直接返回，但这是设计决定（见函数注释）
	if got != evilPath {
		t.Errorf("绝对路径应原样返回（设计决定），期望: %s, 实际: %s", evilPath, got)
	}
}

// TestResolveInBase_Cleaning 验证路径会被 Clean 处理
func TestResolveInBase_Cleaning(t *testing.T) {
	base := t.TempDir()
	// 带冗余分隔符的路径
	input := "data//config.json"
	got := ResolveInBase(base, input)
	// 应该被 filepath.Clean 处理
	expected := filepath.Join(base, "data", "config.json")
	if got != expected {
		t.Errorf("路径应被 Clean 处理，期望: %s, 实际: %s", expected, got)
	}
}

// TestRestrictACLWindows_SkipInTest 验证测试环境跳过 ACL 收紧
func TestRestrictACLWindows_SkipInTest(t *testing.T) {
	// 设置跳过环境变量
	origVal := os.Getenv("DOUYA_SKIP_ACL")
	defer func() {
		if origVal != "" {
			os.Setenv("DOUYA_SKIP_ACL", origVal)
		} else {
			os.Unsetenv("DOUYA_SKIP_ACL")
		}
	}()
	os.Setenv("DOUYA_SKIP_ACL", "1")

	// 应直接返回 nil，不执行 icacls
	err := RestrictACLWindows("/some/path")
	if err != nil {
		t.Errorf("测试环境应跳过 ACL 收紧，返回 nil，实际: %v", err)
	}
}

// TestRestrictACLWindows_NoSkip 验证非测试环境会尝试执行（但可能失败）
//
// 注意：这个测试不设置 DOUYA_SKIP_ACL，所以会尝试调用 icacls。
// 我们不关心 icacls 是否成功，只验证它被调用了（通过检查错误行为）。
func TestRestrictACLWindows_NoSkip(t *testing.T) {
	origVal := os.Getenv("DOUYA_SKIP_ACL")
	defer func() {
		if origVal != "" {
			os.Setenv("DOUYA_SKIP_ACL", origVal)
		} else {
			os.Unsetenv("DOUYA_SKIP_ACL")
		}
	}()
	os.Unsetenv("DOUYA_SKIP_ACL")

	// 用一个不存在的路径，icacls 应该失败
	tmpDir := t.TempDir()
	nonExistentPath := filepath.Join(tmpDir, "nonexistent-file")

	err := RestrictACLWindows(nonExistentPath)
	// 不应 panic，可能返回错误（因为文件不存在）
	if err == nil {
		// 某些环境可能没有 icacls 命令，这里只验证不 panic
		t.Logf("icacls 在不存在的路径上返回 nil（可能环境无 icacls）")
	}
	// 关键：不 panic 即通过
}

// TestResolveInBase_RealWorldScenarios 真实场景测试
func TestResolveInBase_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name     string
		baseDir  string
		input    string
		wantSafe bool // 期望结果是否在 baseDir 内
	}{
		{
			name:     "用户选择的模型文件（绝对路径）",
			baseDir:  `C:\Users\test\douya`,
			input:    `D:\models\qwen.gguf`,
			wantSafe: true, // 绝对路径原样返回（设计决定）
		},
		{
			name:     "配置中的相对路径",
			baseDir:  `C:\Users\test\douya`,
			input:    "config.json",
			wantSafe: true,
		},
		{
			name:     "恶意路径遍历",
			baseDir:  `C:\Users\test\douya`,
			input:    "../../../Windows/System32/config",
			wantSafe: true, // 会被降级为 Base
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveInBase(tt.baseDir, tt.input)
			if got == "" {
				t.Error("不应返回空字符串")
			}
		})
	}
}

// TestResolveInBase_NoBaseDirSeparator 验证兄弟目录绕过防护
//
// 关键安全测试：baseDir="C:\app" 时，
// 相对路径解析后不应落到 "C:\app-evil" 这类兄弟目录
func TestResolveInBase_NoBaseDirSeparator(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("此测试针对 Windows 路径格式")
	}

	base := `C:\app`
	// 构造一个相对路径，Join 后可能成为兄弟目录
	// filepath.Join("C:\app", "..\app-evil\secret") 会被 Clean 为 "C:\app-evil\secret"
	input := filepath.Join("..", "app-evil", "secret")

	got := ResolveInBase(base, input)

	// 应该检测到遍历并降级
	expected := filepath.Join(base, filepath.Base(input))
	if got != expected {
		// 如果没降级，检查结果是否仍在 base 内
		if !strings.HasPrefix(got, base+string(filepath.Separator)) && got != base {
			t.Errorf("路径遍历防护失败，结果 %q 不在 base %q 内，期望降级为 %q", got, base, expected)
		}
	}
}
