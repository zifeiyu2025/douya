# ============================================================
# 豆芽 · 微软商店上架就绪自检脚本
# ------------------------------------------------------------
# 每次发版/提交商店前运行一次，逐项核对上架硬性条件：
#   1. 版本单一事实源一致性（version.go / wails.json / AppxManifest）
#   2. MSIX 清单的 BOM 编码与身份非占位符
#   3. 打包脚本本身的 BOM 编码（PS5.1 解析保护）
#   4. 内置引擎（llama-server.exe）是否就位（商店版开箱即用关键）
#   5. 应用 exe 是否已构建
#   6. Go 测试（可带 -Race 开关做深度竞态检查）
#
# 用法：
#   powershell -NoProfile -ExecutionPolicy Bypass -File .\scripts\store-readiness-check.ps1 [-Race]
#
# 生活类比：飞机起飞前的例行检查清单——起飞前逐项打勾，
# 任何一项不过就直接拦下，绝不让带病飞机上天。
# ============================================================

param(
    [switch]$Race
)

$ErrorActionPreference = "Stop"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = (Resolve-Path (Join-Path $ScriptDir "..")).Path

# 结果统计
$passed = 0
$failed = 0
$failures = New-Object System.Collections.Generic.List[string]

function Report-Chk {
    param([string]$Name, [bool]$Ok, [string]$Detail)
    if ($Ok) {
        $script:passed++
        Write-Host ("  [PASS] " + $Name) -ForegroundColor Green
    } else {
        $script:failed++
        $script:failures.Add($Name + " -> " + $Detail)
        Write-Host ("  [FAIL] " + $Name + " : " + $Detail) -ForegroundColor Red
    }
    if ($Detail) { Write-Host ("         " + $Detail) -ForegroundColor DarkGray }
}

function Test-Utf8Bom {
    # 检查文件前 3 字节是否为 UTF-8 BOM（EF BB BF）
    param([string]$Path)
    if (-not (Test-Path $Path)) { return $false }
    $bytes = New-Object byte[] 3
    $fs = [System.IO.File]::OpenRead($Path)
    try {
        $n = $fs.Read($bytes, 0, 3)
        return ($n -eq 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF)
    } finally {
        $fs.Dispose()
    }
}

Write-Host "=== 豆芽 微软商店上架就绪自检 ===" -ForegroundColor Cyan
Write-Host "项目根: $ProjectRoot"
Write-Host ""

# ---------- 1. 版本单一事实源 ----------
Write-Host "[1] 版本一致性" -ForegroundColor Yellow
$versionGo = Join-Path $ProjectRoot "internal\version\version.go"
if (-not (Test-Path $versionGo)) {
    Report-Chk "version.go 存在" $false "缺少 $versionGo"
} else {
    Report-Chk "version.go 存在" $true ""
    $m = [regex]::Match((Get-Content -Raw $versionGo), 'Version\s*=\s*"([^"]+)"')
    if (-not $m.Success) {
        Report-Chk "version.go 含 Version 常量" $false "未匹配到 Version = \"...\""
    } else {
        $srcVer = $m.Groups[1].Value
        Report-Chk ("version.go 版本格式 (" + $srcVer + ")") ([regex]::IsMatch($srcVer, '^\d+\.\d+\.\d+$')) "应为 X.Y.Z 三段格式"
    }
}

# wails.json ProductVersion 与 version.go 一致（允许四段 vs 三段相差末尾 .0）
$wailsJson = Join-Path $ProjectRoot "wails.json"
$wailsVer = ""
if (-not (Test-Path $wailsJson)) {
    Report-Chk "wails.json 存在" $false ""
} else {
    Report-Chk "wails.json 存在" $true ""
    $wm = [regex]::Match((Get-Content -Raw $wailsJson), '"ProductVersion"\s*:\s*"([^"]+)"')
    if ($wm.Success) { $wailsVer = $wm.Groups[1].Value }
    $normalize = { param($v) ($v -replace '\.0$', '') }
    $same = $srcVer -and ($( & $normalize $srcVer) -eq $( & $normalize $wailsVer))
    Report-Chk ("wails.json ProductVersion 一致 (期望 " + $srcVer + "，实际 " + $wailsVer + ")") ($same -eq $true)
}

