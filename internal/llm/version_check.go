// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"douya/internal/apperror"

	"github.com/rs/zerolog/log"
)

// versionRegexp 匹配旧版 llama-server --version 输出中的构建编号。
//
// 旧格式（无语义版本）：
//
//	version: 10216 (876a43211)
//
// 该正则提取 "version:" 后面的数字（如 10216），即 llama.cpp 的构建编号。
// 构建编号是单调递增的整数，可直接用于大小比较。
//
// 生活类比：就像从快递单上抠出包裹编号（10216），用这个编号判断是不是最新批次。
var versionRegexp = regexp.MustCompile(`version:\s*(\d+)`)

// buildNumberRegexp 匹配新版 llama-server --version 输出中的构建编号。
//
// 上游 b104xx 起引入语义版本（#26838），--version 输出变成（见 common/build-info.cpp.in）：
//
//	version: 0.1.0 (build 10424, commit 030ebb558)
//	built with MSVC 19.44.35227.0 for x64
//
// 此时 "version:" 后面是语义版本号（0.1.0），不再是构建编号，必须改从
// "build 10424" 中提取构建编号。构建编号仍是单调递增整数，可直接比较。
var buildNumberRegexp = regexp.MustCompile(`build\s*(\d+)`)

// commitRegexp 匹配新版 --version 输出中的 commit hash。
// 如 "commit 030ebb558" 或 "commit: 030ebb558"。
var commitRegexp = regexp.MustCompile(`commit[:\s]+([0-9a-f]+)`)

// tagRegexp 匹配 GitHub release tag 名（如 "b10216"）。
// llama.cpp 的 nightly release tag 格式固定为 "b" + 数字，数字与 --version 输出的构建编号一致。
var tagRegexp = regexp.MustCompile(`^b(\d+)$`)

// assetBuildNumberRegexp 从 asset 文件名中提取构建编号。
// 如 "llama-b10549-bin-win-cuda-13.3-x64.zip" 提取 10549。
// 用于 release tag 是语义化版本（vX.Y.Z）但 asset 文件名仍带 b 构建编号的兜底场景。
var assetBuildNumberRegexp = regexp.MustCompile(`-b(\d+)-`)

// VersionInfo 描述版本检查结果。
//
// 生活类比：就像一张"版本对比报告"，写明你现在用的什么版本、最新版是什么、要不要更新。
type VersionInfo struct {
	LocalVersion  int    // 本地 llama-server 构建编号（如 10216），0 表示无法获取
	LocalCommit   string // 本地 llama-server commit hash（如 "876a43211"）
	RemoteVersion int    // GitHub 最新 release 的构建编号（如 10220），0 表示查询失败
	RemoteTag     string // GitHub 最新 release 的 tag（如 "b10220"）
	HasUpdate     bool   // 是否有更新（RemoteVersion > LocalVersion）
	CheckError    string // 检查过程中的错误信息（空表示无错误）
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
		return 0, "", apperror.Wrap(apperror.KindInternal, "执行 llama-server --version 失败", err)
	}

	outputStr := string(output)
	log.Debug().Str("output", outputStr).Msg("[version] llama-server --version output")

	version, commit, err = parseServerVersionOutput(outputStr)

	log.Info().
		Int("version", version).
		Str("commit", commit).
		Msg("[version] 本地 llama-server 版本")
	return version, commit, err
}

