param(
    [Parameter(Mandatory = $true)] [string] $InputReport,
    [Parameter(Mandatory = $true)] [string] $OutputReport,
    [string] $OutputRawDirectory = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

function Sanitize-Text([string] $Value) {
    if ($null -eq $Value) { return $Value }
    [void]($Value = $Value -replace '(?i)C:\\Users\\[^\\]+\\Documents\\ChatGPT\\f4', '<workspace>')
    [void]($Value = $Value -replace '(?i)C:\\Users\\[^\\]+\\AppData\\Local\\f4\\native-conpty', '<pinned-host-cache>')
    [void]($Value = $Value -replace '(?i)C:\\Users\\[^\\]+\\AppData\\Local\\Temp', '<temp>')
    [void]($Value = $Value -replace '(?i)0x[0-9a-f]+', '0xHANDLE')
    return $Value
}

function Convert-Bytes([string] $Base64) {
    [void]([byte[]]$bytes = [Convert]::FromBase64String($Base64))
    [void]([string]$text = [Text.Encoding]::UTF8.GetString($bytes))
    return [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes((Sanitize-Text $text)))
}

$report = Get-Content -LiteralPath $InputReport -Raw | ConvertFrom-Json

# Keep only non-sensitive terminal metadata, even if a future probe adds more
# environment variables.
$safeEnvironment = [ordered]@{}
foreach ($property in $report.environment.PSObject.Properties) {
    if ($property.Name -in @('TERM', 'TERM_PROGRAM', 'CHCP')) {
        $safeEnvironment[$property.Name] = Sanitize-Text ([string]$property.Value)
    }
}
$report.environment = [pscustomobject]$safeEnvironment

foreach ($session in $report.sessions) {
    $session.raw_output = Convert-Bytes ([string]$session.raw_output)
    $hashBytes = [Convert]::FromBase64String($session.raw_output)
    $session.raw_sha256 = ([Security.Cryptography.SHA256]::Create().ComputeHash($hashBytes) | ForEach-Object { $_.ToString('x2') }) -join ''
	foreach ($event in $session.events) {
		if ($event.PSObject.Properties.Name -contains 'bytes' -and $event.bytes) { $event.bytes = Convert-Bytes ([string]$event.bytes) }
	}
}

function Sanitize-Object($Node) {
    if ($null -eq $Node) { return }
    if ($Node -is [string]) { return }
    if ($Node -is [ValueType]) { return }
    if ($Node -is [System.Collections.IEnumerable] -and $Node -isnot [string]) {
        foreach ($item in $Node) { Sanitize-Object $item }
        return
    }
    foreach ($property in $Node.PSObject.Properties) {
        if ($property.Name -in @('raw_output', 'bytes', 'raw_sha256')) { continue }
        if ($property.Value -is [string]) { $property.Value = Sanitize-Text $property.Value }
        else { Sanitize-Object $property.Value }
    }
}
Sanitize-Object $report

$parent = Split-Path -Parent $OutputReport
New-Item -ItemType Directory -Force -Path $parent | Out-Null
$report | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $OutputReport -Encoding utf8
if ([string]::IsNullOrEmpty($OutputRawDirectory)) { $OutputRawDirectory = "$OutputReport.sessions" }
New-Item -ItemType Directory -Force -Path $OutputRawDirectory | Out-Null
foreach ($session in $report.sessions) {
    $name = "{0}x{1}.raw" -f $session.initial_width, $session.initial_height
    [IO.File]::WriteAllBytes((Join-Path $OutputRawDirectory $name), [Convert]::FromBase64String($session.raw_output))
}
