[Console]::OutputEncoding = [System.Text.Encoding]::UTF8
[Console]::InputEncoding = [System.Text.Encoding]::UTF8
$OutputEncoding = [System.Text.Encoding]::UTF8
# 兼容 PowerShell 5.1：不依赖 >$null 2>&1 重定向语法
try { $null = cmd /c 'chcp 65001 > nul' } catch { }

$ErrorActionPreference = "Stop"

$ProjectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$OutputDir = Join-Path $ProjectRoot "release"

Write-Host "=== 豆芽 AI 发布构建 ===" -ForegroundColor Cyan

# 发版前校验 version.go 与 package.json 版本号一致（任务 31）
$versionCheckScript = Join-Path $ProjectRoot "scripts\check_version_consistency.ps1"
if (Test-Path $versionCheckScript) {
    & $versionCheckScript
    if ($LASTEXITCODE -ne 0) { throw "版本一致性校验失败，请同步 version.go 与 package.json 的版本号" }
} else {
    Write-Host "警告: 未找到版本校验脚本 $versionCheckScript，跳过校验" -ForegroundColor Yellow
}

Write-Host "[0/5] 设置 Go 代理..." -ForegroundColor Yellow
$env:GOPROXY = "https://goproxy.cn,direct"
Write-Host "  GOPROXY = $env:GOPROXY" -ForegroundColor Gray

Write-Host "[1/5] 构建前端..." -ForegroundColor Yellow
Set-Location (Join-Path $ProjectRoot "frontend")
npm run build
if ($LASTEXITCODE -ne 0) { throw "前端构建失败" }

Write-Host "[2/5] 执行 Wails 构建..." -ForegroundColor Yellow
Set-Location $ProjectRoot
# 定位 wails.exe：环境变量 WAILS_EXE 优先（支持 CI/自定义路径），
# 回退到项目约定的 D:\Program Files\GoTools\bin\wails.exe（项目记忆硬约束），
# 再回退到 GOPATH/bin 和 PATH
$wailsExe = $env:WAILS_EXE
if (-not $wailsExe -or -not (Test-Path $wailsExe)) { $wailsExe = "D:\Program Files\GoTools\bin\wails.exe" }
if (-not (Test-Path $wailsExe)) { $wailsExe = Join-Path (go env GOPATH) "bin\wails.exe" }
if (-not (Test-Path $wailsExe)) { $wailsExe = "wails" }
# -ldflags "-s -w"：去掉符号表和 DWARF 调试信息，减小发布版二进制体积约 20-30%
# 生活类比：像发货前拆掉商品的精美包装——运输时不需要展示用的包装（调试信息），
# 只保留核心商品（可执行代码），既省空间又不影响使用。
# 注意：去掉后 panic 不显示函数名/行号，仅显示地址，适合发布版；开发版用 wails dev 不受影响。
& $wailsExe build -ldflags "-s -w"
if ($LASTEXITCODE -ne 0) { throw "Wails 构建失败" }

Write-Host "[3/5] 同步发布目录（增量模式）..." -ForegroundColor Yellow
New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null

$BinDir = Join-Path $OutputDir "bin"
New-Item -ItemType Directory -Path $BinDir -Force | Out-Null

$builtExe = Join-Path $ProjectRoot "build\bin\Douya.exe"
if (-not (Test-Path $builtExe)) {
    throw "构建产物缺失: $builtExe`nWails 构建可能未成功完成。"
}

$needCopyExe = $true
$dstExe = Join-Path $BinDir "Douya.exe"
if (Test-Path $dstExe) {
    $srcTime = (Get-Item $builtExe).LastWriteTimeUtc
    $dstTime = (Get-Item $dstExe).LastWriteTimeUtc
    if ($srcTime -le $dstTime) {
        Write-Host "  [增量] Douya.exe 已是最新，跳过" -ForegroundColor Gray
        $needCopyExe = $false
    }
}
if ($needCopyExe) {
    Copy-Item $builtExe $BinDir -Force
    Write-Host "  已复制: Douya.exe" -ForegroundColor Green
}

