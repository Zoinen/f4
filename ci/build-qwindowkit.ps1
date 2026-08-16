param(
    [Parameter(Mandatory = $true)]
    [string]$QtRoot,
    [string]$BuildType = "RelWithDebInfo",
    [ValidateSet("shared", "static")]
    [string]$Linkage = "shared"
)

$ErrorActionPreference = "Stop"
if ($PSVersionTable.PSVersion.Major -ge 7) {
    $PSNativeCommandUseErrorActionPreference = $true
}

$RepoRoot = (Resolve-Path "$PSScriptRoot\..").Path
$QwkSource = Join-Path $RepoRoot "build\qwindowkit-src"
$QwkBuild = Join-Path $RepoRoot "build\qwindowkit-build"
$QwkInstall = Join-Path $RepoRoot "build\qwindowkit-install"
$QtVersion = Split-Path (Split-Path $QtRoot -Parent) -Leaf
$QwkCxxFlags = @()

foreach ($includeDir in @(
    (Join-Path $QtRoot "include\QtQml\$QtVersion"),
    (Join-Path $QtRoot "include\QtQml\$QtVersion\QtQml")
)) {
    if (Test-Path $includeDir) {
        $QwkCxxFlags += "/I$includeDir"
    }
}

Remove-Item -Recurse -Force $QwkSource, $QwkBuild, $QwkInstall -ErrorAction SilentlyContinue
git clone --recursive --branch main https://github.com/stdware/qwindowkit.git $QwkSource

cmake -S $QwkSource -B $QwkBuild -G Ninja `
    "-DCMAKE_BUILD_TYPE=$BuildType" `
    -DCMAKE_PREFIX_PATH="$QtRoot" `
    -DCMAKE_CXX_FLAGS="$($QwkCxxFlags -join ' ')" `
    -DCMAKE_INSTALL_PREFIX="$QwkInstall" `
    -DQWINDOWKIT_BUILD_QUICK=TRUE `
    -DQWINDOWKIT_BUILD_WIDGETS=FALSE `
    -DQWINDOWKIT_BUILD_EXAMPLES=FALSE `
    -DQWINDOWKIT_BUILD_DOCUMENTATIONS=FALSE `
    "-DQWINDOWKIT_BUILD_STATIC=$(@{shared='OFF'; static='ON'}[$Linkage])"

cmake --build $QwkBuild --parallel
cmake --install $QwkBuild

$QwkCmakeDir = @(
    (Join-Path $QwkInstall "lib\cmake\QWindowKit"),
    (Join-Path $QwkInstall "lib64\cmake\QWindowKit")
) | Where-Object { Test-Path (Join-Path $_ "QWindowKitConfig.cmake") } | Select-Object -First 1

if (!$QwkCmakeDir) {
    throw "QWindowKit package config was not installed"
}

Select-String -Path "$QwkCmakeDir\*.cmake" -Pattern "QWindowKit::Quick" | Out-Host
