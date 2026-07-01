# check_version_consistency.ps1
# 校验 internal/version/version.go 与 frontend/package.json 的版本号一致
# 用法：powershell -File scripts\check_version_consistency.ps1
#
# 生活类比：发版前先核对"两张身份证"——Go 端的 Version 常量和前端的 package.json version，
# 两者必须同名同姓（版本号完全一致），否则不允许出门（发版）。

$ErrorActionPreference = "Stop"

# 脚本所在目录的上级即项目根目录
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir

$VersionGoPath = Join-Path $ProjectRoot "internal\version\version.go"
$PackageJsonPath = Join-Path $ProjectRoot "frontend\package.json"

# 检查文件存在
if (-not (Test-Path $VersionGoPath)) {
    Write-Host "错误: 找不到 version.go: $VersionGoPath" -ForegroundColor Red
    exit 1
}
if (-not (Test-Path $PackageJsonPath)) {
    Write-Host "错误: 找不到 package.json: $PackageJsonPath" -ForegroundColor Red
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

# 断言两者相等
if ($goVersion -ne $npmVersion) {
    Write-Host "错误: 版本号不一致" -ForegroundColor Red
    Write-Host "  version.go      Version = `"$goVersion`"" -ForegroundColor Red
    Write-Host "  package.json    version  = `"$npmVersion`"" -ForegroundColor Red
    Write-Host "请同步两者的版本号后再发版。" -ForegroundColor Red
    exit 1
}

# 一致则输出成功信息
Write-Host "版本一致: $goVersion" -ForegroundColor Green
Write-Host "提示: GitHub Release tag 需手动打，请确认 tag 为 v$goVersion 并与上述版本一致。" -ForegroundColor Yellow
exit 0