$syncDirs = @("models", "runtime")
foreach ($dir in $syncDirs) {
    $src = Join-Path $ProjectRoot $dir
    $dst = Join-Path $OutputDir $dir
    if (Test-Path $src) {
        $hasContent = (Get-ChildItem -Path $src -Recurse -File -ErrorAction SilentlyContinue | Measure-Object).Count -gt 0
        if ($hasContent) {
            & robocopy.exe $src $dst /MIR /NJH /NJS /NDL /NFL /NC /NS
            $rc = $LASTEXITCODE
            # robocopy 退出码 0-7 表示成功（0=无变化，1-7=成功复制），8+ 表示错误
            if ($rc -ge 8) { throw "robocopy $dir 失败 (exit code $rc)" }
            Write-Host "  已同步: $dir\" -ForegroundColor Green
        } else {
            New-Item -ItemType Directory -Path $dst -Force | Out-Null
            Write-Host "  已创建空目录: $dir\（源目录无文件）" -ForegroundColor Yellow
        }
    } else {
        New-Item -ItemType Directory -Path $dst -Force | Out-Null
        Write-Host "  已创建空目录: $dir\（源目录不存在）" -ForegroundColor Yellow
    }
}

New-Item -ItemType Directory -Path (Join-Path $OutputDir "data") -Force | Out-Null

Write-Host "[4/5] 验证并修复发布包..." -ForegroundColor Yellow

function CopyFileFromProject {
    param([string]$DstPath, [string[]]$SrcCandidates)

    foreach ($src in $SrcCandidates) {
        if (-not (Test-Path -LiteralPath $src)) { continue }

        if (Test-Path -LiteralPath $DstPath) {
            $srcTime = (Get-Item -LiteralPath $src).LastWriteTimeUtc
            $dstTime = (Get-Item -LiteralPath $DstPath).LastWriteTimeUtc
            if ($srcTime -le $dstTime) {
                return "skipped"
            }
        }

        $dstDir = Split-Path -Parent $DstPath
        if ($dstDir -and -not (Test-Path -LiteralPath $dstDir)) {
            New-Item -ItemType Directory -Path $dstDir -Force | Out-Null
        }
        Copy-Item -LiteralPath $src -Destination $DstPath -Force
        return "fixed"
    }

    return "missing"
}

function SyncDirFromProject {
    param([string]$DstPath, [string]$SrcDir)

    if ($SrcDir -and (Test-Path -LiteralPath $SrcDir)) {
        $hasContent = (Get-ChildItem -Path $SrcDir -Recurse -File -ErrorAction SilentlyContinue | Measure-Object).Count -gt 0
        if ($hasContent) {
            & robocopy.exe $SrcDir $DstPath /MIR /NJH /NJS /NDL /NFL /NC /NS
            $rc = $LASTEXITCODE
            # robocopy 退出码 0-7 表示成功，8+ 表示错误
            if ($rc -ge 8) { throw "robocopy 失败 (exit code $rc)" }
            return "fixed"
        }
    }

    if (-not (Test-Path -LiteralPath $DstPath)) {
        New-Item -ItemType Directory -Path $DstPath -Force | Out-Null
    }
    return "empty"
}

$criticalItems = @(
    @{ Name = "bin\Douya.exe";  Dst = "$BinDir\Douya.exe";
       Src = @("$ProjectRoot\build\bin\Douya.exe") }
)

$optionalItems = @(
    @{ Name = "models\";  Dst = "$OutputDir\models";  IsDir = $true;
       SrcDir = "$ProjectRoot\models" },
    @{ Name = "runtime\"; Dst = "$OutputDir\runtime"; IsDir = $true;
       SrcDir = "$ProjectRoot\runtime" },
    @{ Name = "data\";    Dst = "$OutputDir\data";    IsDir = $true;
       SrcDir = $null }
)

$criticalMissing = @()
foreach ($item in $criticalItems) {
    if (Test-Path -LiteralPath $item.Dst) {
        $size = if ((Get-Item -LiteralPath $item.Dst).PSIsContainer) { "-" } else { "$([math]::Round((Get-Item -LiteralPath $item.Dst).Length / 1MB, 2)) MB" }
        Write-Host "  [关键] 存在: $($item.Name) ($size)" -ForegroundColor Green
    } else {
        $result = CopyFileFromProject $item.Dst $item.Src
        if ($result -eq "fixed") {
            Write-Host "  [关键] 已从项目复制: $($item.Name)" -ForegroundColor Green
        } else {
            Write-Host "  [关键缺失] $($item.Name) - 项目中未找到" -ForegroundColor Red
            $criticalMissing += $item.Name
        }
    }
}

