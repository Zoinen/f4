param(
    [int]$Runs = 3,
    [int]$WarmupFrames = 120,
    [int]$SampleFrames = 360,
    [string]$PreviousQuickLibrary = '',
    [string]$ResultDirectory = '',
    [switch]$SkipBuild,
    [switch]$MeasureUpdates
)

$ErrorActionPreference = 'Stop'
if ($Runs -lt 1 -or $WarmupFrames -lt 1 -or $SampleFrames -lt 10) {
    throw 'Runs and warmup frames must be positive; sample frames must be at least 10.'
}

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$workRoot = Join-Path $projectRoot '.work'
$performanceRoot = if ($ResultDirectory) {
    [System.IO.Path]::GetFullPath($ResultDirectory)
} else {
    Join-Path $projectRoot 'build/performance'
}
$runRoot = Join-Path $performanceRoot 'runs'
$cmake = 'C:\Dev\Qt6.8.1\Tools\CMake_64\bin\cmake.exe'
$ninja = 'C:\Dev\Python\Scripts\ninja.exe'
$toolchain = 'D:\Code\f4-zoin\qt\host\build-static-final5\conan_toolchain.cmake'
$qtDir = 'D:\Code\f4-zoin\qt\host\build-static-final5'
$patchedQuick = Join-Path $workRoot 'Qt6Quick-pixelgrid.lib'
$vsDevCmd = 'C:\Program Files\Microsoft Visual Studio\2022\Community\Common7\Tools\VsDevCmd.bat'
$variants = @(@{ Name = 'stock'; Library = '' })
if ($PreviousQuickLibrary) {
    $previousLibrary = (Resolve-Path -LiteralPath $PreviousQuickLibrary).Path
    $variants += @{ Name = 'previous'; Library = $previousLibrary }
}
$currentName = if ($PreviousQuickLibrary) { 'optimized' } else { 'patched' }
$variants += @{ Name = $currentName; Library = $patchedQuick }

foreach ($path in @($cmake, $ninja, $toolchain, $vsDevCmd, $patchedQuick)) {
    if (-not (Test-Path -LiteralPath $path)) {
        throw "Required profiling input is missing: $path"
    }
}

New-Item -ItemType Directory -Force -Path $workRoot, $runRoot | Out-Null
$allRuns = [System.Collections.Generic.List[object]]::new()
$benchmarkArguments = @('--benchmark', '--benchmark-warmup', $WarmupFrames,
    '--benchmark-frames', $SampleFrames)
if ($MeasureUpdates) { $benchmarkArguments += '--benchmark-measure-updates' }
$env:QT_QUICK_CONTROLS_STYLE = 'Basic'
Remove-Item Env:QT_SCALE_FACTOR -ErrorAction SilentlyContinue
Remove-Item Env:QSG_RHI_BACKEND -ErrorAction SilentlyContinue
Remove-Item Env:QT_QUICK_BACKEND -ErrorAction SilentlyContinue

