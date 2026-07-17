package logger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// TestInit_EmptyLogDir 验证空 logDir 时回退到仅控制台输出
func TestInit_EmptyLogDir(t *testing.T) {
	// 保存原始状态用于恢复
	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)

	Init("")

	// 空目录时应设置 InfoLevel
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("空 logDir 时应设置 InfoLevel，实际: %v", zerolog.GlobalLevel())
	}
}

// TestInit_ValidLogDir 验证有效 logDir 时创建日志文件并初始化
func TestInit_ValidLogDir(t *testing.T) {
	tmpDir := t.TempDir()

	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)
	t.Cleanup(Close) // 确保文件句柄关闭，避免 TempDir 清理失败

	Init(tmpDir)

	// 应设置 InfoLevel
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("应设置 InfoLevel，实际: %v", zerolog.GlobalLevel())
	}

	// 应创建今天的日志文件
	expectedFile := filepath.Join(tmpDir, "douya-"+time.Now().Format(logFileDateFormat)+".log")
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("日志文件未创建: %s, 错误: %v", expectedFile, err)
	}
}

// TestInit_InvalidLogDir 验证无效 logDir 时回退到仅控制台输出
func TestInit_InvalidLogDir(t *testing.T) {
	// 使用一个无法创建的路径（在 Windows 上，带有非法字符的路径）
	invalidDir := filepath.Join(string(os.PathSeparator), "invalid", "path", "with", "null\x00byte")

	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)

	// 不应 panic，应回退
	Init(invalidDir)

	// 应设置 InfoLevel（回退路径）
	if zerolog.GlobalLevel() != zerolog.InfoLevel {
		t.Errorf("无效 logDir 时应回退到 InfoLevel，实际: %v", zerolog.GlobalLevel())
	}
}

// TestOpenLogFile_EmptyDir 验证空目录时返回 nil
func TestOpenLogFile_EmptyDir(t *testing.T) {
	w, path := openLogFile("")
	if w != nil {
		t.Error("空目录时应返回 nil writer")
	}
	if path != "" {
		t.Errorf("空目录时应返回空路径，实际: %s", path)
	}
}

