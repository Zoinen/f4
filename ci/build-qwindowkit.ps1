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
$QwkPatch = Join-Path $RepoRoot "ci\patches\qwindowkit-default-maximize-hint.patch"
$QwkPatchHash = (Get-FileHash $QwkPatch -Algorithm SHA256).Hash.Substring(0, 16).ToLowerInvariant()
$QwkMarker = Join-Path $QwkInstall ".f4-qwindowkit-ready-$Linkage-$BuildType-$QwkPatchHash"
$QtVersion = Split-Path (Split-Path $QtRoot -Parent) -Leaf
$QwkCxxFlags = @()
$QwkPlatformArgs = @(
    "-DQWINDOWKIT_BUILD_STATIC=$(@{shared='OFF'; static='ON'}[$Linkage])"
)
if ($Linkage -eq "static") {
    $QwkPlatformArgs += '-DCMAKE_MSVC_RUNTIME_LIBRARY=MultiThreaded$<$<CONFIG:Debug>:Debug>'
}

$cachedConfig = @(
    (Join-Path $QwkInstall "lib\cmake\QWindowKit\QWindowKitConfig.cmake"),
    (Join-Path $QwkInstall "lib64\cmake\QWindowKit\QWindowKitConfig.cmake")
) | Where-Object { Test-Path $_ } | Select-Object -First 1
if ((Test-Path $QwkMarker) -and $cachedConfig) {
    Write-Host "Reusing cached QWindowKit $Linkage $BuildType install"
    exit 0
}

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

if ($PSVersionTable.PSVersion.Major -ge 7) {
    $PSNativeCommandUseErrorActionPreference = $false
}
& git -C $QwkSource apply --reverse --check $QwkPatch 2>$null
$PatchAlreadyApplied = $LASTEXITCODE -eq 0
if ($PSVersionTable.PSVersion.Major -ge 7) {
    $PSNativeCommandUseErrorActionPreference = $true
}

if ($PatchAlreadyApplied) {
    Write-Host "QWindowKit default maximize-hint fix is already upstream"
} else {
    & git -C $QwkSource apply --check $QwkPatch
    if ($LASTEXITCODE -ne 0) {
        throw "QWindowKit default maximize-hint patch does not apply"
    }
    & git -C $QwkSource apply $QwkPatch
    if ($LASTEXITCODE -ne 0) {
        throw "QWindowKit default maximize-hint patch failed"
    }
}

cmake -S $QwkSource -B $QwkBuild -G Ninja `
    "-DCMAKE_BUILD_TYPE=$BuildType" `
    -DCMAKE_PREFIX_PATH="$QtRoot" `
    -DCMAKE_CXX_FLAGS="$($QwkCxxFlags -join ' ')" `
    -DCMAKE_INSTALL_PREFIX="$QwkInstall" `
    -DQWINDOWKIT_BUILD_QUICK=TRUE `
    -DQWINDOWKIT_BUILD_WIDGETS=FALSE `
    -DQWINDOWKIT_BUILD_EXAMPLES=FALSE `
    -DQWINDOWKIT_BUILD_DOCUMENTATIONS=FALSE `
    $QwkPlatformArgs

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
New-Item -ItemType File -Force $QwkMarker | Out-Null
