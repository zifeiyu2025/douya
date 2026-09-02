# ============================================================
# 豆芽 · 官方预编译引擎下载/组装脚本（本地版）
# ------------------------------------------------------------
# 用途：与 CI 流水线（.github/workflows/release.yml 第 3 步）保持严格一致，
# 从 llama.cpp 官方 GitHub release（版本由 Go 源码 llm.PinnedReleaseTag 锁
# 定，单点事实源）下载并组装三套引擎到 runtime\：
#   - cpu    ：CPU 基底（llama-server.exe + 核心 DLL）
#   - cuda   ：CPU 基底 + ggml-cuda.dll + cudart 配套运行库
#   - vulkan ：CPU 基底 + ggml-vulkan.dll
#
# 两种调用方式：
#   1. 直接运行：自动组装到项目根 runtime\（默认，供 make-msix 打包用）
#   2. 被 dot-source：仅定义 Invoke-AssembleEnginesFromOfficial() 函数，
#      供 publish-store.ps1 复用（传 -SkipRun 时只载入函数不执行）
#
# 用法示例：
#   powershell -ExecutionPolicy Bypass -File scripts\assemble-engines-from-official.ps1
#   . .\scripts\assemble-engines-from-official.ps1 -SkipRun   # 只载入函数
# ============================================================

param(
    [switch]$SkipRun   # 仅载入函数定义，不立即执行（供其他脚本 dot-source 复用）
)