// parseServerVersionOutput 从 llama-server --version 输出中解析构建编号与 commit hash。
//
// 兼容两种输出格式：
//
//	旧格式（无语义版本）：version: 10216 (876a43211)
//	新格式（语义版本）  ：version: 0.1.0 (build 10424, commit 030ebb558)
//
// 构建编号优先取 "build N"（新格式），其次取 "version: N"（旧格式）；
// commit hash 优先取 "commit <hash>"（新格式），其次取括号内容（旧格式）。
func parseServerVersionOutput(output string) (version int, commit string, err error) {
	outputStr := strings.TrimSpace(output)

	// 提取构建编号：优先新版 "build N"，其次旧版 "version: N"。
	// 记录来源格式：新版括号内是 "build N"，旧版括号内才是 commit hash。
	versionStr := ""
	isNewFormat := false
	if matches := buildNumberRegexp.FindStringSubmatch(outputStr); len(matches) >= 2 {
		versionStr = matches[1]
		isNewFormat = true
	} else if matches := versionRegexp.FindStringSubmatch(outputStr); len(matches) >= 2 {
		versionStr = matches[1]
	}
	if versionStr == "" {
		return 0, "", apperror.Newf(apperror.KindInternal, "无法从版本输出中解析构建编号: %s", outputStr)
	}

	version, convErr := strconv.Atoi(versionStr)
	if convErr != nil {
		return 0, "", apperror.Wrapf(apperror.KindInternal, "解析构建编号 %q 失败", convErr, versionStr)
	}

	// 提取 commit hash：优先新版 "commit <hash>"。
	// 仅旧格式才用括号内容作为 commit；新版括号内是 build 号，不作 commit。
	if matches := commitRegexp.FindStringSubmatch(outputStr); len(matches) >= 2 {
		commit = matches[1]
	} else if !isNewFormat {
		if idx := strings.Index(outputStr, "("); idx >= 0 {
			endIdx := strings.Index(outputStr[idx:], ")")
			if endIdx > 1 {
				commit = strings.TrimSpace(outputStr[idx+1 : idx+endIdx])
			}
		}
	}

	return version, commit, nil
}

// GetLatestReleaseTag 查询 GitHub API，获取 llama.cpp 当前锁定版本的 tag 和构建编号。
//
// 生活类比：打电话给应用商店（GitHub API），问"当前适配认可的版本是什么编号？"。
//
// 复用 backend_download.go 中的 fetchGitHubLatestRelease：
// 版本固定策略下返回的是 PinnedReleaseTag 锁定的已验证版本（而非上游绝对最新）。
// 因此 HasUpdate=true 的含义是"存在比本地更新的已验证版本"，用户据此点击
// 更新时下载到的正是该验证版本——提示与下载永远一致，不会引导用户
// 拿到未经适配的后端。
//
// 返回：
//   - version: 构建编号（如 10605），0 表示解析失败
//   - tag: release tag（如 "b10605"）
//   - err: 查询失败时的错误
func GetLatestReleaseTag() (version int, tag string, err error) {
	release, err := fetchGitHubLatestRelease()
	if err != nil {
		return 0, "", err
	}

	tag = release.TagName
	// 从 tag（如 "b10220"）提取构建编号
	if matches := tagRegexp.FindStringSubmatch(tag); len(matches) >= 2 {
		version, err = strconv.Atoi(matches[1])
		if err != nil {
			return 0, tag, apperror.Wrapf(apperror.KindInternal, "解析构建编号 %q 失败", err, matches[1])
		}
	} else {
		// 兜底：tag 是语义化版本（vX.Y.Z，如稳定版 release 直接带二进制），
		// 从 asset 文件名中提取构建编号（如 llama-b10549-bin-win-cuda-13.3-x64.zip → 10549）
		version = extractBuildFromAssets(release.Assets)
		if version == 0 {
			return 0, tag, apperror.Newf(apperror.KindInternal, "无法从 tag %q 及其资产中解析构建编号", tag)
		}
		log.Info().
			Str("tag", tag).
			Int("version", version).
			Msg("[version] tag 为语义化版本，已从 asset 文件名提取构建编号")
	}

	log.Info().
		Int("version", version).
		Str("tag", tag).
		Msg("[version] GitHub 最新 release")
	return version, tag, nil
}

// extractBuildFromAssets 从 release 的资产文件名列表中提取构建编号。
// 遍历所有资产，找到第一个形如 "llama-b10549-..." 的文件名并返回其中的构建编号。
// 所有资产都无法提取时返回 0。
func extractBuildFromAssets(assets []GitHubAsset) int {
	for _, a := range assets {
		if matches := assetBuildNumberRegexp.FindStringSubmatch(a.Name); len(matches) >= 2 {
			if n, err := strconv.Atoi(matches[1]); err == nil {
				return n
			}
		}
	}
	return 0
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
