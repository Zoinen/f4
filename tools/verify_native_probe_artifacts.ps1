param(
    [Parameter(Mandatory = $true)] [string[]] $Report
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ok = $true
foreach ($reportPath in $Report) {
    $document = Get-Content -LiteralPath $reportPath -Raw | ConvertFrom-Json
    foreach ($session in $document.sessions) {
        $raw = [Convert]::FromBase64String([string]$session.raw_output)
        $rawPath = Join-Path "$reportPath.sessions" ("{0}x{1}.raw" -f $session.initial_width, $session.initial_height)
        $file = [IO.File]::ReadAllBytes($rawPath)
        $hash = ([Security.Cryptography.SHA256]::Create().ComputeHash($file) | ForEach-Object { $_.ToString('x2') }) -join ''
        $match = [Convert]::ToBase64String($file) -eq [string]$session.raw_output -and $hash -eq [string]$session.raw_sha256
        [pscustomobject]@{ Report = $reportPath; Session = "{0}x{1}" -f $session.initial_width, $session.initial_height; JSONBytes = $raw.Length; FileBytes = $file.Length; JSONCR = ($raw | Where-Object { $_ -eq 13 }).Count; FileCR = ($file | Where-Object { $_ -eq 13 }).Count; SHA256Match = ($hash -eq [string]$session.raw_sha256); ByteExact = $match }
        $ok = $ok -and $match
    }
}
if (-not $ok) { exit 1 }
