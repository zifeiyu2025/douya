# ============================================================
# 豆芽 · 微软商店一键发布（A 层：本地编排脚本）
# ------------------------------------------------------------
# 目标：在本机复刻 B 层 CI 流水线（.github/workflows/release.yml）的完整
# 发布流程，一条命令产出可提交微软商店的最终产物，方便发版前本地验证、
# 或在不想走 CI 时直接打包。
#
# 流程（与 CI 对齐）：
#   1. 读取主版本号（internal/version/version.go，单点事实源）
#   2. 版本一致性校验（check_version_consistency.ps1，4 处）
#   3. 官方预编译引擎下载/组装（assemble-engines-from-official.ps1，
#      锁定 llm.PinnedReleaseTag 单点版本与运行时严格一致）
#   4. 构建（复用 build.ps1：前端 + Wails + 同步三套引擎到 build\bin\runtime）
#   5. 商店上架自检（store-readiness-check.ps1）
#   6. 打包 .msix（make-msix.ps1 → build\bin\Douya.msix / Douya-<ver>.msix）
#
# 用法：
#   powershell -ExecutionPolicy Bypass -File scripts\publish-store.ps1
#   可选开关：
#     -SkipEngines  跳过引擎下载（引擎已就位时加速）
#     -SkipBuild    跳过构建（仅打包/自检，需已有 build\bin\Douya.exe）
#     -SkipCheck    跳过上架自检
#     -OutputDir <路径>  指定 .msix 输出目录（默认 build\bin）
# ============================================================

param(
    [switch]$SkipEngines,
    [switch]$SkipBuild,
    [switch]$SkipCheck,
    [string]$OutputDir = ""
)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
try { $null = cmd /c 'chcp 65001 > nul' } catch { }

$ErrorActionPreference = "Stop"

$script:ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$script:ProjectRoot = (Resolve-Path (Join-Path $script:ScriptDir "..")).Path

# 统一的步骤报告辅助（成功绿 / 失败红并中止）
$script:stepNo = 0
function Step-Header {
    param([string]$Name, [string]$Detail = "")
    $script:stepNo++
    Write-Host ""
    Write-Host ("[" + $script:stepNo + "/6] " + $Name) -ForegroundColor Yellow
    if ($Detail) { Write-Host "      $Detail" -ForegroundColor Gray }
}
function Assert-Step {
    param([bool]$Ok, [string]$Msg)
    if ($Ok) { Write-Host "  [PASS] $Msg" -ForegroundColor Green }
    else { Write-Host "  [FAIL] $Msg" -ForegroundColor Red; throw "步骤失败: $Msg" }
}

Write-Host "=== 豆芽 微软商店一键发布（A 层 本地编排） ===" -ForegroundColor Cyan
Write-Host "项目根: $ProjectRoot"
Write-Host "开关  : SkipEngines=$SkipEngines  SkipBuild=$SkipBuild  SkipCheck=$SkipCheck"

# ---------- 1. 读取主版本号（唯一事实源） ----------
Step-Header "读取主版本号" "internal/version/version.go"
$verGo = Join-Path $ProjectRoot "internal\version\version.go"
$vm = [regex]::Match((Get-Content -Raw -Path $verGo), 'Version\s*=\s*"([^"]+)"')
Assert-Step $vm.Success "从 version.go 提取版本号"
$DouyaVersion = $vm.Groups[1].Value
Write-Host "  主版本: $DouyaVersion"
$env:DOUYA_VERSION = $DouyaVersion  # 本地导入环境，供后续产物命名等使用

# ---------- 2. 版本一致性校验（4 处，始终执行） ----------
Step-Header "版本一致性校验" "scripts/check_version_consistency.ps1"
if (-not $SkipCheck) {
    & (Join-Path $ProjectRoot "scripts\check_version_consistency.ps1")
    Assert-Step ($LASTEXITCODE -eq 0) "四个版本源一致"
} else {
    Write-Host "  已跳过（-SkipCheck）" -ForegroundColor Gray
}

# ---------- 3. 官方预编译引擎下载/组装（锁定 PinnedReleaseTag） ----------
Step-Header "官方引擎下载/组装" "复用 assemble-engines-from-official.ps1"
if (-not $SkipEngines) {
    . (Join-Path $ProjectRoot "scripts\assemble-engines-from-official.ps1") -SkipRun
    Invoke-AssembleEnginesFromOfficial -RuntimeRoot (Join-Path $ProjectRoot "runtime") -SkipIfReady
} else {
    Write-Host "  已跳过（-SkipEngines），继续用现有 runtime\" -ForegroundColor Gray
}

# ---------- 4. 构建（前端 + Wails + 同步引擎） ----------
Step-Header "构建应用" "复用 build.ps1"
if (-not $SkipBuild) {
    & (Join-Path $ProjectRoot "build.ps1")
    Assert-Step ($LASTEXITCODE -eq 0) "应用构建成功"
} else {
    Write-Host "  已跳过（-SkipBuild）" -ForegroundColor Gray
}
$exePath = Join-Path $ProjectRoot "build\bin\Douya.exe"
Assert-Step (Test-Path $exePath) "Douya.exe 已构建"

# ---------- 5. 商店上架自检（引擎就绪 + exe 就绪时执行） ----------
Step-Header "商店上架自检" "scripts/store-readiness-check.ps1"
if (-not $SkipCheck) {
    & (Join-Path $ProjectRoot "scripts\store-readiness-check.ps1")
    Assert-Step ($LASTEXITCODE -eq 0) "上架自检通过"
} else {
    Write-Host "  已跳过（-SkipCheck）" -ForegroundColor Gray
}

# ---------- 6. 打包 .msix（引擎齐全时） ----------
Step-Header "打包 MSIX" "build/windows/msix/make-msix.ps1"
if (-not $OutputDir) { $OutputDir = Join-Path $ProjectRoot "build\bin" }
& (Join-Path $ProjectRoot "build\windows\msix\make-msix.ps1") -OutputDir $OutputDir
Assert-Step ($LASTEXITCODE -eq 0) "make-appx 打包成功"

$outDefault = Join-Path $OutputDir "Douya.msix"
$outNamed = Join-Path $OutputDir ("Douya-" + $DouyaVersion + ".msix")
if (Test-Path $outDefault) {
    Move-Item -LiteralPath $outDefault -Destination $outNamed -Force
} elseif (-not (Test-Path $outNamed)) {
    throw "打包后未找到产物：$outNamed"
}
$sizeMB = [math]::Round((Get-Item $outNamed).Length / 1MB, 2)
Write-Host "  MSIX 产物: $outNamed ($sizeMB MB)" -ForegroundColor Green

Write-Host ""
Write-Host "=== 发布打包完成 ===" -ForegroundColor Green
Write-Host ""
Write-Host ("后续（微软商店提交）：" ) -ForegroundColor Cyan
Write-Host ("  - 上传产物 : " + $outNamed) -ForegroundColor Gray
Write-Host "  - 提醒     : 确认 AppxManifest.xml 的 Name/Publisher 已替换为商店真实身份。" -ForegroundColor Yellow
Write-Host "  - 版本     : 若需再次上架，请先递增 AppxManifest.xml Version 第四段，再重跑本脚本。" -ForegroundColor Yellow