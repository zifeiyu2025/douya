# ============================================================
# 豆芽 Microsoft Store (MSIX) 打包脚本
# ------------------------------------------------------------
# 功能：
#   1. 从 build/appicon.png 生成全套 MSIX 图标资产
#   2. 复制 Douya.exe + AppxManifest.xml 到暂存目录
#   3. 将 cuda/vulkan/cpu 三套推理引擎打进包内（商店版开箱即用的关键：
#      安装目录只读，引擎必须随包分发，不能依赖首启下载。
#      N 卡开箱即用原生 CUDA，A/I 卡/无独显回退 Vulkan/CPU）
#   4. 用 makeappx.exe 打包为 .msix（提交商店无需本地签名，商店会重签）
#
# 用法（在任意目录运行）：
#   powershell -ExecutionPolicy Bypass -File .\build\windows\msix\make-msix.ps1
#   可选参数：
#     -ExePath   "D:\...\Douya.exe"    指定已构建的 exe（默认 build\bin\Douya.exe）
#     -OutputDir "D:\...\out"          指定 .msix 输出目录（默认 build\bin）
#     -EngineRoot "D:\...\runtime"     指定内置引擎根目录（默认项目根下 runtime，
#                                      回退 build\runtime；其下需含 cuda\vulkan\cpu 三个子目录）
#     -DevSign                        本地测试签名开关：用开发者证书签名 .msix 并临时把
#                                      清单 Publisher 改为证书主题（CN=Douya Store Dev）。
#                                      仅用于本机安装验证；提交商店时不要加此开关（商店会重签）。
#
# 上架前必做（见 AppxManifest.xml 顶部注释）：
#   1. 替换 Identity 的 Name / Publisher 为合作伙伴中心预留的真实身份
#   2. 每次发版递增 Version 的第四段
# ============================================================

param(
    [string]$ExePath = "",
    [string]$OutputDir = "",
    [string]$EngineRoot = "",
    [switch]$DevSign
)

$ErrorActionPreference = "Stop"

# ---------- 定位项目根目录（脚本位于 build\windows\msix\ 下，向上三级即项目根） ----------
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = (Resolve-Path (Join-Path $ScriptDir "..\..\..")).Path

if (-not $ExePath) { $ExePath = Join-Path $ProjectRoot "build\bin\Douya.exe" }
if (-not (Test-Path $ExePath)) {
    Write-Error "未找到 Douya.exe：$ExePath（请先执行 wails build 构建应用）"
}
if (-not $OutputDir) { $OutputDir = Join-Path $ProjectRoot "build\bin" }
$OutMsix = Join-Path $OutputDir "Douya.msix"

# ---------- 定位内置引擎根目录（含 cuda\vulkan\cpu 三套后端，商店版开箱即用的核心资产） ----------
# 优先级：显式参数 > 项目根 runtime > build\runtime。
# 注意：候选根必须同时含 cuda\vulkan\cpu 三套完整引擎才被接受；
# 项目根 runtime 可能是旧的单引擎构建产物，不能据此选中。
$Backends = @("cuda", "vulkan", "cpu")
if (-not $EngineRoot) {
    $candidates = @(
        (Join-Path $ProjectRoot "runtime"),
        (Join-Path $ProjectRoot "build\runtime")
    )
    foreach ($c in $candidates) {
        $complete = $true
        foreach ($b in $Backends) {
            if (-not (Test-Path (Join-Path $c "$b\llama-server.exe"))) { $complete = $false; break }
        }
        if ($complete) { $EngineRoot = $c; break }
    }
}
if (-not ($EngineRoot -and (Test-Path $EngineRoot))) {
    Write-Error "未找到完整的内置引擎根目录 runtime（需同时含 cuda\vulkan\cpu 三套，各含 llama-server.exe）。`n请确认 build\ 下存在 runtime，或用 -EngineRoot 显式指定。缺少引擎打出的包首启会弹下载框（微软认证 10.1.2.10 失败的根因）。"
}
# 需要随包分发的后端：N 卡原生 CUDA、跨厂商通用 Vulkan、无独显兜底 CPU。
# 与 Go 侧 llm.Backend 枚举及 bundledRuntimeDir() 的 runtime\<backend> 相对布局一一对应。
$EngineDirs = @{}
$engineFileCount = 0
$engineTotalMB = 0.0
foreach ($b in $Backends) {
    $dir = Join-Path $EngineRoot $b
    if (-not (Test-Path $dir)) {
        Write-Error "内置引擎子目录缺失：$dir（商店包必须同时包含 cuda\vulkan\cpu）"
    }
    if (-not (Test-Path (Join-Path $dir "llama-server.exe"))) {
        Write-Error "引擎 $b 不完整：未找到 llama-server.exe（$dir）"
    }
    $EngineDirs[$b] = $dir
    $files = Get-ChildItem -Path $dir -Recurse -File
    $engineFileCount += $files.Count
    $engineTotalMB += [Math]::Round(($files | Measure-Object -Property Length -Sum).Sum / 1MB, 1)
}