[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
$ErrorActionPreference = "Stop"
$script:ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$script:ProjectRoot = (Resolve-Path (Join-Path $script:ScriptDir "..")).Path

# ============================================================
# 核心函数：从官方预编译下载并组装三套引擎到 runtimeRoot
# 参数：
#   - RuntimeRoot : 目标引擎根目录（默认项目根 runtime\，含 cpu\cuda\vulkan 子目录）
#   - TempRoot    : 临时下载目录（默认 $env:TEMP\douya-engine-dl）
#   - ProxyRegex  : 加速代理前缀（默认 gh-proxy，可传 '' 关闭）
#   - SkipIfReady : 若三套引擎已就位则直接跳过下载
# 返回：$true 表示就绪
# ============================================================
function Invoke-AssembleEnginesFromOfficial {
    param(
        [string]$RuntimeRoot = "",
        [string]$TempRoot = "",
        [string]$ProxyRegex = "https://gh-proxy.com/",
        [switch]$SkipIfReady
    )

    if (-not $RuntimeRoot) { $RuntimeRoot = Join-Path $script:ProjectRoot "runtime" }
    if (-not $TempRoot) { $TempRoot = Join-Path $env:TEMP "douya-engine-dl" }

    $Backends = @("cpu", "cuda", "vulkan")

    # ---- 0. 幂等：三套引擎已就位则跳过 ----
    if ($SkipIfReady) {
        $ready = $true
        foreach ($b in $Backends) {
            if (-not (Test-Path (Join-Path $RuntimeRoot "$b\llama-server.exe"))) { $ready = $false; break }
        }
        if ($ready) {
            Write-Host "三套引擎已就位（$RuntimeRoot），跳过下载" -ForegroundColor Cyan
            return $true
        }
    }

    # ---- 1. 从 Go 源码读取锁定版本号（唯一事实源，与运行时严格一致） ----
    $backendDownload = Join-Path $script:ProjectRoot "internal\llm\backend_download.go"
    if (-not (Test-Path $backendDownload)) { throw "找不到 backend_download.go：$backendDownload" }
    $m = [regex]::Match((Get-Content -Raw -Path $backendDownload), 'PinnedReleaseTag\s*=\s*"([^"]+)"')
    if (-not $m.Success) { throw '无法从 backend_download.go 提取 PinnedReleaseTag' }
    $tag = $m.Groups[1].Value
    Write-Host "锁定官方版本: $tag" -ForegroundColor Cyan

    # ---- 2. 查询该 release 的资产清单 ----
    $apiUrl = "https://api.github.com/repos/ggml-org/llama.cpp/releases/tags/$tag"
    try {
        $release = Invoke-RestMethod -Uri $apiUrl -Headers @{ 'User-Agent' = 'Douya-Release' } -TimeoutSec 30
    } catch {
        throw "查询 GitHub release 失败（$apiUrl）：$($_.Exception.Message)"
    }
    $assets = @{}
    $release.assets | ForEach-Object { $assets[$_.name] = $_.browser_download_url }
    Write-Host "Release: $($release.name)（$($release.assets.Count) 个资产）" -ForegroundColor Cyan

    # ---- 3. 锁定三套后端 + cudart 的确切官方资产文件名 ----
    $cpuZip    = "llama-$tag-bin-win-cpu-x64.zip"
    $cudaZip   = "llama-$tag-bin-win-cuda-13.3-x64.zip"
    $cudartZip = "cudart-llama-bin-win-cuda-13.3-x64.zip"
    $vulkanZip = "llama-$tag-bin-win-vulkan-x64.zip"
    foreach ($n in @($cpuZip, $cudaZip, $cudartZip, $vulkanZip)) {
        if (-not $assets[$n]) { throw "release $tag 中未找到资产: $n（请核对 PinnedReleaseTag 对应的资产命名规则）" }
    }

    # ---- 临时目录 ----
    New-Item -ItemType Directory -Path $TempRoot -Force | Out-Null
    $funcNewTempZip = { param($name) Join-Path $TempRoot $name }

    # ---- 4. 工具：加速下载（代理优先，失败回落官方源） ----
    function Invoke-DownloadEngine {
        param([string]$Name, [string]$OutFile)
        $url = $assets[$Name]
        $candidates = @()
        if ($ProxyRegex) { $candidates += ($ProxyRegex + $url) }
        $candidates += $url
        foreach ($u in $candidates) {
            Write-Host "  下载 $Name -> $u" -ForegroundColor Yellow
            try {
                Invoke-WebRequest -Uri $u -OutFile $OutFile -TimeoutSec 600 -UseBasicParsing
                return
            } catch {
                Write-Host "  源失败（$u）：$($_.Exception.Message)，换下一源..." -ForegroundColor Gray
            }
        }
        throw "所有下载源均失败: $Name"
    }

    # ---- 5. 工具：拍平——解压后若 llama-server.exe 不在顶层（套一层目录），上移内容 ----
    function Flatten-Dir([string]$dir) {
        if (-not (Test-Path (Join-Path $dir 'llama-server.exe'))) {
            $nested = Get-ChildItem -Path $dir -Directory | Select-Object -First 1
            if ($nested -and (Test-Path (Join-Path $nested.FullName 'llama-server.exe'))) {
                Get-ChildItem -Path $nested.FullName | Move-Item -Destination $dir -Force
                Remove-Item -LiteralPath $nested.FullName -Recurse -Force
            }
        }
    }

    # ---- 6. 工具：清理冗余 CLI 工具（与运行时 isRedundantFile 同款），只留 llama-server 运行链路 ----
    function Remove-Redundant([string]$dir) {
        if (-not (Test-Path $dir)) { return }
        Get-ChildItem -Path $dir -File -ErrorAction SilentlyContinue | ForEach-Object {
            $n = $_.Name -replace '\.(exe|dll)$',''
            if ($n -match '^(llama-batched-bench|llama-bench|llama-cli$|llama-completion|llama-fit-params|llama-gguf-split|llama-imatrix|llama-llava-cli|llama-minicpmv-cli|llama-mtmd-cli|llama-perplexity|llama-quantize|llama-qwen2vl-cli|llama-results|llama-template-analysis|llama-tokenize|llama-export-lora|ggml-rpc-server)$') {
                Remove-Item $_.FullName -Force
            }
        }
    }

    # ---- 输出目录准备 ----
    foreach ($b in $Backends) {
        $dir = Join-Path $RuntimeRoot $b
        if (Test-Path $dir) { Remove-Item $dir -Recurse -Force }
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }

    # ---- 7. CPU 基底（llama-server.exe + 核心 DLL） ----
    $down = & $funcNewTempZip 'cpu.zip'
    Invoke-DownloadEngine -Name $cpuZip -OutFile $down
    Expand-Archive -Path $down -DestinationPath (Join-Path $RuntimeRoot "cpu") -Force
    Flatten-Dir (Join-Path $RuntimeRoot "cpu")
    Remove-Redundant (Join-Path $RuntimeRoot "cpu")
    if (-not (Test-Path (Join-Path $RuntimeRoot "cpu\llama-server.exe"))) { throw "CPU 基底解压缺少 llama-server.exe" }
    Write-Host "  CPU 基底就绪" -ForegroundColor Gray

    # ---- 8. cuda：CPU 基底 + ggml-cuda.dll + cudart 运行库 ----
    Get-ChildItem -Path (Join-Path $RuntimeRoot "cpu\*") | Copy-Item -Destination (Join-Path $RuntimeRoot "cuda") -Recurse -Force
    $down = & $funcNewTempZip 'cuda.zip'
    Invoke-DownloadEngine -Name $cudaZip -OutFile $down
    Expand-Archive -Path $down -DestinationPath (Join-Path $RuntimeRoot "cuda") -Force
    Flatten-Dir (Join-Path $RuntimeRoot "cuda"); Remove-Redundant (Join-Path $RuntimeRoot "cuda")
    $down = & $funcNewTempZip 'cudart.zip'
    Invoke-DownloadEngine -Name $cudartZip -OutFile $down
    Expand-Archive -Path $down -DestinationPath (Join-Path $RuntimeRoot "cuda") -Force
    if (-not (Test-Path (Join-Path $RuntimeRoot "cuda\ggml-cuda.dll"))) { throw "cuda 缺少 ggml-cuda.dll" }
    if (-not (Get-ChildItem (Join-Path $RuntimeRoot "cuda") -Filter 'cudart64_*.dll')) { throw "cuda 缺少 cudart64_*.dll" }
    Write-Host "  cuda 就绪" -ForegroundColor Gray

    # ---- 9. vulkan：CPU 基底 + ggml-vulkan.dll ----
    Get-ChildItem -Path (Join-Path $RuntimeRoot "cpu\*") | Copy-Item -Destination (Join-Path $RuntimeRoot "vulkan") -Recurse -Force
    $down = & $funcNewTempZip 'vulkan.zip'
    Invoke-DownloadEngine -Name $vulkanZip -OutFile $down
    Expand-Archive -Path $down -DestinationPath (Join-Path $RuntimeRoot "vulkan") -Force
    Flatten-Dir (Join-Path $RuntimeRoot "vulkan"); Remove-Redundant (Join-Path $RuntimeRoot "vulkan")
    if (-not (Test-Path (Join-Path $RuntimeRoot "vulkan\ggml-vulkan.dll"))) { throw "vulkan 缺少 ggml-vulkan.dll" }
    Write-Host "  vulkan 就绪" -ForegroundColor Gray

    # ---- 10. 汇总校验 ----
    $ready = $true
    $totalMB = 0.0
    foreach ($b in $Backends) {
        $dir = Join-Path $RuntimeRoot $b
        if (-not (Test-Path (Join-Path $dir "llama-server.exe"))) { $ready = $false; Write-Host "错误: $b 缺 llama-server.exe" -ForegroundColor Red }
        $totalMB += [Math]::Round((Get-ChildItem $dir -Recurse -File | Measure-Object Length -Sum).Sum / 1MB, 1)
    }
    if (-not $ready) { throw '引擎组装/校验失败' }
    Write-Host ""
    Write-Host "三套引擎组装完成（cpu/cuda/vulkan），锁定版本: $tag，共约 $totalMB MB" -ForegroundColor Green
    Write-Host "目标目录: $RuntimeRoot"

    # 清理下载临时文件
    if (Test-Path $TempRoot) { Remove-Item $TempRoot -Recurse -Force -ErrorAction SilentlyContinue }
    return $true
}

# ---- 直接运行时执行 ----
if (-not $SkipRun) {
    Invoke-AssembleEnginesFromOfficial
}