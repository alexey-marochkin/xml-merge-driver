$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.Drawing
$project = Split-Path $PSScriptRoot -Parent
$art = Join-Path $project 'assets'
New-Item -ItemType Directory -Force $art | Out-Null

# Keep the editable vector and Windows raster icon based on the same geometry.
$svg = @'
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 256 256">
  <rect x="12" y="12" width="232" height="232" rx="52" fill="#173e33" stroke="#37795f" stroke-width="4"/>
  <g fill="none" stroke-linecap="round" stroke-linejoin="round">
    <path d="M78 94 L48 128 L78 162 M178 94 L208 128 L178 162" stroke="#eef7f2" stroke-width="13"/>
    <path d="M102 174 V153 C102 132 128 140 128 116 V74 M154 174 V153 C154 132 128 140 128 116 M114 89 L128 74 L142 89" stroke="#8fe0b3" stroke-width="12"/>
  </g>
  <g fill="#173e33" stroke="#8fe0b3" stroke-width="9"><circle cx="102" cy="178" r="10"/><circle cx="154" cy="178" r="10"/></g>
</svg>
'@
[IO.File]::WriteAllText((Join-Path $art 'xmlmerge.svg'), $svg, [Text.UTF8Encoding]::new($false))
$bitmap = [Drawing.Bitmap]::new(1024,1024)
$g = [Drawing.Graphics]::FromImage($bitmap)
$g.SmoothingMode = [Drawing.Drawing2D.SmoothingMode]::AntiAlias
$g.ScaleTransform(4,4)
$bg = [Drawing.SolidBrush]::new([Drawing.ColorTranslator]::FromHtml('#173e33'))
$border = [Drawing.Pen]::new([Drawing.ColorTranslator]::FromHtml('#37795f'),4)
$white = [Drawing.Pen]::new([Drawing.ColorTranslator]::FromHtml('#eef7f2'),13)
$mint = [Drawing.Pen]::new([Drawing.ColorTranslator]::FromHtml('#8fe0b3'),12)
$ring = [Drawing.Pen]::new($mint.Color,9)
foreach ($pen in @($white,$mint)) { $pen.StartCap='Round'; $pen.EndCap='Round'; $pen.LineJoin='Round' }
$tile = [Drawing.Drawing2D.GraphicsPath]::new()
$tile.AddArc(12,12,104,104,180,90); $tile.AddArc(140,12,104,104,270,90)
$tile.AddArc(140,140,104,104,0,90); $tile.AddArc(12,140,104,104,90,90); $tile.CloseFigure()
$g.FillPath($bg,$tile); $g.DrawPath($border,$tile)
function Draw-Lines($pen, [float[]]$coords) {
    $points = for ($i=0; $i -lt $coords.Length; $i+=2) { [Drawing.PointF]::new($coords[$i],$coords[$i+1]) }
    $g.DrawLines($pen,[Drawing.PointF[]]$points)
}
Draw-Lines $white @(78,94,48,128,78,162)
Draw-Lines $white @(178,94,208,128,178,162)
$branches = [Drawing.Drawing2D.GraphicsPath]::new()
$branches.AddLine(102,174,102,153); $branches.AddBezier(102,153,102,132,128,140,128,116); $branches.AddLine(128,116,128,74)
$branches.StartFigure(); $branches.AddLine(154,174,154,153); $branches.AddBezier(154,153,154,132,128,140,128,116)
$g.DrawPath($mint,$branches)
Draw-Lines $mint @(114,89,128,74,142,89)
foreach ($cx in @(102,154)) { $g.FillEllipse($bg,($cx-10),168,20,20); $g.DrawEllipse($ring,($cx-10),168,20,20) }
$sizes = @(16,24,32,48,64,128,256)
$frames = @()
foreach ($size in $sizes) {
    $small = [Drawing.Bitmap]::new($size,$size)
    $sg = [Drawing.Graphics]::FromImage($small)
    $sg.InterpolationMode = [Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
    $sg.PixelOffsetMode = [Drawing.Drawing2D.PixelOffsetMode]::HighQuality
    $sg.DrawImage($bitmap,0,0,$size,$size)
    $stream = [IO.MemoryStream]::new()
    $small.Save($stream,[Drawing.Imaging.ImageFormat]::Png)
    $frames += ,$stream.ToArray()
    if ($size -eq 256) { $small.Save((Join-Path $art 'xmlmerge.png'),[Drawing.Imaging.ImageFormat]::Png) }
    $stream.Dispose(); $sg.Dispose(); $small.Dispose()
}
$output = [IO.File]::Create((Join-Path $art 'xmlmerge.ico'))
$writer = [IO.BinaryWriter]::new($output)
$writer.Write([uint16]0); $writer.Write([uint16]1); $writer.Write([uint16]$sizes.Count)
$offset = 6 + 16*$sizes.Count
for ($i=0; $i -lt $sizes.Count; $i++) {
    $dimension = $sizes[$i] % 256
    $writer.Write([byte]$dimension); $writer.Write([byte]$dimension); $writer.Write([uint16]0)
    $writer.Write([uint16]1); $writer.Write([uint16]32)
    $writer.Write([uint32]$frames[$i].Length); $writer.Write([uint32]$offset)
    $offset += $frames[$i].Length
}
foreach ($frame in $frames) { $writer.Write([byte[]]$frame) }
$writer.Dispose(); $output.Dispose()
foreach ($item in @($branches,$tile,$border,$white,$mint,$ring,$bg,$g,$bitmap)) { $item.Dispose() }
Copy-Item -LiteralPath (Join-Path $art 'xmlmerge.svg') -Destination (Join-Path $project 'internal/ui/assets/icon.svg')

# Windows SDK resource compiler + .NET COFF converter; no Go runtime dependency.
$rc = Get-ChildItem "${env:ProgramFiles(x86)}/Windows Kits/10/bin/*/x64/rc.exe" | Sort-Object FullName -Descending | Select-Object -First 1
$cvtres = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/cvtres.exe'
if (!$rc -or !(Test-Path -LiteralPath $cvtres)) { throw 'Windows SDK rc.exe and .NET cvtres.exe are required to regenerate icon resources.' }
$res = Join-Path $project '.cache/icon.res'
New-Item -ItemType Directory -Force (Split-Path $res) | Out-Null
Push-Location $project
try {
    & $rc.FullName /nologo /fo $res assets/icon.rc
    if ($LASTEXITCODE -ne 0) { throw 'Icon resource compilation failed' }
    foreach ($target in @(@('amd64','X64'),@('386','X86'),@('arm64','ARM64'))) {
        $syso = "cmd/xmlmerge-ui/icon_windows_$($target[0]).syso"
        & $cvtres /NOLOGO /MACHINE:$($target[1]) /OUT:$syso $res
        if ($LASTEXITCODE -ne 0) { throw "Icon COFF conversion failed: $($target[0])" }
        # cvtres emits an absolute compiler-version symbol which Go's internal
        # linker rejects. Mark just that metadata symbol as debug-only, retaining
        # all symbol indexes and resource relocations unchanged.
        $coff = [IO.File]::ReadAllBytes((Join-Path $project $syso))
        $symbols = [BitConverter]::ToUInt32($coff,8)
        $count = [BitConverter]::ToUInt32($coff,12)
        for ($i=0; $i -lt $count;) {
            $position = $symbols + 18*$i
            if ([Text.Encoding]::ASCII.GetString($coff,$position,8) -eq '@comp.id' -and [BitConverter]::ToInt16($coff,($position+12)) -eq -1) {
                $coff[$position+12] = 254; $coff[$position+13] = 255
            }
            $i += 1 + $coff[$position+17]
        }
        [IO.File]::WriteAllBytes((Join-Path $project $syso),$coff)
        Copy-Item -LiteralPath $syso -Destination "cmd/xmlmerge/icon_windows_$($target[0]).syso"
    }
} finally { Pop-Location }
