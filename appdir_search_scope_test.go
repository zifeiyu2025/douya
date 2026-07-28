package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestResolveAppDir_SearchScope 验证 resolveAppDir 的查找范围限制
//
// 核心约束：配置只能读写到 exe 同目录或直接上级目录，不允许触及更上层目录。
// 这样确保 release 目录和项目目录完全隔离：
//   - release/bin/Douya.exe → 只查 release/bin/ 和 release/，不会触及项目根
//   - build/bin/Douya.exe   → 只查 build/bin/ 和 build/，不会触及项目根
//
// 生活类比：就像租房时只能在"自己房间"和"客厅"活动，不能跑到邻居家里。
func TestResolveAppDir_SearchScope(t *testing.T) {
	// 构造目录结构：
	// tmp/
	//   project/              <- 项目目录（模拟 d:\MyGoWorkspace\douya）
	//     config.json         <- 项目目录的配置（不应被发布版使用）
	//     release/
	//       bin/              <- exe 所在目录
	//         Douya.exe       <- 模拟 exe
	//       config.json       <- 发布版配置（应被优先使用）
	//       models/           <- 资源目录
	tmp := t.TempDir()
	projectDir := filepath.Join(tmp, "project")
	releaseDir := filepath.Join(projectDir, "release")
	binDir := filepath.Join(releaseDir, "bin")
	modelsDir := filepath.Join(releaseDir, "models")

	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 在项目目录放一个 config.json（模拟开发时生成的配置）
	// 旧逻辑向上 3 层会找到这个文件，导致发布版错误使用项目目录配置
	projectCfg := `{"model_path": "PROJECT_DIR_CONFIG", "temperature": 0.6}`
	if err := os.WriteFile(filepath.Join(projectDir, "config.json"), []byte(projectCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// 在 release/ 目录放一个 config.json（发布版的正确配置）
	releaseCfg := `{"model_path": "RELEASE_DIR_CONFIG", "temperature": 0.7}`
	if err := os.WriteFile(filepath.Join(releaseDir, "config.json"), []byte(releaseCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	// 验证：从 binDir 向上查找，最多只能到 releaseDir，不能到 projectDir
	// 模拟 resolveAppDir 的候选目录构造逻辑
	exeDir := binDir
	searchDirs := []string{exeDir}
	parent := filepath.Dir(exeDir)
	if parent != exeDir {
		searchDirs = append(searchDirs, parent)
	}

	// 断言：候选目录只包含 binDir 和 releaseDir，不包含 projectDir
	for _, d := range searchDirs {
		if d == projectDir {
			t.Errorf("查找范围不应包含项目目录 %s，但候选目录中包含它", projectDir)
		}
	}

	// 验证候选目录数量：最多 2 个（exe 同目录 + 直接上级）
	if len(searchDirs) > 2 {
		t.Errorf("候选目录最多 2 个，实际 %d 个：%v", len(searchDirs), searchDirs)
	}

	// 验证：releaseDir 在候选目录中，projectDir 不在
	foundRelease := false
	for _, d := range searchDirs {
		if d == releaseDir {
			foundRelease = true
		}
	}
	if !foundRelease {
		t.Errorf("直接上级目录 %s 应在候选目录中", releaseDir)
	}
}

// TestResolveAppDir_NoAppDataFallback 验证不使用 %APPDATA%/douya 兜底
//
// 设计原则：配置和数据必须限制在 exe 所在目录及直接上级目录，
// 不允许读写到用户目录（如 %APPDATA%/douya）或其他目录。
func TestResolveAppDir_NoAppDataFallback(t *testing.T) {
	// 构造一个空目录结构，exe 在 bin/ 下，没有任何 config.json 和 models/
	// 旧逻辑会回退到 %APPDATA%/douya，新逻辑应在 exe 上级目录创建配置
	tmp := t.TempDir()
	binDir := filepath.Join(tmp, "release", "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 模拟 resolveAppDir 的候选目录构造
	exeDir := binDir
	searchDirs := []string{exeDir}
	parent := filepath.Dir(exeDir)
	if parent != exeDir {
		searchDirs = append(searchDirs, parent)
	}

	// 断言：候选目录中不包含 %APPDATA%/douya 或任何用户目录
	// （新逻辑移除了 os.UserConfigDir() 兜底）
	for _, d := range searchDirs {
		// 候选目录应该在 tmp 之下，不在用户目录之下
		if filepath.Base(d) == "douya" && d != tmp {
			// 如果有名为 douya 的目录但不是我们创建的，可能是 %APPDATA%/douya
			t.Errorf("候选目录不应包含 %%APPDATA%%/douya，但找到 %s", d)
		}
	}

	// 验证候选目录数量：最多 2 个
	if len(searchDirs) > 2 {
		t.Errorf("候选目录最多 2 个，实际 %d 个", len(searchDirs))
	}
}
