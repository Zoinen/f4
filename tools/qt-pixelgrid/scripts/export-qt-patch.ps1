$ErrorActionPreference = 'Stop'

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$workRoot = Join-Path $projectRoot '.work'
$sourceRoot = Join-Path $workRoot 'qt-everywhere-src-6.11.1'
$sourceArchive = Join-Path $workRoot 'qt-everywhere-src-6.11.1.tar.xz'
$expectedSha256 = '252acef8c5ae68074d91cadba2ee4a83465051bbb970dd26e8f0daa0f3904e03'
$relativeQuickPath = 'qtdeclarative/src/quick'
$relativeItemsPath = "$relativeQuickPath/items"
$sourceFiles = @(
    'items/qquickitem.cpp',
    'items/qquickwindow.cpp',
    'items/qquickrectangle.cpp',
    'items/qquickimage.cpp',
    'items/qquicktext.cpp',
    'items/qquicktextinput.cpp',
    'items/qquicktextedit.cpp',
    'items/qquickshadereffect.cpp',
    'scenegraph/adaptations/software/qsgsoftwarerenderablenode.cpp',
    'scenegraph/adaptations/software/qsgsoftwareinternalrectanglenode.cpp',
    'scenegraph/adaptations/software/qsgsoftwareinternalrectanglenode_p.h'
)

if (-not (Test-Path -LiteralPath $sourceArchive)) {
    throw "Verified Qt source archive is missing: $sourceArchive"
}
$actualSha256 = (Get-FileHash -LiteralPath $sourceArchive -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actualSha256 -ne $expectedSha256) {
    throw "Qt source archive SHA256 mismatch: expected $expectedSha256, got $actualSha256"
}

$scratchRoot = Join-Path $workRoot ('patch-export-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $scratchRoot | Out-Null
$members = @($sourceFiles | ForEach-Object {
    "qt-everywhere-src-6.11.1/$relativeQuickPath/$_"
})
& tar.exe -xf $sourceArchive -C $scratchRoot @members
if ($LASTEXITCODE -ne 0) {
    throw 'Could not extract the original Qt Quick translation units from the verified source archive.'
}

$repoRoot = Join-Path $scratchRoot 'qt-everywhere-src-6.11.1'
& git -C $repoRoot init --quiet
if ($LASTEXITCODE -ne 0) { throw 'Could not initialize the temporary patch baseline repository.' }
& git -C $repoRoot config core.autocrlf false
& git -C $repoRoot config user.name 'Qt Pixel Grid Patch Export'
& git -C $repoRoot config user.email 'qt-pixel-grid@example.invalid'

$baselinePaths = @($sourceFiles | ForEach-Object { "$relativeQuickPath/$_" })
& git -C $repoRoot add -- @baselinePaths
if ($LASTEXITCODE -ne 0) { throw 'Could not stage the pristine Qt source baseline.' }
& git -C $repoRoot commit --quiet -m 'Verified Qt 6.11.1 pristine source baseline'
if ($LASTEXITCODE -ne 0) { throw 'Could not commit the temporary source baseline.' }

$currentItemsRoot = Join-Path $sourceRoot $relativeItemsPath
foreach ($file in $sourceFiles) {
    Copy-Item -LiteralPath (Join-Path (Join-Path $sourceRoot $relativeQuickPath) $file) `
        -Destination (Join-Path (Join-Path $repoRoot $relativeQuickPath) $file) -Force
}
$privateHeader = 'qquickpixelgrid_p.h'
Copy-Item -LiteralPath (Join-Path $currentItemsRoot $privateHeader) `
    -Destination (Join-Path (Join-Path $repoRoot $relativeItemsPath) $privateHeader) -Force

$patchPaths = @($baselinePaths + "$relativeItemsPath/$privateHeader")
& git -C $repoRoot add -- @patchPaths
if ($LASTEXITCODE -ne 0) { throw 'Could not stage the Qt pixel-grid patch.' }
$patchLines = & git -C $repoRoot diff --cached --binary HEAD
if ($LASTEXITCODE -ne 0 -or $patchLines.Count -eq 0) {
    throw 'Could not generate a non-empty Qt pixel-grid patch.'
}

$patchPath = Join-Path $projectRoot 'patches/qt-6.11.1-f4-pixel-grid.patch'
$patchText = ($patchLines -join "`n") + "`n"
[System.IO.File]::WriteAllText($patchPath, $patchText, [System.Text.UTF8Encoding]::new($false))
& git -C $repoRoot reset --hard HEAD --quiet
if ($LASTEXITCODE -ne 0) { throw 'Could not reset the temporary repository to its pristine baseline.' }
& git -C $repoRoot apply --check $patchPath
if ($LASTEXITCODE -ne 0) { throw 'The exported patch did not apply cleanly to its verified Qt source baseline.' }
Write-Output "Wrote $patchPath"
Write-Output "Patch baseline source SHA256: $actualSha256"
Write-Output 'Patch application check: passed against pristine Qt 6.11.1 files.'
