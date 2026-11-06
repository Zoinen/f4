param(
    [switch]$ReuseObjects,
    [string]$OutputLibrary = 'Qt6Quick-pixelgrid.lib'
)

$ErrorActionPreference = 'Stop'

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$workRoot = Join-Path $projectRoot '.work'
$archiveWork = Join-Path $workRoot 'archive-work'
$objectRoot = Join-Path $archiveWork 'qtdeclarative\src\quick\CMakeFiles\Quick.dir'
$qtSourceRoot = Join-Path $workRoot 'qt-everywhere-src-6.11.1\qtdeclarative'
$qtBaseSourceRoot = Join-Path $workRoot 'qt-everywhere-src-6.11.1\qtbase'
$qtBuildRoot = 'C:\Users\xs\.conan2\p\b\qt647a04cbff674\b\build\Release'
$qtOriginalQtBaseSourceRoot = 'C:\Users\xs\.conan2\p\qtb1af5f4a7d8c8\s\src\qtbase'
$qtOriginalSourceRoot = 'C:\Users\xs\.conan2\p\qtb1af5f4a7d8c8\s\src\qtdeclarative'
$originalLibrary = 'C:\Users\xs\.conan2\p\b\qt5cd48f2ea3d24\p\lib\Qt6Quick.lib'
$compilerTools = 'C:\Program Files\Microsoft Visual Studio\2022\Community\VC\Tools\MSVC\14.44.35207\bin\Hostx64\x64'
$libTool = Join-Path $compilerTools 'lib.exe'
$vsDevCmd = 'C:\Program Files\Microsoft Visual Studio\2022\Community\Common7\Tools\VsDevCmd.bat'
$ninja = 'C:\Dev\Python\Scripts\ninja.exe'
$files = @(
    'items\qquickitem.cpp',
    'items\qquickwindow.cpp',
    'items\qquickrectangle.cpp',
    'items\qquickimage.cpp',
    'items\qquicktext.cpp',
    'items\qquicktextinput.cpp',
    'items\qquicktextedit.cpp',
    'items\qquickshadereffect.cpp',
    'scenegraph\adaptations\software\qsgsoftwarerenderablenode.cpp',
    'scenegraph\adaptations\software\qsgsoftwareinternalrectanglenode.cpp'
)
$memberPrefix = 'qtdeclarative\src\quick\CMakeFiles\Quick.dir\'

if ([IO.Path]::GetFileName($OutputLibrary) -ne $OutputLibrary -or
    [IO.Path]::GetExtension($OutputLibrary) -ne '.lib') {
    throw 'OutputLibrary must be a .lib filename inside the experiment .work directory.'
}

foreach ($requiredPath in @($qtBaseSourceRoot, $qtSourceRoot, $qtBuildRoot, $originalLibrary, $libTool, $ninja)) {
    if (-not (Test-Path -LiteralPath $requiredPath)) {
        throw "Required Qt build input is missing: $requiredPath"
    }
}

New-Item -ItemType Directory -Force -Path $objectRoot | Out-Null
$commands = & $ninja -C $qtBuildRoot -t commands
if ($LASTEXITCODE -ne 0) {
    throw 'Could not query the retained Qt 6.11.1 build commands.'
}

