package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"douya/internal/apperror"
	"douya/internal/llm"
	"douya/internal/version"

	zlog "github.com/rs/zerolog/log"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

// GetAppVersion 返回当前应用版本号
func (a *App) GetAppVersion() string {
	return version.Version
}

// GetGitHubURL 返回 GitHub 仓库主页 URL（供前端"访问主页"按钮使用，URL 唯一来源为 version 包）
func (a *App) GetGitHubURL() string {
	return version.GitHubURL()
}

// CheckUpdate 检查是否有新版本
// 生活类比：就像手机应用商店的"检查更新"功能，看看有没有新版本可以下载
func (a *App) CheckUpdate() (*UpdateInfo, error) {
	// 拼接 GitHub API 地址（URL 构造统一走 version 包，避免多处硬编码）
	apiURL := version.GitHubAPIURL() + "/releases/latest"

	// 安全：使用带超时的 HTTP 客户端，避免网络异常时无限期挂起
	// 基于 GO-HTTPCLIENT-001 安全实践
	client := &http.Client{Timeout: githubAPITimeout}
	resp, err := client.Get(apiURL)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "请求 GitHub API 失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// L-14：检查 ReadAll 错误，失败时用占位符避免显示空字符串造成误解
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if err != nil {
			body = []byte("<unreadable>")
		}
		return nil, apperror.Newf(apperror.KindUnavailable, "GitHub API 返回状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 解析 JSON 响应
	// 安全实践（基于 GO-HTTP-002 #5）：限制响应体大小为 1MB，防止恶意/异常响应耗尽内存
	// GitHub Release API 响应通常 < 100KB，1MB 足够且留有余量
	var release llm.GitHubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1024*1024)).Decode(&release); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "解析 GitHub Release 信息失败", err)
	}

	// 去掉 tag_name 的 "v" 前缀，得到纯版本号
	latestVersion := strings.TrimPrefix(release.TagName, "v")

	// 查找匹配的 Windows 资产（兼容 windows.zip 与 windows-amd64.zip 两种命名）
	downloadURL := findWindowsAsset(release.Assets)

	if downloadURL == "" {
		return nil, apperror.New(apperror.KindNotFound, "未找到 Windows 版本的下载资源")
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
	// 安全：校验下载 URL，防止 SSRF 攻击
	// 基于 GO-SSRF-001 安全实践：不信任前端传入的 URL，必须为 HTTPS 且来自 GitHub 域名
	if err := validateUpdateURL(downloadURL); err != nil {
		return apperror.Wrap(apperror.KindInvalidInput, "下载地址校验失败", err)
	}

	// 通知前端：开始下载
	wailsRuntime.EventsEmit(a.ctx, EventUpdateProgress, map[string]any{
		"stage":   "downloading",
		"percent": 0,
	})

	// 创建临时目录
	tempDir, err := os.MkdirTemp("", "douya-update-*")
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "创建临时目录失败", err)
	}

	// 下载 zip 文件（临时文件名与发布资产命名保持一致）
	zipPath := filepath.Join(tempDir, fmt.Sprintf("Douya-v%s-windows.zip", latestVersion))
	if err := a.downloadWithProgress(downloadURL, zipPath); err != nil {
		os.RemoveAll(tempDir)
		return apperror.Wrap(apperror.KindUnavailable, "下载更新包失败", err)
	}

	// 通知前端：开始安装
	wailsRuntime.EventsEmit(a.ctx, EventUpdateProgress, map[string]any{
		"stage": "installing",
	})

	// 生成并启动更新脚本
	if err := a.launchUpdateScript(zipPath, tempDir); err != nil {
		os.RemoveAll(tempDir) // 清理临时目录，避免残留
		return apperror.Wrap(apperror.KindInternal, "启动更新脚本失败", err)
	}

	// 退出当前应用，让更新脚本替换文件后重启
	// P0-1 修复：使用 GracefulExit 而非裸 wailsRuntime.Quit，
	// 确保 exiting 标志被设置（避免 beforeClose 拦截）和 systray.Quit 被调用（避免托盘残留），
	// 同时触发完整的资源清理（停 llama-server、关 DB），确保更新脚本能替换所有文件。
	a.GracefulExit()

	return nil
}

