package appdata

import (
	"io"
	"os"
	"path/filepath"

	zlog "github.com/rs/zerolog/log"
)

// markerFileName 一次性迁移完成标记。存在即表示迁移流程已执行过，
// 避免每次启动都扫描候选目录。
const markerFileName = ".legacy_migrated"

// MigrationResult 记录一次性迁移的执行情况，供日志与测试断言。
type MigrationResult struct {
	// Skipped 为 true 表示无需迁移（目标已初始化或已迁移过）。
	Skipped bool
	// MigratedConfig 表示 config.json 是否被迁移。
	MigratedConfig bool
	// MigratedFiles 表示 data/ 目录下成功复制的文件数。
	MigratedFiles int
	// FailedFiles 表示复制失败的文件数（失败不阻塞启动，仅记日志）。
	FailedFiles int
}

// MigrateLegacyData 将旧版本遗留在安装目录旁的数据一次性迁移到
// 应用数据目录（%LOCALAPPDATA%\Douya）。见 docs/code-audit-report.md §4.1：
// 数据目录迁移需提供一次性迁移逻辑，否则老用户升级后聊天记录"凭空消失"。
//
// 迁移规则：
//   - dst 中已存在 config.json：视为新目录已初始化，直接跳过（用户已在新版产生数据，
//     绝不能用旧数据覆盖）；
//   - 存在迁移完成标记：跳过（幂等）；
//   - 依次检查 candidates（通常为 exe 同目录、exe 上级目录），取第一个含
//     config.json 或 data/ 的目录作为旧数据源；
//   - 只迁移轻量个人数据：config.json 与 data/ 目录（聊天记录、数据库、密钥、RAG 等）；
//     models/、runtime/ 体积巨大且按需下载，不迁移；
//   - 单个文件失败不中断整体迁移，仅计数并记日志（失败降级，保证启动不受阻）。
//
// 该函数为纯路径驱动的独立函数，便于单元测试注入临时目录。
func MigrateLegacyData(dst string, candidates []string) MigrationResult {
	cfgPath := filepath.Join(dst, "config.json")
	if _, err := os.Stat(cfgPath); err == nil {
		zlog.Info().Str("dst", dst).Msg("[appdata] 目标目录已有配置，跳过旧数据迁移")
		return MigrationResult{Skipped: true}
	}
	markerPath := filepath.Join(dst, markerFileName)
	if _, err := os.Stat(markerPath); err == nil {
		return MigrationResult{Skipped: true}
	}

	src := findLegacySource(candidates)
	if src == "" {
		// 无旧数据也写入标记，避免每次启动重复扫描
		_ = writeMarker(markerPath)
		return MigrationResult{Skipped: true}
	}
	zlog.Info().Str("src", src).Str("dst", dst).Msg("[appdata] 发现旧版遗留数据，开始一次性迁移")

	var result MigrationResult

	// 1. 迁移 config.json
	if err := copyFile(filepath.Join(src, "config.json"), cfgPath); err == nil {
		result.MigratedConfig = true
	} else if !os.IsNotExist(err) {
		result.FailedFiles++
		zlog.Warn().Err(err).Msg("[appdata] 迁移 config.json 失败")
	}

	// 2. 迁移 data/ 目录（逐文件复制，单文件失败继续）
	result.MigratedFiles, result.FailedFiles = copyDir(
		filepath.Join(src, "data"), filepath.Join(dst, "data"),
		result.MigratedFiles, result.FailedFiles,
	)

	if err := writeMarker(markerPath); err != nil {
		zlog.Warn().Err(err).Msg("[appdata] 写入迁移标记失败（下次启动将重新扫描）")
	}
	zlog.Info().
		Int("files", result.MigratedFiles).
		Int("failed", result.FailedFiles).
		Bool("config", result.MigratedConfig).
		Msg("[appdata] 旧数据迁移完成")
	return result
}

// findLegacySource 返回第一个含 config.json 或 data/ 的候选目录；无则返回空串。
func findLegacySource(candidates []string) string {
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
			return dir
		}
		if info, err := os.Stat(filepath.Join(dir, "data")); err == nil && info.IsDir() {
			return dir
		}
	}
	return ""
}

// copyDir 递归复制目录内容，返回 (成功数, 失败数)。src 不存在时原样返回计数。
func copyDir(src, dst string, copied, failed int) (int, int) {
	entries, err := os.ReadDir(src)
	if err != nil {
		return copied, failed
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		zlog.Warn().Err(err).Str("dir", dst).Msg("[appdata] 创建目标子目录失败")
		return copied, failed + len(entries)
	}
	for _, entry := range entries {
		s := filepath.Join(src, entry.Name())
		d := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			copied, failed = copyDir(s, d, copied, failed)
			continue
		}
		if err := copyFile(s, d); err != nil {
			failed++
			zlog.Warn().Err(err).Str("file", s).Msg("[appdata] 迁移文件失败")
			continue
		}
		copied++
	}
	return copied, failed
}

// copyFile 复制单个文件；源不存在时返回包装了 NotExist 的错误，由调用方区分处理。
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func writeMarker(markerPath string) error {
	return os.WriteFile(markerPath, []byte("migrated"), 0o644)
}
