// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"os"
	"path/filepath"
)

// FindInstalledBackend 在多个候选 runtime 目录中按优先级查找已安装的后端，
// 返回 llama-server.exe 的绝对路径；全部未命中返回空串。
//
// 背景：Microsoft Store (MSIX) 版的安装目录（WindowsApps 下）只读，但引擎随包分发；
// 数据目录（%LOCALAPPDATA%\Douya\runtime）可写，存放运行期下载的后端。
// 便携版两个目录通常是同一个，自动去重由调用方保证。
// 目录顺序即优先级：包内置目录在前，保证商店版"开箱即用"，不被数据目录旧文件干扰。
//
// 生活类比：找发动机时先查出厂自带的车库（安装目录），再查后补货的车库（数据目录），
// 哪个车库有现成的就直接用，不再下单购买（下载）。
//
// 参数：
//   - bt: 后端类型（BackendAuto 无具体子目录，直接返回空串）
//   - runtimeDirs: 候选 runtime 目录列表，顺序即查找优先级
func FindInstalledBackend(bt BackendType, runtimeDirs []string) string {
	info := GetBackendInfo(bt)
	if info.Subdir == "" {
		return ""
	}
	for _, dir := range runtimeDirs {
		if dir == "" {
			continue
		}
		serverPath := filepath.Join(dir, info.Subdir, llamaServerExe)
		if _, err := os.Stat(serverPath); err == nil {
			return serverPath
		}
	}
	return ""
}

// IsBackendInstalledIn 是 IsBackendInstalled 的多目录版本：
// 任一候选目录中存在该后端的 llama-server.exe 即视为已安装。
func IsBackendInstalledIn(bt BackendType, runtimeDirs []string) bool {
	return FindInstalledBackend(bt, runtimeDirs) != ""
}
