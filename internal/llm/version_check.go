// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

// versionRegexp 匹配 llama-server --version 输出中的版本号。
//
// llama-server --version 输出示例：
//
//	version: 10216 (876a43211)
//	built with MSVC 19.44.35227.0 for x64
//
// 本正则提取 "version:" 后面的数字（如 10216），即 llama.cpp 的构建编号。
// 构建编号是单调递增的整数，可直接用于大小比较。
//
// 生活类比：就像从快递单上抠出包裹编号（10216），用这个编号判断是不是最新批次。
var versionRegexp = regexp.MustCompile(`version:\s*(\d+)`)

// tagRegexp 匹配 GitHub release tag 名（如 "b10216"）。
// llama.cpp 的 release tag 格式固定为 "b" + 数字，数字与 --version 输出的构建编号一致。
var tagRegexp = regexp.MustCompile(`^b(\d+)$`)

// VersionInfo 描述版本检查结果。
//
// 生活类比：就像一张"版本对比报告"，写明你现在用的什么版本、最新版是什么、要不要更新。
type VersionInfo struct {
	LocalVersion    int    // 本地 llama-server 构建编号（如 10216），0 表示无法获取
	LocalCommit     string // 本地 llama-server commit hash（如 "876a43211"）
	RemoteVersion   int    // GitHub 最新 release 的构建编号（如 10220），0 表示查询失败
	RemoteTag       string // GitHub 最新 release 的 tag（如 "b10220"）
	HasUpdate       bool   // 是否有更新（RemoteVersion > LocalVersion）
	CheckError      string // 检查过程中的错误信息（空表示无错误）
}

// GetLocalVersion 执行 llama-server --version，解析本地版本号。
//
// 生活类比：去车库看一眼车的仪表盘，读出当前这辆车的批次编号。
//
// 参数：
//   - serverPath: llama-server.exe 的绝对路径
//
// 返回：
//   - version: 构建编号（如 10216），0 表示解析失败
//   - commit: commit hash（如 "876a43211"），空表示未输出
//   - err: 执行失败时的错误
func GetLocalVersion(serverPath string) (version int, commit string, err error) {
	cmd := exec.Command(serverPath, "--version")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, "", fmt.Errorf("执行 llama-server --version 失败: %w", err)
	}

	outputStr := string(output)
	log.Debug().Str("output", outputStr).Msg("[version] llama-server --version output")

	// 提取版本号（如 10216）
	matches := versionRegexp.FindStringSubmatch(outputStr)
	if len(matches) < 2 {
		return 0, "", fmt.Errorf("无法从版本输出中解析版本号: %s", strings.TrimSpace(outputStr))
	}
	version, err = strconv.Atoi(matches[1])
	if err != nil {
		return 0, "", fmt.Errorf("解析版本号 %q 失败: %w", matches[1], err)
	}

	// 提取 commit hash（括号内的字符串，如 "876a43211"）
	// 格式：version: 10216 (876a43211)
	if idx := strings.Index(outputStr, "("); idx >= 0 {
		endIdx := strings.Index(outputStr[idx:], ")")
		if endIdx > 1 {
			commit = strings.TrimSpace(outputStr[idx+1 : idx+endIdx])
		}
	}

	log.Info().
		Int("version", version).
		Str("commit", commit).
		Msg("[version] 本地 llama-server 版本")
	return version, commit, nil
}

// GetLatestReleaseTag 查询 GitHub API，获取 llama.cpp 最新 release 的 tag 和构建编号。
//
// 生活类比：打电话给应用商店（GitHub API），问"llama.cpp 最新版是什么编号？"。
//
// 复用 backend_download.go 中的 GitHubReleasesAPI 常量和 githubRelease 结构。
//
// 返回：
//   - version: 构建编号（如 10220），0 表示解析失败
//   - tag: release tag（如 "b10220"）
//   - err: 查询失败时的错误
func GetLatestReleaseTag() (version int, tag string, err error) {
	// 复用 backend_download.go 中的 GitHubReleasesAPI 常量
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", GitHubReleasesAPI, nil)
	if err != nil {
		return 0, "", fmt.Errorf("创建 GitHub API 请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Douya-LocalAI")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("GitHub API 返回非 200 状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", fmt.Errorf("读取 GitHub API 响应失败: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return 0, "", fmt.Errorf("解析 GitHub API 响应失败: %w", err)
	}

	tag = release.TagName
	// 从 tag（如 "b10220"）提取构建编号
	matches := tagRegexp.FindStringSubmatch(tag)
	if len(matches) < 2 {
		return 0, tag, fmt.Errorf("无法从 tag %q 解析构建编号", tag)
	}
	version, err = strconv.Atoi(matches[1])
	if err != nil {
		return 0, tag, fmt.Errorf("解析构建编号 %q 失败: %w", matches[1], err)
	}

	log.Info().
		Int("version", version).
		Str("tag", tag).
		Msg("[version] GitHub 最新 release")
	return version, tag, nil
}

// CheckForUpdate 组合本地版本查询和远程版本查询，返回完整的版本对比结果。
//
// 生活类比：先看自己车什么编号，再看商店最新什么编号，对比一下要不要换。
//
// 设计说明：
//   - 本地版本查询失败不阻断检查（仍返回 RemoteVersion 供调用方参考）
//   - 远程版本查询失败不阻断检查（仍返回 LocalVersion 供调用方参考）
//   - 两者都失败时返回 CheckError
//   - 本地版本 >= 远程版本时 HasUpdate=false（已是最新或领先）
//   - 本地版本 < 远程版本时 HasUpdate=true（有更新可用）
//
// 参数：
//   - serverPath: llama-server.exe 的绝对路径
//
// 返回：VersionInfo 结构体（即使出错也填充已获取的字段）
func CheckForUpdate(serverPath string) VersionInfo {
	info := VersionInfo{}

	// 查询本地版本
	localVer, localCommit, localErr := GetLocalVersion(serverPath)
	info.LocalVersion = localVer
	info.LocalCommit = localCommit
	if localErr != nil {
		log.Warn().Err(localErr).Msg("[version] 获取本地版本失败")
		info.CheckError = fmt.Sprintf("获取本地版本失败: %v", localErr)
		// 本地版本失败仍继续查询远程版本，供调用方参考
	}

	// 查询远程版本
	remoteVer, remoteTag, remoteErr := GetLatestReleaseTag()
	info.RemoteVersion = remoteVer
	info.RemoteTag = remoteTag
	if remoteErr != nil {
		log.Warn().Err(remoteErr).Msg("[version] 获取远程版本失败")
		if info.CheckError != "" {
			info.CheckError += "; "
		}
		info.CheckError += fmt.Sprintf("获取远程版本失败: %v", remoteErr)
		// 远程失败时无法判断是否有更新
		return info
	}

	// 两者都成功：对比版本
	if localErr != nil {
		// 本地失败但远程成功，无法判断
		return info
	}
	info.HasUpdate = remoteVer > localVer

	if info.HasUpdate {
		log.Info().
			Int("local", localVer).
			Int("remote", remoteVer).
			Str("remote_tag", remoteTag).
			Msg("[version] 检测到 llama.cpp 有更新")
	} else {
		log.Info().
			Int("local", localVer).
			Int("remote", remoteVer).
			Msg("[version] llama-server 已是最新版本")
	}

	return info
}
