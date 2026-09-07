package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestNormalizeMmprojFileName 测试下载完成后 mmproj 文件名的规范化重命名逻辑。
func TestNormalizeMmprojFileName(t *testing.T) {
	t.Run("重命名为规范化文件名", func(t *testing.T) {
		dir := t.TempDir()
		src := filepath.Join(dir, "mmproj-qwen3-vl-9b-bf16.gguf")
		if err := os.WriteFile(src, []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}

		a := &App{}
		renamed, final := a.normalizeMmprojFileName(dir, "Qwen3.8-9B-Q4_K_M.gguf", "mmproj-qwen3-vl-9b-bf16.gguf")
		if !renamed {
			t.Fatal("期望发生重命名")
		}
		want := "mmproj-Qwen3.8-9B-BF16.gguf"
		if final != want {
			t.Errorf("final = %q, 期望 %q", final, want)
		}
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Errorf("目标文件不存在: %v", err)
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Errorf("源文件应已被重命名移除，err = %v", err)
		}
	})

	t.Run("文件名已符合规范不重命名", func(t *testing.T) {
		dir := t.TempDir()
		name := "mmproj-Qwen3.8-9B-BF16.gguf"
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0o644); err != nil {
			t.Fatal(err)
		}

		a := &App{}
		renamed, final := a.normalizeMmprojFileName(dir, "Qwen3.8-9B-Q4_K_M.gguf", name)
		if renamed {
			t.Error("期望不重命名")
		}
		if final != name {
			t.Errorf("final = %q, 期望 %q", final, name)
		}
	})

	t.Run("目标已存在时删除源文件保留目标", func(t *testing.T) {
		dir := t.TempDir()
		target := "mmproj-Qwen3.8-9B-BF16.gguf"
		src := filepath.Join(dir, "mmproj-old-name.gguf")
		if err := os.WriteFile(filepath.Join(dir, target), []byte("existing"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
			t.Fatal(err)
		}

		a := &App{}
		renamed, final := a.normalizeMmprojFileName(dir, "Qwen3.8-9B-Q4_K_M.gguf", "mmproj-old-name.gguf")
		if !renamed {
			t.Error("期望返回规范化文件名")
		}
		if final != target {
			t.Errorf("final = %q, 期望 %q", final, target)
		}
		if _, err := os.Stat(src); !os.IsNotExist(err) {
			t.Errorf("重复的源文件应被删除，err = %v", err)
		}
		if _, err := os.Stat(filepath.Join(dir, target)); err != nil {
			t.Errorf("既有目标文件应保留: %v", err)
		}
	})
}