# Conan's generated public-header forwarding stubs contain absolute paths into
# its missing recipe source checkout. Copy those stubs into a task-local include
# overlay and redirect them to the verified source archive without touching the
# Conan cache.
$overlayRoot = Join-Path $workRoot 'qt-header-overlay-v2'
$oldQtBaseForwardSlash = $qtOriginalQtBaseSourceRoot.Replace('\', '/')
$newQtBaseForwardSlash = $qtBaseSourceRoot.Replace('\', '/')
$oldSourceForwardSlash = $qtOriginalSourceRoot.Replace('\', '/')
$newSourceForwardSlash = $qtSourceRoot.Replace('\', '/')
$includeRoots = [System.Collections.Generic.List[string]]::new()
foreach ($line in $commands) {
    foreach ($match in [Regex]::Matches($line, '(?:^|\s)-I([^\s]+)')) {
        $includePath = $match.Groups[1].Value.Trim('"')
        if ($includePath.StartsWith((Join-Path $qtBuildRoot 'qtbase\include'), [StringComparison]::OrdinalIgnoreCase) -and
            (Test-Path -LiteralPath $includePath -PathType Container) -and
            -not $includeRoots.Contains($includePath)) {
            $includeRoots.Add($includePath)
        }
    }
}
$overlayFlags = @("-I`"$overlayRoot`"")
$encoding = [System.Text.UTF8Encoding]::new($false)
$includeBase = Join-Path $qtBuildRoot 'qtbase\include'
$redirectingHeaders = & rg -l --glob '*.h' --glob '*.hpp' -F $oldSourceForwardSlash $includeBase
if ($LASTEXITCODE -gt 1) {
    throw 'Could not locate generated Qt header forwarding stubs.'
}
$redirectingHeaders += & rg -l --glob '*.h' --glob '*.hpp' -F $oldQtBaseForwardSlash $includeBase
if ($LASTEXITCODE -gt 1) {
    throw 'Could not locate generated QtBase header forwarding stubs.'
}
$redirectingHeaders = @($redirectingHeaders | Sort-Object -Unique)
foreach ($header in $redirectingHeaders) {
    $content = [System.IO.File]::ReadAllText($header)
    $redirected = $content.Replace($oldSourceForwardSlash, $newSourceForwardSlash)
    $redirected = $redirected.Replace($oldQtBaseForwardSlash, $newQtBaseForwardSlash)
    foreach ($includeRoot in $includeRoots) {
        if ($header.StartsWith($includeRoot, [StringComparison]::OrdinalIgnoreCase)) {
            $relativeHeader = $header.Substring($includeRoot.Length).TrimStart('\')
            $destination = Join-Path $overlayRoot $relativeHeader
            New-Item -ItemType Directory -Force -Path (Split-Path -Parent $destination) | Out-Null
            [System.IO.File]::WriteAllText($destination, $redirected, $encoding)
        }
    }
}

# The retained AUTOMOC outputs also contain relative paths back to the absent
# Conan recipe checkout. Reuse their generated Qt 6.11.1 code after redirecting
# only those source-header includes into the verified local source tree.
$mocIncludeRoot = Join-Path $qtBuildRoot 'qtdeclarative\src\quick\Quick_autogen\include'
$mocSourcePattern = '(?m)^#include\s+"[^"]*qtb1af5f4a7d8c8/s/src/qtdeclarative/([^"\r\n]+)"'
foreach ($file in $files) {
    $sourcePath = Join-Path $qtSourceRoot "src\quick\$file"
    $sourceText = [System.IO.File]::ReadAllText($sourcePath)
    foreach ($include in [Regex]::Matches($sourceText, '#include\s*[<"]([^">]+)[">]')) {
        $mocName = $include.Groups[1].Value
        if ($mocName -notmatch '^(moc_.*\.cpp|.*\.moc)$') {
            continue
        }
        $mocPath = Join-Path $mocIncludeRoot $mocName
        if (-not (Test-Path -LiteralPath $mocPath)) {
            throw "Retained Qt AUTOMOC output is missing: $mocPath"
        }
        $mocText = [System.IO.File]::ReadAllText($mocPath)
        $mocText = [Regex]::Replace($mocText, $mocSourcePattern,
            ('#include "' + $newSourceForwardSlash + '/$1"'))
        [System.IO.File]::WriteAllText((Join-Path $overlayRoot $mocName), $mocText, $encoding)
    }
}

$commandLines = [System.Collections.Generic.List[string]]::new()
$commandLines.Add('@echo off')
$commandLines.Add("call `"$vsDevCmd`" -arch=x64")
$commandLines.Add('if errorlevel 1 exit /b 1')
$commandLines.Add("pushd `"$qtBuildRoot`"")

foreach ($file in $files) {
    $objectPath = Join-Path $objectRoot $file.Replace('.cpp', '.cpp.obj')
    New-Item -ItemType Directory -Force -Path (Split-Path -Parent $objectPath) | Out-Null
    if ($ReuseObjects) {
        if (-not (Test-Path -LiteralPath $objectPath)) {
            throw "Cannot reuse a missing Qt object: $objectPath"
        }
        continue
    }

    $escapedName = [Regex]::Escape($file)
    $matches = @($commands | Where-Object {
        $_ -match "/Fo[^\s]*\\$escapedName\.obj(?:\s|$)"
    })
    if ($matches.Count -ne 1) {
        throw "Expected exactly one retained compile command for $file; found $($matches.Count)."
    }

    $command = $matches[0].Replace($qtOriginalSourceRoot, $qtSourceRoot)
    $command = $command.Replace($qtOriginalQtBaseSourceRoot, $qtBaseSourceRoot)
    if (-not $command.Contains($qtSourceRoot)) {
        throw "The Qt compile command for $file did not point at the patched source tree."
    }
    if ($overlayFlags.Count -gt 0) {
        $firstInclude = $command.IndexOf(' -I', [StringComparison]::Ordinal)
        if ($firstInclude -lt 0) {
            throw "The Qt compile command for $file has no include flags for the header overlay."
        }
        $command = $command.Insert($firstInclude + 1, ($overlayFlags -join ' ') + ' ')
    }
    $command = [Regex]::Replace($command, '/Fo[^\s]+', "/Fo`"$objectPath`"")
    $command = [Regex]::Replace($command, '/Fd[^\s]+', "/Fd`"$(Join-Path $archiveWork 'Quick-pixelgrid.pdb')`"")
    $command = [Regex]::Replace($command, '\s/Yu[^\s]+', '')
    $command = [Regex]::Replace($command, '\s/Fp[^\s]+', '')
    $command = [Regex]::Replace($command, '\s/FI[^\s]+', '')
    $command = $command.Replace('/showIncludes', '')
    $commandLines.Add($command)
    $commandLines.Add('if errorlevel 1 exit /b 1')
}

$originalCopy = Join-Path $workRoot 'Qt6Quick-original.lib'
$filteredLibrary = Join-Path $workRoot 'Qt6Quick-filtered.lib'
$patchedLibrary = Join-Path $workRoot $OutputLibrary
Copy-Item -LiteralPath $originalLibrary -Destination $originalCopy -Force
$removeArguments = foreach ($file in $files) { "/REMOVE:$memberPrefix$file.obj" }
$newObjects = foreach ($file in $files) { "$memberPrefix$file.obj" }
$commandLines.Add('popd')
$commandLines.Add("pushd `"$archiveWork`"")
$commandLines.Add("`"$libTool`" /NOLOGO /OUT:`"$filteredLibrary`" $($removeArguments -join ' ') `"$originalCopy`"")
$commandLines.Add('if errorlevel 1 exit /b 1')
$commandLines.Add("`"$libTool`" /NOLOGO /OUT:`"$patchedLibrary`" `"$filteredLibrary`" $($newObjects -join ' ')")
$commandLines.Add('if errorlevel 1 exit /b 1')
$commandLines.Add('popd')

$buildScript = Join-Path $workRoot 'build-qtquick-experiment.cmd'
Set-Content -LiteralPath $buildScript -Value $commandLines -Encoding ASCII
& $env:ComSpec /d /c $buildScript
if ($LASTEXITCODE -ne 0) {
    throw "Patched Qt Quick compile/archive failed with exit code $LASTEXITCODE."
}

$members = & $libTool /LIST $patchedLibrary
if ($LASTEXITCODE -ne 0) {
    throw 'Could not list members in the experiment-only Qt Quick archive.'
}
foreach ($file in $files) {
    $member = "$memberPrefix$file.obj"
    $count = @($members | Where-Object { $_.Trim() -ieq $member }).Count
    if ($count -ne 1) {
        throw "Expected exactly one replacement archive member for $file; found $count."
    }
}

$sha = Get-FileHash -LiteralPath $patchedLibrary -Algorithm SHA256
$provenance = @(
    'Qt Quick experiment archive provenance',
    "Source version: Qt 6.11.1",
    'Source archive SHA256: 252acef8c5ae68074d91cadba2ee4a83465051bbb970dd26e8f0daa0f3904e03',
    "Installed baseline library: $originalLibrary",
    "Baseline library SHA256: $((Get-FileHash -LiteralPath $originalLibrary -Algorithm SHA256).Hash)",
    "Experiment library: $patchedLibrary",
    "Experiment library SHA256: $($sha.Hash)",
    'Compiler: MSVC 19.44.35226 x64, Release, /MT; matching retained Qt build flags and source headers, without its path-sensitive PCH',
    "Replaced archive members: $($files -join ', ')"
)
Set-Content -LiteralPath (Join-Path $projectRoot 'build-provenance.txt') -Value $provenance -Encoding UTF8
Write-Output ($provenance -join [Environment]::NewLine)