# ---------- 2. MSIX 清单检查 ----------
Write-Host ""
Write-Host "[2] AppxManifest.xml" -ForegroundColor Yellow
$manifest = Join-Path $ProjectRoot "build\windows\msix\AppxManifest.xml"
if (-not (Test-Path $manifest)) {
    Report-Chk "AppxManifest.xml 存在" $false ""
} else {
    Report-Chk "AppxManifest.xml 存在" $true ""
    Report-Chk "AppxManifest 为 UTF-8 BOM（MSIX 清单规范要求）" (Test-Utf8Bom $manifest) "无 BOM 时 makepri 会把中文按 ANSI 解析导致 PRI191"
    $mx = Get-Content -Raw $manifest
    # 身份非占位符
    $nameM = [regex]::Match($mx, '<Identity\s+Name="([^"]+)"')
    $pubM = [regex]::Match($mx, '<Identity\s+Name="[^"]+"\s+Publisher="([^"]+)"')
    if ($nameM.Success) {
        $pkgName = $nameM.Groups[1].Value
        $isPlaceholder = $pkgName -eq "PLACEHOLDER" -or $pkgName -match '<[^>]+>'
        Report-Chk ("Identity Name 非占位符 (" + $pkgName + ")") (-not $isPlaceholder) "上架前必须替换为合作伙伴中心预留的真实包名"
    } else {
        Report-Chk "Identity Name 存在" $false ""
    }
    if ($pubM.Success) {
        $pub = $pubM.Groups[1].Value
        $isPlaceholder = $pub -eq "CN=PLACEHOLDER" -or $pub -match '<[^>]+>'
        Report-Chk ("Publisher 非占位符 (" + $pub + ")") (-not $isPlaceholder) "上架前必须与开发者证书/合作伙伴中心身份一致"
    } else {
        Report-Chk "Publisher 存在" $false ""
    }
    # 清单版本 == version.go（补 .0 后比较）
    $verM = [regex]::Match($mx, '<Identity\s[^>]*Version="([0-9.]+)"')
    if ($verM.Success) {
        $manVer = $verM.Groups[1].Value
        $expect = $srcVer
        if (($expect -split '\.').Count -lt 4) { $expect += ".0" }
        Report-Chk ("清单版本一致 (期望 " + $expect + "，实际 " + $manVer + ")") ($manVer -eq $expect) "若不一致说明没有用 make-msix.ps1 打包（其会自动同步版本）"
    } else {
        Report-Chk "清单 Version 属性存在" $false ""
    }
    Report-Chk "runFullTrust 能力已声明" ($mx -match 'runFullTrust') "缺少该能力则商店版无法拉起引擎/托盘/写数据"
}

# ---------- 3. 打包脚本检查 ----------
Write-Host ""
Write-Host "[3] make-msix.ps1" -ForegroundColor Yellow
$makeMsix = Join-Path $ProjectRoot "build\windows\msix\make-msix.ps1"
if (-not (Test-Path $makeMsix)) {
    Report-Chk "make-msix.ps1 存在" $false ""
} else {
    Report-Chk "make-msix.ps1 存在" $true ""
    Report-Chk "make-msix.ps1 为 UTF-8 BOM" (Test-Utf8Bom $makeMsix) "无 BOM 时 PS5.1 将按 ANSI 解析中文注释导致脚本报语法错误"
}

# ---------- 4. 内置引擎检查 ----------
Write-Host ""
Write-Host "[4] 内置推理引擎（商店版开箱即用关键）" -ForegroundColor Yellow
$engineDirs = @(
    (Join-Path $ProjectRoot "runtime\vulkan"),
    (Join-Path $ProjectRoot "build\runtime\vulkan")
)
$engineFound = $false
foreach ($ed in $engineDirs) {
    $engineExe = Join-Path $ed "llama-server.exe"
    if (Test-Path $engineExe) {
        $engineFound = $true
        $cnt = (Get-ChildItem $ed -File).Count
        Report-Chk ("引擎目录就位 (" + $ed.Replace($ProjectRoot + "\", "") + "，$cnt 文件)") $true ""
        break
    }
}
if (-not $engineFound) {
    Report-Chk "引擎目录含 llama-server.exe" $false "请先运行 .\scripts\deploy-runtime.ps1 部署引擎，或确认 runtime\vulkan 存在"
}

# ---------- 5. 应用 exe 检查 ----------
Write-Host ""
Write-Host "[5] Douya.exe 构建产物" -ForegroundColor Yellow
$appExe = Join-Path $ProjectRoot "build\bin\Douya.exe"
if (Test-Path $appExe) {
    $ft = (Get-Item $appExe).LastWriteTime.ToString("yyyy-MM-dd HH:mm:ss")
    Report-Chk ("Douya.exe 已构建 (" + $ft + ")") $true ""
} else {
    Report-Chk "Douya.exe 已构建" $false "请先执行 wails build"
}

# ---------- 6. Go 测试 ----------
Write-Host ""
Write-Host ("[6] Go 测试（" + $(if ($Race) { "含 -race" } else { "常规" }) + "）") -ForegroundColor Yellow
Push-Location $ProjectRoot
try {
    # PS 5.1 兼容：& 调用的命令与参数必须分开传，不能合并进一个数组
    & "go" "vet" "./..." 2>&1 | Out-Host
    Report-Chk "go vet ./..." ($LASTEXITCODE -eq 0) "静态检查应无错误"
    if ($Race) {
        & "go" "test" "-race" "./..." 2>&1 | Select-Object -Last 5 | Out-Host
    } else {
        & "go" "test" "./..." 2>&1 | Select-Object -Last 5 | Out-Host
    }
    Report-Chk ("go test " + $(if ($Race) { "-race" } else { "常规" }) + " ./...") ($LASTEXITCODE -eq 0) "全量测试应全绿"
} finally {
    Pop-Location
}

# ---------- 汇总 ----------
Write-Host ""
Write-Host "=== 自检汇总 ===" -ForegroundColor Cyan
Write-Host ("  通过: $passed  失败: $failed") -ForegroundColor $(if ($failed -eq 0) { "Green" } else { "Red" })
if ($failed -gt 0) {
    Write-Host "以下项目需处理：" -ForegroundColor Red
    $failures | ForEach-Object { Write-Host ("  - " + $_) -ForegroundColor Red }
    Write-Host "处理完上述失败项后重新运行本脚本，全部 PASS 再提交商店。"
    exit 1
} else {
    Write-Host "全部 PASS —— 可以执行 make-msix.ps1 打包并提交微软商店。" -ForegroundColor Green
    Write-Host "提示：提交商店的包应为【未签名】版本（商店会自动重签），本地开发者签名仅用于自测。"
    exit 0
}