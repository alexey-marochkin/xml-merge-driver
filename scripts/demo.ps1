# Creates an isolated example requiring a three-attribute manual key.
$ErrorActionPreference = 'Stop'
$project = Split-Path $PSScriptRoot -Parent
$binary = Join-Path $project 'bin\xmlmerge.exe'
if (-not (Test-Path -LiteralPath $binary) -or -not (Test-Path -LiteralPath (Join-Path $project 'bin\xmlmerge-ui.exe'))) {
    throw 'Build both executables first: scripts\build.ps1'
}
$demo = Join-Path $project ('.cache\demo-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $demo | Out-Null
function Document([string]$side) {
    $xml = "<?xml version=`"1.0`" encoding=`"UTF-8`"?>`r`n<Catalog>`r`n"
    for ($i = 0; $i -lt 8; $i++) {
        $label = 'Название'
        if ($side -eq 'local' -and $i -eq 0) { $label = 'Изменено в local' }
        if ($side -eq 'remote' -and $i -eq 1) { $label = 'Изменено в remote' }
        $xml += '  <Item section="' + ($i -band 1) + '" language="' + (($i -shr 1) -band 1) + '" variant="' + (($i -shr 2) -band 1) + '" label="' + $label + '" />' + "`r`n"
    }
    return $xml + "</Catalog>`r`n"
}
foreach ($side in @('base','local','remote')) {
    [IO.File]::WriteAllText((Join-Path $demo ($side + '.xml')), (Document $side), [Text.UTF8Encoding]::new($false))
}
Write-Output "Демонстрация: $demo"
Write-Output 'Выберите атрибуты language, section, variant, проверьте и сохраните правило.'
& $binary compare --rules (Join-Path $demo 'rules.xml') --base (Join-Path $demo 'base.xml') --local (Join-Path $demo 'local.xml') --remote (Join-Path $demo 'remote.xml') --output (Join-Path $demo 'result.xml')
exit $LASTEXITCODE
