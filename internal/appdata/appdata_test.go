package appdata

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDataDir_PrefersLocalAppData 验证数据目录优先取 %LOCALAPPDATA%\Douya
func TestDataDir_PrefersLocalAppData(t *testing.T) {
	t.Setenv("LOCALAPPDATA", `C:\Users\u\AppData\Local`)
	t.Setenv("APPDATA", `C:\Users\u\AppData\Roaming`)
	got := DataDir(`D:\Apps\Douya.exe`)
	if want := `C:\Users\u\AppData\Local\Douya`; got != want {
		t.Errorf("DataDir() = %q, 期望 %q", got, want)
	}
}

// TestDataDir_FallsBackToAppData 验证 LOCALAPPDATA 缺失时回退 APPDATA
func TestDataDir_FallsBackToAppData(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("APPDATA", `C:\Users\u\AppData\Roaming`)
	got := DataDir(`D:\Apps\Douya.exe`)
	if want := `C:\Users\u\AppData\Roaming\Douya`; got != want {
		t.Errorf("DataDir() = %q, 期望 %q", got, want)
	}
}

// TestDataDir_FallsBackToExeDir 验证两个环境变量均缺失时回退 exe 同目录
func TestDataDir_FallsBackToExeDir(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("APPDATA", "")
	got := DataDir(`D:\Apps\Douya.exe`)
	if want := `D:\Apps\Douya`; got != want {
		t.Errorf("DataDir() = %q, 期望 %q", got, want)
	}
}

// TestMigrateLegacyData_NoLegacy 验证无旧数据时跳过并写标记（幂等）
func TestMigrateLegacyData_NoLegacy(t *testing.T) {
	dst := t.TempDir()
	got := MigrateLegacyData(dst, []string{filepath.Join(dst, "nowhere")})
	if !got.Skipped {
		t.Errorf("无旧数据应 Skipped=true，实际: %+v", got)
	}
	if _, err := os.Stat(filepath.Join(dst, markerFileName)); err != nil {
		t.Errorf("无旧数据也应写入标记避免重复扫描: %v", err)
	}
}

// TestMigrateLegacyData_DstInitialized 验证目标已有配置时绝不覆盖
func TestMigrateLegacyData_DstInitialized(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "config.json"), []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	// 目标目录已初始化（用户已在新目录产生数据）
	newCfg := filepath.Join(dst, "config.json")
	if err := os.WriteFile(newCfg, []byte(`{"new":true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := MigrateLegacyData(dst, []string{src})
	if !got.Skipped {
		t.Errorf("目标已初始化应 Skipped=true，实际: %+v", got)
	}
	data, _ := os.ReadFile(newCfg)
	if string(data) != `{"new":true}` {
		t.Errorf("新配置被旧数据覆盖，属于数据丢失事故，实际内容: %s", data)
	}
}

// TestMigrateLegacyData_CopiesConfigAndData 验证核心迁移能力
func TestMigrateLegacyData_CopiesConfigAndData(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()

	// 构造旧数据：config.json + data/（含子目录与多文件）
	if err := os.WriteFile(filepath.Join(src, "config.json"), []byte(`{"v":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(src, "data")
	logsDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(dataDir, "douya.db"): "DB",
		filepath.Join(dataDir, ".enc_key"): "KEY",
		filepath.Join(logsDir, "app.log"):  "LOG",
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := MigrateLegacyData(dst, []string{src})
	if got.Skipped || !got.MigratedConfig || got.MigratedFiles != len(files) || got.FailedFiles != 0 {
		t.Fatalf("迁移结果不符预期: %+v", got)
	}

	// 抽验内容完整性
	for path, want := range files {
		rel, _ := filepath.Rel(src, path)
		gotBytes, err := os.ReadFile(filepath.Join(dst, rel))
		if err != nil {
			t.Errorf("迁移后文件缺失 %s: %v", rel, err)
			continue
		}
		if string(gotBytes) != want {
			t.Errorf("文件 %s 内容不一致: 期望 %s 实际 %s", rel, want, gotBytes)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "config.json")); err != nil {
		t.Errorf("config.json 未迁移: %v", err)
	}
}

// TestMigrateLegacyData_Idempotent 验证重复调用幂等（标记生效）
func TestMigrateLegacyData_Idempotent(t *testing.T) {
	dst := t.TempDir()
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "data.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	dataDir := filepath.Join(src, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "a.txt"), []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}

	first := MigrateLegacyData(dst, []string{src})
	if first.Skipped || first.MigratedFiles != 1 {
		t.Fatalf("首次迁移结果异常: %+v", first)
	}

	// 模拟用户随后删除了 dst 的 data/（如清理磁盘），再次启动不应"复活"旧数据扫描
	second := MigrateLegacyData(dst, []string{src})
	if !second.Skipped {
		t.Errorf("第二次调用应因标记跳过，实际: %+v", second)
	}
}

// TestMigrateLegacyData_PicksFirstCandidate 验证候选顺序：优先 exe 同目录
func TestMigrateLegacyData_PicksFirstCandidate(t *testing.T) {
	dst := t.TempDir()
	exeDir := t.TempDir()
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(exeDir, "config.json"), []byte(`{"from":"exeDir"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(parent, "config.json"), []byte(`{"from":"parent"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	MigrateLegacyData(dst, []string{exeDir, parent})
	data, err := os.ReadFile(filepath.Join(dst, "config.json"))
	if err != nil {
		t.Fatalf("config.json 未迁移: %v", err)
	}
	if string(data) != `{"from":"exeDir"}` {
		t.Errorf("应优先取第一个候选目录，实际: %s", data)
	}
}
