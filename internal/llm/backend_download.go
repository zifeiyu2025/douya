// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

// PinnedReleaseTag 是当前锁定的 llama.cpp 官方预编译版本 tag。
//
// 设计意图（版本固定策略）：应用只下载经过人工适配验证的官方预编译资产，
// 不再自动跟随上游最新构建。原因：llama.cpp 迭代很快，若上游出现较大变化
// （参数改名、行为变更、资产结构调整），自动跟随会导致新下载的后端与
// 应用适配逻辑不兼容，出现模型加载失败等问题。
//
// 当前锁定 b10675（2026-08-29 升级）：已完成三重验证——
//  1. 服务端参数面：本应用传递的全部参数（含 -b/-c/-t 短选项与 --no-* 变体）
//     与 b10675 的 common/arg.cpp 逐一比对，无一缺失（上游 b10599→b10675 仅新增
//     --kv-unified-per-slot / --n-cpu-ffn / --video-* 等参数，无删除改名）；
//  2. 资产命名：该 release 实测含 cpu-x64 / cuda-12.4 / cuda-13.3-x64 /
//     vulkan-x64 引擎包及 cudart-13.3 配套包，均匹配现有资产正则；
//  3. 实测：按官方 release.yml 参数本地编译的三套引擎（b10675）已通过
//     CUDA 后端加载与应用端到端冒烟，引擎与下载包同源同版本。
//
// 升级流程：上游发布新版后，人工验证兼容性（重点核对 --server 参数面与
// 资产命名规则），确认无误后将此常量改为新 tag（如 "b10800"）即可，
// 后端下载与版本更新检查会同时跟随新版本。
const PinnedReleaseTag = "b10675"

// githubReleasesTagsBase 是按 tag 查询单个 release 的 GitHub API 地址前缀。
const githubReleasesTagsBase = "https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/"

// GitHubReleasesAPI 是 llama.cpp 固定版本的 release 详情 API 地址。
// 已从 releases/latest 改为指向 PinnedReleaseTag 锁定的 release，
// 确保下载与更新检查只面向经过验证的版本，不随上游漂移。
const GitHubReleasesAPI = githubReleasesTagsBase + PinnedReleaseTag

// GitHubReleasesListAPI 是 llama.cpp releases 列表的 GitHub API 地址（从新到旧）。
// llama.cpp 2026-08-21 起采用语义化版本双轨发布：稳定版（vX.Y.Z）release 只含
// nightly-tag.txt 指针文件，没有二进制；二进制包仍在 nightly（bXXXXX）release 中。
// 版本固定模式下，固定 tag 的 release 直接含二进制资产，此列表查询仅作为
// 异常情况的兜底路径保留（见 fetchLatestBinaryRelease）。
const GitHubReleasesListAPI = "https://api.github.com/repos/ggml-org/llama.cpp/releases?per_page=15"

// githubUA 是请求 GitHub API 时使用的 User-Agent 标识。
// GitHub 要求所有 API 请求必须携带 User-Agent（https://docs.github.com/en/rest/overview/resources-in-the-rest-api#user-agent-required），
// 使用项目名标识而非个人标识，保护用户隐私。
const githubUA = "Douya-LocalAI"

// githubHTTPClient 复用 TCP 连接池，避免每次 GitHub API 查询都新建 http.Client。
// 生活类比：保留固定电话线而不是每次都临时拉一根。
var githubHTTPClient = &http.Client{
	Timeout: 30 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
	},
}

// downloadHTTPClient 用于文件下载，不限超时（大文件下载可能耗时很长）。
var downloadHTTPClient = &http.Client{
	Timeout: 0,
	Transport: &http.Transport{
		MaxIdleConns:    10,
		IdleConnTimeout: 90 * time.Second,
	},
}

