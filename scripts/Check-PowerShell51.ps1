$ErrorActionPreference = 'Stop'
$scripts = Get-ChildItem -LiteralPath $PSScriptRoot -Filter '*.ps1' -File
foreach ($script in $scripts) {
    $bytes = [IO.File]::ReadAllBytes($script.FullName)
    $hasNonAscii = @($bytes | Where-Object { $_ -ge 128 }).Count -gt 0
    $hasUtf8Bom = $bytes.Length -ge 3 -and $bytes[0] -eq 239 -and $bytes[1] -eq 187 -and $bytes[2] -eq 191
    if ($hasNonAscii -and -not $hasUtf8Bom) {
        throw "PowerShell 5.1 requires a UTF-8 BOM for non-ASCII script: $($script.Name)"
    }
    $tokens = $null
    $errors = $null
    [void][Management.Automation.Language.Parser]::ParseFile($script.FullName, [ref]$tokens, [ref]$errors)
    if ($errors.Count -gt 0) {
        throw "PowerShell 5.1 syntax error in $($script.Name): $($errors[0].Message)"
    }
}
Write-Host "PowerShell 5.1 scripts verified: $($scripts.Count)"
