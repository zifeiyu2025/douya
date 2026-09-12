# 引擎本地编译 + 部署脚本（CPU / CUDA / Vulkan 三套）
#
# 目的：把"引擎编译配方"固化下来，避免每次升级靠 CMakeCache / 构建日志反推 cmake 命令行。
#
# 事实源：
#   - 目标版本：internal\llm\backend_download.go 的 PinnedReleaseTag（默认自动解析，保证引擎与官方兜底包同版本）
#   - 产物落点：项目根 runtime\{cpu,cuda,vulkan}（商店包与其它下游的源头，make-msix.ps1 首选此处）
#
# 关键约束：llama.cpp 的构建号 = git rev-list --count HEAD，因此必须 checkout 到目标 tag 再编译，
#           在 master/HEAD 上编译会得到错误的 build 号（例如 b10883 编成 10884）。
#
# 前置：Visual Studio 2022 BuildTools、CUDA Toolkit 13.x（CUDA_PATH）、Vulkan SDK、cmake。
#
# 示例：
#   .\scripts\build-engines.ps1                      # 全流程：checkout 锁定 tag -> 编三套 -> 部署到 runtime
#   .\scripts\build-engines.ps1 -SkipCheckout        # 源码树已在正确 tag 上，跳过 checkout
#   .\scripts\build-engines.ps1 -Backends cpu,vulkan # 只编部分后端（不触发部署）
#   .\scripts\build-engines.ps1 -SkipBuild           # 跳过编译，仅用现有产物重新部署
param(
    [string]$LlamaRoot = "D:\AI\llama.cpp",
    [string]$ProjectRoot = "",
    [string]$Tag = "",
    [string[]]$Backends = @("cpu", "vulkan", "cuda"),
    [string]$CMake = "D:\Toolchains\mingw64\bin\cmake.exe",
    [string]$VcVars = "C:\Program Files (x86)\Microsoft Visual Studio\2022\BuildTools\VC\Auxiliary\Build\vcvars64.bat",
    [switch]$SkipCheckout,
    [switch]$SkipBuild,
    [switch]$SkipDeploy
)
$ErrorActionPreference = "Stop"

if (-not $ProjectRoot) { $ProjectRoot = Split-Path -Parent $PSScriptRoot }
if (-not (Test-Path $LlamaRoot)) { throw "llama.cpp 源码目录不存在: $LlamaRoot" }
if (-not (Test-Path $CMake)) { $CMake = (Get-Command cmake -ErrorAction Stop).Source }
if (-not (Test-Path $VcVars)) { throw "vcvars64.bat 不存在: $VcVars（用 -VcVars 指定）" }

# ---------- 解析锁定 tag ----------
if (-not $Tag) {
    $dl = Join-Path $ProjectRoot "internal\llm\backend_download.go"
    $m = [regex]::Match((Get-Content -Raw $dl), 'PinnedReleaseTag\s*=\s*"([^"]+)"')
    if (-not $m.Success) { throw "无法从 $dl 解析 PinnedReleaseTag" }
    $Tag = $m.Groups[1].Value
}
if ($Tag -notmatch '^b(\d+)$') { throw "本脚本仅支持 nightly tag（形如 b10883），当前: $Tag" }
$ExpectedBuild = [int]$Matches[1]
Write-Host "引擎版本: $Tag (期望 build $ExpectedBuild)" -ForegroundColor Cyan

# ---------- checkout tag ----------
if (-not $SkipCheckout) {
    & git -C $LlamaRoot fetch --tags --quiet
    & git -C $LlamaRoot checkout $Tag
    if ($LASTEXITCODE -ne 0) { throw "git checkout $Tag 失败" }
    $count = [int](& git -C $LlamaRoot rev-list --count HEAD)
    if ($count -ne $ExpectedBuild) { throw "rev-list count=$count，与 tag 期望 build $ExpectedBuild 不符" }
    Write-Host "已切换到 $Tag（rev-list count=$count）" -ForegroundColor Green
}

# ---------- 环境预检（避免长编译跑完才发现环境失效）----------
if (-not $SkipBuild -and $Backends -contains 'cuda') {
    if (-not $env:CUDA_PATH) { throw "CUDA_PATH 未设置，无法编译 CUDA 后端" }
    $cudaBin = Join-Path $env:CUDA_PATH "bin\x64"
    if (-not (Test-Path $cudaBin)) {
        throw "CUDA_PATH 无效: $env:CUDA_PATH（路径不存在。若 CUDA 工具链升级过版本，请刷新环境变量，并删除旧 build-cuda 目录后重试——其 CMakeCache 缓存了旧工具链路径）"
    }
    if (-not (Get-ChildItem (Join-Path $cudaBin 'cudart64_*.dll') -ErrorAction SilentlyContinue)) {
        throw "CUDA_PATH bin 下未找到 cudart64_*.dll: $cudaBin"
    }
}
if (-not $SkipBuild -and $Backends -contains 'vulkan') {
    foreach ($p in @('D:/AI/vulkan-sdk/sys/include', 'D:/AI/vulkan-sdk/sys/Lib/vulkan-1.lib')) {
        if (-not (Test-Path $p)) { throw "Vulkan SDK 路径无效: $p（build-engines.ps1 的 vulkan 选项与预检均引用此路径）" }
    }
}

