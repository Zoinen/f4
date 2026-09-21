#!/usr/bin/env python3
"""Apply the small Qt recipe fixes needed by the portable build matrix."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '            self.requires("freetype/[>=2.13 <3]")\n'
_PATCH = '            self.requires("freetype/2.13.2")\n'
_MARKER = 'self.requires("freetype/2.13.2")'
_HOST_PATH_ANCHOR = (
    '            tc.cache_variables["QT_HOST_PATH"] = '
    'self.dependencies.direct_build["qt"].package_folder\n'
)
_HOST_PATH_PATCH = _HOST_PATH_ANCHOR + (
    '            tc.cache_variables["QT_HOST_PATH_CMAKE_DIR"] = os.path.join(\n'
    '                self.dependencies.direct_build["qt"].package_folder, "lib", "cmake"\n'
    '            )\n'
    '            tc.cache_variables["QT_ADDITIONAL_PACKAGES_PREFIX_PATH"] = os.path.join(\n'
    '                self.dependencies.direct_build["qt"].package_folder, "lib", "cmake"\n'
    '            )\n'
    '            native_qt_package = self.dependencies.direct_build["qt"].package_folder\n'
    '            native_qsb_name = "qsb.exe" if str(self.settings_build.os) == "Windows" else "qsb"\n'
    '            native_qsb = os.path.join(native_qt_package, "bin", native_qsb_name)\n'
    '            native_qsb_config = os.path.join(\n'
    '                native_qt_package, "lib", "cmake", "Qt6ShaderToolsTools",\n'
    '                "Qt6ShaderToolsToolsConfig.cmake"\n'
    '            )\n'
    '            if not os.path.isfile(native_qsb) or not os.path.isfile(native_qsb_config):\n'
    '                raise ConanInvalidConfiguration(\n'
    '                    "Qt cross-build requires a complete native qsb tool package: "\n'
    '                    + native_qsb\n'
    '                )\n'
)
_HOST_PATH_MARKER = 'tc.cache_variables["QT_HOST_PATH_CMAKE_DIR"]'
_ADDITIONAL_HOST_PATH_MARKER = 'tc.cache_variables["QT_ADDITIONAL_PACKAGES_PREFIX_PATH"]'
_QUICK_PACKAGE_GUARD_ANCHOR = "        cmake.install()\n"
_QUICK_PACKAGE_GUARD_MARKER = "missing_quick_libraries = []"
_QUICK_PACKAGE_GUARD = '''        if cross_building(self) and self.options.qtdeclarative and self.options.qtshadertools and self.options.gui:
            # A cross-build can otherwise finish with Qt Quick silently
            # disabled when the native qsb tool is not discoverable. Conan's
            # package_info() still advertises these components, so reject the
            # incomplete package before it reaches a cache or binary remote.
            quick_lib_dir = os.path.join(self.package_folder, "lib")
            required_quick_libraries = (
                "Qt6Quick",
                "Qt6QuickControls2",
                "Qt6QuickTemplates2",
            )
            binary_suffixes = (".a", ".dylib", ".dll", ".lib", ".so")
            missing_quick_libraries = []
            for library in required_quick_libraries:
                if not os.path.isdir(quick_lib_dir) or not any(
                    (filename.startswith(library) or filename.startswith(f"lib{library}"))
                    and (filename.endswith(binary_suffixes) or ".so." in filename)
                    for filename in os.listdir(quick_lib_dir)
                ):
                    missing_quick_libraries.append(library)
            if missing_quick_libraries:
                raise ConanInvalidConfiguration(
                    "Qt Quick was not built; missing packaged libraries: "
                    + ", ".join(missing_quick_libraries)
                )
'''


def _patch_freetype(text: str) -> str:
    if _MARKER in text:
        return text
    if text.count(_ANCHOR) != 1:
        raise SystemExit(
            "unexpected Qt recipe: freetype requirement is absent or ambiguous"
        )
    return text.replace(_ANCHOR, _PATCH)


def _patch_host_path(text: str) -> str:
    if _HOST_PATH_MARKER in text:
        if _ADDITIONAL_HOST_PATH_MARKER in text:
            return text
        raise SystemExit(
            "unexpected Qt recipe: host package path is customized without the additional prefix path"
        )
    if text.count(_HOST_PATH_ANCHOR) != 1:
        raise SystemExit(
            "unexpected Qt recipe: cross-build host path is absent or ambiguous"
        )
    return text.replace(_HOST_PATH_ANCHOR, _HOST_PATH_PATCH)


def _patch_quick_package_guard(text: str) -> str:
    if _QUICK_PACKAGE_GUARD_MARKER in text:
        return text
    if text.count(_QUICK_PACKAGE_GUARD_ANCHOR) != 1:
        raise SystemExit(
            "unexpected Qt recipe: package install anchor is absent or ambiguous"
        )
    return text.replace(
        _QUICK_PACKAGE_GUARD_ANCHOR,
        _QUICK_PACKAGE_GUARD_ANCHOR + _QUICK_PACKAGE_GUARD,
    )


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    patched = _patch_quick_package_guard(_patch_host_path(_patch_freetype(text)))
    if patched != text:
        args.recipe.write_text(patched, encoding="utf-8")


if __name__ == "__main__":
    main()