// githubDownloadProxyPrefixes 是 GitHub release 下载的加速代理镜像前缀列表。
// 生活类比：主路（GitHub 原始链接）太堵，就绕几条"小路"（国内加速镜像）去取货。
//
// 构造方式：在原 GitHub release 下载直链前拼上镜像前缀，例如：
//
//	原始：https://github.com/ggml-org/llama.cpp/releases/download/b8581/xxx.zip
//	代理：https://gh-proxy.com/https://github.com/ggml-org/llama.cpp/releases/download/b8581/xxx.zip
//
// 注意：此类社区加速镜像时效性不稳定，可能失效。若某个镜像不可用，
// 直接在此列表中删除该条目或替换为新镜像即可，无需改动下载逻辑。
var githubDownloadProxyPrefixes = []string{
	"https://gh-proxy.com/",
}

// buildDownloadURLs 根据原始 GitHub 下载直链，生成"候选下载源列表"。
// 列表顺序即下载尝试顺序：优先走加速代理（国内快），最后回落原始 GitHub 源兜底。
// 对非 GitHub 域名（如未来接入其他源）的 URL 直接原样返回单个候选，不做代理拼接。
func buildDownloadURLs(originalURL string) []string {
	urls := make([]string, 0, len(githubDownloadProxyPrefixes)+1)
	for _, prefix := range githubDownloadProxyPrefixes {
		urls = append(urls, prefix+originalURL)
	}
	urls = append(urls, originalURL)
	return urls
}

// fetchGitHubLatestRelease 查询 GitHub API，获取 llama.cpp 当前锁定版本的 release。
// 该函数被 FindReleaseAsset、FindCudartAsset 和 GetLatestReleaseTag 共用。
//
// 版本固定策略：不再查询 releases/latest 追上游最新，而是查询固定 tag
// （PinnedReleaseTag）对应的 release，保证下载与更新检查只面向经过验证的版本，
// 防止上游较大变化导致新下载的后端加载失败。
//
// 兼容两种发布模式的兜底逻辑（fetchLatestBinaryRelease）保持不变：
// 正常情况下固定 tag 的 release 直接含二进制 zip（旧模式直通路径）；
// 若固定 tag 意外不含二进制资产（异常情况），仍会尝试从 release 列表寻找可用二进制。
//
// 生活类比：不再每天问仓库"最新一批货是啥"，而是固定去熟悉的批次号提货——
// 那一批是我们开箱验过货的，不会拿到对不上型号的零件。
func fetchGitHubLatestRelease() (*GitHubRelease, error) {
	return fetchLatestBinaryRelease(GitHubReleasesAPI, GitHubReleasesListAPI)
}

// fetchLatestBinaryRelease 是 fetchGitHubLatestRelease 的可测试核心。
// latestURL 查询单个 release，listURL 查询 release 列表（从新到旧）。
// 由 fetchGitHubLatestRelease 用真实 GitHub API 地址调用，测试用 httptest 地址调用。
func fetchLatestBinaryRelease(latestURL, listURL string) (*GitHubRelease, error) {
	release, err := fetchRelease(latestURL)
	if err != nil {
		return nil, err
	}

	// release 含 zip 二进制资产（旧模式 nightly）→ 直接使用
	if releaseHasBinaryAsset(release) {
		return release, nil
	}

	// 新模式：latest 是稳定版指针（如 v0.2.0，只有 nightly-tag.txt）
	// 改查 releases 列表，从新到旧找第一个含二进制资产的 release（最新 nightly）
	log.Info().
		Str("tag", release.TagName).
		Msg("[backend] 最新 release 无二进制资产（语义化版本发布模式），改查 release 列表")

	list, err := fetchReleaseList(listURL)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if releaseHasBinaryAsset(&list[i]) {
			log.Info().
				Str("tag", list[i].TagName).
				Msg("[backend] 已找到最新含二进制资产的 release")
			return &list[i], nil
		}
	}

	return nil, apperror.Newf(apperror.KindNotFound, "最近 %d 个 release 中均未找到二进制资产", len(list))
}

// releaseHasBinaryAsset 判断 release 是否含有二进制 zip 资产。
// llama.cpp 稳定版（vX.Y.Z）release 只含 nightly-tag.txt 指针文件；
// nightly（bXXXXX）release 含 *.zip 二进制包。
func releaseHasBinaryAsset(release *GitHubRelease) bool {
	for _, a := range release.Assets {
		if strings.HasSuffix(a.Name, ".zip") {
			return true
		}
	}
	return false
}

