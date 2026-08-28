[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
# 兼容 PowerShell 5.1：不依赖 >$null 2>&1 重定向语法
try { $null = cmd /c 'chcp 65001 > nul' } catch { }

$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path

Write-Host "=== 豆芽 AI 构建（微软商店版） ===" -ForegroundColor Cyan

# 发版前校验 version.go 与 package.json 版本号一致（任务 31）
$versionCheckScript = Join-Path $ProjectRoot "scripts\check_version_consistency.ps1"
if (Test-Path $versionCheckScript) {
    & $versionCheckScript
    if ($LASTEXITCODE -ne 0) { throw "版本一致性校验失败，请同步 version.go 与 package.json 的版本号" }
} else {
    Write-Host "警告: 未找到版本校验脚本 $versionCheckScript，跳过校验" -ForegroundColor Yellow
}

Write-Host "[0/4] 设置 Go 代理..." -ForegroundColor Yellow
$env:GOPROXY = "https://goproxy.cn,direct"
Write-Host "  GOPROXY = $env:GOPROXY" -ForegroundColor Gray

Write-Host "[1/4] 构建前端..." -ForegroundColor Yellow
Set-Location (Join-Path $ProjectRoot "frontend")
npm run build
if ($LASTEXITCODE -ne 0) { throw "前端构建失败" }

Write-Host "[2/4] 执行 Wails 构建..." -ForegroundColor Yellow
Set-Location $ProjectRoot
# 定位 wails.exe：环境变量 WAILS_EXE 优先（支持 CI/自定义路径），
# 再回退到 GOPATH/bin 和 PATH。
$wailsExe = $env:WAILS_EXE
if (-not $wailsExe -or -not (Test-Path $wailsExe)) { $wailsExe = Join-Path (go env GOPATH) "bin\wails.exe" }
if (-not (Test-Path $wailsExe)) { $wailsExe = "wails" }
# -ldflags "-s -w"：去掉符号表和 DWARF 调试信息，减小发布版二进制体积约 20-30%
# 生活类比：像发货前拆掉商品的精美包装——运输时不需要展示用的包装（调试信息），
# 只保留核心商品（可执行代码），既省空间又不影响使用。
# 注意：去掉后 panic 不显示函数名/行号，仅显示地址，适合发布版；开发版用 wails dev 不受影响。
& $wailsExe build -ldflags "-s -w"
if ($LASTEXITCODE -ne 0) { throw "Wails 构建失败" }

$builtExe = Join-Path $ProjectRoot "build\bin\Douya.exe"
if (-not (Test-Path $builtExe)) {
    throw "构建产物缺失: $builtExe`nWails 构建可能未成功完成。"
}
$size = [math]::Round((Get-Item $builtExe).Length / 1MB, 2)
Write-Host "[3/4] 构建产物校验..." -ForegroundColor Yellow
Write-Host "  已生成: build\bin\Douya.exe ($size MB)" -ForegroundColor Green

# ---------------------------------------------------------------------------
# [4/4] 内置引擎同步：build\bin\runtime\{cuda,vulkan,cpu}
#
# 为什么需要：bundledRuntimeDir() 把"包内引擎目录"定位到 exe 旁边
# （build\bin\runtime），MSIX 由 make-msix.ps1 打入三套引擎；但本地直接
# 运行 build\bin\Douya.exe 时，此前该目录为空（或只有个别引擎），首启会
# 命中 installBackend 的包内兜底，静默把 CUDA 换成包内现成的 Vulkan
# （N 卡性能大打折扣）。此处把与 make-msix.ps1 相同的三套引擎同步到
# exe 旁，使本地布局与商店包一致。
#
# 引擎根候选与 make-msix.ps1 相同：项目根 runtime\ 优先，回退 build\runtime\；
# 候选必须同时含 cuda\vulkan\cpu 三套（各含 llama-server.exe）才算完整，
# 单引擎的旧产物（如项目根 runtime\ 只剩 cuda）不能作为同步源。
# ---------------------------------------------------------------------------
Write-Host "[4/4] 同步内置引擎到 build\bin\runtime..." -ForegroundColor Yellow
$Backends = @("cuda", "vulkan", "cpu")
$engineRoot = $null
foreach ($cand in @((Join-Path $ProjectRoot "runtime"), (Join-Path $ProjectRoot "build\runtime"))) {
    $complete = $true
    foreach ($b in $Backends) {
        if (-not (Test-Path (Join-Path $cand "$b\llama-server.exe"))) { $complete = $false; break }
    }
    if ($complete) { $engineRoot = $cand; break }
}
if ($engineRoot) {
    foreach ($b in $Backends) {
        $dst = Join-Path $ProjectRoot "build\bin\runtime\$b"
        robocopy (Join-Path $engineRoot $b) $dst /MIR /NJH /NJS /NDL /NFL /NC /NS | Out-Null
        if ($LASTEXITCODE -ge 8) { throw "引擎同步失败: $b (robocopy exit $($LASTEXITCODE))" }
    }
    $global:LASTEXITCODE = 0
    Write-Host "  已同步: $engineRoot -> build\bin\runtime\{$($Backends -join ',')}" -ForegroundColor Green
} else {
    Write-Host "  警告: 未找到含 cuda/vulkan/cpu 三套引擎的目录（runtime\ 或 build\runtime\）。" -ForegroundColor Yellow
    Write-Host "  本地直接运行 build\bin\Douya.exe 时，首启会回退到包内现有引擎并弹出警告。" -ForegroundColor Yellow
    Write-Host "  请先运行 scripts\deploy-runtime.ps1 部署引擎，再重新执行本脚本。" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "=== 应用构建成功 ===" -ForegroundColor Green
Write-Host ""
Write-Host "后续步骤（微软商店打包）：" -ForegroundColor Cyan
Write-Host "  1. 引擎部署：scripts\deploy-runtime.ps1（首次或引擎更新后）" -ForegroundColor Gray
Write-Host "  2. 上架自检：scripts\store-readiness-check.ps1" -ForegroundColor Gray
Write-Host "  3. 商店打包：build\windows\msix\make-msix.ps1（产出 build\bin\Douya.msix）" -ForegroundColor Gray