// TestOpenLogFile_ValidDir 验证有效目录时返回文件 writer
func TestOpenLogFile_ValidDir(t *testing.T) {
	tmpDir := t.TempDir()

	w, path := openLogFile(tmpDir)
	if w == nil {
		t.Fatal("有效目录时应返回非 nil writer")
	}
	if path == "" {
		t.Error("应返回非空路径")
	}
	if !strings.Contains(path, "douya-") || !strings.HasSuffix(path, ".log") {
		t.Errorf("日志文件名格式不正确: %s", path)
	}
	// 清理
	if closer, ok := w.(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestOpenLogFile_CreatesDir 验证目录不存在时会创建
func TestOpenLogFile_CreatesDir(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "subdir", "logs")

	w, _ := openLogFile(subDir)
	if w == nil {
		t.Fatal("应创建嵌套目录并返回 writer")
	}
	if _, err := os.Stat(subDir); err != nil {
		t.Errorf("目录未创建: %s, 错误: %v", subDir, err)
	}
	if closer, ok := w.(interface{ Close() error }); ok {
		closer.Close()
	}
}

// TestCleanOldLogs_RemovesExpired 验证过期日志文件被清理
func TestCleanOldLogs_RemovesExpired(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建一个 10 天前的日志文件（超过保留期 7 天）
	oldDate := time.Now().AddDate(0, 0, -10).Format(logFileDateFormat)
	oldFile := filepath.Join(tmpDir, "douya-"+oldDate+".log")
	if err := os.WriteFile(oldFile, []byte("old log"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 创建今天的日志文件（应保留）
	todayFile := filepath.Join(tmpDir, "douya-"+time.Now().Format(logFileDateFormat)+".log")
	if err := os.WriteFile(todayFile, []byte("today log"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 执行清理
	cleanOldLogs(tmpDir)

	// 过期文件应被删除
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Errorf("过期日志文件应被删除: %s", oldFile)
	}
	// 今天的文件应保留
	if _, err := os.Stat(todayFile); err != nil {
		t.Errorf("今天的日志文件应保留: %s, 错误: %v", todayFile, err)
	}
}

// TestCleanOldLogs_KeepsRecent 验证近期日志文件被保留
func TestCleanOldLogs_KeepsRecent(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建 3 天前的日志文件（在保留期内）
	recentDate := time.Now().AddDate(0, 0, -3).Format(logFileDateFormat)
	recentFile := filepath.Join(tmpDir, "douya-"+recentDate+".log")
	if err := os.WriteFile(recentFile, []byte("recent"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanOldLogs(tmpDir)

	if _, err := os.Stat(recentFile); err != nil {
		t.Errorf("近期日志文件应保留: %s, 错误: %v", recentFile, err)
	}
}

// TestCleanOldLogs_IgnoresNonLogFiles 验证非日志格式的文件不被清理
func TestCleanOldLogs_IgnoresNonLogFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建一个不符合命名格式的旧文件
	nonLogFile := filepath.Join(tmpDir, "other-2020-01-01.txt")
	if err := os.WriteFile(nonLogFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanOldLogs(tmpDir)

	// 非日志格式文件应保留
	if _, err := os.Stat(nonLogFile); err != nil {
		t.Errorf("非日志格式文件应保留: %s, 错误: %v", nonLogFile, err)
	}
}

// TestCleanOldLogs_IgnoresInvalidDate 验证日期格式错误的文件不被清理
func TestCleanOldLogs_IgnoresInvalidDate(t *testing.T) {
	tmpDir := t.TempDir()

	// 日期格式错误的日志文件
	invalidDateFile := filepath.Join(tmpDir, "douya-invalid-date.log")
	if err := os.WriteFile(invalidDateFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	cleanOldLogs(tmpDir)

	if _, err := os.Stat(invalidDateFile); err != nil {
		t.Errorf("日期格式错误的文件应保留（不处理）: %s, 错误: %v", invalidDateFile, err)
	}
}

// TestCleanOldLogs_IgnoresDirectories 验证目录不被当作日志清理
func TestCleanOldLogs_IgnoresDirectories(t *testing.T) {
	tmpDir := t.TempDir()

	// 创建一个名字像日志的目录
	oldDate := time.Now().AddDate(0, 0, -10).Format(logFileDateFormat)
	dirName := filepath.Join(tmpDir, "douya-"+oldDate+".log")
	if err := os.Mkdir(dirName, 0o755); err != nil {
		t.Fatal(err)
	}

	cleanOldLogs(tmpDir)

	// 目录应保留（cleanOldLogs 跳过目录）
	if _, err := os.Stat(dirName); err != nil {
		t.Errorf("目录不应被清理: %s, 错误: %v", dirName, err)
	}
}

// TestCleanOldLogs_NonExistentDir 验证不存在的目录不 panic
func TestCleanOldLogs_NonExistentDir(t *testing.T) {
	// 不应 panic
	cleanOldLogs("/nonexistent/path/that/does/not/exist")
}

// TestLoggerWriteAndRead 集成测试：验证日志能写入文件
func TestLoggerWriteAndRead(t *testing.T) {
	tmpDir := t.TempDir()

	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)
	t.Cleanup(Close)

	Init(tmpDir)

	// 写入一条日志
	log.Info().Str("key", "value").Msg("test message")

	// 读取日志文件验证内容
	logFile := filepath.Join(tmpDir, "douya-"+time.Now().Format(logFileDateFormat)+".log")
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "test message") {
		t.Errorf("日志文件应包含测试消息，实际内容: %s", string(content))
	}
}

// TestClose 验证 Close 能正确关闭文件句柄
func TestClose(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(Close)

	Init(tmpDir)

	// Close 后 currentLogFile 应为 nil
	Close()
	if currentLogFile != nil {
		t.Error("Close 后 currentLogFile 应为 nil")
	}

	// 多次调用不应 panic
	Close()
}

// ===== D3: 日志级别动态调整测试 =====

// TestSetLevel_ValidLevels 验证所有合法级别都能正确设置
func TestSetLevel_ValidLevels(t *testing.T) {
	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)

	cases := []struct {
		input string
		want  zerolog.Level
	}{
		{"trace", zerolog.TraceLevel},
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"warning", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"fatal", zerolog.FatalLevel},
		{"panic", zerolog.PanicLevel},
		{"disabled", zerolog.Disabled},
		{"off", zerolog.Disabled},
		{"none", zerolog.Disabled},
		// 大小写不敏感
		{"DEBUG", zerolog.DebugLevel},
		{"  Warn  ", zerolog.WarnLevel},
	}
	for _, c := range cases {
		if err := SetLevel(c.input); err != nil {
			t.Errorf("SetLevel(%q) 返回错误: %v", c.input, err)
			continue
		}
		if zerolog.GlobalLevel() != c.want {
			t.Errorf("SetLevel(%q) 后级别 = %v, 期望 %v", c.input, zerolog.GlobalLevel(), c.want)
		}
	}
}

// TestSetLevel_InvalidLevel 验证无效级别返回错误且不改变当前级别
func TestSetLevel_InvalidLevel(t *testing.T) {
	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)

	// 先设一个已知级别
	if err := SetLevel("info"); err != nil {
		t.Fatalf("初始化 SetLevel(info) 失败: %v", err)
	}

	invalidCases := []string{"verbose", "infos", "123", "all", "?"}
	for _, c := range invalidCases {
		err := SetLevel(c)
		if err == nil {
			t.Errorf("SetLevel(%q) 期望返回错误，实际 nil", c)
			continue
		}
		// 级别应保持不变
		if zerolog.GlobalLevel() != zerolog.InfoLevel {
			t.Errorf("SetLevel(%q) 后级别被改变为 %v，应保持 info", c, zerolog.GlobalLevel())
		}
	}
}

// TestGetLevel 验证 GetLevel 返回当前级别字符串
func TestGetLevel(t *testing.T) {
	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)

	cases := []struct {
		set    string
		expect string
	}{
		{"debug", "debug"},
		{"info", "info"},
		{"warn", "warn"},
		{"error", "error"},
		{"disabled", "disabled"},
	}
	for _, c := range cases {
		if err := SetLevel(c.set); err != nil {
			t.Fatalf("SetLevel(%q) 失败: %v", c.set, err)
		}
		got := GetLevel()
		if got != c.expect {
			t.Errorf("GetLevel() = %q, 期望 %q（设置 %q 后）", got, c.expect, c.set)
		}
	}
}

// TestParseLevel 验证 parseLevel 内部函数
func TestParseLevel(t *testing.T) {
	cases := []struct {
		input string
		want  zerolog.Level
	}{
		{"trace", zerolog.TraceLevel},
		{"DEBUG", zerolog.DebugLevel},
		{"  info  ", zerolog.InfoLevel},
		{"warning", zerolog.WarnLevel},
		{"off", zerolog.Disabled},
		{"", zerolog.NoLevel}, // 空字符串
		{"unknown", zerolog.NoLevel},
	}
	for _, c := range cases {
		got := parseLevel(c.input)
		if got != c.want {
			t.Errorf("parseLevel(%q) = %v, 期望 %v", c.input, got, c.want)
		}
	}
}

// TestInit_WithEnvLogLevel 验证环境变量控制初始日志级别
func TestInit_WithEnvLogLevel(t *testing.T) {
	origLevel := zerolog.GlobalLevel()
	defer zerolog.SetGlobalLevel(origLevel)
	origEnv := os.Getenv(LogLevelEnvVar)
	defer os.Setenv(LogLevelEnvVar, origEnv)

	cases := []struct {
		env  string
		want zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"", zerolog.InfoLevel}, // 空环境变量 → 默认 info
		{"invalid", zerolog.InfoLevel}, // 无效值 → 默认 info
	}
	for _, c := range cases {
		os.Setenv(LogLevelEnvVar, c.env)
		Init("") // 空目录，仅控制台
		if zerolog.GlobalLevel() != c.want {
			t.Errorf("DOUYA_LOG_LEVEL=%q 后级别 = %v, 期望 %v", c.env, zerolog.GlobalLevel(), c.want)
		}
	}
}
