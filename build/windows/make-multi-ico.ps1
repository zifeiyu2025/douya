# 从透明底 appicon.png 生成多尺寸真透明 icon.ico
# 任务栏高 DPI 需要 256 等大尺寸；只有真透明通道的系统图标才能像 Edge 一样透出背景
Add-Type -AssemblyName System.Drawing

$src = [System.Drawing.Image]::FromFile("D:\MyGoWorkspace\douya\build\appicon.png")
$sizes = @(16, 24, 32, 48, 64, 128, 256)

# 生成每尺寸的 PNG 字节流（保留透明通道）
$pngs = @()
foreach ($s in $sizes) {
    $bmp = New-Object System.Drawing.Bitmap($s, $s)
    $g = [System.Drawing.Graphics]::FromImage($bmp)
    $g.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
    $g.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $g.Clear([System.Drawing.Color]::Transparent)
    $ratio = [Math]::Min($s / $src.Width, $s / $src.Height)
    $nw = [int]($src.Width * $ratio)
    $nh = [int]($src.Height * $ratio)
    $dx = [int](($s - $nw) / 2)
    $dy = [int](($s - $nh) / 2)
    $g.DrawImage($src, $dx, $dy, $nw, $nh)
    $g.Dispose()
    $ms = New-Object System.IO.MemoryStream
    $bmp.Save($ms, [System.Drawing.Imaging.ImageFormat]::Png)
    $pngs += ,$ms.ToArray()
    $ms.Dispose()
    $bmp.Dispose()
}

# 写 ICO 容器
$out = New-Object System.IO.MemoryStream
$bw = New-Object System.IO.BinaryWriter($out)
$count = $sizes.Count
$bw.Write([uint16]0)          # reserved
$bw.Write([uint16]1)          # type=icon
$bw.Write([uint16]$count)     # 帧数

$offset = 6 + (16 * $count)   # 第一条 PNG 数据的起始偏移
# 先写目录项（每条固定 16 字节）
$entryQ = [System.IO.MemoryStream]::new()
$eb = [System.IO.BinaryWriter]::new($entryQ)
for ($i = 0; $i -lt $count; $i++) {
    $s = $sizes[$i]; $len = $pngs[$i].Length
    $dim = if ($s -ge 256) { 0 } else { $s }
    $eb.Write([byte]$dim)               # width
    $eb.Write([byte]$dim)               # height
    $eb.Write([byte]0)                  # color count
    $eb.Write([byte]0)                  # reserved
    $eb.Write([uint16]1)                # planes
    $eb.Write([uint16]32)               # bit count
    $eb.Write([uint32]$len)             # 数据字节数
    $eb.Write([uint32]$offset)          # 数据偏移
    $offset += $len
}
$eb.Flush()
$bw.Write($entryQ.ToArray())
# 依次写入各尺寸 PNG 数据
foreach ($d in $pngs) { $bw.Write($d) }
$bw.Flush()
[System.IO.File]::WriteAllBytes("D:\MyGoWorkspace\douya\build\windows\icon.ico", $out.ToArray())
$eb.Dispose(); $entryQ.Dispose(); $bw.Dispose(); $out.Dispose(); $src.Dispose()
Write-Host "generated multi-size icon.ico frames=$count"