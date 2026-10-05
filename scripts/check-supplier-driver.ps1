param([switch]$UseGlobal)
$ErrorActionPreference = 'Stop'
$project = Split-Path $PSScriptRoot -Parent
$binary = (Join-Path $project 'bin/xmlmerge.exe').Replace('\','/')
$repo = Join-Path $project ('.cache/supplier test ' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $repo | Out-Null
function Git {
    $result = & git.exe -C $repo @args
    if ($LASTEXITCODE -ne 0) { throw "Git failed: $args" }
    return $result
}
function Write-Data($name,$text) {
    $path = Join-Path $repo $name
    [IO.Directory]::CreateDirectory((Split-Path $path)) | Out-Null
    [IO.File]::WriteAllText($path,$text,[Text.UTF8Encoding]::new($false))
}
Git init -q -b main
Git config user.name 'XML Merge Test'
Git config user.email 'xmlmerge-test@example.invalid'
Git config commit.gpgsign false
Git config core.autocrlf false
if (!$UseGlobal) {
    $rules = (Join-Path $repo 'rules.xml').Replace('\','/')
    $policy = (Join-Path $project 'bin/merge-policy.xml').Replace('\','/')
    $driver = '"'+$binary+'" git-driver --base "%O" --local "%A" --remote "%B" --path %P --remote-label %Y --rules "'+$rules+'" --policy "'+$policy+'"'
    Git config merge.xmlmerge.driver $driver
    Git config merge.xmlmerge.recursive binary
}
Write-Data '.gitattributes' "*.xml merge=xmlmerge`nrules.xml merge=text`n"
# A quoted Unicode path exercises Git's own escaping of %P.
$form = "каталог с пробелами/it's here/Form.xml"
Write-Data $form '<r a="base"/>'
Write-Data 'data.xml' '<r a="1" b="1"/>'
Git add .
Git commit -qm base
Git checkout -qb "supplier'branch"
Write-Data $form '<r a="supplier"/>'
Write-Data 'data.xml' '<r a="1" b="2"/>'
Git commit -qam supplier
$supplierCommit = Git rev-parse HEAD
# The selected SHA is an older export on the first-parent chain of origin1c.
Git checkout -qb origin1c
Git commit --allow-empty -qm later-supplier
Git checkout -q main
Write-Data $form '<r a="local"/>'
Write-Data 'data.xml' '<r a="2" b="1"/>'
Git commit -qam local
Git merge --no-edit $supplierCommit
if ([IO.File]::ReadAllText((Join-Path $repo $form)) -ne '<r a="supplier"/>') { throw 'Supplier policy did not select remote' }
if ([IO.File]::ReadAllText((Join-Path $repo 'data.xml')) -ne '<r a="2" b="2"/>') { throw 'Independent XML edits did not merge' }
Git checkout -qb conflict
Write-Data 'data.xml' '<r a="remote-conflict" b="2"/>'
Git commit -qam conflict
Git checkout -q main
Write-Data 'data.xml' '<r a="local-conflict" b="2"/>'
Git commit -qam local-conflict
& git.exe -C $repo merge --no-edit conflict
if ($LASTEXITCODE -eq 0) { throw 'Conflict was reported as success' }
if ([IO.File]::ReadAllText((Join-Path $repo 'data.xml')) -ne '<r a="local-conflict" b="2"/>') { throw 'Conflict modified local XML' }
if (!(Git ls-files -u)) { throw 'Git lost conflict stages' }
Write-Output "PASS: older supplier SHA on first-parent history, escaped paths, three-way merge, conflict without rewriting local. Repository: $repo"
