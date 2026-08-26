// Package distinfo 是发行渠道（便携版/商店版）信息的唯一事实来源。
//
// 生活类比：应用出厂时贴的"身份标签"——是用户拎包即用的便携版（portable），
// 还是从微软商店安装、由商店统一管理的商店版（store）。全项目所有需要
// 区分发行渠道的逻辑（数据目录选择、自更新禁用等）都必须从这里读取，
// 禁止再散落各自的路径检测，避免多处判断不一致（见 docs/code-audit-report.md §4.1）。
package distinfo

import (
	"os"
	"strings"
	"sync"
)

// Channel 标识应用的发行渠道。
type Channel string

const (
	// ChannelPortable 便携版：exe 与配置/数据同目录（或直接上级），功能完整。
	ChannelPortable Channel = "portable"
	// ChannelStore 微软商店版（MSIX）：安装目录只读，数据写入 %LOCALAPPDATA%\Douya，
	// 由商店负责更新（政策 10.1.5，禁止应用内自更新）。
	ChannelStore Channel = "store"
)

var (
	detected   Channel
	detectOnce sync.Once
)

// DetectChannel 根据可执行文件路径检测发行渠道。
//
// MSIX 打包的应用安装在 %ProgramFiles%\WindowsApps\<PackageFullName>\ 下，
// 该目录为系统保护的只读目录。因此以 exe 路径是否位于 WindowsApps 目录
// 作为商店版的判定依据（大小写不敏感）。
//
// exePath 为空或获取失败时保守返回便携版（与既有行为一致：检测失败按便携版处理）。
func DetectChannel(exePath string) Channel {
	if exePath == "" {
		return ChannelPortable
	}
	if strings.Contains(strings.ToLower(exePath), `\windowsapps\`) {
		return ChannelStore
	}
	return ChannelPortable
}

// Detect 检测当前进程的发行渠道，结果进程内缓存（只检测一次）。
func Detect() Channel {
	detectOnce.Do(func() {
		exePath, err := os.Executable()
		if err != nil {
			detected = ChannelPortable
			return
		}
		detected = DetectChannel(exePath)
	})
	return detected
}

// IsStore 返回当前进程是否为微软商店版（MSIX）。
func IsStore() bool {
	return Detect() == ChannelStore
}
