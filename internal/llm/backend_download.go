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
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

// GitHubReleasesAPI 是 llama.cpp releases 的 GitHub API 地址。
// 默认查询 latest release，获取最新构建的 Windows 后端 zip 包。
const GitHubReleasesAPI = "https://api.github.com/repos/ggml-org/llama.cpp/releases/latest"

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

// fetchGitHubLatestRelease 查询 GitHub API 获取 llama.cpp 最新 release。
// 该函数被 FindReleaseAsset 和 FindCudartAsset 共用，避免重复的 HTTP 请求逻辑。
//
// 生活类比：总台接线员——无论是问发动机型号还是配件型号，都打同一个电话，
// 不用每次重新拨号。
func fetchGitHubLatestRelease() (*GitHubRelease, error) {
	req, err := http.NewRequest("GET", GitHubReleasesAPI, nil)
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
func downloadFile(downloadURL, destPath string, totalSize int64, bt BackendType, assetName, tagName string, progressCB func(DownloadProgress)) error {
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

	req, err := http.NewRequest("GET", downloadURL, nil)
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

	log.Info().
		Str("backend", bt.String()).
		Str("url", asset.BrowserDownloadURL).
		Str("dest", destPath).
		Int64("size", asset.Size).
		Msg("[backend] 开始下载后端 zip 包")

	// 使用带 context 的请求，支持取消
	req, err := http.NewRequestWithContext(ctx, "GET", asset.BrowserDownloadURL, nil)
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
		totalSize = asset.Size
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
					AssetName:  asset.Name,
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
		Str("asset", asset.Name).
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
			AssetName:  asset.Name,
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
