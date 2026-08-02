// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
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
)

// GitHubReleasesAPI 是 llama.cpp releases 的 GitHub API 地址。
// 默认查询 latest release，获取最新构建的 Windows 后端 zip 包。
const GitHubReleasesAPI = "https://api.github.com/repos/ggml-org/llama.cpp/releases/latest"

// DownloadProgress 下载进度信息，通过回调函数推送给调用方。
//
// 生活类比：就像快递追踪——已发货（开始下载）、运输中（下载中，进度 45%）、
// 已签收（下载完成）。
type DownloadProgress struct {
	Backend     BackendType // 后端类型
	AssetName   string      // GitHub asset 文件名，如 "llama-b10167-bin-win-cuda-13.3-x64.zip"
	TagName     string      // GitHub release 标签，如 "b10167"
	TotalBytes  int64       // 文件总大小（字节），未知时为 0
	Downloaded  int64       // 已下载字节数
	Percent     float64     // 下载百分比（0-100）
	Status      string      // 状态："downloading" / "completed" / "failed" / "installing" / "retrying"
	Error       string      // 失败时的错误信息
	Label       string      // 当前下载内容的描述，如"推理后端"、"cudart 依赖包"
}

// githubAsset 表示 GitHub release 中的一个资源文件。
// 仅提取下载所需的字段，忽略其他元数据。
type githubAsset struct {
	Name               string `json:"name"`                 // 文件名，如 "llama-b10167-bin-win-cuda-13.3-x64.zip"
	BrowserDownloadURL string `json:"browser_download_url"` // 直链下载地址
	Size               int64  `json:"size"`                 // 文件大小（字节）
}

