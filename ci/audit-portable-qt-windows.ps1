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
$imports | Write-Host
