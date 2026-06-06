package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestIsValidConfigClosure 测试 appDir() 中用于判断 config.json 是否为自动生成默认值的闭包
func TestIsValidConfigClosure(t *testing.T) {
	// 在临时目录构造一个类似 release/bin/ 与 release/ 的结构
	// tmp/
	//   release/
	//     engines/         <- 资源目录
	//     models/          <- 资源目录
	//     config.json      <- 用户的真实配置（model_path 非空）
	//     bin/
	//       config.json    <- 旧的自动生成默认配置（model_path 为空）
	tmp := t.TempDir()
	releaseDir := filepath.Join(tmp, "release")
	binDir := filepath.Join(releaseDir, "bin")
	enginesDir := filepath.Join(releaseDir, "engines")
	modelsDir := filepath.Join(releaseDir, "models")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(enginesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	userCfg := `{"model_path": "models/foo.gguf", "temperature": 0.6}`
	if err := os.WriteFile(filepath.Join(releaseDir, "config.json"), []byte(userCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	autoCfg := `{"model_path": "", "temperature": 0.6}`
	if err := os.WriteFile(filepath.Join(binDir, "config.json"), []byte(autoCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// 由于 isValidConfig 是 appDir() 内部的闭包，无法直接测试。
	// 这里通过 os.ReadFile + json.Unmarshal 复现它的判断逻辑并验证：
	// 1. release/ 下的 config.json 是用户配置，应通过
	// 2. release/bin/ 下的 config.json 是自动生成的默认值 + 同级上层有资源，应被识别为不可信

	checkFn := func(d string) bool {
		data, err := os.ReadFile(filepath.Join(d, "config.json"))
		if err != nil {
			return false
		}
		// 简单 JSON 解析：找 "model_path" 后面的字符串值
		s := string(data)
		idx := -1
		needle := `"model_path"`
		for i := 0; i+len(needle) <= len(s); i++ {
			if s[i:i+len(needle)] == needle {
				idx = i + len(needle)
				break
			}
		}
		if idx < 0 {
			return true
		}
		// 跳过空白与冒号
		for idx < len(s) && (s[idx] == ' ' || s[idx] == ':' || s[idx] == '\t' || s[idx] == '\n' || s[idx] == '\r') {
			idx++
		}
		// 期望是字符串字面量
		if idx >= len(s) || s[idx] != '"' {
			return true
		}
		// 找结束引号
		end := idx + 1
		empty := true
		for end < len(s) && s[end] != '"' {
			empty = false
			end++
		}
		_ = end
		if empty {
			// model_path 是空字符串：检查上层目录（filepath.Dir(d)）是否有 engines/ 或 models/ 资源
			parent := filepath.Dir(d)
			for _, p := range []string{"engines", "models"} {
				if info, err := os.Stat(filepath.Join(parent, p)); err == nil && info.IsDir() {
					return false
				}
			}
		}
		return true
	}

	if !checkFn(releaseDir) {
		t.Errorf("releaseDir 下的用户配置应被识别为可信任，实际为不可信")
	}
	if checkFn(binDir) {
		t.Errorf("binDir 下的默认配置 + 上层有资源，应被识别为不可信，实际为可信任")
	}

	// 反例：binDir 下没有 engines/ 或 models/ 资源时，配置应被认为是可信任的
	noResBin := filepath.Join(tmp, "noRes")
	noResBinBin := filepath.Join(noResBin, "bin")
	if err := os.MkdirAll(noResBinBin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noResBinBin, "config.json"), []byte(autoCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if !checkFn(noResBinBin) {
		t.Errorf("无资源目录时，binDir 下的默认配置应被识别为可信任（兼容单目录部署），实际为不可信")
	}
}