foreach ($variant in $variants) {
    $buildDir = Join-Path $projectRoot "build/perf-$($variant.Name)"
    $variant.Executable = Join-Path $buildDir 'qt-pixelgrid-zoin.exe'
    if ($SkipBuild) {
        if (-not (Test-Path -LiteralPath $variant.Executable)) {
            throw "Missing benchmark executable: $($variant.Executable)"
        }
        continue
    }
    $libraryArgument = if ($variant.Library) {
        "-DQT_PIXELGRID_PATCHED_QUICK_LIBRARY:FILEPATH=`"$($variant.Library.Replace('\', '/'))`""
    } else {
        '-DQT_PIXELGRID_PATCHED_QUICK_LIBRARY:FILEPATH='
    }
    $configureCommand = @(
        '@echo off',
        "call `"$vsDevCmd`" -arch=x64",
        'if errorlevel 1 exit /b 1',
        "`"$cmake`" -S `"$($projectRoot.Replace('\', '/'))`" -B `"$($buildDir.Replace('\', '/'))`" -G Ninja -DCMAKE_MAKE_PROGRAM=`"$($ninja.Replace('\', '/'))`" -DCMAKE_TOOLCHAIN_FILE=`"$($toolchain.Replace('\', '/'))`" -DQt6_DIR=`"$($qtDir.Replace('\', '/'))`" -DCMAKE_BUILD_TYPE=Release -DCMAKE_CXX_FLAGS=-DQT_QML_DEBUG $libraryArgument"
    ) -join "`r`n"
    $buildCommand = @(
        '@echo off',
        "call `"$vsDevCmd`" -arch=x64",
        'if errorlevel 1 exit /b 1',
        "`"$cmake`" --build `"$($buildDir.Replace('\', '/'))`" --config Release --parallel 3"
    ) -join "`r`n"
    $configureFile = Join-Path $workRoot "configure-perf-$($variant.Name).cmd"
    $buildFile = Join-Path $workRoot "build-perf-$($variant.Name).cmd"
    [System.IO.File]::WriteAllText($configureFile, $configureCommand, [System.Text.Encoding]::ASCII)
    [System.IO.File]::WriteAllText($buildFile, $buildCommand, [System.Text.Encoding]::ASCII)

    Write-Output "Configuring $($variant.Name) profile build: $buildDir"
    & cmd.exe /c $configureFile
    if ($LASTEXITCODE -ne 0) { throw "CMake configuration failed for $($variant.Name)." }
    & cmd.exe /c $buildFile
    if ($LASTEXITCODE -ne 0) { throw "Build failed for $($variant.Name)." }

    $executable = Join-Path $buildDir 'qt-pixelgrid-zoin.exe'
    if (-not (Test-Path -LiteralPath $executable)) {
        throw "Benchmark executable was not produced: $executable"
    }
}

foreach ($variant in $variants) {
    $variant.ExecutableSha256 = (Get-FileHash -LiteralPath $variant.Executable -Algorithm SHA256).Hash
}

# Rotate which variant runs first to reduce ordering/temperature bias. All
# variants use the same application sources and scene; only Qt Quick differs.
for ($run = 1; $run -le $Runs; ++$run) {
    for ($offset = 0; $offset -lt $variants.Count; ++$offset) {
        $variant = $variants[($run - 1 + $offset) % $variants.Count]
        $executable = $variant.Executable
        $logPath = Join-Path $runRoot "$($variant.Name)-run-$run.log"
        Write-Output "Running $($variant.Name) benchmark $run/$Runs"
        $started = [DateTime]::UtcNow.ToString('o')
        $output = & $executable @benchmarkArguments 2>&1
        $exitCode = $LASTEXITCODE
        $output | Tee-Object -FilePath $logPath
        if ($exitCode -ne 0) {
            throw "$($variant.Name) benchmark $run failed with exit code $exitCode."
        }
        $summaryLine = $output | ForEach-Object { [string]$_ } |
            Where-Object { $_ -match '^BENCHMARK \{.*\}$' } | Select-Object -Last 1
        if (-not $summaryLine) { throw "No benchmark JSON was printed by $($variant.Name) run $run." }
        $summary = $summaryLine.Substring('BENCHMARK '.Length) | ConvertFrom-Json
        if ($variant.Library -and $summary.unsupported_items -ne 0) {
            throw "Unsupported items appeared in $($variant.Name)."
        }
        $summary | Add-Member -NotePropertyName variant -NotePropertyValue $variant.Name
        $summary | Add-Member -NotePropertyName run -NotePropertyValue $run
        $summary | Add-Member -NotePropertyName started_utc -NotePropertyValue $started
        $summary | Add-Member -NotePropertyName executable -NotePropertyValue $executable
        $summary | Add-Member -NotePropertyName executable_sha256 -NotePropertyValue $variant.ExecutableSha256
        $allRuns.Add($summary)
    }
}

$resultPath = Join-Path $performanceRoot 'benchmark-results.json'
$allRuns | ConvertTo-Json -Depth 5 |
    Set-Content -LiteralPath $resultPath -Encoding utf8
Write-Output "Saved benchmark samples: $resultPath"
