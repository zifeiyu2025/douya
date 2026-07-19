package logger

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// 日志保留天数
const logRetentionDays = 7

// 日志文件名日期格式
const logFileDateFormat = "2006-01-02"

// LogLevelEnvVar 是控制初始日志级别的环境变量名
// 生活类比：像电视机的初始音量——开机时先看遥控器有没有设定值，没有就用默认音量
const LogLevelEnvVar = "DOUYA_LOG_LEVEL"

// currentLogFile 当前打开的日志文件句柄，供 Close 使用
var currentLogFile io.Closer

// Init 初始化 zerolog，同时输出到控制台和日志文件。
// logDir 为日志文件存放目录（通常为 <appDir>/data/logs）。
// 若 logDir 为空或创建失败，则回退到仅控制台输出。
//
// 日志级别优先级：
// 1. 环境变量 DOUYA_LOG_LEVEL（如 "debug"、"warn"）
// 2. 默认 InfoLevel
func Init(logDir string) {
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	// 读取初始日志级别（环境变量优先，默认 info）
	initialLevel := parseLevel(os.Getenv(LogLevelEnvVar))
	if initialLevel == zerolog.NoLevel {
		initialLevel = zerolog.InfoLevel
	}

	// 尝试启用文件输出
	fileWriter, logFilePath := openLogFile(logDir)
	if fileWriter == nil {
		// 回退：仅控制台
		log.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()
		zerolog.SetGlobalLevel(initialLevel)
		log.Info().Str("level", initialLevel.String()).Msg("日志级别已设置（仅控制台输出）")
		return
	}

	// 保存句柄供 Close 使用
	currentLogFile = fileWriter

	// 同时输出到控制台（彩色）和文件（JSON 格式，便于分析）。
	//
	// 关键：用 ignoreErrorWriter 包装 consoleWriter。
	// 原因：Wails 在 Windows 下编译为 GUI 程序（-ldflags -H=windowsgui），
	// os.Stdout 是无效句柄，ConsoleWriter 写入会返回错误。
	// io.MultiWriter 的行为是「任一 writer 返回错误就停止，不再写后续 writer」，
	// 这会导致 fileWriter 永远收不到数据，日志文件保持 0 字节。
	// 生活类比：邮递员同时投两个信箱，第一个拒收就把整封信撕掉——
	// 所以给第一个信箱配个「宽容的代收员」，拒收也无所谓，不影响投第二个。
	multi := io.MultiWriter(ignoreErrorWriter{consoleWriter}, fileWriter)
	log.Logger = zerolog.New(multi).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(initialLevel)

	// 清理过期日志文件
	cleanOldLogs(logDir)

	log.Info().Str("file", logFilePath).Str("level", initialLevel.String()).Msg("日志文件已启用")
}

// ignoreErrorWriter 包装一个 io.Writer，吞掉所有写入错误。
// 用于 io.MultiWriter 中那些「失败了也无所谓」的 writer（如 GUI 程序的 stdout），
// 防止其错误中断 MultiWriter 对后续 writer 的写入。
type ignoreErrorWriter struct{ w io.Writer }

func (iww ignoreErrorWriter) Write(p []byte) (int, error) {
	n, _ := iww.w.Write(p)
	return n, nil
}

// SetLevel 动态调整全局日志级别。
//
// 生活类比：像电视机调音量——不用关机重启，运行中直接调高调低。
// 调成 "debug" 能看到更详细的信息（排查问题），调成 "warn" 能减少噪音（日常使用）。
//
// 支持的级别（不区分大小写）：trace / debug / info / warn / error / fatal / panic / disabled
// 传入无法识别的字符串会返回错误，不改变当前级别。
func SetLevel(level string) error {
	l := parseLevel(level)
	if l == zerolog.NoLevel {
		return fmt.Errorf("unknown log level: %q (supported: trace/debug/info/warn/error/fatal/panic/disabled)", level)
	}
	zerolog.SetGlobalLevel(l)
	log.Info().Str("level", l.String()).Msg("日志级别已动态调整")
	return nil
}

// GetLevel 返回当前全局日志级别的字符串表示
func GetLevel() string {
	return zerolog.GlobalLevel().String()
}

// parseLevel 将字符串解析为 zerolog.Level，无法识别时返回 zerolog.NoLevel
// 支持不区分大小写匹配
func parseLevel(s string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	case "disabled", "off", "none":
		return zerolog.Disabled
	case "":
		return zerolog.NoLevel // 空字符串，让调用方决定默认值
	default:
		return zerolog.NoLevel
	}
}

// Close 关闭日志文件句柄，释放资源。
// 应在应用退出时调用。多次调用是安全的。
func Close() {
	if currentLogFile != nil {
		_ = currentLogFile.Close()
		currentLogFile = nil
	}
}

// openLogFile 打开当天日志文件（按日期命名，追加模式）。
// 返回文件句柄和文件路径；失败时返回 nil。
func openLogFile(logDir string) (io.WriteCloser, string) {
	if logDir == "" {
		return nil, ""
	}

	if err := os.MkdirAll(logDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[logger] 创建日志目录失败: %v\n", err)
		return nil, ""
	}

	fileName := fmt.Sprintf("douya-%s.log", time.Now().Format(logFileDateFormat))
	logPath := filepath.Join(logDir, fileName)

	// 注：日志文件不收紧 ACL（icacls），本地单用户应用收益有限且可能导致运行时权限问题。
	// 见安全审查 #21（已评估，风险可接受）。
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[logger] 打开日志文件失败: %v\n", err)
		return nil, ""
	}

	return f, logPath
}

// cleanOldLogs 清理超过保留天数的日志文件。
func cleanOldLogs(logDir string) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return
	}

	cutoff := time.Now().AddDate(0, 0, -logRetentionDays)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 仅匹配 douya-YYYY-MM-DD.log 格式
		if !strings.HasPrefix(name, "douya-") || !strings.HasSuffix(name, ".log") {
			continue
		}

		dateStr := strings.TrimSuffix(strings.TrimPrefix(name, "douya-"), ".log")
		fileTime, err := time.Parse(logFileDateFormat, dateStr)
		if err != nil {
			continue
		}
		// 用日期的末尾时刻比较
		if fileTime.Add(24 * time.Hour).Before(cutoff) {
			if err := os.Remove(filepath.Join(logDir, name)); err == nil {
				log.Debug().Str("file", name).Msg("已清理过期日志")
			}
		}
	}
}
