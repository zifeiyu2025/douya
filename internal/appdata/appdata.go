// Package appdata 管理应用数据目录（Microsoft Store 版唯一数据根）。
//
// 豆芽仅发布微软商店（MSIX）一个版本：安装目录（WindowsApps 下）只读，
// 配置/数据/模型统一写入 %LOCALAPPDATA%\Douya。本包是该目录的唯一事实来源，
// 并负责旧版本遗留数据的一次性迁移（见 migrate.go）。
package appdata

import (
	"os"
	"path/filepath"

	zlog "github.com/rs/zerolog/log"
)

// DataDir 返回应用数据目录的原始路径（不创建目录）。
//
// 回退顺序：LOCALAPPDATA → APPDATA → exe 所在目录，
// 最终统一追加 "Douya" 子目录。
func DataDir(exePath string) string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.Getenv("APPDATA")
	}
	if base == "" {
		base = filepath.Dir(exePath)
	}
	return filepath.Join(base, "Douya")
}

// EnsureDataDir 确保应用数据目录存在并完成旧版本遗留数据的一次性迁移，返回该目录。
//
// exePath 同时用于 LOCALAPPDATA 缺失时的数据目录回退定位，
// 以及旧数据迁移的候选源目录（exe 同目录、exe 上级目录）。
// 迁移失败降级为日志告警，不阻塞启动。
func EnsureDataDir(exePath string) string {
	dir := DataDir(exePath)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		zlog.Error().Err(err).Str("dir", dir).Msg("[appdata] 创建数据目录失败")
	}

	exeDir := filepath.Dir(exePath)
	candidates := []string{exeDir}
	if parent := filepath.Dir(exeDir); parent != exeDir {
		candidates = append(candidates, parent)
	}
	MigrateLegacyData(dir, candidates)

	zlog.Info().Str("dir", dir).Msg("[appdata] 应用数据目录")
	return dir
}
