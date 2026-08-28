# 注意：param() 必须是脚本第一条可执行语句，前面只允许注释/空行。
# 编码设置、chcp 等初始化语句必须全部放在 param 之后，否则解析器会把
# param 当成普通命令，后续的类型化赋值全部报 InvalidLeftHandSide。
param(
    [string]$SourceDir = "D:\AI\llama.cpp\build\bin",
    [string]$Backend = "cuda"
)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
# 兼容 PowerShell 5.1
try { $null = cmd /c 'chcp 65001 > nul' } catch { }

$ErrorActionPreference = "Stop"

# ============================================================
# llama.cpp 编译产物一键部署脚本（runtime 产物唯一事实源管线）
# ------------------------------------------------------------
# 职责链（隔离设计）：
#   1. llama.cpp 构建输出（外部源码树）
#      -> 项目根 runtime\后端名\        [唯一事实源，发布同步的源头]
#      -> build\runtime\后端名\         [wails dev 数据区镜像，开发即时可用]
#
# 规则：
#   - 项目根 runtime\ 只放"当前版本"，不放备份（保持事实源干净）
#   - 旧版本备份留在 build\runtime\xxx.b构建号（本地回滚用，MSIX 打包只取项目根 runtime\）
#   - models 同理：个人模型放 build\models\（dev 测试），项目根 models\ 只留占位结构
#
# 用法示例：
#   powershell -ExecutionPolicy Bypass -File scripts\deploy-runtime.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\deploy-runtime.ps1 -Backend vulkan
# ============================================================

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir

Write-Host "=== 豆芽 runtime 产物部署 ($Backend) ===" -ForegroundColor Cyan

# 取 llama-server 版本横幅：它走 stderr 而非 stdout，PS5.1 会把重定向的
# stderr 包装成错误记录；这里临时放宽 EAP 并把所有行转成纯文本再拼接。
function Get-LlamaServerVersion {
    param([string]$ExePath)
    $prev = $ErrorActionPreference
    $ErrorActionPreference = "Continue"
    try {
        $out = & $ExePath --version 2>&1
        return (($out | ForEach-Object { $_.ToString() }) -join "`n")
    } finally {
        $ErrorActionPreference = $prev
    }
}

# ---------------------------------------------------------------------------
# 1. 定位源目录：优先 SourceDir 本身，其次 SourceDir\Release（MSVC 多配置布局）
# ---------------------------------------------------------------------------
$srcCandidates = @($SourceDir, (Join-Path $SourceDir "Release"))
$Src = $null
foreach ($c in $srcCandidates) {
    if (Test-Path (Join-Path $c "llama-server.exe")) { $Src = $c; break }
}
if (-not $Src) {
    $tried = $srcCandidates -join ", "
    throw "源目录中未找到 llama-server.exe，已探测：$tried 。请确认 llama.cpp 已编译完成。"
}
Write-Host "[1/5] 源目录: $Src" -ForegroundColor Yellow

# ---------------------------------------------------------------------------
# 2. 部署前版本冒烟：源 exe 必须能报出版本号（防止半成品/残留产物混入）
#    生活类比：收货前先验序列号——版本号都报不出来的货不能收。
# ---------------------------------------------------------------------------
$srcVersion = Get-LlamaServerVersion (Join-Path $Src "llama-server.exe")
if ($srcVersion -notmatch 'build (\d+), commit ([0-9a-f]+)') {
    throw "源 exe 版本冒烟失败，输出异常：[$srcVersion]"
}
$buildNo = $Matches[1]
$commit = $Matches[2]
Write-Host "[2/5] 源版本: build $buildNo, commit $commit" -ForegroundColor Yellow

