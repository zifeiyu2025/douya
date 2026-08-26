# check_version_consistency.ps1
# 校验 internal/version/version.go、frontend/package.json、wails.json 与 AppxManifest.xml 的版本号一致
# 用法：powershell -File scripts\check_version_consistency.ps1
#
# 生活类比：发版前先核对"四张身份证"——Go 端的 Version 常量、前端的 package.json version、
# wails.json 的 ProductVersion（会写入 exe 文件属性），以及微软商店清单 AppxManifest.xml 的
# Identity Version。前三张必须同名同姓（三段式 x.y.z 完全一致）；第四张是商店专用证件，
# 采用四段式 x.y.z.n，其中前三位必须与主版本一致，末位 n 是商店递增位（每次上架需递增）。
# 任何一张对不上，都不允许出门（发版）。

$ErrorActionPreference = "Stop"

# 脚本所在目录的上级即项目根目录
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir

$VersionGoPath = Join-Path $ProjectRoot "internal\version\version.go"
$PackageJsonPath = Join-Path $ProjectRoot "frontend\package.json"
$WailsJsonPath = Join-Path $ProjectRoot "wails.json"
$AppxManifestPath = Join-Path $ProjectRoot "build\windows\msix\AppxManifest.xml"

# 检查文件存在
if (-not (Test-Path $VersionGoPath)) {
    Write-Host "错误: 找不到 version.go: $VersionGoPath" -ForegroundColor Red
    exit 1
}
if (-not (Test-Path $PackageJsonPath)) {
    Write-Host "错误: 找不到 package.json: $PackageJsonPath" -ForegroundColor Red
    exit 1
}
if (-not (Test-Path $WailsJsonPath)) {
    Write-Host "错误: 找不到 wails.json: $WailsJsonPath" -ForegroundColor Red
    exit 1
}
if (-not (Test-Path $AppxManifestPath)) {
    Write-Host "错误: 找不到 AppxManifest.xml: $AppxManifestPath" -ForegroundColor Red
    exit 1
}

# 从 version.go 用正则提取 Version = "x.y.z"
$versionGoContent = Get-Content -Raw -Path $VersionGoPath
$match = [regex]::Match($versionGoContent, 'Version\s*=\s*"([^"]+)"')
if (-not $match.Success) {
    Write-Host "错误: 无法从 version.go 提取 Version 常量" -ForegroundColor Red
    exit 1
}
$goVersion = $match.Groups[1].Value

# 从 package.json 用 ConvertFrom-Json 读取 version
$packageJsonContent = Get-Content -Raw -Path $PackageJsonPath
$packageJson = ConvertFrom-Json $packageJsonContent
$npmVersion = $packageJson.version

if (-not $npmVersion) {
    Write-Host "错误: 无法从 package.json 读取 version 字段" -ForegroundColor Red
    exit 1
}

# 从 wails.json 用 ConvertFrom-Json 读取 info.ProductVersion（写入 exe 文件属性）
$wailsJsonContent = Get-Content -Raw -Path $WailsJsonPath
$wailsJson = ConvertFrom-Json $wailsJsonContent
$wailsVersion = $wailsJson.info.ProductVersion

if (-not $wailsVersion) {
    Write-Host "错误: 无法从 wails.json 读取 info.ProductVersion 字段" -ForegroundColor Red
    exit 1
}

# 从 AppxManifest.xml 提取 Identity Version = "x.y.z.n"（微软商店 MSIX 清单，四段式）
$manifestContent = Get-Content -Raw -Path $AppxManifestPath
$msixMatch = [regex]::Match($manifestContent, '<Identity[^>]*\bVersion\s*=\s*"([^"]+)"')
if (-not $msixMatch.Success) {
    Write-Host "错误: 无法从 AppxManifest.xml 提取 Identity Version 属性" -ForegroundColor Red
    exit 1
}
$msixVersion = $msixMatch.Groups[1].Value

# 断言主版本一致：version.go 与 package.json 必须三段式完全相等
if ($goVersion -ne $npmVersion) {
    Write-Host "错误: 主版本号不一致" -ForegroundColor Red
    Write-Host "  version.go       Version       = `"$goVersion`"" -ForegroundColor Red
    Write-Host "  package.json     version       = `"$npmVersion`"" -ForegroundColor Red
    Write-Host "请同步两者的版本号后再发版。" -ForegroundColor Red
    exit 1
}

# 校验四段式版本（wails.json 与 AppxManifest.xml）：
# Windows 文件属性与 MSIX 清单要求四段式 x.y.z.n，前三段必须与主版本一致
$msixParts = $msixVersion.Split('.')
if ($msixParts.Count -lt 4 -or ("$($msixParts[0]).$($msixParts[1]).$($msixParts[2])") -ne $goVersion) {
    Write-Host "错误: 微软商店清单版本与主版本不一致" -ForegroundColor Red
    Write-Host "  主版本 (version.go 等)            = `"$goVersion`"" -ForegroundColor Red
    Write-Host "  AppxManifest.xml Identity Version = `"$msixVersion`"" -ForegroundColor Red
    Write-Host "清单版本必须为四段式 x.y.z.n，且前三位等于主版本；末位 n 为商店递增位（每次上架需递增）。" -ForegroundColor Red
    exit 1
}
$wailsParts = $wailsVersion.Split('.')
if ($wailsParts.Count -lt 4 -or ("$($wailsParts[0]).$($wailsParts[1]).$($wailsParts[2])") -ne $goVersion) {
    Write-Host "错误: wails.json 版本与主版本不一致" -ForegroundColor Red
    Write-Host "  主版本 (version.go 等)     = `"$goVersion`"" -ForegroundColor Red
    Write-Host "  wails.json ProductVersion = `"$wailsVersion`"" -ForegroundColor Red
    Write-Host "ProductVersion 写入 exe 文件属性，必须为四段式 x.y.z.n，且前三位等于主版本。" -ForegroundColor Red
    exit 1
}

# 一致则输出成功信息
Write-Host "版本一致: $goVersion（exe 属性/MSIX 清单: $wailsVersion / $msixVersion）" -ForegroundColor Green
Write-Host "提示: GitHub Release tag 需手动打，请确认 tag 为 v$goVersion 并与上述版本一致。" -ForegroundColor Yellow
exit 0