// 更新流程超时/大小常量（命名自文档化，避免魔法数字）
const (
	githubAPITimeout      = 30 * time.Second  // 检查更新 API 请求超时
	updateDownloadTimeout = 10 * time.Minute  // 大文件下载超时
	maxUpdatePackageSize  = 500 * 1024 * 1024 // 500MB，限制最大下载大小
)

// downloadWithProgress 下载文件并报告进度
// M19 修复：限制最大下载大小为 500MB，避免异常大响应耗尽磁盘
// 生活类比：快递员收件时检查包裹大小，超过限制的拒收，避免仓库被撑爆

func (a *App) downloadWithProgress(downloadURL, destPath string) error {
	// 安全：使用带超时的 HTTP 客户端，避免下载挂起耗尽资源
	// 基于 GO-HTTPCLIENT-001 安全实践：大文件下载设置 10 分钟超时
	client := &http.Client{Timeout: updateDownloadTimeout}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "下载请求失败", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return apperror.Newf(apperror.KindUnavailable, "下载返回状态码 %d", resp.StatusCode)
	}

	// M19 修复：ContentLength 已知时预先校验大小
	if resp.ContentLength > 0 && resp.ContentLength > maxUpdatePackageSize {
		return apperror.Newf(apperror.KindInvalidInput, "更新包大小 %dMB 超过限制 %dMB，拒绝下载",
			resp.ContentLength/1024/1024, maxUpdatePackageSize/1024/1024)
	}

	// 创建目标文件
	out, err := os.Create(destPath)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "创建下载文件失败", err)
	}
	// 兜底关闭：显式 Close 后将 out 置为 nil，避免 defer 重复关闭
	defer func() {
		if out != nil {
			out.Close()
		}
	}()

	// 获取文件总大小
	contentLength := resp.ContentLength

	// 使用自定义 writer 追踪下载进度
	progressWriter := &downloadProgressWriter{
		ctx:          a.ctx,
		total:        contentLength,
		lastPercent:  -1,
		underlying:   out,
		bytesWritten: 0,
	}

	_, err = io.Copy(progressWriter, resp.Body)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "下载写入失败", err)
	}

	// 显式关闭文件并检查错误，确保缓冲数据落盘
	if err := out.Close(); err != nil {
		return apperror.Wrap(apperror.KindInternal, "关闭下载文件失败", err)
	}
	out = nil // 防止 defer 重复关闭
	return nil
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

	// M19 修复：运行时大小检查，防止 ContentLength 未知或被篡改时下载无限增长
	if w.bytesWritten > maxUpdatePackageSize {
		return n, apperror.Newf(apperror.KindInvalidInput, "下载大小超过限制 %dMB，已写入 %dMB",
			maxUpdatePackageSize/1024/1024, w.bytesWritten/1024/1024)
	}

	// 每变化 5% 推送一次进度事件
	if w.total > 0 {
		percent := int(float64(w.bytesWritten) / float64(w.total) * 100)
		if percent >= w.lastPercent+5 {
			w.lastPercent = percent
			if percent > 100 {
				percent = 100
			}
			wailsRuntime.EventsEmit(w.ctx, EventUpdateProgress, map[string]any{
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
		return apperror.Wrap(apperror.KindInternal, "获取可执行文件路径失败", err)
	}

	tempExtract := filepath.Join(tempDir, "extracted")

	// 安全：对插入 PowerShell 脚本的路径变量进行转义
	// 基于 GO-INJECT-002 安全实践：防止路径中含 PowerShell 特殊字符（" $ `）导致注入
	psEscape := func(s string) string {
		s = strings.ReplaceAll(s, "`", "``")
		s = strings.ReplaceAll(s, "\"", "`\"")
		s = strings.ReplaceAll(s, "$", "`$")
		return s
	}

	// 生成 PowerShell 更新脚本
	// P0-2 修复：采用"备份→替换→验证→成功删备份/失败回滚"策略，确保更新失败时应用可恢复
	// bin/ 和 runtime/ 统一采用 Rename-Item 整体替换（而非逐文件 Copy-Item -Force），
	// 确保新版本目录只包含新文件，旧文件自动清理。
	// runtime/ 替换后从备份恢复用户下载的 *.zip 包（后端下载功能产生的文件）。
	// 失败时写错误日志到 %TEMP%\douya-update-error.log，并尝试启动旧版本。
	script := fmt.Sprintf(`$ErrorActionPreference = "Stop"
$appDir = "%s"
$zipPath = "%s"
$tempExtract = "%s"
$exePath = "%s"
$logFile = Join-Path $env:TEMP "douya-update-error.log"

function Write-UpdateLog($msg) {
    $ts = Get-Date -Format "yyyy-MM-dd HH:mm:ss"
    Add-Content -Path $logFile -Value "[$ts] $msg"
}

function Restore-Backup($dirName) {
    $bakDir = Join-Path $appDir "${dirName}_bak"
    $curDir = Join-Path $appDir $dirName
    if (Test-Path $bakDir) {
        if (Test-Path $curDir) {
            Remove-Item $curDir -Recurse -Force
        }
        Rename-Item $bakDir $dirName
        Write-Host "  Rolled back $dirName from backup"
    }
}

try {
    # Step 1: Wait for Douya.exe to exit
    Write-Host "Waiting for Douya to exit..."
    $process = Get-Process -Name "Douya" -ErrorAction SilentlyContinue
    if ($process) {
        $process.WaitForExit()
        Start-Sleep -Seconds 2
    }

    # Step 2: Extract zip
    Write-Host "Extracting update..."
    Expand-Archive -Path $zipPath -DestinationPath $tempExtract -Force

    # Step 3: Backup and replace bin/
    if (Test-Path "$tempExtract\bin") {
        Write-Host "Updating bin/..."
        $binBak = Join-Path $appDir "bin_bak"
        if (Test-Path $binBak) { Remove-Item $binBak -Recurse -Force }
        if (Test-Path "$appDir\bin") {
            Rename-Item "$appDir\bin" "bin_bak"
        }
        try {
            New-Item -ItemType Directory -Path "$appDir\bin" -Force | Out-Null
            Copy-Item "$tempExtract\bin\*" "$appDir\bin\" -Recurse -Force
        } catch {
            Write-UpdateLog "bin/ replace failed: $($_.Exception.Message)"
            Restore-Backup "bin"
            throw
        }
    }

    # Step 4: Backup and replace runtime/
    if (Test-Path "$tempExtract\runtime") {
        Write-Host "Updating runtime/..."
        $rtBak = Join-Path $appDir "runtime_bak"
        if (Test-Path $rtBak) { Remove-Item $rtBak -Recurse -Force }
        if (Test-Path "$appDir\runtime") {
            Rename-Item "$appDir\runtime" "runtime_bak"
        }
        try {
            New-Item -ItemType Directory -Path "$appDir\runtime" -Force | Out-Null
            Copy-Item "$tempExtract\runtime\*" "$appDir\runtime\" -Recurse -Force
            # Preserve user-downloaded backend zip packages
            if (Test-Path "$appDir\runtime_bak") {
                Get-ChildItem "$appDir\runtime_bak" -Filter "*.zip" -File | ForEach-Object {
                    Copy-Item $_.FullName "$appDir\runtime\" -Force
                    Write-Host "  Preserved user zip: $($_.Name)"
                }
            }
        } catch {
            Write-UpdateLog "runtime/ replace failed: $($_.Exception.Message)"
            Restore-Backup "runtime"
            # Also restore bin if it was already replaced
            Restore-Backup "bin"
            throw
        }
    }

    # Step 5: Verify critical files exist
    if (-not (Test-Path "$appDir\bin\Douya.exe")) {
        Write-UpdateLog "Verification failed: bin\Douya.exe not found after update"
        Restore-Backup "runtime"
        Restore-Backup "bin"
        throw "Verification failed: Douya.exe not found"
    }

    # Step 6: Success - remove backups
    Write-Host "Verification passed, cleaning up backups..."
    if (Test-Path "$appDir\bin_bak") { Remove-Item "$appDir\bin_bak" -Recurse -Force }
    if (Test-Path "$appDir\runtime_bak") { Remove-Item "$appDir\runtime_bak" -Recurse -Force }

    # Step 7: Start new version
    Write-Host "Starting Douya..."
    Start-Process $exePath

    # Step 8: Cleanup temp files
    Start-Sleep -Seconds 3
    Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $zipPath -Force -ErrorAction SilentlyContinue
    Write-Host "Update complete!"

} catch {
    $errMsg = $_.Exception.Message
    $stack = $_.ScriptStackTrace
    Write-UpdateLog "Update FAILED: $errMsg"
    Write-UpdateLog "Stack: $stack"
    # Try to start old version if it still exists
    if (Test-Path $exePath) {
        Write-Host "Attempting to start previous version..."
        Start-Process $exePath
    }
    # Cleanup temp files
    Remove-Item $tempExtract -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Item $zipPath -Force -ErrorAction SilentlyContinue
}
`, psEscape(appDirPath), psEscape(zipPath), psEscape(tempExtract), psEscape(exePath))

	// 写入脚本文件（UTF-8 BOM 编码）
	scriptPath := filepath.Join(tempDir, "update.ps1")
	if err := writeUTF8BOM(scriptPath, script); err != nil {
		return apperror.Wrap(apperror.KindInternal, "写入更新脚本失败", err)
	}

	// 启动 PowerShell 脚本（分离进程，不显示窗口）
	cmd := exec.Command("powershell", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008, // CREATE_NO_WINDOW
	}

	if err := cmd.Start(); err != nil {
		return apperror.Wrap(apperror.KindInternal, "启动更新脚本失败", err)
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

	// 写入 UTF-8 BOM（0xEF, 0xBB, 0xBF）
	if _, err := f.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		f.Close() // 关闭失败在此路径为次要错误，优先返回写错误
		return err
	}

	// 写入内容
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		return err
	}
	// 关闭时刷新缓冲；若关闭失败视为写失败
	return f.Close()
}

// windowsAssetPatterns Windows 安装包资产匹配正则（包级缓存，避免每次检查更新时重复编译）
var windowsAssetPatterns = []*regexp.Regexp{
	// 优先匹配更精确的 windows-amd64（历史命名），避免未来命名回退后误选其他平台资产
	regexp.MustCompile(`^Douya-v.+windows-amd64\.zip$`),
	// 回退匹配当前发布命名 windows.zip
	regexp.MustCompile(`^Douya-v.+windows\.zip$`),
}

// findWindowsAsset 在发布资产中查找 Windows 安装包
// 兼容两种命名：Douya-v0.11.6-windows.zip（当前发布命名）与 Douya-v0.11.6-windows-amd64.zip（历史命名）
func findWindowsAsset(assets []llm.GitHubAsset) string {
	for _, pattern := range windowsAssetPatterns {
		for _, asset := range assets {
			if pattern.MatchString(asset.Name) {
				return asset.BrowserDownloadURL
			}
		}
	}
	return ""
}

// compareVersions 比较两个语义化版本号
// 返回值：1 表示 a > b，-1 表示 a < b，0 表示相等
// 生活类比：就像比较两个学生的成绩，先比总分，总分一样再比单科
func compareVersions(a, b string) int {
	partsA := strings.Split(a, ".")
	partsB := strings.Split(b, ".")

	maxLen := max(len(partsB), len(partsA))

	for i := range maxLen {
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

// validateUpdateURL 校验更新下载 URL 的安全性
// 基于 GO-SSRF-001 安全实践：仅允许 HTTPS 协议、仅允许 GitHub 官方域名、拒绝内网/本地地址
// 生活类比：就像快递员只认准官方发货地址，拒绝从陌生地址取件
//
// TOCTOU 风险说明（BUG-6，已评估并接受）：
// 本函数对 DNS 解析结果做了内网/本地地址检查，但 HTTP 客户端在实际建立连接时会再次解析 DNS，
// 两次解析可能返回不同 IP（DNS rebinding 攻击），理论上存在 TOCTOU（Time-Of-Check vs Time-Of-Use）窗口。
// 经评估，本应用接受该风险，不做完全修复，理由如下：
//  1. 本应用为本地客户端，URL 实际来源于 GitHub API 返回的资产地址，并非用户可直接控制的输入；
//  2. 已具备 HTTPS 协议校验 + GitHub 官方域名白名单 + DNS 内网地址检查的多层防护；
//  3. 完全修复需自定义 http.Transport.DialContext 在 TCP 连接前再次校验目标 IP，复杂度较高；
//  4. 攻击者若能实施 DNS rebinding，通常已具备更高等级的系统控制权，SSRF 防护价值有限。
//
// 如未来需进一步加固，可在 downloadWithProgress 中使用自定义 Dialer 复用本函数解析得到的 IP 直连。
func validateUpdateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return apperror.Wrap(apperror.KindInvalidInput, "URL 解析失败", err)
	}

	// 仅允许 HTTPS 协议
	if parsed.Scheme != "https" {
		return apperror.Newf(apperror.KindInvalidInput, "仅允许 HTTPS 协议，当前: %s", parsed.Scheme)
	}

	// 仅允许 GitHub 官方域名
	allowedHosts := map[string]bool{
		"github.com":                           true,
		"objects.githubusercontent.com":        true,
		"release-assets.githubusercontent.com": true,
	}
	hostname := parsed.Hostname()
	if !allowedHosts[hostname] {
		return apperror.Newf(apperror.KindInvalidInput, "仅允许 GitHub 官方域名，当前: %s", hostname)
	}

	// P0-3 修复：校验 URL 路径前缀，防止前端传入其他 GitHub 仓库的 URL。
	// 生活类比：不仅检查快递是不是从官方仓库发出的，还要检查收件地址是不是自家的，
	// 防止有人把别家的包裹发过来冒充更新包。
	// github.com 域名下的 release 下载 URL 形如：
	//   /{owner}/{repo}/releases/download/{tag}/{asset}
	// 必须匹配本项目的 owner/repo 路径前缀。
	// objects.githubusercontent.com 和 release-assets.githubusercontent.com 是 GitHub CDN，
	// 路径不包含 owner/repo，跳过此校验。
	if hostname == "github.com" {
		expectedPrefix := fmt.Sprintf("/%s/%s/", version.GitHubOwner, version.GitHubRepo)
		if !strings.HasPrefix(parsed.Path, expectedPrefix) {
			return apperror.Newf(apperror.KindInvalidInput, "URL 路径不匹配本项目仓库，预期前缀: %s，当前路径: %s",
				expectedPrefix, parsed.Path)
		}
	}

	// 解析 DNS 并拒绝内网/本地地址（防止 DNS 重绑定攻击）
	ips, err := net.LookupIP(hostname)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "DNS 解析失败", err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return apperror.Newf(apperror.KindInvalidInput, "检测到内网/本地地址: %s", ip.String())
		}
	}

	return nil
}
