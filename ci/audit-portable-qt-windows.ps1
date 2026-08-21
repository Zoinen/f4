param(
    [Parameter(Mandatory = $true)]
    [string]$HostPath
)

$ErrorActionPreference = "Stop"
if (!(Test-Path -LiteralPath $HostPath -PathType Leaf)) {
    throw "Qt host executable is missing: $HostPath"
}

$imports = (& dumpbin.exe /DEPENDENTS $HostPath | Out-String)
$forbidden = '(?im)^\s*(Qt[56]|QWindowKit|ZoinGallery|lib(raw|tiff|png|jpeg|turbojpeg|heif|webp|de265|jbig|jasper|zstd|lzma)|MSVCP|VCRUNTIME|ucrtbase)[^\r\n]*\.dll\s*$'
if ($imports -match $forbidden) {
    $imports | Write-Host
    throw "Application-owned DLL or dynamic MSVC runtime remains in portable Qt host"
}
$importNames = @(
    [regex]::Matches($imports, '(?im)^\s*([A-Za-z0-9_.-]+\.dll)\s*$') |
        ForEach-Object { $_.Groups[1].Value } |
        Sort-Object -Unique
)
if ($importNames.Count -eq 0) {
    throw "Could not read the Qt host's DLL import table"
}
$nonSystemImports = @(
    $importNames | Where-Object {
        # API-set contracts are resolved by the Windows loader and are not
        # necessarily materialized as individual System32 files.
        $_ -notmatch '^(api-ms-win|ext-ms-win)-' -and
        !(Test-Path -LiteralPath (Join-Path $env:SystemRoot "System32/$_") -PathType Leaf)
    }
)
if ($nonSystemImports.Count -ne 0) {
    $imports | Write-Host
    throw "Portable Qt host imports DLL(s) outside the Windows system ABI: $($nonSystemImports -join ', ')"
}
$imports | Write-Host