// fetchRelease 查询单个 release API 并解析为 GitHubRelease。
func fetchRelease(apiURL string) (*GitHubRelease, error) {
	req, err := http.NewRequest("GET", apiURL, http.NoBody)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "创建 GitHub API 请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := githubHTTPClient.Do(req)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "请求 GitHub API 失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, apperror.Newf(apperror.KindUnavailable, "GitHub API 返回非 200 状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "读取 GitHub API 响应失败", err)
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "解析 GitHub API 响应失败", err)
	}

	return &release, nil
}

// fetchReleaseList 查询 GitHub releases 列表 API（返回结果从新到旧）。
// 用于 releases/latest 是稳定版指针时，从列表中查找最新的含二进制资产的 nightly release。
func fetchReleaseList(apiURL string) ([]GitHubRelease, error) {
	req, err := http.NewRequest("GET", apiURL, http.NoBody)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "创建 GitHub API 请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := githubHTTPClient.Do(req)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "请求 GitHub API 失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, apperror.Newf(apperror.KindUnavailable, "GitHub API 返回非 200 状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "读取 GitHub API 响应失败", err)
	}

	var list []GitHubRelease
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "解析 GitHub API 响应失败", err)
	}

	return list, nil
}

// DownloadProgress 下载进度信息，通过回调函数推送给调用方。
//
// 生活类比：就像快递追踪——已发货（开始下载）、运输中（下载中，进度 45%）、
// 已签收（下载完成）。
type DownloadProgress struct {
	Backend    BackendType // 后端类型
	AssetName  string      // GitHub asset 文件名，如 "llama-b10167-bin-win-cuda-13.3-x64.zip"
	TagName    string      // GitHub release 标签，如 "b10167"
	TotalBytes int64       // 文件总大小（字节），未知时为 0
	Downloaded int64       // 已下载字节数
	Percent    float64     // 下载百分比（0-100）
	Status     string      // 状态："downloading" / "completed" / "failed" / "installing" / "retrying"
	Error      string      // 失败时的错误信息
	Label      string      // 当前下载内容的描述，如"推理后端"、"cudart 依赖包"
}

// GitHubAsset 表示 GitHub release 中的一个资源文件。
// 仅提取下载所需的字段，忽略其他元数据。
// 同时被 main 包更新检查（app_update.go）复用，字段变更需两端同步确认。
type GitHubAsset struct {
	Name               string `json:"name"`                 // 文件名，如 "llama-b10167-bin-win-cuda-13.3-x64.zip"
	BrowserDownloadURL string `json:"browser_download_url"` // 直链下载地址
	Size               int64  `json:"size"`                 // 文件大小（字节）
}

