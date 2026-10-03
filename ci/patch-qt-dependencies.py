#!/usr/bin/env python3
"""Apply the small Qt recipe fixes needed by the portable build matrix."""

from __future__ import annotations

import argparse
from pathlib import Path


_ANCHOR = '            self.requires("freetype/[>=2.13 <3]")\n'
_PATCH = '            self.requires("freetype/2.13.2")\n'
_MARKER = 'self.requires("freetype/2.13.2")'
_FFMPEG_OPTION_ANCHOR = '        "with_gstreamer": [True, False],\n'
_FFMPEG_OPTION_PATCH = _FFMPEG_OPTION_ANCHOR + (
    '        "with_ffmpeg": [True, False],\n'
)
_FFMPEG_OPTION_MARKER = '        "with_ffmpeg": [True, False],\n'
_FFMPEG_DEFAULT_ANCHOR = '        "with_gstreamer": False,\n'
_FFMPEG_DEFAULT_PATCH = _FFMPEG_DEFAULT_ANCHOR + (
    '        "with_ffmpeg": False,\n'
)
_FFMPEG_DEFAULT_MARKER = '        "with_ffmpeg": False,\n'
_FFMPEG_REQUIREMENT_ANCHOR = (
    '        if self.options.get_safe("with_gstreamer", False):\n'
)
_FFMPEG_REQUIREMENT_PATCH = (
    '        if self.options.get_safe("with_ffmpeg", False):\n'
    '            self.requires("ffmpeg/7.1.5")\n'
        + _FFMPEG_REQUIREMENT_ANCHOR
)
_FFMPEG_REQUIREMENT_MARKER = 'self.requires("ffmpeg/7.1.5")'
_FFMPEG_GENERATE_ANCHOR = '        tc.variables["FEATURE_pkg_config"] = "ON"\n'
_FFMPEG_GENERATE_PATCH = _FFMPEG_GENERATE_ANCHOR + (
    '        if self.options.get_safe("with_ffmpeg", False):\n'
    '            # Qt Multimedia uses its FindFFmpeg module during Qt\'s own\n'
    '            # configure step. Point it at the Conan package directly;\n'
    '            # relying on a system pkg-config database would silently\n'
    '            # disable the backend on clean CI runners.\n'
    '            tc.variables["INPUT_ffmpeg"] = "yes"\n'
    '            tc.cache_variables["FFMPEG_DIR"] = (\n'
    '                self.dependencies["ffmpeg"].package_folder\n'
    '            )\n'
)
_FFMPEG_GENERATE_MARKER = 'tc.cache_variables["FFMPEG_DIR"]'
_FFMPEG_SOURCE_ANCHOR = '        apply_conandata_patches(self)\n'
_FFMPEG_SOURCE_PATCH = _FFMPEG_SOURCE_ANCHOR + (
    '        # Conan deliberately forbids self.options access in source().\n'
    '        # This source-level relaxation is harmless when with_ffmpeg is\n'
    '        # false: Qt still controls whether the backend is configured from\n'
    '        # the option, while the patched recipe can safely be exported for\n'
    '        # every platform and package configuration.\n'
    '        # Qt normally restricts FFmpeg on Linux to builds that also have\n'
    '        # PulseAudio or PipeWire. f4 uses FFmpeg for local video\n'
    '        # decoding/thumbnails and intentionally has neither runtime audio\n'
    '        # dependency, so make the FFmpeg backend a valid Linux Multimedia\n'
    '        # backend in this patched static recipe.\n'
    '        qtmultimedia_configure = os.path.join(\n'
    '            self.source_folder, "qtmultimedia", "src", "multimedia",\n'
    '            "configure.cmake"\n'
    '        )\n'
    '        replace_in_file(\n'
    '            self,\n'
    '            qtmultimedia_configure,\n'
    '            "AND (APPLE OR WIN32 OR ANDROID OR QNX OR "\n'
    '            "QT_FEATURE_pulseaudio OR QT_FEATURE_pipewire)",\n'
    '            "AND (APPLE OR WIN32 OR ANDROID OR QNX OR LINUX OR "\n'
    '            "QT_FEATURE_pulseaudio OR QT_FEATURE_pipewire)",\n'
    '            strict=True,\n'
    '        )\n'
)
_FFMPEG_SOURCE_MARKER = 'qtmultimedia_configure = os.path.join'
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
    '            tc.cache_variables["Qt6QuickTools_DIR"] = os.path.join(\n'
    '                self.dependencies.direct_build["qt"].package_folder, "lib", "cmake", "Qt6QuickTools"\n'
    '            )\n'
    '            native_qt_package = self.dependencies.direct_build["qt"].package_folder\n'
    '            native_qsb_name = "qsb.exe" if str(self.settings_build.os) == "Windows" else "qsb"\n'
    '            native_qsb = os.path.join(native_qt_package, "bin", native_qsb_name)\n'
    '            native_svgtoqml_name = "svgtoqml.exe" if str(self.settings_build.os) == "Windows" else "svgtoqml"\n'
    '            native_svgtoqml = os.path.join(native_qt_package, "bin", native_svgtoqml_name)\n'
    '            native_qsb_config = os.path.join(\n'
    '                native_qt_package, "lib", "cmake", "Qt6ShaderToolsTools",\n'
    '                "Qt6ShaderToolsToolsConfig.cmake"\n'
    '            )\n'
    '            native_quick_tools_config = os.path.join(\n'
    '                native_qt_package, "lib", "cmake", "Qt6QuickTools",\n'
    '                "Qt6QuickToolsConfig.cmake"\n'
    '            )\n'
    '            if not all(os.path.isfile(path) for path in (\n'
    '                native_qsb, native_svgtoqml, native_qsb_config, native_quick_tools_config\n'
    '            )):\n'
    '                raise ConanInvalidConfiguration(\n'
    '                    "Qt cross-build requires native qsb/svgtoqml and Qt6QuickTools packages: "\n'
    '                    + native_qsb + "; " + native_svgtoqml + "; " + native_quick_tools_config\n'
    '                )\n'
)
_HOST_PATH_MARKER = 'tc.cache_variables["QT_HOST_PATH_CMAKE_DIR"]'
_ADDITIONAL_HOST_PATH_MARKER = 'tc.cache_variables["QT_ADDITIONAL_PACKAGES_PREFIX_PATH"]'
_QUICK_TOOLS_DIR_MARKER = 'tc.cache_variables["Qt6QuickTools_DIR"]'
_NATIVE_QUICK_TOOLS_CONFIG_MARKER = 'native_quick_tools_config = os.path.join'
_NATIVE_SVGTOQML_MARKER = 'native_svgtoqml_name = "svgtoqml.exe"'
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


