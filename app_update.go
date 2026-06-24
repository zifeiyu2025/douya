package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"douya/internal/version"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	zlog "github.com/rs/zerolog/log"
)

// UpdateInfo 更新信息结构体
type UpdateInfo struct {
	HasUpdate      bool   `json:"has_update"`
	LatestVersion  string `json:"latest_version"`
	CurrentVersion string `json:"current_version"`
	DownloadURL    string `json:"download_url"`
	ReleaseNotes   string `json:"release_notes"`
	PublishedAt    string `json:"published_at"`
}

// githubRelease GitHub Release API 响应结构
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Body        string `json:"body"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// GetAppVersion 返回当前应用版本号
func (a *App) GetAppVersion() string {
	return version.Version
}

// CheckUpdate 检查是否有新版本
// 生活类比：就像手机应用商店的"检查更新"功能，看看有没有新版本可以下载
func (a *App) CheckUpdate() (*UpdateInfo, error) {
	// 拼接 GitHub API 地址
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest",
		version.GitHubOwner, version.GitHubRepo)

	// 发起 HTTP 请求
	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("请求 GitHub API 失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("GitHub API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 解析 JSON 响应
	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("解析 GitHub Release 信息失败: %w", err)
	}

	// 去掉 tag_name 的 "v" 前缀，得到纯版本号
	latestVersion := strings.TrimPrefix(release.TagName, "v")

	// 查找匹配的 Windows amd64 资产
	// 匹配模式：Douya-v*-windows-amd64.zip
	assetPattern := regexp.MustCompile(`^Douya-v.+windows-amd64\.zip$`)
	var downloadURL string
	for _, asset := range release.Assets {
		if assetPattern.MatchString(asset.Name) {
			downloadURL = asset.BrowserDownloadURL
			break
		}
	}

	if downloadURL == "" {
		return nil, fmt.Errorf("未找到 Windows amd64 版本的下载资源")
	}

	// 比较版本号：当前版本 vs 最新版本
	hasUpdate := compareVersions(latestVersion, version.Version) > 0

	return &UpdateInfo{
		HasUpdate:      hasUpdate,
		LatestVersion:  latestVersion,
		CurrentVersion: version.Version,
		DownloadURL:    downloadURL,
		ReleaseNotes:   release.Body,
		PublishedAt:    release.PublishedAt,
	}, nil
}

// PerformUpdate 执行自动更新
// 生活类比：就像手机下载更新包后自动安装——先下载，然后重启应用完成替换
func (a *App) PerformUpdate(downloadURL string, latestVersion string) error {
	// 通知前端：开始下载
	wailsRuntime.EventsEmit(a.ctx, "update:progress", map[string]interface{}{
		"stage":   "downloading",
		"percent": 0,
	})

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "douya-update-*")
	if err != nil {
		return fmt.Errorf("创建临时目录失败: %w", err)
	}

	// 下载 zip 文件
	zipPath := filepath.Join(tempDir, fmt.Sprintf("Douya-v%s-windows-amd64.zip", latestVersion))
	if err := a.downloadWithProgress(downloadURL, zipPath); err != nil {
		os.RemoveAll(tempDir)
		return fmt.Errorf("下载更新包失败: %w", err)
	}

	// 通知前端：开始安装
	wailsRuntime.EventsEmit(a.ctx, "update:progress", map[string]interface{}{
		"stage": "installing",
	})

	// 生成并启动更新脚本
	if err := a.launchUpdateScript(zipPath, tempDir); err != nil {
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}

	// 退出当前应用，让更新脚本替换文件后重启
	wailsRuntime.Quit(a.ctx)

	return nil
}

// downloadWithProgress 下载文件并报告进度
func (a *App) downloadWithProgress(url, destPath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载返回状态码 %d", resp.StatusCode)
	}

	// 创建目标文件
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("创建下载文件失败: %w", err)
	}
	defer out.Close()

	// 获取文件总大小
	contentLength := resp.ContentLength

	// 使用自定义 writer 追踪下载进度
	progressWriter := &downloadProgressWriter{
		ctx:           a.ctx,
		total:         contentLength,
		lastPercent:   -1,
		underlying:    out,
		bytesWritten:  0,
	}

	_, err = io.Copy(progressWriter, resp.Body)
	return err
}

// downloadProgressWriter 追踪下载进度的 io.Writer
type downloadProgressWriter struct {
	ctx          context.Context
	total        int64
	lastPercent  int
	underlying   *os.File
	bytesWritten int64
}

func (w *downloadProgressWriter) Write(p []byte) (int, error) {
	n, err := w.underlying.Write(p)
	if err != nil {
		return n, err
	}
	w.bytesWritten += int64(n)

	// 每变化 5% 推送一次进度事件
	if w.total > 0 {
		percent := int(float64(w.bytesWritten) / float64(w.total) * 100)
		if percent >= w.lastPercent+5 {
			w.lastPercent = percent
			if percent > 100 {
				percent = 100
			}
			wailsRuntime.EventsEmit(w.ctx, "update:progress", map[string]interface{}{
				"stage":   "downloading",
				"percent": percent,
			})
		}
	}

	return n, nil
}

// launchUpdateScript 生成并启动 PowerShell 更新脚本
func (a *App) launchUpdateScript(zipPath string, tempDir string) error {
	appDirPath := appDir()
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("获取可执行文件路径失败: %w", err)
	}

	tempExtract := filepath.Join(tempDir, "extracted")

	// 生成 PowerShell 更新脚本
	script := fmt.Sprintf(`$ErrorActionPreference = "Stop"