// GitHubRelease 表示 GitHub release 的精简结构。
// 同时被 main 包更新检查（app_update.go）复用，字段变更需两端同步确认。
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`     // release 标签，如 "b10167"
	Body        string        `json:"body"`         // release 说明（更新检查展示用）
	PublishedAt string        `json:"published_at"` // release 发布时间（更新检查展示用）
	Assets      []GitHubAsset `json:"assets"`       // release 包含的资源文件列表
}

// FindReleaseAsset 查询 GitHub API，在最新 release 中匹配指定后端的 asset。
//
// 生活类比：打电话给仓库（GitHub API）问"最新一批货里有没有 CUDA 13 的发动机？"，
// 仓库返回货品名称和提货地址（download URL）。
//
// 流程：
//  1. 调用 GitHub /releases/latest API
//  2. 用 ReleaseAssetRegex 匹配 asset 名称
//  3. 返回匹配到的 asset 信息
//
// 参数：
//   - bt: 后端类型（不能是 BackendAuto）
//
// 返回：匹配到的 asset、release 标签名，或错误
func FindReleaseAsset(bt BackendType) (asset GitHubAsset, tagName string, err error) {
	if bt == BackendAuto {
		return GitHubAsset{}, "", apperror.New(apperror.KindInvalidInput, "BackendAuto 需先通过 ResolveBackendType 解析成具体后端")
	}

	info := GetBackendInfo(bt)
	if info.ReleaseAssetRegex == "" {
		return GitHubAsset{}, "", apperror.Newf(apperror.KindInvalidConfig, "后端 %s 没有定义 ReleaseAssetRegex", info.DisplayName)
	}

	// 编译正则
	re, err := regexp.Compile(info.ReleaseAssetRegex)
	if err != nil {
		return GitHubAsset{}, "", apperror.Wrap(apperror.KindInvalidConfig, "编译 ReleaseAssetRegex 失败", err)
	}

	// 查询 GitHub API（使用复用连接池的共享 client）
	release, err := fetchGitHubLatestRelease()
	if err != nil {
		return GitHubAsset{}, "", err
	}

	// 在 assets 中匹配对应后端，收集所有匹配项
	// CUDA 后端官方同时提供 12.x 和 13.x 两个版本，需要优先选 13.x
	var matched []GitHubAsset
	for _, a := range release.Assets {
		if re.MatchString(a.Name) {
			matched = append(matched, a)
		}
	}

	if len(matched) == 0 {
		return GitHubAsset{}, release.TagName, apperror.Newf(apperror.KindNotFound, "在最新 release %s 中未找到匹配 %s 后端的 asset（正则: %s）",
			release.TagName, info.DisplayName, info.ReleaseAssetRegex)
	}

	// CUDA 后端优先选高版本（13.x 优于 12.x），豆芽主用 cudart64_13.dll
	// 其他后端只有一个匹配项，排序不影响
	if bt == BackendCUDA && len(matched) > 1 {
		sort.Slice(matched, func(i, j int) bool {
			return cudaMajorVersion(matched[i].Name) > cudaMajorVersion(matched[j].Name)
		})
		log.Info().
			Str("backend", bt.String()).
			Ints("versions", []int{cudaMajorVersion(matched[0].Name), cudaMajorVersion(matched[len(matched)-1].Name)}).
			Msg("[backend] CUDA 多版本匹配，已优先选择高版本")
	}

	a := matched[0]
	log.Info().
		Str("backend", bt.String()).
		Str("tag", release.TagName).
		Str("asset", a.Name).
		Str("url", a.BrowserDownloadURL).
		Int64("size", a.Size).
		Msg("[backend] 匹配到 GitHub release asset")
	return a, release.TagName, nil
}

// cudaMajorVersion 从 CUDA asset 名中提取大版本号（如 13.3 → 13），用于优先级排序。
// 无法提取时返回 0（排序时排在最后）。
func cudaMajorVersion(assetName string) int {
	matches := cudaVersionPriorityRegex.FindStringSubmatch(assetName)
	if len(matches) < 2 {
		return 0
	}
	n, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return n
}

// DownloadBackendZip 从 GitHub 下载指定后端的 zip 包到 runtimeDir 目录。
//
// 生活类比：下单提货——从 GitHub 仓库把货（zip 包）运到本地车库（runtime 目录），
// 运输过程中实时报告进度（已运多少、还剩多少）。
//
// 流程：
//  1. 调用 FindReleaseAsset 获取最新 release 中匹配的 asset
//  2. 流式下载到 {runtimeDir}/{assetName}.tmp（临时文件）
//  3. 下载完成后重命名为最终文件名
//  4. 通过 progressCB 回调推送下载进度
//
// 参数：
//   - bt: 后端类型（不能是 BackendAuto）
//   - runtimeDir: runtime 目录的绝对路径
//   - progressCB: 下载进度回调（可为 nil）
//
// 返回：下载完成的 zip 文件绝对路径，或错误
//
// P3.3 重构：原实现与 DownloadBackendZipWithContext 几乎完全重复（~150 行），
// 现统一委托给带 context 的实现（传 context.Background()），消除复制粘贴。
func DownloadBackendZip(bt BackendType, runtimeDir string, progressCB func(DownloadProgress)) (string, error) {
	return DownloadBackendZipWithContext(context.Background(), bt, runtimeDir, progressCB)
}

// cudartAssetRegex 匹配 CUDA 后端附带的 cudart 包
// 全量适配：同时匹配 CUDA 12.x 和 13.x
// 例如：cudart-llama-bin-win-cuda-12.4-x64.zip、cudart-llama-bin-win-cuda-13.3-x64.zip
// 优先级见 FindCudartAsset 的优先选择逻辑
var cudartAssetRegex = regexp.MustCompile(`^cudart-llama-bin-win-cuda-1[23]\.\d+-x64\.zip$`)

// cudaVersionPriorityRegex 用于从 CUDA asset 名中提取版本号，用于优先级排序。
// 例如 "llama-b10228-bin-win-cuda-13.3-x64.zip" 提取 "13.3"，13.x 优先于 12.x
var cudaVersionPriorityRegex = regexp.MustCompile(`cuda-(\d+)\.(\d+)`)

// FindCudartAsset 在 GitHub latest release 中查找 CUDA 的 cudart 附带包。
// 仅在 CUDA 后端下载时需要额外下载此包，解压到同一目录提供 cudart64_*.dll 等厂商 DLL。
//
// 生活类比：买发动机时附带的"配件包"——发动机主包是引擎本体，cudart 包是配套的管线和接头。
func FindCudartAsset() (asset GitHubAsset, tagName string, err error) {
	// 查询 GitHub API（使用复用连接池的共享 client）
	release, err := fetchGitHubLatestRelease()
	if err != nil {
		return GitHubAsset{}, "", err
	}

	// 收集所有匹配的 cudart asset（12.x 和 13.x）
	var matched []GitHubAsset
	for _, a := range release.Assets {
		if cudartAssetRegex.MatchString(a.Name) {
			matched = append(matched, a)
		}
	}

	if len(matched) == 0 {
		return GitHubAsset{}, release.TagName, apperror.Newf(apperror.KindNotFound, "在最新 release %s 中未找到 cudart 包", release.TagName)
	}

	// 优先选高版本（13.x 优于 12.x），与主后端版本保持一致
	if len(matched) > 1 {
		sort.Slice(matched, func(i, j int) bool {
			return cudaMajorVersion(matched[i].Name) > cudaMajorVersion(matched[j].Name)
		})
		log.Info().
			Str("tag", release.TagName).
			Ints("versions", []int{cudaMajorVersion(matched[0].Name), cudaMajorVersion(matched[len(matched)-1].Name)}).
			Msg("[backend] cudart 多版本匹配，已优先选择高版本")
	}

	a := matched[0]
	log.Info().
		Str("tag", release.TagName).
		Str("asset", a.Name).
		Str("url", a.BrowserDownloadURL).
		Int64("size", a.Size).
		Msg("[backend] 匹配到 cudart release asset")
	return a, release.TagName, nil
}

// DownloadCudartZip 下载 CUDA 的 cudart 附带包到 runtimeDir 目录。
// CUDA 后端需要主包（llama-*-bin-win-cuda-*.zip）和 cudart 包一起解压到同一目录，
// cudart 包提供 cudart64_*.dll、cublas64_*.dll 等厂商运行时 DLL。
func DownloadCudartZip(runtimeDir string, progressCB func(DownloadProgress)) (string, error) {
	asset, tagName, err := FindCudartAsset()
	if err != nil {
		return "", apperror.Wrap(apperror.KindUnavailable, "查找 cudart release asset 失败", err)
	}

	destPath := filepath.Join(runtimeDir, asset.Name)
	if _, err := os.Stat(destPath); err == nil {
		log.Info().Str("path", destPath).Msg("[backend] cudart zip 包已存在，跳过下载")
		return destPath, nil
	}

	log.Info().
		Str("asset", asset.Name).
		Str("tag", tagName).
		Str("url", asset.BrowserDownloadURL).
		Msg("[backend] 开始下载 cudart zip 包")

	if err := downloadFile(asset.BrowserDownloadURL, destPath, asset.Size, BackendCUDA, asset.Name, tagName, progressCB); err != nil {
		return "", apperror.Wrap(apperror.KindUnavailable, "下载 cudart zip 包失败", err)
	}

	log.Info().
		Str("path", destPath).
		Str("size", fmt.Sprintf("%d", asset.Size)).
		Msg("[backend] cudart zip 包下载完成")
	return destPath, nil
}

// downloadFile 是通用的文件下载函数，支持进度回调。
// 从 downloadURL 下载到 destPath，先写入 .tmp 临时文件，完成后原子重命名。
// 下载过程中实时计算 SHA256 哈希并记录日志，便于完整性审计。
//
// 支持"候选源自动切换"：优先走加速代理镜像下载，代理失败时自动回落原始 GitHub 源。
// 生活类比：先走快捷小路（代理镜像）取货，小路堵了再绕回主路（GitHub 原始源）。
func downloadFile(downloadURL, destPath string, totalSize int64, bt BackendType, assetName, tagName string, progressCB func(DownloadProgress)) error {
	urls := buildDownloadURLs(downloadURL)
	var lastErr error
	for i, u := range urls {
		// 非首个候选（即代理失败需要换源）时，清理可能的残留临时文件，并从新源重新下载
		if i > 0 {
			_ = os.Remove(destPath + ".tmp")
			if progressCB != nil {
				progressCB(DownloadProgress{
					Backend:    bt,
					AssetName:  assetName,
					TagName:    tagName,
					TotalBytes: totalSize,
					Status:     "retrying",
					Label:      "切换下载源重试",
				})
			}
			log.Warn().
				Str("asset", assetName).
				Str("url", u).
				Err(lastErr).
				Msg("[backend] 上一个下载源失败，自动切换到下一个下载源")
		}
		if err := downloadFileFromURL(u, destPath, totalSize, bt, assetName, tagName, progressCB); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return apperror.Wrap(apperror.KindUnavailable, "所有下载源均失败", lastErr)
}

// downloadFileFromURL 是 downloadFile 的单一来源下载核心。
// 从指定 downloadURL 下载到 destPath，所有网络/写入/校验逻辑都在此完成。
// 由 downloadFile 在候选源列表中逐个调用，实现"一源失败换下一源"。
func downloadFileFromURL(downloadURL, destPath string, totalSize int64, bt BackendType, assetName, tagName string, progressCB func(DownloadProgress)) error {
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "创建临时文件失败", err)
	}
	// outClosed 防止 defer 与后续显式 Close 双重关闭文件句柄（在 Windows 上可能导致 panic）
	var outClosed bool
	defer func() {
		if !outClosed {
			out.Close()
		}
	}()

	req, err := http.NewRequest("GET", downloadURL, http.NoBody)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "创建下载请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)

	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "下载请求失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apperror.Newf(apperror.KindUnavailable, "下载返回非 200 状态码: %d", resp.StatusCode)
	}

	if totalSize <= 0 {
		totalSize = resp.ContentLength
	}

	var downloaded int64
	hash := sha256.New()
	buf := make([]byte, 32*1024)
	lastReport := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			hash.Write(buf[:n]) // 实时计算 SHA256
			if _, werr := out.Write(buf[:n]); werr != nil {
				_ = os.Remove(tmpPath) // 写入失败时清理已下载的临时文件
				return apperror.Wrap(apperror.KindInternal, "写入文件失败", werr)
			}
			downloaded += int64(n)

			// 限流推送进度（最多每 500ms 一次）
			now := time.Now()
			if progressCB != nil && now.Sub(lastReport) >= 500*time.Millisecond {
				var percent float64
				if totalSize > 0 {
					percent = float64(downloaded) / float64(totalSize) * 100
				}
				progressCB(DownloadProgress{
					Backend:    bt,
					AssetName:  assetName,
					TagName:    tagName,
					TotalBytes: totalSize,
					Downloaded: downloaded,
					Percent:    percent,
					Status:     "downloading",
				})
				lastReport = now
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return apperror.Wrap(apperror.KindUnavailable, "读取响应失败", readErr)
		}
	}

	if err := out.Close(); err != nil {
		return apperror.Wrap(apperror.KindInternal, "关闭文件失败", err)
	}
	outClosed = true

	// P0-1 修复：校验下载字节数，防止截断响应被误认为成功
	if totalSize > 0 && downloaded != totalSize {
		_ = os.Remove(tmpPath)
		return apperror.Newf(apperror.KindUnavailable, "下载文件不完整：已下载 %d 字节，预期 %d 字节", downloaded, totalSize)
	}

	sha256Hex := hex.EncodeToString(hash.Sum(nil))
	log.Info().
		Str("asset", assetName).
		Int64("size", totalSize).
		Str("sha256", sha256Hex).
		Msg("[backend] 下载完成，SHA256 哈希已记录")

	if err := os.Rename(tmpPath, destPath); err != nil {
		return apperror.Wrap(apperror.KindInternal, "重命名临时文件失败", err)
	}

	if progressCB != nil {
		progressCB(DownloadProgress{
			Backend:    bt,
			AssetName:  assetName,
			TagName:    tagName,
			TotalBytes: totalSize,
			Downloaded: totalSize,
			Percent:    100,
			Status:     "completed",
		})
	}

	return nil
}

// DownloadBackendZipWithContext 是 DownloadBackendZip 的 context 版本，
// 支持通过 context 取消下载（例如用户主动取消）。
//
// 生活类比：和 DownloadBackendZip 一样，但额外支持"中途喊停"——
// 用户取消下载时，context 被取消，下载立即终止并清理临时文件。
func DownloadBackendZipWithContext(ctx context.Context, bt BackendType, runtimeDir string, progressCB func(DownloadProgress)) (string, error) {
	if bt == BackendAuto {
		return "", apperror.New(apperror.KindInvalidInput, "BackendAuto 需先通过 ResolveBackendType 解析成具体后端")
	}

	// 步骤 1：查找匹配的 GitHub asset
	asset, tagName, err := FindReleaseAsset(bt)
	if err != nil {
		return "", apperror.Wrapf(apperror.KindUnavailable, "查找 %s 后端 release asset 失败", err, GetBackendInfo(bt).DisplayName)
	}

	destPath := filepath.Join(runtimeDir, asset.Name)
	tmpPath := destPath + ".tmp"

	// 如果目标文件已存在，直接返回
	if _, err := os.Stat(destPath); err == nil {
		log.Info().
			Str("backend", bt.String()).
			Str("path", destPath).
			Msg("[backend] zip 包已存在，跳过下载")
		if progressCB != nil {
			progressCB(DownloadProgress{
				Backend:    bt,
				AssetName:  asset.Name,
				TagName:    tagName,
				TotalBytes: asset.Size,
				Downloaded: asset.Size,
				Percent:    100,
				Status:     "completed",
			})
		}
		return destPath, nil
	}

	_ = os.Remove(tmpPath)

	// 候选源自动切换：优先走加速代理镜像（国内快），失败自动回落原始 GitHub 源。
	// 生活类比：与 downloadFile 相同——先走快捷小路取货，小路堵了再绕回主路。
	urls := buildDownloadURLs(asset.BrowserDownloadURL)
	var lastErr error
	for i, u := range urls {
		// 非首个候选（即代理失败需要换源）时，清理残留临时文件并通知前端重试状态
		if i > 0 {
			_ = os.Remove(tmpPath)
			if progressCB != nil {
				progressCB(DownloadProgress{
					Backend:    bt,
					AssetName:  asset.Name,
					TagName:    tagName,
					TotalBytes: asset.Size,
					Status:     "retrying",
					Label:      "切换下载源重试",
				})
			}
			log.Warn().
				Str("asset", asset.Name).
				Str("url", u).
				Err(lastErr).
				Msg("[backend] 上一个下载源失败，自动切换到下一个下载源")
		}
		path, err := downloadBackendZipFromURL(ctx, u, tmpPath, destPath, bt, asset.Name, tagName, asset.Size, progressCB)
		if err != nil {
			lastErr = err
			// 用户主动取消时不换源重试，直接返回
			if ctx.Err() != nil {
				return "", err
			}
			continue
		}
		return path, nil
	}
	return "", apperror.Wrap(apperror.KindUnavailable, "所有下载源均失败", lastErr)
}

// downloadBackendZipFromURL 是 DownloadBackendZipWithContext 的单源下载核心。
// 从指定 downloadURL 下载后端 zip 到 destPath，支持 context 取消。
// 由 DownloadBackendZipWithContext 在候选源列表中逐个调用，实现"一源失败换下一源"。
func downloadBackendZipFromURL(ctx context.Context, downloadURL, tmpPath, destPath string, bt BackendType, assetName, tagName string, assetSize int64, progressCB func(DownloadProgress)) (string, error) {
	log.Info().
		Str("backend", bt.String()).
		Str("url", downloadURL).
		Str("dest", destPath).
		Int64("size", assetSize).
		Msg("[backend] 开始下载后端 zip 包")

	// 使用带 context 的请求，支持取消
	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, http.NoBody)
	if err != nil {
		return "", apperror.Wrap(apperror.KindUnavailable, "创建下载请求失败", err)
	}
	req.Header.Set("User-Agent", githubUA)

	resp, err := downloadHTTPClient.Do(req)
	if err != nil {
		return "", apperror.Wrap(apperror.KindUnavailable, "下载请求失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", apperror.Newf(apperror.KindUnavailable, "下载返回非 200 状态码: %d", resp.StatusCode)
	}

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "创建临时文件失败", err)
	}
	// tmpClosed 防止 defer 与异常路径中显式 Close 双重关闭
	var tmpClosed bool
	defer func() {
		if !tmpClosed {
			tmpFile.Close()
		}
	}()

	totalSize := resp.ContentLength
	if totalSize <= 0 {
		totalSize = assetSize
	}

	buf := make([]byte, 64*1024)
	hash := sha256.New()
	var downloaded int64
	lastReport := time.Now()

	for {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			tmpClosed = true
			tmpFile.Close()
			_ = os.Remove(tmpPath)
			if progressCB != nil {
				progressCB(DownloadProgress{
					Backend: bt,
					Status:  "failed",
					Error:   "用户取消下载",
				})
			}
			return "", ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			hash.Write(buf[:n]) // 实时计算 SHA256
			if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
				tmpClosed = true
				tmpFile.Close()
				_ = os.Remove(tmpPath)
				return "", apperror.Wrap(apperror.KindInternal, "写入临时文件失败", writeErr)
			}
			downloaded += int64(n)

			if progressCB != nil && time.Since(lastReport) >= 500*time.Millisecond {
				percent := 0.0
				if totalSize > 0 {
					percent = float64(downloaded) / float64(totalSize) * 100
				}
				progressCB(DownloadProgress{
					Backend:    bt,
					AssetName:  assetName,
					TagName:    tagName,
					TotalBytes: totalSize,
					Downloaded: downloaded,
					Percent:    percent,
					Status:     "downloading",
				})
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmpClosed = true
			tmpFile.Close()
			_ = os.Remove(tmpPath)
			return "", apperror.Wrap(apperror.KindUnavailable, "下载读取失败", readErr)
		}
	}

	tmpFile.Close()
	tmpClosed = true

	// P0-1 修复：校验下载字节数，防止截断响应被误认为成功
	if totalSize > 0 && downloaded != totalSize {
		_ = os.Remove(tmpPath)
		return "", apperror.Newf(apperror.KindUnavailable, "下载文件不完整：已下载 %d 字节，预期 %d 字节", downloaded, totalSize)
	}

	sha256Hex := hex.EncodeToString(hash.Sum(nil))
	log.Info().
		Str("asset", assetName).
		Int64("size", totalSize).
		Str("sha256", sha256Hex).
		Msg("[backend] 下载完成，SHA256 哈希已记录")

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", apperror.Wrap(apperror.KindInternal, "重命名临时文件失败", err)
	}

	if progressCB != nil {
		progressCB(DownloadProgress{
			Backend:    bt,
			AssetName:  assetName,
			TagName:    tagName,
			TotalBytes: totalSize,
			Downloaded: totalSize,
			Percent:    100,
			Status:     "completed",
		})
	}

	log.Info().
		Str("backend", bt.String()).
		Str("path", destPath).
		Int64("size", totalSize).
		Msg("[backend] 后端 zip 包下载完成")
	return destPath, nil
}