Write-Host "=== 豆芽 MSIX 打包 ===" -ForegroundColor Cyan
Write-Host "exe   : $ExePath"
Write-Host "引擎  : $EngineRoot（cuda/vulkan/cpu 共 $engineFileCount 个文件，约 $([Math]::Round($engineTotalMB,1)) MB）"
Write-Host "输出  : $OutMsix"

# ---------- 1. 定位 makeappx.exe（Windows SDK） ----------
$MakeAppx = $null
$kitRoots = @(
    (Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"),
    (Join-Path $env:ProgramFiles "Windows Kits\10\bin")
)
foreach ($root in $kitRoots) {
    if (Test-Path $root) {
        $found = Get-ChildItem -Path $root -Filter "makeappx.exe" -Recurse -ErrorAction SilentlyContinue
        if ($found) {
            # 优先 x64 版本：按字母倒序时 "x86" 会排在 "x64" 之前，导致误选 32 位版
            $MakeAppx = ($found |
                Where-Object { $_.DirectoryName -like "*\x64" } |
                Sort-Object FullName -Descending |
                Select-Object -First 1).FullName
            if (-not $MakeAppx) {
                $MakeAppx = ($found | Sort-Object FullName -Descending | Select-Object -First 1).FullName
            }
            break
        }
    }
}
if (-not $MakeAppx) {
    Write-Error "未找到 makeappx.exe。请安装 Windows SDK（或 Visual Studio 的 [Windows 应用开发] 组件）。"
}
Write-Host "[1/4] makeappx : $MakeAppx"

# ---------- 2. 准备暂存目录 ----------
$Staging = Join-Path $env:TEMP ("douya-msix-" + [guid]::NewGuid().ToString("N"))
$AssetsDir = Join-Path $Staging "Assets"
New-Item -ItemType Directory -Path $AssetsDir -Force | Out-Null

Copy-Item $ExePath (Join-Path $Staging "Douya.exe") -Force
Copy-Item (Join-Path $ScriptDir "AppxManifest.xml") (Join-Path $Staging "AppxManifest.xml") -Force
# 将三套引擎复制进包内（保持 runtime\<backend> 相对布局，与 Go 侧 bundledRuntimeDir() 对应）
foreach ($b in $Backends) {
    $StagingEngine = Join-Path $Staging "runtime\$b"
    # 注意：必须先完整创建目标目录（含叶子目录）。PS 5.1 下若目标不存在，
    # Copy-Item 通配符复制会"只建目录、不进文件"，导致打包出缺引擎的坏包。
    New-Item -ItemType Directory -Path $StagingEngine -Force | Out-Null
    Copy-Item -Path (Join-Path $EngineDirs[$b] "*") -Destination $StagingEngine -Recurse -Force
    # 打包后复核：确认引擎已进入暂存目录，防止源目录为空时静默打出坏包
    if (-not (Test-Path (Join-Path $StagingEngine "llama-server.exe"))) {
        Write-Error "引擎 $b 复制后校验失败：暂存目录中缺少 llama-server.exe，中止打包。"
    }
}
Write-Host "[2/4] 已复制 Douya.exe、AppxManifest.xml 与 runtime\{cuda,vulkan,cpu} 三套引擎"

# ---------- 2.1 版本号单一事实源同步（商店上架一致性关键） ----------
# 从 internal/version/version.go 读取版本并覆写暂存清单的 Version，
# 保证 MSIX 包裹版本 = exe 内嵌版本 = GitHub 发布版本，杜绝手工三处维护漂移。
$StagingManifest = Join-Path $Staging "AppxManifest.xml"
$VersionGoPath = Join-Path $ProjectRoot "internal\version\version.go"
if (-not (Test-Path $VersionGoPath)) { throw "找不到 version.go：$VersionGoPath" }
$VerMatch = [regex]::Match((Get-Content -Raw $VersionGoPath), 'Version\s*=\s*"([^"]+)"')
if (-not $VerMatch.Success) { throw "无法从 version.go 提取 Version 常量" }
$ManifestVer = $VerMatch.Groups[1].Value
# 商店版本要求 a.b.c.d 四段，不足则补 .0
if (($ManifestVer -split '\.').Count -lt 4) { $ManifestVer += ".0" }
$ManifestXml = Get-Content -Raw $StagingManifest
# 关键：必须把替换锚定在 <Identity ...> 元素内部，绝不能全盘替换！
# 若用全局正则 'Version="[0-9.]+"'，会把 XML 声明的 <?xml version="1.0"...?>
# 和 TargetDeviceFamily 的 MinVersion="10.0.17763.0" 一并改坏，
# 导致整个清单不再是 well-formed XML（曾因此触发 makepri PRI191 失败）。
# 这里用 <Identity ...> 标签边界内的 \bVersion= 精确替换，只影响 Identity 元素。
$ManifestXml = [regex]::Replace(
    $ManifestXml,
    '(<Identity\b[^>]*?\b)Version="[0-9.]+"',
    "`${1}Version=`"$ManifestVer`""
)
# 校验用 (?s) 单行模式：<Identity> 的属性跨多行（Name/Publisher/Version 各占一行），
# 普通 .*? 不匹配换行符，会误判为"找不到 Version"。
if ($ManifestXml -notmatch '(?s)Identity.*?Version="[0-9.]+"') {
    throw "版本同步失败：未能在 <Identity> 元素中找到 Version 属性，中止打包以防生成错误清单。"
}
# 关键：必须用带 BOM 的 UTF-8 写回清单！Windows SDK（makepri/makeappx）对无 BOM 的
# UTF-8 会按 ANSI 解码，中文双字节字符可能在标签边界错位，导致 XML 结构破坏、
# makepri 报 PRI191（曾因此反复失败）。MSIX 官方规范要求清单为带 BOM 的 UTF-8。
[System.IO.File]::WriteAllText($StagingManifest, $ManifestXml, (New-Object System.Text.UTF8Encoding($true)))
Write-Host "  清单版本已同步: $ManifestVer（UTF-8 BOM，来自 internal/version/version.go）" -ForegroundColor Green

# ---------- 2.2 本地测试签名：临时把清单 Publisher 改为开发者证书主题 ----------
# MSIX 本地安装要求"清单 Publisher 主题"与"签名证书主题"完全一致（否则 0x8007000B）。
# 仓库中的 AppxManifest.xml 保留商店真实占位身份（CN=0B884B65-...，商店会重签），
# 只有本地测试（-DevSign）才把暂存清单的 Publisher 临时改成开发证书主题 CN=Douya Store Dev，
# 既不改动仓库源文件，又让本机 Add-AppxPackage 能通过校验。
$DevCertThumbprint = "AC14CF9EEF4DBE521F5189F86A8BA5093D180F92"
$DevCertSubject = "CN=Douya Store Dev"
if ($DevSign) {
    $ManifestXml = Get-Content -Raw $StagingManifest
    $NewManifestXml = [regex]::Replace(
        $ManifestXml,
        '(<Identity\b[^>]*?\b)Publisher="[^"]*"',
        "`${1}Publisher=`"$DevCertSubject`""
    )
    if ($NewManifestXml -notmatch "(?s)Identity.*?Publisher=`"CN=Douya Store Dev`"") {
        throw "本地测试签名：无法将清单 Publisher 替换为 $DevCertSubject，中止打包。"
    }
    [System.IO.File]::WriteAllText($StagingManifest, $NewManifestXml, (New-Object System.Text.UTF8Encoding($true)))
    Write-Host "  本地测试签名: 清单 Publisher 已临时改为 $DevCertSubject（提交商店请勿加 -DevSign）" -ForegroundColor Yellow
}

# ---------- 2.5 VC++ 运行库补齐（商店版开箱即用关键） ----------
# llama-server.exe 是 C++ 构建，依赖 vcruntime140.dll / msvcp140.dll 等；
# 目标机器（尤其精简版 Win10）可能没有安装 VC++ Redistributable。
# 打包时把本机可用的 VC 运行库 DLL 复制进引擎目录（与 exe 同级，DLL 搜索优先
# 落地目录，进程即可脱离系统 VC 依赖直接运行），保证商店版免装 VC++ 运行时。
$VcDlls = @("vcruntime140.dll", "vcruntime140_1.dll", "msvcp140.dll", "vcomp140.dll")
$CopiedVc = 0
foreach ($b in $Backends) {
    $StagingEngine = Join-Path $Staging "runtime\$b"
    foreach ($vc in $VcDlls) {
        $VcDst = Join-Path $StagingEngine $vc
        if (Test-Path $VcDst) { continue } # 引擎目录已有，跳过
        $VcCandidates = @(
            (Join-Path $env:WINDIR "System32\$vc"),
            (Join-Path ${env:ProgramFiles(x86)} "Microsoft Visual Studio\2022\*\VC\Redist\MSVC\*\x64\$vc"),
            (Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\Redist\*\ucrt\DLLs\x64\$vc")
        )
        foreach ($vcSrc in $VcCandidates) {
            # 含通配符的路径用 Get-Item 解析（PS 5.1 兼容）
            $VcFound = Get-Item $vcSrc -ErrorAction SilentlyContinue | Select-Object -First 1
            if ($VcFound) {
                Copy-Item $VcFound.FullName $VcDst -Force
                $script:CopiedVc++
                break
            }
        }
    }
}
if ($CopiedVc -gt 0) {
    Write-Host "  已补齐 VC 运行库: $CopiedVc 个（$($VcDlls -join ', ')）" -ForegroundColor Green
} else {
    Write-Warning "本机未找到 VC++ 运行库 DLL，若目标机器缺少 VC++ Redistributable，引擎将无法启动"
}

# ---------- 3. 生成全套 MSIX 图标资产 ----------
Add-Type -AssemblyName System.Drawing

# 素材：build\appicon.png（透明底、去水印后的完整原图构图）。
# 全部尺寸统一使用完整原图，不裁剪特写，保证细节与原图一致。
$IconSource = Join-Path $ProjectRoot "build\appicon.png"
if (-not (Test-Path $IconSource)) {
    Write-Warning "未找到源图标 build\appicon.png，请补齐图标后重试。"
} else {

    # 带板图标：白底圆角卡（OS 不负责圆角，四角 alpha=0 呈现圆角形状），
    # 内容按 -Scale 居中缩放，留出安全边距避免人物顶边被裁
    function New-AppxIcon {
        param([string]$Source, [string]$Dest, [int]$W, [int]$H, [double]$Scale = 0.94)
        $srcImg = [System.Drawing.Image]::FromFile($Source)
        $bmp = New-Object System.Drawing.Bitmap($W, $H)
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
        $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        # 圆角半径：取短边的 ~24%，各尺寸等比，任务栏/开始菜单圆角更明显
        $r = [int]([Math]::Max(2, [Math]::Min($W, $H) * 0.24))
        $d = $r * 2
        $path = New-Object System.Drawing.Drawing2D.GraphicsPath
        $path.AddArc(0, 0, $d, $d, 180, 90)
        $path.AddArc($W - $d - 1, 0, $d, $d, 270, 90)
        $path.AddArc($W - $d - 1, $H - $d - 1, $d, $d, 0, 90)
        $path.AddArc(0, $H - $d - 1, $d, $d, 90, 90)
        $path.CloseFigure()
        # 先全透明，再按圆角路径裁剪绘制：
        # 主体仍是白底实底（不透明），Windows 不会垫默认色兜底，
        # 四角 alpha=0 让图标呈现圆角形状。
        $g.Clear([System.Drawing.Color]::Transparent)
        $g.SetClip($path)
        $g.Clear([System.Drawing.Color]::FromArgb(255, 255, 255, 255))
        # 等比例缩放并居中（Scale<1 时留边距）
        $ratio = [Math]::Min($W / $srcImg.Width, $H / $srcImg.Height) * $Scale
        $newW = [int]($srcImg.Width * $ratio)
        $newH = [int]($srcImg.Height * $ratio)
        $dx = [int](($W - $newW) / 2)
        $dy = [int](($H - $newH) / 2)
        $g.DrawImage($srcImg, $dx, $dy, $newW, $newH)
        $g.ResetClip()
        $path.Dispose()
        $bmp.Save($Dest, [System.Drawing.Imaging.ImageFormat]::Png)
        $g.Dispose(); $bmp.Dispose(); $srcImg.Dispose()
    }

    $iconMap = @(
        @{ Name = "StoreLogo.png";           W = 50;  H = 50 },
        @{ Name = "Square44x44Logo.png";     W = 44;  H = 44 },
        @{ Name = "Square71x71Logo.png";     W = 71;  H = 71 },
        @{ Name = "Square150x150Logo.png";   W = 150; H = 150 },
        @{ Name = "Square310x310Logo.png";   W = 310; H = 310 },
        @{ Name = "Wide310x150Logo.png";     W = 310; H = 150 }
    )
    foreach ($it in $iconMap) {
        # 磁贴 Logo 在白圆角卡上留 8% 边距（Wide 横幅按高度适配同比例）
        New-AppxIcon -Source $IconSource -Dest (Join-Path $AssetsDir $it.Name) -W $it.W -H $it.H -Scale 0.92
    }

    # 任务栏/右键菜单/开始列表用的是"app-list icon"的 target-size 变体，
    # 这套资源支持真透明背景（微软官方支持透明背景）。
    # 生成透明 logo 的 target-size 变体：16/20/24/32/48/64/96/256。
    # 命名规则：Square44x44Logo.targetsize-<N>.png（关联到 VisualElements 的 Square44x44Logo）。
    function New-TransparentLogo {
        param([string]$Source, [string]$Dest, [int]$S, [double]$Scale = 0.96)
        $srcImg = [System.Drawing.Image]::FromFile($Source)
        $bmp = New-Object System.Drawing.Bitmap($S, $S)
        $g = [System.Drawing.Graphics]::FromImage($bmp)
        $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
        $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        # 纯透明背景：只画 logo 本身（无蓝底的关键——存在 altform-unplated 透明变体时，
        # 任务栏/开始列表直接使用透明原图，不会垫主题强调色方块）。
        # Scale 取 0.96：与原图视觉大小一致，仅留极小余量防止相邻图标贴边
        $g.Clear([System.Drawing.Color]::Transparent)
        $ratio = [Math]::Min($S / $srcImg.Width, $S / $srcImg.Height) * $Scale
        $nw = [int]($srcImg.Width * $ratio)
        $nh = [int]($srcImg.Height * $ratio)
        $dx = [int](($S - $nw) / 2)
        $dy = [int](($S - $nh) / 2)
        $g.DrawImage($srcImg, $dx, $dy, $nw, $nh)
        $g.Dispose()
        $bmp.Save($Dest, [System.Drawing.Imaging.ImageFormat]::Png)
        $bmp.Dispose(); $srcImg.Dispose()
    }
    # 每个 targetsize 一对变体：
    #   targetsize-N.png                     白底圆角"带板"候选（轻微留边 0.94）
    #   targetsize-N_altform-unplated.png    纯透明"无板"候选——
    #     有它存在，任务栏/右键菜单等才不会把图标垫进主题强调色方块(蓝底)。
    foreach ($ts in @(16, 20, 24, 32, 48, 64, 96, 256)) {
        New-AppxIcon -Source $IconSource -Dest (Join-Path $AssetsDir "Square44x44Logo.targetsize-$ts.png") -W $ts -H $ts -Scale 0.94
        New-TransparentLogo -Source $IconSource -Dest (Join-Path $AssetsDir "Square44x44Logo.targetsize-${ts}_altform-unplated.png") -S $ts -Scale 0.96
    }
    Write-Host "[3/4] 已生成 6 个磁贴资产 + 8 对 target-size(带板/无板) 变体（完整原图构图）"
}

# ---------- 3.5 用 makepri 生成 resources.pri（MRT 资源索引） ----------
# targetsize / altform-unplated 变体只有进了 PRI 才会被 Windows 资源系统识别，
# 否则任务栏找不到"无板"候选，会把图标垫进主题强调色方块（蓝底）。
$MakePri = Join-Path (Split-Path -Parent $MakeAppx) "makepri.exe"
if (Test-Path $MakePri) {
    # 一次性根目录：只放清单 + Assets，避免 makepri 遍历 runtime\cuda 引擎大文件
    $PriRoot = Join-Path $env:TEMP ("douya-priroot-" + [guid]::NewGuid().ToString("N"))
    New-Item -ItemType Directory -Path $PriRoot -Force | Out-Null
    Copy-Item (Join-Path $Staging "AppxManifest.xml") (Join-Path $PriRoot "AppxManifest.xml") -Force
    Copy-Item $AssetsDir (Join-Path $PriRoot "Assets") -Recurse -Force
    # PRI 输出放 PriRoot 内部（与清单同根）：makepri 要求输出位于项目根下才能
    # 正确关联 index；输出到 Staging 会导致 PRI191 "manifest not found" 误报。
    $PriOut = Join-Path $PriRoot "resources.pri"
    & $MakePri new /pr $PriRoot /cf (Join-Path $ScriptDir "priconfig.xml") /of $PriOut /o /v 2>&1 | Write-Host
    if ($LASTEXITCODE -ne 0) {
        Write-Error "makepri 生成 resources.pri 失败（退出码 $LASTEXITCODE）"
    }
    # 将 PRI 从 PriRoot 移入暂存目录（供后续 makeappx 打包）
    Copy-Item $PriOut (Join-Path $Staging "resources.pri") -Force
    # 防拆分校验：只允许一个 resources.pri，不能出现 resources.scale-*.pri 等资源分包
    $splitPri = Get-ChildItem -Path $Staging -Filter "resources.*.pri" -ErrorAction SilentlyContinue
    if ($splitPri) {
        Write-Error "检测到拆分的资源分包：$($splitPri.Name)。请检查 priconfig.xml 的 <packaging> 是否误留 autoResourcePackage。"
    }
    Write-Host "[3.5] resources.pri 已生成（含 targetsize/unplated 候选索引）"
} else {
    Write-Warning "未找到 makepri.exe，跳过 PRI 生成（任务栏可能仍垫蓝底）"
}

# ---------- 4. 打包 ----------
Write-Host "[4/4] 正在打包 .msix ..."
# 注意：不要用管道接 Out-Host——在部分宿主环境下会阻塞原生 exe 的输出导致假死
& $MakeAppx pack /d $Staging /p $OutMsix /o /h SHA256
$packExit = $LASTEXITCODE

# 本地测试签名：用开发者证书对 MSIX 签名（SHA256 块映射对应 /fd SHA256）。
# 提交商店时无需（商店会重签），因此仅在 -DevSign 时执行。
if (($packExit -eq 0) -and $DevSign -and (Test-Path $OutMsix)) {
    $SignTool = Join-Path (Split-Path -Parent $MakeAppx) "signtool.exe"
    if (-not (Test-Path $SignTool)) { $SignTool = "signtool.exe" }
    Write-Host "  正在用开发者证书签名 ..."
    & $SignTool sign /fd SHA256 /sha1 $DevCertThumbprint $OutMsix 2>&1 | Select-Object -Last 8
    if ($LASTEXITCODE -ne 0) {
        $packExit = $LASTEXITCODE
        Write-Warning "开发者证书签名失败（退出码 $LASTEXITCODE），产物未签名，无法本地安装。"
    } else {
        Write-Host "  本地测试签名完成（$DevCertSubject）" -ForegroundColor Green
    }
}

# 清理暂存目录
Remove-Item -Path $Staging -Recurse -Force -ErrorAction SilentlyContinue
if ($PriRoot) { Remove-Item -Path $PriRoot -Recurse -Force -ErrorAction SilentlyContinue }

if ((Test-Path $OutMsix) -and ($packExit -eq 0)) {
    # 重新读取产物大小（覆盖旧包后需刷新缓存信息）
    Remove-Item -Path (Join-Path $env:TEMP "douya-msix-*") -Recurse -Force -ErrorAction SilentlyContinue
    $size = [Math]::Round((Get-Item $OutMsix).Length / 1MB, 2)
    Write-Host ""
    Write-Host "=== 打包完成 ===" -ForegroundColor Green
    Write-Host "产物：$OutMsix（$size MB，已内置 CUDA/Vulkan/CPU 三套引擎）"
    Write-Host ""
    if ($DevSign) {
        Write-Host "已用开发者证书（$DevCertSubject）签名，可直接本机安装：Add-AppxPackage $OutMsix" -ForegroundColor Green
        Write-Host "提醒：此包仅用于本地测试，不可上架。上架请勿加 -DevSign（商店会重签，且需用商店真实 Publisher）。" -ForegroundColor Yellow
    } else {
        Write-Host "提交 Microsoft Store：无需本地签名，商店会在审核通过后自动重签。" -ForegroundColor Yellow
        Write-Host "本地安装测试：需先用开发者证书签名，再运行 Add-AppxPackage 安装。" -ForegroundColor Yellow
    }
    Write-Host ""
    Write-Host "提醒：上架前请确认 AppxManifest.xml 中的 Name/Publisher 已替换为真实身份。" -ForegroundColor Yellow
} else {
    Write-Error "打包失败（makeappx 退出码：$packExit），未生成有效 .msix 文件。"
}