$ErrorActionPreference = 'Stop'
$project = Split-Path $PSScriptRoot -Parent
Push-Location $project
try {
    $env:GOCACHE = Join-Path $project '.cache\go-build'
    & go build -o bin/xmlmerge.exe ./cmd/xmlmerge
    if ($LASTEXITCODE -ne 0) { throw 'CLI build failed' }
    & go build -ldflags '-H windowsgui' -o bin/xmlmerge-ui.exe ./cmd/xmlmerge-ui
    if ($LASTEXITCODE -ne 0) { throw 'UI build failed' }
    & go build -ldflags '-H windowsgui' -o bin/xmlmerge-select.exe ./cmd/xmlmerge-select
    if ($LASTEXITCODE -ne 0) { throw 'Merge tool dispatcher build failed' }
    if (!(Test-Path -LiteralPath 'bin/merge-policy.xml')) {
        Copy-Item -LiteralPath 'merge-policy.xml' -Destination 'bin/merge-policy.xml'
    }
    if (!(Test-Path -LiteralPath 'bin/rules.xml')) {
        Copy-Item -LiteralPath 'rules.xml' -Destination 'bin/rules.xml'
    }
} finally { Pop-Location }