def _patch_ffmpeg(text: str) -> str:
    if _FFMPEG_OPTION_MARKER not in text:
        if text.count(_FFMPEG_OPTION_ANCHOR) != 1:
            raise SystemExit(
                "unexpected Qt recipe: FFmpeg option anchor is absent or ambiguous"
            )
        text = text.replace(_FFMPEG_OPTION_ANCHOR, _FFMPEG_OPTION_PATCH)
    if _FFMPEG_DEFAULT_MARKER not in text:
        if text.count(_FFMPEG_DEFAULT_ANCHOR) != 1:
            raise SystemExit(
                "unexpected Qt recipe: FFmpeg default anchor is absent or ambiguous"
            )
        text = text.replace(_FFMPEG_DEFAULT_ANCHOR, _FFMPEG_DEFAULT_PATCH)
    if _FFMPEG_REQUIREMENT_MARKER not in text:
        if text.count(_FFMPEG_REQUIREMENT_ANCHOR) != 1:
            raise SystemExit(
                "unexpected Qt recipe: FFmpeg requirement anchor is absent or ambiguous"
            )
        text = text.replace(_FFMPEG_REQUIREMENT_ANCHOR, _FFMPEG_REQUIREMENT_PATCH)
    if _FFMPEG_GENERATE_MARKER not in text:
        if text.count(_FFMPEG_GENERATE_ANCHOR) != 1:
            raise SystemExit(
                "unexpected Qt recipe: FFmpeg CMake anchor is absent or ambiguous"
            )
        text = text.replace(_FFMPEG_GENERATE_ANCHOR, _FFMPEG_GENERATE_PATCH)
    if _FFMPEG_SOURCE_MARKER not in text:
        if text.count(_FFMPEG_SOURCE_ANCHOR) != 1:
            raise SystemExit(
                "unexpected Qt recipe: Qt source patch anchor is absent or ambiguous"
            )
        text = text.replace(_FFMPEG_SOURCE_ANCHOR, _FFMPEG_SOURCE_PATCH)
    return text


def _patch_host_path(text: str) -> str:
    if _HOST_PATH_MARKER in text:
        if (
            _ADDITIONAL_HOST_PATH_MARKER in text
            and _QUICK_TOOLS_DIR_MARKER in text
            and _NATIVE_QUICK_TOOLS_CONFIG_MARKER in text
            and _NATIVE_SVGTOQML_MARKER in text
        ):
            return text
        raise SystemExit(
            "unexpected Qt recipe: native Qt tool paths are incomplete"
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
    patched = _patch_quick_package_guard(
        _patch_host_path(_patch_ffmpeg(_patch_freetype(text)))
    )
    if patched != text:
        args.recipe.write_text(patched, encoding="utf-8")


if __name__ == "__main__":
    main()
