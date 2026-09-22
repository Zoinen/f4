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
$QmsetupHostBuild = Join-Path $RepoRoot "build\qwindowkit-qmsetup-host-build"
$QmsetupHostInstall = Join-Path $RepoRoot "build\qwindowkit-qmsetup-host-install"
$QmsetupHostConfig = Join-Path $QmsetupHostInstall "lib\cmake\qmsetup\qmsetupConfig.cmake"
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

# qt/host/conanfile.py generates this include for cross builds so CMake uses
# native Qt qsb/QML tools instead of same-named target-architecture tools.
# QWindowKit is configured separately from qt/host, so pass the include here
# as well when the Conan-generated file is available.
$QwkCrossTools = Join-Path $QtRoot "f4-qt-cross-tools.cmake"
if (Test-Path -LiteralPath $QwkCrossTools -PathType Leaf) {
    $QwkPlatformArgs += "-DCMAKE_PROJECT_INCLUDE_BEFORE=$QwkCrossTools"
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

# QWindowKit builds qmsetup from its submodule when no host package is
# available. On the GitHub Windows ARM job the outer project is configured
# with the ARM64 MSVC environment, so that fallback produces an ARM64
# qmcorecmd.exe which cannot run on the x64 GitHub runner during configure.
# Build this tiny helper package explicitly for the build machine and point
# QWindowKit at it; the QWindowKit libraries themselves remain ARM64.
if ($env:VSCMD_ARG_TGT_ARCH -eq "arm64") {
    $hostArchitecture = $env:VSCMD_ARG_HOST_ARCH
    if ([string]::IsNullOrWhiteSpace($hostArchitecture)) {
        $hostArchitecture = "x64"
    }

    if (!(Test-Path -LiteralPath $QmsetupHostConfig -PathType Leaf)) {
        Remove-Item -Recurse -Force $QmsetupHostBuild, $QmsetupHostInstall -ErrorAction SilentlyContinue
        $vsDevCmd = Join-Path $env:VSINSTALLDIR "Common7\Tools\VsDevCmd.bat"
        if (!(Test-Path -LiteralPath $vsDevCmd -PathType Leaf)) {
            throw "Visual Studio developer command file is missing: $vsDevCmd"
        }

        $qmsetupSource = Join-Path $QwkSource "qmsetup"
        $hostCommand = @(
            "call `"$vsDevCmd`" -arch=x64 -host_arch=$hostArchitecture",
            "cmake -S `"$qmsetupSource`" -B `"$QmsetupHostBuild`" -G Ninja -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX=`"$QmsetupHostInstall`" -DCMAKE_INSTALL_LIBDIR=lib -DQMSETUP_STATIC_RUNTIME=ON",
            "cmake --build `"$QmsetupHostBuild`" --target install --parallel"
        ) -join " && "

        Write-Host "Building native $hostArchitecture qmsetup tools for ARM64 QWindowKit configure"
        & cmd.exe /d /s /c $hostCommand
        if ($LASTEXITCODE -ne 0) {
            throw "Native qmsetup host-tool build failed"
        }
    } else {
        Write-Host "Reusing native qmsetup host tools"
    }

    if (!(Test-Path -LiteralPath $QmsetupHostConfig -PathType Leaf)) {
        throw "Native qmsetup package config was not installed"
    }
    $QwkPlatformArgs += "-Dqmsetup_DIR=$($QmsetupHostInstall)\lib\cmake\qmsetup"
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
