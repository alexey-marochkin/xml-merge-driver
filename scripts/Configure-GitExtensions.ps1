param(
    [Parameter(Mandatory=$true)][string]$RepositoryPath,
    [Parameter(Mandatory=$true)][string]$AraxisExe,
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\XmlMerge')
)

$ErrorActionPreference = 'Stop'
$RepositoryPath = [IO.Path]::GetFullPath($RepositoryPath)
$AraxisExe = [IO.Path]::GetFullPath($AraxisExe)
$InstallDir = [IO.Path]::GetFullPath($InstallDir)
if (!(Test-Path -LiteralPath $AraxisExe -PathType Leaf)) { throw "Не найден Araxis: $AraxisExe" }
foreach ($name in @('xmlmerge-select.exe','xmlmerge-ui.exe','rules.xml')) {
    if (!(Test-Path -LiteralPath (Join-Path $InstallDir $name) -PathType Leaf)) { throw "Не найден установленный файл: $name" }
}
& git.exe -C $RepositoryPath rev-parse --show-toplevel | Out-Null
if ($LASTEXITCODE -ne 0) { throw "Не найден репозиторий Git: $RepositoryPath" }
$dir = $InstallDir.Replace('\', '/')
$araxis = $AraxisExe.Replace('\', '/')
$cmd = '"' + $dir + '/xmlmerge-select.exe" --rules "' + $dir + '/rules.xml" --araxis "' + $araxis + '" --base "$BASE" --local "$LOCAL" --remote "$REMOTE" --output "$MERGED"'
if ($PSVersionTable.PSVersion.Major -lt 7) { $cmd = $cmd.Replace('"', '\"') }
& git.exe -C $RepositoryPath config --local mergetool.xmlmerge-select.cmd $cmd
if ($LASTEXITCODE -ne 0) { throw 'Не удалось записать команду mergetool' }
& git.exe -C $RepositoryPath config --local mergetool.xmlmerge-select.trustExitCode false
if ($LASTEXITCODE -ne 0) { throw 'Не удалось записать trustExitCode' }
& git.exe -C $RepositoryPath config --local merge.guitool xmlmerge-select
if ($LASTEXITCODE -ne 0) { throw 'Не удалось выбрать merge.guitool' }
Write-Host "Git Extensions настроен для репозитория: $RepositoryPath"