# ---------- 每套后端的 cmake 选项（与 b10816/b10883 已验证配方一致）----------
$CommonOpts = @('-DGGML_NATIVE=OFF', '-DGGML_BACKEND_DL=ON', '-DGGML_OPENMP=ON', '-DLLAMA_BUILD_SERVER=ON')
$BackendOpts = @{
    'cpu'    = $CommonOpts + @('-DGGML_CPU=ON', '-DGGML_CPU_ALL_VARIANTS=ON', '-DGGML_CUDA=OFF', '-DGGML_VULKAN=OFF', '-DGGML_RPC=OFF')
    'cuda'   = $CommonOpts + @('-DGGML_CPU=OFF', '-DGGML_CPU_ALL_VARIANTS=OFF', '-DGGML_CUDA=ON', '-DGGML_VULKAN=OFF', '-DGGML_RPC=OFF')
    'vulkan' = $CommonOpts + @('-DGGML_CPU=OFF', '-DGGML_CPU_ALL_VARIANTS=OFF', '-DGGML_VULKAN=ON', '-DGGML_CUDA=OFF', '-DGGML_RPC=OFF',
                               '-DVulkan_INCLUDE_DIR=D:/AI/vulkan-sdk/sys/include', '-DVulkan_LIBRARY=D:/AI/vulkan-sdk/sys/Lib/vulkan-1.lib')
}

function Build-Backend([string]$b) {
    if (-not $BackendOpts.ContainsKey($b)) { throw "未知后端: $b（可选 cpu/cuda/vulkan）" }
    $dir = Join-Path $LlamaRoot "build-$b"
    $optStr = ($BackendOpts[$b] -join ' ')
    Write-Host "=== 配置 + 编译 $b ===" -ForegroundColor Cyan
    $cmd = "call `"$VcVars`" >nul 2>&1 && set `"PATH=D:\Toolchains\mingw64\bin;%PATH%`" && `"$CMake`" -S `"$LlamaRoot`" -B `"$dir`" -G `"Visual Studio 17 2022`" -A x64 $optStr && `"$CMake`" --build `"$dir`" --config Release --target llama-server mtmd"
    cmd.exe /c $cmd
    if ($LASTEXITCODE -ne 0) { throw "编译 $b 失败（exit $LASTEXITCODE）" }

    # 版本自检：确认 build 号等于锁定 tag
    $exe = Join-Path $dir "bin\Release\llama-server.exe"
    if (-not (Test-Path $exe)) { throw "$b 产物缺失: $exe" }
    # llama-server --version 输出走 stderr，PS5.1 在 Stop 模式下会把它当致命错误，需临时放宽
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    $ver = (& $exe --version 2>&1 | Out-String)
    $ErrorActionPreference = $prevEap
    $vm = [regex]::Match($ver, 'build (\d+), commit')
    if (-not $vm.Success) { throw "无法解析 $b 引擎版本: $ver" }
    if ([int]$vm.Groups[1].Value -ne $ExpectedBuild) {
        throw "$b 引擎 build 号 $($vm.Groups[1].Value) 与锁定 tag $ExpectedBuild 不符"
    }
    Write-Host "$b 引擎校验通过: $($ver.Trim().Split("`n")[0])" -ForegroundColor Green
}

if (-not $SkipBuild) {
    foreach ($b in $Backends) { Build-Backend $b }
}

# ---------- 部署到项目根 runtime\（源头）----------
if (-not $SkipDeploy) {
    if ($Backends.Count -ne 3) {
        Write-Warning "-Backends 未包含全部三套（cpu/cuda/vulkan），deploy-built-engines.ps1 需要三套产物才部署，已跳过。"
    }
    else {
        & (Join-Path $PSScriptRoot "deploy-built-engines.ps1") -LlamaRoot $LlamaRoot -ProjectRoot $ProjectRoot
        if ($LASTEXITCODE -ne 0) { throw "部署失败（exit $LASTEXITCODE）" }
        Write-Host "部署完成：项目根 runtime\{cpu,cuda,vulkan}（源头）" -ForegroundColor Green
    }
}

Write-Host "全部完成。后续：.\build.ps1 构建应用；build\windows\msix\make-msix.ps1 打包商店。" -ForegroundColor Green