$optionalMissing = @()
foreach ($item in $optionalItems) {
    if (Test-Path -LiteralPath $item.Dst) {
        $isDir = (Get-Item -LiteralPath $item.Dst).PSIsContainer
        if ($isDir) {
            $fileCount = (Get-ChildItem -Path $item.Dst -Recurse -File -ErrorAction SilentlyContinue | Measure-Object).Count
            if ($fileCount -eq 0 -and $item.SrcDir) {
                $result = SyncDirFromProject $item.Dst $item.SrcDir
                if ($result -eq "fixed") {
                    $newCount = (Get-ChildItem -Path $item.Dst -Recurse -File -ErrorAction SilentlyContinue | Measure-Object).Count
                    Write-Host "  [可选] 已从项目同步: $($item.Name) ($newCount 个文件)" -ForegroundColor Green
                } else {
                    Write-Host "  [可选] 存在但为空: $($item.Name)（项目源也无文件）" -ForegroundColor Yellow
                }
            } elseif ($fileCount -eq 0) {
                Write-Host "  [可选] 存在但为空: $($item.Name)" -ForegroundColor Yellow
            } else {
                Write-Host "  [可选] 存在: $($item.Name) ($fileCount 个文件)" -ForegroundColor Green
            }
        } else {
            $sizeMB = [math]::Round((Get-Item -LiteralPath $item.Dst).Length / 1MB, 2)
            Write-Host "  [可选] 存在: $($item.Name) ($sizeMB MB)" -ForegroundColor Green
        }
    } else {
        if ($isDir = $item.IsDir) {
            if ($item.SrcDir) {
                $result = SyncDirFromProject $item.Dst $item.SrcDir
                if ($result -eq "fixed") {
                    $newCount = (Get-ChildItem -Path $item.Dst -Recurse -File -ErrorAction SilentlyContinue | Measure-Object).Count
                    Write-Host "  [可选] 已从项目同步: $($item.Name) ($newCount 个文件)" -ForegroundColor Green
                } else {
                    New-Item -ItemType Directory -Path $item.Dst -Force | Out-Null
                    Write-Host "  [可选] 已创建空目录: $($item.Name)" -ForegroundColor Yellow
                }
            } else {
                New-Item -ItemType Directory -Path $item.Dst -Force | Out-Null
                Write-Host "  [可选] 已创建空目录: $($item.Name)" -ForegroundColor Yellow
            }
        } else {
            $result = CopyFileFromProject $item.Dst $item.Src
            if ($result -eq "fixed") {
                Write-Host "  [可选] 已从项目复制: $($item.Name)" -ForegroundColor Green
            } else {
                Write-Host "  [可选缺失] $($item.Name) - 项目中未找到，运行时需手动放置" -ForegroundColor Yellow
                $optionalMissing += $item.Name
            }
        }
    }
}

if ($criticalMissing.Count -eq 0 -and $optionalMissing.Count -eq 0) {
    Write-Host ""
    Write-Host "=== 构建成功 ===" -ForegroundColor Green
    Write-Host "发布包位于: $OutputDir" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "目录结构:" -ForegroundColor Cyan
    Write-Host "  $OutputDir\"
    Write-Host "    bin\Douya.exe"
    Write-Host "    models\"
    Write-Host "    runtime\"
    Write-Host "    data\"
} elseif ($criticalMissing.Count -eq 0) {
    Write-Host ""
    Write-Host "=== 构建完成，部分可选文件缺失 ===" -ForegroundColor Yellow
    Write-Host "发布包位于: $OutputDir" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "缺失的可选文件（运行前需手动放置）:" -ForegroundColor Yellow
    foreach ($m in $optionalMissing) {
        Write-Host "  - $m" -ForegroundColor Yellow
    }
} else {
    Write-Host ""
    Write-Host "=== 构建失败，关键文件缺失 ===" -ForegroundColor Red
    Write-Host "发布包位于: $OutputDir" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "缺失的关键文件:" -ForegroundColor Red
    foreach ($m in $criticalMissing) {
        Write-Host "  - $m" -ForegroundColor Red
    }
    throw "关键文件缺失，构建未完成"
}