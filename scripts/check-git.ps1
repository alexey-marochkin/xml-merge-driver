param([string]$Binary = (Join-Path $PSScriptRoot '..\bin\xmlmerge.exe'))
$ErrorActionPreference = 'Stop'
$Binary = (Resolve-Path -LiteralPath $Binary).Path.Replace('\', '/')
$repo = Join-Path ([System.IO.Path]::GetTempPath()) ('xmlmerge проверка ' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $repo | Out-Null
function Git {
    & git.exe -C $repo @args
    if ($LASTEXITCODE -ne 0) { throw "Git failed: $args" }
}
function Write-Xml([string]$text) {
    [System.IO.File]::WriteAllText((Join-Path $repo 'data.xml'), $text, [System.Text.UTF8Encoding]::new($false))
}
Git init -q --initial-branch=main
Git config user.name 'XML Merge Test'
Git config user.email 'xmlmerge-test@example.invalid'
Git config core.autocrlf false
$rules = (Join-Path $repo 'rules.xml').Replace('\', '/')
$driver = '"' + $Binary + '" merge --base "%O" --local "%A" --remote "%B" --rules "' + $rules + '"'
Git config merge.xmlmerge.driver $driver
[System.IO.File]::WriteAllText((Join-Path $repo '.gitattributes'), '*.xml merge=xmlmerge', [System.Text.UTF8Encoding]::new($false))
Write-Xml '<r a="1" b="1"/>'
Git add .gitattributes data.xml
Git commit -qm base
Git checkout -qb remote
Write-Xml '<r a="1" b="2"/>'
Git commit -qam remote
Git checkout -q main
Write-Xml '<r a="2" b="1"/>'
Git commit -qam local
Git merge --no-edit remote
$actual = [System.IO.File]::ReadAllText((Join-Path $repo 'data.xml'))
if ($actual -ne '<r a="2" b="2"/>') { throw "Unexpected result: $actual" }

Git checkout -qb conflicting
Write-Xml '<r a="remote" b="2"/>'
Git commit -qam conflicting
Git checkout -q main
$expected = '<r a="local" b="2"/>'
Write-Xml $expected
Git commit -qam local-conflict
& git.exe -C $repo merge --no-edit conflicting
if ($LASTEXITCODE -eq 0) { throw 'Expected a merge conflict' }
if ([System.IO.File]::ReadAllText((Join-Path $repo 'data.xml')) -ne $expected) { throw 'Conflict modified local XML' }
$unmerged = & git.exe -C $repo ls-files -u
if (-not $unmerged) { throw 'Git did not retain unmerged stages' }
Write-Output 'PASS: successful merge and conflict preserving local XML, with spaces and Cyrillic in paths.'
Write-Output "Test repository: $repo"
