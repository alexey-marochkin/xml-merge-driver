param(
    [string]$Destination = (Join-Path $env:LOCALAPPDATA 'Programs\XmlMerge'),
    [string]$GitConfigFile = ''
)

$ErrorActionPreference = 'Stop'
$source = $PSScriptRoot
$required = @('xmlmerge.exe', 'xmlmerge-ui.exe', 'xmlmerge-select.exe', 'rules.xml', 'merge-policy.xml')
foreach ($name in $required) {
    if (!(Test-Path -LiteralPath (Join-Path $source $name) -PathType Leaf)) { throw "Не найден файл комплекта: $name" }
}
$Destination = [IO.Path]::GetFullPath($Destination)
[IO.Directory]::CreateDirectory($Destination) | Out-Null
foreach ($name in $required) {
    $target = Join-Path $Destination $name
    if ($name.EndsWith('.xml') -and (Test-Path -LiteralPath $target)) {
        Write-Host "Сохранены действующие настройки: $target"
        continue
    }
    if ([IO.Path]::GetFullPath((Join-Path $source $name)) -ne [IO.Path]::GetFullPath($target)) {
        Copy-Item -LiteralPath (Join-Path $source $name) -Destination $target -Force
    }
}

$gitArgs = @('--global')
if ($GitConfigFile -ne '') {
    $GitConfigFile = [IO.Path]::GetFullPath($GitConfigFile)
    $gitArgs = @('--file', $GitConfigFile)
}
function GitConfig([string]$key, [string]$value) {
    $result = & git.exe config @gitArgs $key $value
    if ($LASTEXITCODE -ne 0) { throw "Ошибка настройки Git: $key" }
    return $result
}
$previous = & git.exe config @gitArgs --get merge.xmlmerge.driver
$dir = $Destination.Replace('\', '/')
$driver = '"' + $dir + '/xmlmerge.exe" git-driver --base "%O" --local "%A" --remote "%B" --path %P --remote-label %Y --rules "' + $dir + '/rules.xml" --policy "' + $dir + '/merge-policy.xml"'
if ($LASTEXITCODE -eq 0 -and $previous -and $previous -ne $driver -and !(Test-Path -LiteralPath (Join-Path $Destination 'git-driver.previous.txt'))) {
    [IO.File]::WriteAllText((Join-Path $Destination 'git-driver.previous.txt'), [string]$previous, [Text.UTF8Encoding]::new($false))
}
GitConfig 'merge.xmlmerge.name' 'Structural XML merge'
GitConfig 'merge.xmlmerge.driver' $driver
GitConfig 'merge.xmlmerge.recursive' 'binary'
Write-Host "XML Merge установлен: $Destination"
Write-Host 'Git-драйвер xmlmerge настроен для текущего пользователя.'
