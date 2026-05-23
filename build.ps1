[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
chcp 65001 > $null 2>&1

$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$OutputDir = Join-Path $ProjectRoot "release"

Write-Host "=== 豆芽 AI 发布构建 ===" -ForegroundColor Cyan

Write-Host "[0/5] 设置 Go 代理..." -ForegroundColor Yellow
$env:GOPROXY = "https://goproxy.cn,direct"
Write-Host "  GOPROXY = $env:GOPROXY" -ForegroundColor Gray

Write-Host "[1/5] 构建前端..." -ForegroundColor Yellow
Set-Location (Join-Path $ProjectRoot "frontend")
npm run build
if ($LASTEXITCODE -ne 0) { throw "前端构建失败" }

Write-Host "[2/5] 执行 Wails 构建..." -ForegroundColor Yellow
Set-Location $ProjectRoot
wails build
if ($LASTEXITCODE -ne 0) { throw "Wails 构建失败" }

Write-Host "[3/5] 准备发布目录（增量模式）..." -ForegroundColor Yellow
New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null

$BinDir = Join-Path $OutputDir "bin"
New-Item -ItemType Directory -Path $BinDir -Force | Out-Null

Copy-Item (Join-Path $ProjectRoot "build\bin\douya.exe") $BinDir -Force
Copy-Item (Join-Path $ProjectRoot "config.json") $OutputDir -Force

$syncDirs = @("engines", "models", "runtime")
foreach ($dir in $syncDirs) {
    $src = Join-Path $ProjectRoot $dir
    $dst = Join-Path $OutputDir $dir
    if (Test-Path $src) {
        robocopy $src $dst /MIR /NJH /NJS /NDL /NFL /NC /NS
        if ($LASTEXITCODE -ge 8) { throw "robocopy $dir 失败 (exit code $LASTEXITCODE)" }
    }
}

New-Item -ItemType Directory -Path (Join-Path $OutputDir "data") -Force | Out-Null

Write-Host "[4/5] 验证发布包..." -ForegroundColor Yellow
$requiredFiles = @(
    @{ Path = (Join-Path $BinDir "douya.exe"); Name = "douya.exe" },
    @{ Path = (Join-Path $OutputDir "config.json"); Name = "config.json" },
    @{ Path = (Join-Path $OutputDir "engines\llama-server.exe"); Name = "engines\llama-server.exe" },
    @{ Path = (Join-Path $OutputDir "runtime"); Name = "runtime\" }
)

$allOk = $true
foreach ($f in $requiredFiles) {
    if (-not (Test-Path $f.Path)) {
        Write-Host "  缺失: $($f.Name)" -ForegroundColor Red
        $allOk = $false
    } else {
        Write-Host "  存在: $($f.Name)" -ForegroundColor Green
    }
}

if ($allOk) {
    Write-Host ""
    Write-Host "=== 构建成功 ===" -ForegroundColor Green
    Write-Host "发布包位于: $OutputDir" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "目录结构:" -ForegroundColor Cyan
    Write-Host "  $OutputDir\"
    Write-Host "    config.json"
    Write-Host "    bin\douya.exe"
    Write-Host "    engines\"
    Write-Host "    models\"
    Write-Host "    runtime\"
    Write-Host "    data\"
} else {
    Write-Host ""
    Write-Host "=== 构建完成，但部分文件缺失 ===" -ForegroundColor Yellow
    Write-Host "发布包位于: $OutputDir" -ForegroundColor Cyan
}
