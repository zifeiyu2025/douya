# 引擎组装 + 部署脚本（本地自编译三套引擎：cpu/cuda/vulkan）
# 1. 组装 staging：cpu 产物为基底，cuda/vulkan 各自叠加后端 DLL 与厂商运行库
# 2. 调用 scripts\deploy-runtime.ps1 逐套部署（事实源 runtime\<b> + dev 镜像 build\runtime\<b>）
# 3. 同步到 build\bin\runtime（与 build.ps1 [4/4] 相同逻辑）
param(
    [string]$LlamaRoot = "D:\AI\llama.cpp",
    [string]$ProjectRoot = "D:\MyGoWorkspace\douya"
)
$ErrorActionPreference = "Stop"

$CpuBin = Join-Path $LlamaRoot "build-cpu\bin\Release"
$CudaBin = Join-Path $LlamaRoot "build-cuda\bin\Release"
$VulkanBin = Join-Path $LlamaRoot "build-vulkan\bin\Release"
$Staging = Join-Path $LlamaRoot "build-staging"

$Backends = @("cpu", "cuda", "vulkan")
$Sources = @{
    "cpu"     = @($CpuBin)
    "cuda"    = @($CpuBin, $CudaBin)
    "vulkan"  = @($CpuBin, $VulkanBin)
}

# 必须存在的后端标志文件（确保对应构建已完成）
$Marker = @{ "cpu" = "llama-server.exe"; "cuda" = "ggml-cuda.dll"; "vulkan" = "ggml-vulkan.dll" }

foreach ($b in $Backends) {
    $stage = Join-Path $Staging $b
    if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
    New-Item -ItemType Directory -Path $stage -Force | Out-Null
    foreach ($src in $Sources[$b]) {
        if (-not (Test-Path $src)) { throw "源目录缺失: $src（对应引擎尚未构建完成）" }
        Copy-Item -Path (Join-Path $src "*") -Destination $stage -Recurse -Force
    }
    if (-not (Test-Path (Join-Path $stage $Marker[$b]))) { throw "staging/$b 缺少 $($Marker[$b])" }
    # cuda：从 CUDA 工具包补齐厂商运行库（官方发布流程同款做法）
    if ($b -eq "cuda") {
        $cudaBinDir = Join-Path $env:CUDA_PATH "bin\x64"
        if (-not (Test-Path $cudaBinDir)) { throw "未找到 CUDA 工具包 bin 目录: $cudaBinDir" }
        foreach ($p in @("cudart64_*.dll", "cublas64_*.dll", "cublasLt64_*.dll")) {
            Copy-Item -Path (Join-Path $cudaBinDir $p) -Destination $stage -Force
        }
    }
    Write-Host "staging/$b 就绪: $((Get-ChildItem $stage -File).Count) 个文件"
}

# 逐套部署
# 注：deploy-runtime.ps1 内部 robocopy 的退出码 1-7 均属成功，且其自身
# $ErrorActionPreference=Stop 会在真实失败时 throw 终止，故此处不检查 $LASTEXITCODE。
foreach ($b in $Backends) {
    Write-Host "=== 部署 $b ===" -ForegroundColor Cyan
    & (Join-Path $ProjectRoot "scripts\deploy-runtime.ps1") -SourceDir (Join-Path $Staging $b) -Backend $b
}

# 同步到 build\bin\runtime（与 build.ps1 [4/4] 相同）
$engineRoot = $null
foreach ($cand in @((Join-Path $ProjectRoot "runtime"), (Join-Path $ProjectRoot "build\runtime"))) {
    $complete = $true
    foreach ($bk in $Backends) {
        if (-not (Test-Path (Join-Path $cand "$bk\llama-server.exe"))) { $complete = $false; break }
    }
    if ($complete) { $engineRoot = $cand; break }
}
if (-not $engineRoot) { throw "未找到完整引擎根（runtime\ 与 build\runtime\ 均不完整）" }
foreach ($b in $Backends) {
    $dst = Join-Path $ProjectRoot "build\bin\runtime\$b"
    robocopy (Join-Path $engineRoot $b) $dst /MIR /NJH /NJS /NDL /NFL /NC /NS | Out-Null
    if ($LASTEXITCODE -ge 8) { throw "同步失败: $b (robocopy exit $($LASTEXITCODE))" }
}
$global:LASTEXITCODE = 0
Write-Host "已同步: $engineRoot -> build\bin\runtime\{$($Backends -join ',')}" -ForegroundColor Green
