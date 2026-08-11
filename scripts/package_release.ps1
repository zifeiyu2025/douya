[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
# 兼容 PowerShell 5.1
try { $null = cmd /c 'chcp 65001 > nul' } catch { }

$ErrorActionPreference = "Stop"

# 脚本所在目录的上级即项目根目录（scripts/ -> 项目根）
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir

Write-Host "=== 豆芽 AI 发布包打包 ===" -ForegroundColor Cyan

# ---------------------------------------------------------------------------
# 1. 从内部唯一版本源 internal/version/version.go 读取版本号，构造 zip 文件名
#    生活类比：版本号只在 version.go 这一处写，这里来"查户口"，
#    保证 zip 名 Douya-vX.X.X-win64.zip 与 exe 内嵌版本、GitHub tag 完全一致。
# ---------------------------------------------------------------------------
$VersionGoPath = Join-Path $ProjectRoot "internal\version\version.go"
if (-not (Test-Path $VersionGoPath)) { throw "找不到 version.go: $VersionGoPath" }
$match = [regex]::Match((Get-Content -Raw $VersionGoPath), 'Version\s*=\s*"([^"]+)"')
if (-not $match.Success) { throw "无法从 version.go 提取 Version 常量" }
$version = $match.Groups[1].Value

$zipName = "Douya-v$version-win64.zip"
$zipPath = Join-Path $ProjectRoot $zipName
Write-Host "版本: $version  ->  产出: $zipName" -ForegroundColor Gray

# ---------------------------------------------------------------------------
# 2. 校验前置：build.ps1 必须先产出 release/
# ---------------------------------------------------------------------------
$ReleaseDir = Join-Path $ProjectRoot "release"
if (-not (Test-Path (Join-Path $ReleaseDir "bin\Douya.exe"))) {
    throw "未找到 release\bin\Douya.exe，请先运行 build.ps1 构建发布包"
}

# ---------------------------------------------------------------------------
# 3. 构建干净的发布暂存目录（排除项见下）
# ---------------------------------------------------------------------------
$Staging = Join-Path $ProjectRoot "release-staging"
Remove-Item $Staging -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Path $Staging -Force | Out-Null

# 3a. 应用本体（exe）+ 后端运行时（runtime，含预编译的 CUDA/Vulkan 等 DLL）
foreach ($d in @("bin", "runtime")) {
    $src = Join-Path $ReleaseDir $d
    if (Test-Path $src) {
        Copy-Item $src (Join-Path $Staging $d) -Recurse -Force
        Write-Host "  已包含: $d\" -ForegroundColor Green
    }
}

# 3b. models 仅保留占位结构，排除个人模型 *.gguf（用户私有、体积大、GitHub 单文件 2GB 上限）
$ModelsDst = Join-Path $Staging "models"
New-Item -ItemType Directory -Path $ModelsDst -Force | Out-Null
$ggufCount = 0
Get-ChildItem (Join-Path $ReleaseDir "models") -File -ErrorAction SilentlyContinue | ForEach-Object {
    if ($_.Extension -eq '.gguf') { $script:ggufCount++; return }
    Copy-Item $_.FullName $ModelsDst -Force
}
Write-Host "  models: 已排除 $ggufCount 个个人模型(*.gguf)，仅保留占位" -ForegroundColor Green

# 3c. 明确排除：个人配置文件与运行时数据（属于用户本地私有，不应分发）
#     - config.json / mcp_servers.json / router-preset.ini（个人配置）
#     - data\（聊天记录、设置等运行时数据）
#     如需随包分发默认配置，可在此处改为复制模板而非用户实际文件。
Write-Host "  已排除: 个人配置文件(config.json/mcp_servers.json/router-preset.ini) 与 data\" -ForegroundColor Gray

# ---------------------------------------------------------------------------
# 4. 体积预警（GitHub Releases 单文件上限 2GB；PowerShell Compress-Archive 流上限约 2GB）
# ---------------------------------------------------------------------------
$size = (Get-ChildItem $Staging -Recurse | Measure-Object -Property Length -Sum).Sum
$sizeMB = [math]::Round($size / 1MB, 1)
Write-Host "  暂存体积: $sizeMB MB" -ForegroundColor Cyan
if ($size -gt 1.8GB) {
    Write-Host "  警告: 暂存体积超过 1.8GB，逼近 GitHub 2GB 上限，上传可能失败！" -ForegroundColor Yellow
}

# ---------------------------------------------------------------------------
# 5. 打包 zip（优先 Compress-Archive；超大时回退 tar.exe，绕开 .NET 2GB 流限制）
# ---------------------------------------------------------------------------
if (Test-Path $zipPath) { Remove-Item $zipPath -Force }
try {
    Compress-Archive -Path (Join-Path $Staging "*") -DestinationPath $zipPath -CompressionLevel Optimal
} catch {
    Write-Host "Compress-Archive 失败，回退 tar.exe ..." -ForegroundColor Yellow
    & tar.exe -a -c -f $zipPath -C $Staging .
    if ($LASTEXITCODE -ne 0) { throw "tar.exe 打包失败" }
}
Remove-Item $Staging -Recurse -Force -ErrorAction SilentlyContinue

$zipSizeMB = [math]::Round((Get-Item $zipPath).Length / 1MB, 1)
Write-Host ""
Write-Host "=== 打包完成 ===" -ForegroundColor Green
Write-Host "产物: $zipPath ($zipSizeMB MB)" -ForegroundColor Cyan
Write-Host ""
Write-Host "后续发布步骤：" -ForegroundColor Yellow
Write-Host "  git tag -a v$version -m ""v$version""" -ForegroundColor Gray
Write-Host "  git push origin v$version" -ForegroundColor Gray
Write-Host "  gh release create v$version --title ""v$version"" --notes ""..."" $zipName" -ForegroundColor Gray
