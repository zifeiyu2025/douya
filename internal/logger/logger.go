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

// Init 初始化 zerolog，同时输出到控制台和日志文件。
// logDir 为日志文件存放目录（通常为 <appDir>/data/logs）。
// 若 logDir 为空或创建失败，则回退到仅控制台输出。
func Init(logDir string) {
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	// 尝试启用文件输出
	fileWriter, logFilePath := openLogFile(logDir)
	if fileWriter == nil {
		// 回退：仅控制台
		log.Logger = zerolog.New(consoleWriter).With().Timestamp().Logger()
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
		return
	}

	// 同时输出到控制台（彩色）和文件（JSON 格式，便于分析）
	multi := io.MultiWriter(consoleWriter, fileWriter)
	log.Logger = zerolog.New(multi).With().Timestamp().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// 清理过期日志文件
	cleanOldLogs(logDir)

	log.Info().Str("file", logFilePath).Msg("日志文件已启用")
}

// openLogFile 打开当天日志文件（按日期命名，追加模式）。
// 返回文件句柄和文件路径；失败时返回 nil。
func openLogFile(logDir string) (io.Writer, string) {
	if logDir == "" {
		return nil, ""
	}

	if err := os.MkdirAll(logDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "[logger] 创建日志目录失败: %v\n", err)
		return nil, ""
	}

	fileName := fmt.Sprintf("douya-%s.log", time.Now().Format(logFileDateFormat))
	logPath := filepath.Join(logDir, fileName)

	// 注：日志文件不收紧 ACL（icacls），本地单用户应用收益有限且可能导致运行时权限问题。
	// 见安全审查 #21（已评估，风险可接受）。
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
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