$appDir = "%s"
$zipPath = "%s"
$tempExtract = "%s"
$exePath = "%s"

# Wait for Douya.exe to exit
Write-Host "Waiting for Douya to exit..."
$process = Get-Process -Name "Douya" -ErrorAction SilentlyContinue
if ($process) {
    $process.WaitForExit()
    Start-Sleep -Seconds 2
}

# Extract zip
Write-Host "Extracting update..."
Expand-Archive -Path $zipPath -DestinationPath $tempExtract -Force

# Replace bin/ directory
Write-Host "Updating bin/..."
if (Test-Path "$tempExtract\bin") {
    Copy-Item "$tempExtract\bin\*" "$appDir\bin\" -Recurse -Force
}

# Replace runtime/ directory (including deleting old files)
Write-Host "Updating runtime/..."
if (Test-Path "$tempExtract\runtime") {
    # Get list of files in new runtime
    $newFiles = Get-ChildItem "$tempExtract\runtime" -File -Recurse | ForEach-Object {
        $_.FullName.Substring("$tempExtract\runtime\".Length)
    }
    # Delete old runtime files not in new version
    Get-ChildItem "$appDir\runtime" -File -Recurse | ForEach-Object {
        $relPath = $_.FullName.Substring("$appDir\runtime\".Length)
        if ($newFiles -notcontains $relPath) {
            Remove-Item $_.FullName -Force
            Write-Host "  Removed old file: $relPath"
        }
    }
    # Copy new runtime files
    Copy-Item "$tempExtract\runtime\*" "$appDir\runtime\" -Recurse -Force
}

# Start new version
Write-Host "Starting Douya..."
Start-Process $exePath

# Cleanup
Start-Sleep -Seconds 3
Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
Remove-Item $zipPath -Force -ErrorAction SilentlyContinue
Write-Host "Update complete!"
`, appDirPath, zipPath, tempExtract, exePath)

	// 写入脚本文件（UTF-8 BOM 编码）
	scriptPath := filepath.Join(tempDir, "update.ps1")
	if err := writeUTF8BOM(scriptPath, script); err != nil {
		return fmt.Errorf("写入更新脚本失败: %w", err)
	}

	// 启动 PowerShell 脚本（分离进程，不显示窗口）
	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // CREATE_NO_WINDOW
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动更新脚本失败: %w", err)
	}

	// 分离进程，不等待其完成
	// 生活类比：就像你按了电梯按钮后离开，电梯自己会运行
	if err := cmd.Process.Release(); err != nil {
		zlog.Warn().Err(err).Msg("[update] 释放更新进程失败")
	}

	zlog.Info().Str("script", scriptPath).Msg("[update] 更新脚本已启动")
	return nil
}

// writeUTF8BOM 将内容以 UTF-8 BOM 编码写入文件
// PowerShell 需要 BOM 头才能正确识别 UTF-8 编码的中文
func writeUTF8BOM(path, content string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// 写入 UTF-8 BOM（0xEF, 0xBB, 0xBF）
	if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}

	// 写入内容
	_, err = f.WriteString(content)
	return err
}

// compareVersions 比较两个语义化版本号
// 返回值：1 表示 a > b，-1 表示 a < b，0 表示相等
// 生活类比：就像比较两个学生的成绩，先比总分，总分一样再比单科
func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := len(partsA)
	if len(partsB) > maxLen {
		maxLen = len(partsB)
	}

	for i := 0; i < maxLen; i++ {
		var numA, numB int
		if i < len(partsA) {
			numA, _ = strconv.Atoi(partsA[i])
		}
		if i < len(partsB) {
			numB, _ = strconv.Atoi(partsB[i])
		}

		if numA > numB {
			return 1
		}
		if numA < numB {
			return -1
		}
	}

	return 0
}
