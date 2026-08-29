// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import "douya/internal/version"

// GetAppVersion 返回当前应用版本号。
// 应用更新统一由 Microsoft Store 接管（商店政策 10.1.5 禁止应用内自更新），
// 此处仅用于"关于"页的版本展示。
func (a *App) GetAppVersion() string {
	return version.Version
}