# ---------------------------------------------------------------------------
# 3. 白名单复制 -> 项目根 runtime\$Backend\（唯一事实源）
#    只复制豆芽运行所需文件，排除 llama-cli.exe 等工具类产物。
# ---------------------------------------------------------------------------
$DstRoot = Join-Path $ProjectRoot ("runtime\" + $Backend)
New-Item -ItemType Directory -Path $DstRoot -Force | Out-Null

# 白名单：核心服务链 + ggml 全家（含 CPU 微架构变体）+ CUDA 厂商运行库 + 许可证
$patterns = @(
    "llama-server.exe",
    "llama*.dll", "ggml*.dll", "mtmd.dll", "libomp.dll",
    "cudart64_*.dll", "cublas64_*.dll", "cublasLt64_*.dll",
    "LICENSE*"
)
$copied = 0
foreach ($p in $patterns) {
    Get-ChildItem -Path $Src -Filter $p -File -ErrorAction SilentlyContinue | ForEach-Object {
        Copy-Item $_.FullName (Join-Path $DstRoot $_.Name) -Force
        $script:copied++
    }
}
if ($copied -eq 0) { throw "白名单匹配到 0 个文件，请检查源目录内容与模式清单" }

# 事实源版本复验：落位后的 exe 必须与源版本一致
$dstVersion = Get-LlamaServerVersion (Join-Path $DstRoot "llama-server.exe")
if ($dstVersion -ne $srcVersion) { throw "事实源落位后版本不一致：源[$srcVersion] vs 目标[$dstVersion]" }
Write-Host "[3/5] 事实源: runtime\$Backend\ ($copied 个文件, 版本复验通过)" -ForegroundColor Yellow

# ---------------------------------------------------------------------------
# 4. dev 镜像同步 -> build\runtime\$Backend\（wails dev 的 appDir 数据区）
#    旧版本先归档为 $Backend.b旧构建号（本地回滚用，不入发布包）。
# ---------------------------------------------------------------------------
$DevDir = Join-Path $ProjectRoot ("build\runtime\" + $Backend)
if (Test-Path (Join-Path $DevDir "llama-server.exe")) {
    $oldVersion = Get-LlamaServerVersion (Join-Path $DevDir "llama-server.exe")
    $oldBuild = if ($oldVersion -match 'build (\d+)') { $Matches[1] } else { "unknown" }
    if ($oldBuild -ne $buildNo) {
        $archive = Join-Path $ProjectRoot ("build\runtime\" + $Backend + ".b" + $oldBuild)
        if (Test-Path $archive) { Remove-Item $archive -Recurse -Force }
        Move-Item $DevDir $archive
        Write-Host "  已归档旧版: $($archive.Replace($ProjectRoot + '\', ''))" -ForegroundColor Gray
    }
}
New-Item -ItemType Directory -Path $DevDir -Force | Out-Null
robocopy $DstRoot $DevDir /MIR /NJH /NJS /NDL /NFL /NC /NS | Out-Null
if ($LASTEXITCODE -ge 8) { throw "dev 镜像同步失败 (robocopy exit $($LASTEXITCODE))" }
Write-Host "[4/5] dev 镜像: build\runtime\$Backend\ (wails dev 立即可用)" -ForegroundColor Yellow

# ---------------------------------------------------------------------------
# 5. 产物身份报告（可追溯：什么版本、何时、从哪来、落在哪）
# ---------------------------------------------------------------------------
$sizeMB = [math]::Round((Get-ChildItem $DstRoot -Recurse -File | Measure-Object Length -Sum).Sum / 1MB, 1)
Write-Host "[5/5] 部署完成" -ForegroundColor Yellow
Write-Host ""
Write-Host "=== 产物身份 ===" -ForegroundColor Green
Write-Host "  版本    : build $buildNo, commit $commit"
Write-Host "  后端    : $Backend"
Write-Host "  体积    : $sizeMB MB ($copied 个文件)"
Write-Host "  事实源  : runtime\$Backend\"
Write-Host "  dev镜像 : build\runtime\$Backend\"
Write-Host "  部署时间: $((Get-Date).ToString('yyyy-MM-dd HH:mm:ss'))"
Write-Host ""
Write-Host "后续：" -ForegroundColor Yellow
Write-Host "  - 本地自测  : .\build.ps1 构建应用，引擎位于 build\runtime\$Backend（dev 数据区）" -ForegroundColor Gray
Write-Host "  - 商店 MSIX : build\windows\msix\make-msix.ps1（内置 runtime\{cuda,vulkan,cpu} 三套引擎随包分发）" -ForegroundColor Gray
