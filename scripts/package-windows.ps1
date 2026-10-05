param([string]$Version = '0.1.0')

$ErrorActionPreference = 'Stop'
if ($Version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+$') { throw 'Версия должна иметь вид 0.1.0' }
& powershell.exe -NoProfile -ExecutionPolicy Bypass -File (Join-Path $PSScriptRoot 'Check-PowerShell51.ps1')
if ($LASTEXITCODE -ne 0) { throw 'Проверка PowerShell 5.1 не пройдена' }
$project = Split-Path $PSScriptRoot -Parent
& (Join-Path $PSScriptRoot 'build.ps1')
if ($LASTEXITCODE -ne 0) { throw 'Сборка не завершилась' }
$stage = Join-Path $project ('.cache\release-' + $Version)
$dist = Join-Path $project 'dist'
[IO.Directory]::CreateDirectory($stage) | Out-Null
[IO.Directory]::CreateDirectory($dist) | Out-Null
$files = @{
    'xmlmerge.exe' = 'bin\xmlmerge.exe'
    'xmlmerge-ui.exe' = 'bin\xmlmerge-ui.exe'
    'xmlmerge-select.exe' = 'bin\xmlmerge-select.exe'
    'rules.xml' = 'rules.xml'
    'merge-policy.xml' = 'merge-policy.xml'
    'Install-XmlMerge.ps1' = 'scripts\Install-XmlMerge.ps1'
    'Configure-GitExtensions.ps1' = 'scripts\Configure-GitExtensions.ps1'
    'INSTALL-RU.md' = 'INSTALL-RU.md'
}
foreach ($name in $files.Keys) {
    Copy-Item -LiteralPath (Join-Path $project $files[$name]) -Destination (Join-Path $stage $name) -Force
}
$sums = foreach ($name in ($files.Keys | Sort-Object)) {
    $hash = (Get-FileHash -LiteralPath (Join-Path $stage $name) -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $name"
}
[IO.File]::WriteAllLines((Join-Path $stage 'SHA256SUMS.txt'), [string[]]$sums, [Text.UTF8Encoding]::new($false))
$archive = Join-Path $dist ("XmlMerge-Windows-x64-$Version.zip")
if (Test-Path -LiteralPath $archive) { Remove-Item -LiteralPath $archive }
Compress-Archive -LiteralPath (Get-ChildItem -LiteralPath $stage -File | ForEach-Object FullName) -DestinationPath $archive
Write-Host "Готов комплект: $archive"