// githubRelease 表示 GitHub release 的精简结构。
type githubRelease struct {
	TagName string         `json:"tag_name"` // release 标签，如 "b10167"
	Assets  []githubAsset `json:"assets"`   // release 包含的资源文件列表
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
func FindReleaseAsset(bt BackendType) (asset githubAsset, tagName string, err error) {
	if bt == BackendAuto {
		return githubAsset{}, "", fmt.Errorf("BackendAuto 需先通过 ResolveBackendType 解析成具体后端")
	}

	info := GetBackendInfo(bt)
	if info.ReleaseAssetRegex == "" {
		return githubAsset{}, "", fmt.Errorf("后端 %s 没有定义 ReleaseAssetRegex", info.DisplayName)
	}

	// 编译正则
	re, err := regexp.Compile(info.ReleaseAssetRegex)
	if err != nil {
		return githubAsset{}, "", fmt.Errorf("编译 ReleaseAssetRegex 失败: %w", err)
	}

	// 查询 GitHub API
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", GitHubReleasesAPI, nil)
	if err != nil {
		return githubAsset{}, "", fmt.Errorf("创建 GitHub API 请求失败: %w", err)
	}
	// GitHub API 要求设置 User-Agent
	req.Header.Set("User-Agent", "Douya-LocalAI")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return githubAsset{}, "", fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return githubAsset{}, "", fmt.Errorf("GitHub API 返回非 200 状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return githubAsset{}, "", fmt.Errorf("读取 GitHub API 响应失败: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return githubAsset{}, "", fmt.Errorf("解析 GitHub API 响应失败: %w", err)
	}

	// 在 assets 中匹配对应后端，收集所有匹配项
	// CUDA 后端官方同时提供 12.x 和 13.x 两个版本，需要优先选 13.x
	var matched []githubAsset
	for _, a := range release.Assets {
		if re.MatchString(a.Name) {
			matched = append(matched, a)
		}
	}

	if len(matched) == 0 {
		return githubAsset{}, release.TagName, fmt.Errorf("在最新 release %s 中未找到匹配 %s 后端的 asset（正则: %s）",
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
func DownloadBackendZip(bt BackendType, runtimeDir string, progressCB func(DownloadProgress)) (string, error) {
	if bt == BackendAuto {
		return "", fmt.Errorf("BackendAuto 需先通过 ResolveBackendType 解析成具体后端")
	}

	// 步骤 1：查找匹配的 GitHub asset
	asset, tagName, err := FindReleaseAsset(bt)
	if err != nil {
		return "", fmt.Errorf("查找 %s 后端 release asset 失败: %w", GetBackendInfo(bt).DisplayName, err)
	}

	destPath := filepath.Join(runtimeDir, asset.Name)
	tmpPath := destPath + ".tmp"

	// 如果目标文件已存在（之前下载过），直接返回
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

	// 清理可能残留的临时文件
	_ = os.Remove(tmpPath)

	// 步骤 2：流式下载
	log.Info().
		Str("backend", bt.String()).
		Str("url", asset.BrowserDownloadURL).
		Str("dest", destPath).
		Int64("size", asset.Size).
		Msg("[backend] 开始下载后端 zip 包")

	client := &http.Client{Timeout: 0} // 下载不设超时，大文件可能需要几分钟
	req, err := http.NewRequest("GET", asset.BrowserDownloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("创建下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Douya-LocalAI")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载返回非 200 状态码: %d", resp.StatusCode)
	}

	// 创建临时文件
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}

	totalSize := resp.ContentLength
	if totalSize <= 0 {
		totalSize = asset.Size // 回退到 API 返回的大小
	}

	// 流式写入，带进度回调
	buf := make([]byte, 64*1024) // 64KB 缓冲区
	var downloaded int64
	lastReport := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
				tmpFile.Close()
				_ = os.Remove(tmpPath)
				return "", fmt.Errorf("写入临时文件失败: %w", writeErr)
			}
			downloaded += int64(n)

			// 限流推送进度：最多每 500ms 推送一次，避免频繁事件刷爆前端
			if progressCB != nil && time.Since(lastReport) >= 500*time.Millisecond {
				percent := 0.0
				if totalSize > 0 {
					percent = float64(downloaded) / float64(totalSize) * 100
				}
				progressCB(DownloadProgress{
					Backend:     bt,
					AssetName:   asset.Name,
					TagName:     tagName,
					TotalBytes:  totalSize,
					Downloaded:  downloaded,
					Percent:     percent,
					Status:      "downloading",
				})
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("下载读取失败: %w", readErr)
		}
	}

	tmpFile.Close()

	// P0-1 修复：下载完成后校验字节数，防止服务器截断响应被误认为下载成功。
	// 生活类比：快递签收前先称重，重量对不上说明包裹不完整，直接拒收。
	// 只在 ContentLength 已知（totalSize > 0）时校验，未知时不校验（chunked 传输）。
	if totalSize > 0 && downloaded != totalSize {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("下载文件不完整：已下载 %d 字节，预期 %d 字节（可能网络中断或服务器截断响应）",
			downloaded, totalSize)
	}

	// 步骤 3：重命名临时文件为最终文件名
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("重命名临时文件失败: %w", err)
	}

	// 推送完成进度
	if progressCB != nil {
		progressCB(DownloadProgress{
			Backend:     bt,
			AssetName:   asset.Name,
			TagName:     tagName,
			TotalBytes:  totalSize,
			Downloaded:  totalSize,
			Percent:     100,
			Status:      "completed",
		})
	}

	log.Info().
		Str("backend", bt.String()).
		Str("path", destPath).
		Str("size", fmt.Sprintf("%d", totalSize)).
		Msg("[backend] 后端 zip 包下载完成")
	return destPath, nil
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
func FindCudartAsset() (asset githubAsset, tagName string, err error) {
	// 查询 GitHub API（复用 FindReleaseAsset 的请求逻辑，但不走后端正则）
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", GitHubReleasesAPI, nil)
	if err != nil {
		return githubAsset{}, "", fmt.Errorf("创建 GitHub API 请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Douya-LocalAI")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := client.Do(req)
	if err != nil {
		return githubAsset{}, "", fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return githubAsset{}, "", fmt.Errorf("GitHub API 返回非 200 状态码: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return githubAsset{}, "", fmt.Errorf("读取 GitHub API 响应失败: %w", err)
	}

	var release githubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return githubAsset{}, "", fmt.Errorf("解析 GitHub API 响应失败: %w", err)
	}

	// 收集所有匹配的 cudart asset（12.x 和 13.x）
	var matched []githubAsset
	for _, a := range release.Assets {
		if cudartAssetRegex.MatchString(a.Name) {
			matched = append(matched, a)
		}
	}

	if len(matched) == 0 {
		return githubAsset{}, release.TagName, fmt.Errorf("在最新 release %s 中未找到 cudart 包", release.TagName)
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
// CUDA 后端需要主包（llama-b*-bin-win-cuda-*.zip）和 cudart 包一起解压到同一目录，
// cudart 包提供 cudart64_*.dll、cublas64_*.dll 等厂商运行时 DLL。
func DownloadCudartZip(runtimeDir string, progressCB func(DownloadProgress)) (string, error) {
	asset, tagName, err := FindCudartAsset()
	if err != nil {
		return "", fmt.Errorf("查找 cudart release asset 失败: %w", err)
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
		return "", fmt.Errorf("下载 cudart zip 包失败: %w", err)
	}

	log.Info().
		Str("path", destPath).
		Str("size", fmt.Sprintf("%d", asset.Size)).
		Msg("[backend] cudart zip 包下载完成")
	return destPath, nil
}

// downloadFile 是通用的文件下载函数，支持进度回调。
// 从 downloadURL 下载到 destPath，先写入 .tmp 临时文件，完成后原子重命名。
func downloadFile(downloadURL, destPath string, totalSize int64, bt BackendType, assetName, tagName string, progressCB func(DownloadProgress)) error {
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer out.Close()

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return fmt.Errorf("创建下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Douya-LocalAI")

	client := &http.Client{Timeout: 0} // 下载不限超时
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回非 200 状态码: %d", resp.StatusCode)
	}

	if totalSize <= 0 {
		totalSize = resp.ContentLength
	}

	var downloaded int64
	buf := make([]byte, 32*1024)
	lastReport := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return fmt.Errorf("写入文件失败: %w", werr)
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
					Backend:     bt,
					AssetName:   assetName,
					TagName:     tagName,
					TotalBytes:  totalSize,
					Downloaded:  downloaded,
					Percent:     percent,
					Status:      "downloading",
				})
				lastReport = now
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("读取响应失败: %w", readErr)
		}
	}

	if err := out.Close(); err != nil {
		return fmt.Errorf("关闭文件失败: %w", err)
	}

	// P0-1 修复：校验下载字节数，防止截断响应被误认为成功
	if totalSize > 0 && downloaded != totalSize {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("下载文件不完整：已下载 %d 字节，预期 %d 字节", downloaded, totalSize)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("重命名临时文件失败: %w", err)
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
		return "", fmt.Errorf("BackendAuto 需先通过 ResolveBackendType 解析成具体后端")
	}

	// 步骤 1：查找匹配的 GitHub asset
	asset, tagName, err := FindReleaseAsset(bt)
	if err != nil {
		return "", fmt.Errorf("查找 %s 后端 release asset 失败: %w", GetBackendInfo(bt).DisplayName, err)
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
		return "", fmt.Errorf("创建下载请求失败: %w", err)
	}
	req.Header.Set("User-Agent", "Douya-LocalAI")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载返回非 200 状态码: %d", resp.StatusCode)
	}

	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}

	totalSize := resp.ContentLength
	if totalSize <= 0 {
		totalSize = asset.Size
	}

	buf := make([]byte, 64*1024)
	var downloaded int64
	lastReport := time.Now()

	for {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
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
			if _, writeErr := tmpFile.Write(buf[:n]); writeErr != nil {
				tmpFile.Close()
				_ = os.Remove(tmpPath)
				return "", fmt.Errorf("写入临时文件失败: %w", writeErr)
			}
			downloaded += int64(n)

			if progressCB != nil && time.Since(lastReport) >= 500*time.Millisecond {
				percent := 0.0
				if totalSize > 0 {
					percent = float64(downloaded) / float64(totalSize) * 100
				}
				progressCB(DownloadProgress{
					Backend:     bt,
					AssetName:   asset.Name,
					TagName:     tagName,
					TotalBytes:  totalSize,
					Downloaded:  downloaded,
					Percent:     percent,
					Status:      "downloading",
				})
				lastReport = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmpFile.Close()
			_ = os.Remove(tmpPath)
			return "", fmt.Errorf("下载读取失败: %w", readErr)
		}
	}

	tmpFile.Close()

	// P0-1 修复：校验下载字节数，防止截断响应被误认为成功
	if totalSize > 0 && downloaded != totalSize {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("下载文件不完整：已下载 %d 字节，预期 %d 字节", downloaded, totalSize)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("重命名临时文件失败: %w", err)
	}

	if progressCB != nil {
		progressCB(DownloadProgress{
			Backend:     bt,
			AssetName:   asset.Name,
			TagName:     tagName,
			TotalBytes:  totalSize,
			Downloaded:  totalSize,
			Percent:     100,
			Status:      "completed",
		})
	}

	log.Info().
		Str("backend", bt.String()).
		Str("path", destPath).
		Int64("size", totalSize).
		Msg("[backend] 后端 zip 包下载完成")
	return destPath, nil
}
