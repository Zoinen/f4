#!/usr/bin/env python3
"""Apply the small Qt recipe fixes needed by the portable build matrix."""

from __future__ import annotations

import argparse
from pathlib import Path
import runpy


_ANCHOR = '            self.requires("freetype/[>=2.13 <3]")\n'
_PATCH = '            self.requires("freetype/2.13.2")\n'
_MARKER = 'self.requires("freetype/2.13.2")'
_MD4C_REQUIREMENT_ANCHOR = '        self.requires("md4c/[>=0.4.8 <1]")\n'
_MD4C_REQUIREMENT_PATCH = (
    '        # md4c 0.6.0 changes flags that Qt 6.11.1 checks at compile time.\n'
    '        self.requires("md4c/[>=0.4.8 <0.6]")\n'
)
_MD4C_REQUIREMENT_MARKER = 'self.requires("md4c/[>=0.4.8 <0.6]")'
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
_FFMPEG_PACKAGE_INFO_ANCHOR = (
    '            if self.options.get_safe("with_pulseaudio", False):\n'
    '                multimedia_reqs.append("pulseaudio::pulse")\n'
    '            _create_module("Multimedia", multimedia_reqs)\n'
)
_FFMPEG_PACKAGE_INFO_PATCH = (
    '            if self.options.get_safe("with_pulseaudio", False):\n'
    '                multimedia_reqs.append("pulseaudio::pulse")\n'
    '            if self.options.get_safe("with_ffmpeg", False):\n'
    '                multimedia_reqs.extend([\n'
    '                    "ffmpeg::avcodec",\n'
    '                    "ffmpeg::avformat",\n'
    '                    "ffmpeg::avutil",\n'
    '                    "ffmpeg::swresample",\n'
    '                    "ffmpeg::swscale",\n'
    '                ])\n'
    '            _create_module("Multimedia", multimedia_reqs)\n'
)
_FFMPEG_PACKAGE_INFO_MARKER = '"ffmpeg::avcodec"'
_FFMPEG_SOURCE_ANCHOR = '        apply_conandata_patches(self)\n'
_FFMPEG_LINUX_VIDEO_ONLY_NOTE = '''        # Qt normally restricts FFmpeg on Linux to builds that also have
        # PulseAudio or PipeWire. f4 uses FFmpeg for local video
        # decoding/thumbnails and intentionally has neither runtime audio
        # dependency, so make the FFmpeg backend a valid Linux Multimedia
        # backend in this patched static recipe.
'''
_FFMPEG_LINUX_AUDIO_NOTE = '''        # Keep Qt Multimedia's Linux FFmpeg gate intact. The portable Linux
        # configuration enables PulseAudio below, so Qt only enables FFmpeg
        # when an audio output backend is available too.
'''
_FFMPEG_SOURCE_NEUTRAL_NOTE = '''        # Conan deliberately forbids self.options access in source().
        # These source-level compatibility patches are harmless when
        # with_ffmpeg is false: Qt still controls whether the backend is
        # configured from the option, while the recipe can safely be exported
        # for every platform and package configuration.
'''
_FFMPEG_LINUX_VIDEO_ONLY_RELAXATION = '''        replace_in_file(
            self,
            qtmultimedia_configure,
            "AND (APPLE OR WIN32 OR ANDROID OR QNX OR "
            "QT_FEATURE_pulseaudio OR QT_FEATURE_pipewire)",
            "AND (APPLE OR WIN32 OR ANDROID OR QNX OR LINUX OR "
            "QT_FEATURE_pulseaudio OR QT_FEATURE_pipewire)",
            strict=True,
        )
'''
_FFMPEG_MODULE_PATCH = (
    '        ffmpeg_find_module = os.path.join(\n'
    '            self.source_folder, "qtmultimedia", "cmake", "FindFFmpeg.cmake"\n'
    '        )\n'
    '        replace_in_file(\n'
    '            self,\n'
    '            ffmpeg_find_module,\n'
    '            "include(FindPackageHandleStandardArgs)\\n",\n'
    '            "include(FindPackageHandleStandardArgs)\\n"\n'
    '            "\\n"\n'
    '            "# Conan CMakeDeps exports lower-case ffmpeg:: component targets.\\n"\n'
    '            "find_package(ffmpeg CONFIG QUIET)\\n",\n'
    '            strict=True,\n'
    '        )\n'
    '        replace_in_file(\n'
    '            self,\n'
    '            ffmpeg_find_module,\n'
    '            \'            target_link_libraries(${_target} INTERFACE "${${_component}_LIBRARY_NAME}")\\n\'\n'
    '            \'            target_link_directories(${_target} INTERFACE ${${_component}_LIBRARY_DIR})\\n\'\n'
    '            \'\\n\'\n'
    '            \'            __ffmpeg_internal_set_dependencies(${_component})\\n\',\n'
    '            \'            if (TARGET ffmpeg::${_lowerComponent})\\n\'\n'
    '            \'                # Conan carries the static codec, framework, and system-library dependencies.\\n\'\n'
    '            \'                target_link_libraries(${_target} INTERFACE ffmpeg::${_lowerComponent})\\n\'\n'
    '            \'            else()\\n\'\n'
    '            \'                target_link_libraries(${_target} INTERFACE "${${_component}_LIBRARY_NAME}")\\n\'\n'
    '            \'                __ffmpeg_internal_set_dependencies(${_component})\\n\'\n'
    '            \'            endif()\\n\'\n'
    '            \'            target_link_directories(${_target} INTERFACE ${${_component}_LIBRARY_DIR})\\n\',\n'
    '            strict=True,\n'
    '        )\n'
)
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
    '        # Conan CMakeDeps also emits ffmpeg-config.cmake and\n'
    '        # vaapi-config.cmake. Those packages use Conan target names\n'
    '        # (ffmpeg::avformat, vaapi::vaapi), while Qt Multimedia\n'
    '        # expects the targets created by its own Find modules\n'
    '        # (FFmpeg::avformat, VAAPI::VAAPI). Force module mode for\n'
    '        # these two lookups so a config package cannot shadow the\n'
    '        # compatible Qt finders.\n'
    '        replace_in_file(\n'
    '            self,\n'
    '            qtmultimedia_configure,\n'
    '            "qt_find_package(FFmpeg OPTIONAL_COMPONENTS",\n'
    '            "qt_find_package(FFmpeg MODULE OPTIONAL_COMPONENTS",\n'
    '            strict=True,\n'
    '        )\n'
    '        replace_in_file(\n'
    '            self,\n'
    '            qtmultimedia_configure,\n'
    '            "qt_find_package(VAAPI COMPONENTS",\n'
    '            "qt_find_package(VAAPI MODULE COMPONENTS",\n'
    '            strict=True,\n'
    '        )\n'
    '        ffmpeg_plugin_cmake = os.path.join(\n'
    '            self.source_folder, "qtmultimedia", "src", "plugins",\n'
    '            "multimedia", "ffmpeg", "CMakeLists.txt"\n'
    '        )\n'
    '        replace_in_file(\n'
    '            self,\n'
    '            ffmpeg_plugin_cmake,\n'
    '            "qt_find_package(VAAPI COMPONENTS",\n'
    '            "qt_find_package(VAAPI MODULE COMPONENTS",\n'
    '            strict=True,\n'
    '        )\n'
) + _FFMPEG_MODULE_PATCH + (
    '        # The glibc 2.27 baseline can provide older V4L2 UAPI headers\n'
    '        # without the 32-bit alpha pixel-format aliases introduced by\n'
    '        # newer kernel headers. Keep Qt Multimedia\'s format table\n'
    '        # buildable while preserving the Linux UAPI FOURCC values.\n'
    '        qv4l2camera = os.path.join(\n'
    '            self.source_folder, "qtmultimedia", "src", "plugins",\n'
    '            "multimedia", "ffmpeg", "qv4l2camera.cpp"\n'
    '        )\n'
    '        replace_in_file(\n'
    '            self,\n'
    '            qv4l2camera,\n'
    '            "#include <qloggingcategory.h>\\n",\n'
    '            "#include <qloggingcategory.h>\\n"\n'
    '            "\\n"\n'
    '            "#ifndef V4L2_PIX_FMT_BGRA32\\n"\n'
    '            "#define V4L2_PIX_FMT_BGRA32 v4l2_fourcc(\'R\', \'A\', \'2\', \'4\')\\n"\n'
    '            "#endif\\n"\n'
    '            "#ifndef V4L2_PIX_FMT_RGBA32\\n"\n'
    '            "#define V4L2_PIX_FMT_RGBA32 v4l2_fourcc(\'A\', \'B\', \'2\', \'4\')\\n"\n'
    '            "#endif\\n",\n'
    '            strict=True,\n'
    '        )\n'
)
_FFMPEG_SOURCE_MARKER = 'qtmultimedia_configure = os.path.join'
_FFMPEG_LINUX_AUDIO_SOURCE_PATCH = (
    _FFMPEG_SOURCE_PATCH
    .replace(
        '''        # Conan deliberately forbids self.options access in source().
        # This source-level relaxation is harmless when with_ffmpeg is
        # false: Qt still controls whether the backend is configured from
        # the option, while the patched recipe can safely be exported for
        # every platform and package configuration.
''',
        _FFMPEG_SOURCE_NEUTRAL_NOTE,
    )
    .replace(_FFMPEG_LINUX_VIDEO_ONLY_NOTE, _FFMPEG_LINUX_AUDIO_NOTE)
    .replace(_FFMPEG_LINUX_VIDEO_ONLY_RELAXATION, '')
)
_FFMPEG_LINUX_AUDIO_RELAXATION_MARKER = 'OR QNX OR LINUX OR'
_FFMPEG_MODULE_SOURCE_MARKER = (
    'qt_find_package(FFmpeg MODULE OPTIONAL_COMPONENTS'
)
_FFMPEG_VAAPI_MODULE_MARKER = 'qt_find_package(VAAPI MODULE COMPONENTS'
_FFMPEG_CONFIG_SOURCE_MARKER = 'find_package(ffmpeg CONFIG QUIET)'
_V4L2_FORMAT_MARKER = '#ifndef V4L2_PIX_FMT_BGRA32'
_FFMPEG_TARGET_BRIDGE_MARKER = 'TARGET ffmpeg::${_lowerComponent}'
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

_PULSEAUDIO_VALIDATION_ANCHOR = '''        if self.options.get_safe("with_pulseaudio", False) or self.options.get_safe("with_libalsa", False):
            raise ConanInvalidConfiguration("alsa and pulseaudio are not supported (QTBUG-95116), please disable them.")
'''
_PULSEAUDIO_VALIDATION_PATCH = '''        if self.options.get_safe("with_libalsa", False):
            raise ConanInvalidConfiguration("The portable Linux Qt package uses PulseAudio; ALSA must remain disabled.")
'''
_PULSEAUDIO_GENERATE_ANCHOR = '        tc.variables["FEATURE_pkg_config"] = "ON"\n'
_PULSEAUDIO_GENERATE_PATCH = _PULSEAUDIO_GENERATE_ANCHOR + (
    '        if self.options.get_safe("with_pulseaudio", False):\n'
    '            tc.variables["FEATURE_pulseaudio"] = "ON"\n'
    '            tc.variables["FEATURE_alsa"] = "OFF"\n'
)
_PULSEAUDIO_FEATURE_MARKER = 'tc.variables["FEATURE_pulseaudio"] = "ON"'
_ALSA_FEATURE_MARKER = 'tc.variables["FEATURE_alsa"] = "OFF"'
_PULSEAUDIO_FIND_MODULE_ANCHOR = '''if(TARGET WrapPulseAudio::WrapPulseAudio)
    set(WrapPulseAudio_FOUND ON)
    return()
endif()
'''
_PULSEAUDIO_FIND_MODULE_PATCH = _PULSEAUDIO_FIND_MODULE_ANCHOR + '''
# Conan CMakeDeps exports PulseAudio as pulseaudio::pulse instead of the
# legacy PULSEAUDIO_LIBRARY and PULSEAUDIO_INCLUDE_DIR variables.
find_package(pulseaudio CONFIG QUIET)
if(TARGET pulseaudio::pulse)
    add_library(WrapPulseAudio::WrapPulseAudio INTERFACE IMPORTED)
    target_link_libraries(WrapPulseAudio::WrapPulseAudio
                          INTERFACE pulseaudio::pulse)
    set(WrapPulseAudio_FOUND ON)
    return()
endif()
'''
_PULSEAUDIO_FIND_MODULE_MARKER = 'find_package(pulseaudio CONFIG QUIET)'
_PULSEAUDIO_SOURCE_PATCH_MARKER = 'pulseaudio_finder = os.path.join'
_PULSEAUDIO_SOURCE_PATCH = (
    '        pulseaudio_finder = os.path.join(\n'
    '            self.source_folder, "qtmultimedia", "cmake",\n'
    '            "FindWrapPulseAudio.cmake"\n'
    '        )\n'
    '        replace_in_file(\n'
    '            self,\n'
    '            pulseaudio_finder,\n'
    f'            {_PULSEAUDIO_FIND_MODULE_ANCHOR!r},\n'
    f'            {_PULSEAUDIO_FIND_MODULE_PATCH!r},\n'
    '            strict=True,\n'
    '        )\n'
)
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


def _patch_md4c(text: str) -> str:
    if _MD4C_REQUIREMENT_MARKER in text:
        return text
    if text.count(_MD4C_REQUIREMENT_ANCHOR) != 1:
        raise SystemExit(
            "unexpected Qt recipe: md4c requirement is absent or ambiguous"
        )
    return text.replace(_MD4C_REQUIREMENT_ANCHOR, _MD4C_REQUIREMENT_PATCH)


def _ffmpeg_source_patch(linux_audio: bool) -> str:
    if not linux_audio:
        return _FFMPEG_SOURCE_PATCH
    if (
        _FFMPEG_LINUX_VIDEO_ONLY_NOTE not in _FFMPEG_SOURCE_PATCH
        or _FFMPEG_LINUX_VIDEO_ONLY_RELAXATION not in _FFMPEG_SOURCE_PATCH
    ):
        raise SystemExit("internal error: Linux video-only Qt patch is incomplete")
    patched = _FFMPEG_LINUX_AUDIO_SOURCE_PATCH
    if (
        _FFMPEG_LINUX_VIDEO_ONLY_RELAXATION in patched
        or _FFMPEG_LINUX_VIDEO_ONLY_NOTE in patched
        or _FFMPEG_LINUX_AUDIO_RELAXATION_MARKER in patched
    ):
        raise SystemExit("internal error: audio-enabled Qt patch relaxes the Linux FFmpeg gate")
    return patched


def _patch_ffmpeg(text: str, linux_audio: bool = False) -> str:
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
        text = text.replace(
            _FFMPEG_SOURCE_ANCHOR,
            _ffmpeg_source_patch(linux_audio),
        )
    elif linux_audio and _FFMPEG_LINUX_VIDEO_ONLY_RELAXATION in text:
        if text.count(_FFMPEG_LINUX_VIDEO_ONLY_NOTE) != 1:
            raise SystemExit(
                "unexpected Qt recipe: legacy Linux FFmpeg relaxation is ambiguous"
            )
        text = text.replace(
            _FFMPEG_LINUX_VIDEO_ONLY_NOTE,
            _FFMPEG_LINUX_AUDIO_NOTE,
        ).replace(_FFMPEG_LINUX_VIDEO_ONLY_RELAXATION, "")
    elif linux_audio and _FFMPEG_LINUX_AUDIO_NOTE not in text:
        raise SystemExit(
            "unexpected Qt recipe: existing Linux multimedia patch does not require audio"
        )
    if (
        _FFMPEG_MODULE_SOURCE_MARKER not in text
        or text.count(_FFMPEG_VAAPI_MODULE_MARKER) < 2
        or _FFMPEG_CONFIG_SOURCE_MARKER not in text
        or _FFMPEG_TARGET_BRIDGE_MARKER not in text
        or _V4L2_FORMAT_MARKER not in text
    ):
        raise SystemExit(
            "unexpected Qt recipe: existing FFmpeg source patch lacks module-mode "
            "finders, Conan target bridge, or V4L2 compatibility definitions"
        )
    if _FFMPEG_PACKAGE_INFO_MARKER not in text:
        if text.count(_FFMPEG_PACKAGE_INFO_ANCHOR) != 1:
            raise SystemExit(
                "unexpected Qt recipe: Multimedia package-info anchor is absent "
                "or ambiguous"
            )
        text = text.replace(
            _FFMPEG_PACKAGE_INFO_ANCHOR,
            _FFMPEG_PACKAGE_INFO_PATCH,
        )
    return text


def _patch_linux_audio(text: str) -> str:
    if _PULSEAUDIO_VALIDATION_PATCH not in text:
        if text.count(_PULSEAUDIO_VALIDATION_ANCHOR) != 1:
            raise SystemExit(
                "unexpected Qt recipe: PulseAudio validation guard is absent or ambiguous"
            )
        text = text.replace(
            _PULSEAUDIO_VALIDATION_ANCHOR,
            _PULSEAUDIO_VALIDATION_PATCH,
        )

    has_pulseaudio_feature = _PULSEAUDIO_FEATURE_MARKER in text
    has_alsa_feature = _ALSA_FEATURE_MARKER in text
    if not has_pulseaudio_feature and not has_alsa_feature:
        if text.count(_PULSEAUDIO_GENERATE_ANCHOR) != 1:
            raise SystemExit(
                "unexpected Qt recipe: PulseAudio CMake feature anchor is absent or ambiguous"
            )
        text = text.replace(
            _PULSEAUDIO_GENERATE_ANCHOR,
            _PULSEAUDIO_GENERATE_PATCH,
        )
    elif has_pulseaudio_feature != has_alsa_feature:
        raise SystemExit(
            "unexpected Qt recipe: PulseAudio and ALSA CMake feature selection is incomplete"
        )

    if _FFMPEG_LINUX_AUDIO_RELAXATION_MARKER in text:
        raise SystemExit(
            "unexpected Qt recipe: audio-enabled Linux builds must preserve Qt's FFmpeg gate"
        )
    if _PULSEAUDIO_SOURCE_PATCH_MARKER not in text:
        source_anchor = '        apply_conandata_patches(self)\n'
        if text.count(source_anchor) != 1:
            raise SystemExit(
                "unexpected Qt recipe: source patch anchor is absent or ambiguous"
            )
        text = text.replace(
            source_anchor,
            source_anchor + _PULSEAUDIO_SOURCE_PATCH,
        )
    return text


def _patch_pulseaudio_finder(text: str) -> str:
    if _PULSEAUDIO_FIND_MODULE_MARKER in text:
        return text
    if text.count(_PULSEAUDIO_FIND_MODULE_ANCHOR) != 1:
        raise SystemExit(
            "unexpected Qt source: PulseAudio finder anchor is absent or ambiguous"
        )
    return text.replace(
        _PULSEAUDIO_FIND_MODULE_ANCHOR,
        _PULSEAUDIO_FIND_MODULE_PATCH,
    )


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


_WINDOWS_HDR_COORDINATES_MARKER = "# f4: map HDR output coordinates relative to their screen"
_WINDOWS_HDR_COORDINATES_SOURCE_PATCH = '''        # f4: map HDR output coordinates relative to their screen
        _f4_hdr_path = os.path.join(self.source_folder, 'qtbase', 'src', 'gui',
                                   'rhi', 'qdxgihdrinfo.cpp')
        replace_in_file(self, _f4_hdr_path,
                        '#include "qdxgihdrinfo_p.h"',
                        '#include "qdxgihdrinfo_p.h"\\n#include <QtGui/private/qhighdpiscaling_p.h>')
        replace_in_file(self, _f4_hdr_path,
                        '    QRect wr = w->geometry();\\n'
                        '    wr = QRect(wr.topLeft() * w->devicePixelRatio(), wr.size() * w->devicePixelRatio());',
                        '    const QRect wr = QHighDpi::toNativePixels(w->geometry(), w);')
'''


def _patch_windows_hdr_coordinates(text: str) -> str:
    if _WINDOWS_HDR_COORDINATES_MARKER in text:
        return text
    if text.count(_FFMPEG_SOURCE_ANCHOR) != 1:
        raise SystemExit("unexpected Qt recipe: HDR coordinate patch anchor is absent or ambiguous")
    return text.replace(_FFMPEG_SOURCE_ANCHOR,
                        _FFMPEG_SOURCE_ANCHOR + _WINDOWS_HDR_COORDINATES_SOURCE_PATCH)


_MACOS_COLORSPACE_MARKER = "# f4: retain macOS image color space through native conversion"
_MACOS_COLORSPACE_REPLACEMENTS = (
    ("painting/qcoregraphics.mm",
     "        colorSpace, bitmapInfo, 0, nullptr,",
     "        CGColorSpaceRetain(colorSpace), bitmapInfo, 0, nullptr,"),
    ("painting/qcoregraphics.mm",
     "    context = CGBitmapContextCreate((void *)image->bits(),",
     "    QCFType<CGColorSpaceRef> colorSpace = cgImageFormat->colorSpace;\n"
     "    context = CGBitmapContextCreate((void *)image->bits(),"),
    ("image/qimage_darwin.mm",
     "    // Create a data provider that owns a copy of the QImage",
     "    QCFType<CGColorSpaceRef> colorSpace = cgImageFormat->colorSpace;\n\n"
     "    // Create a data provider that owns a copy of the QImage"),
    ("text/coretext/qfontengine_coretext.mm",
     "    QCFType<CGContextRef> ctx = CGBitmapContextCreate(im.bits(),",
     "    QCFType<CGColorSpaceRef> colorSpace = cgImageFormat->colorSpace;\n"
     "    QCFType<CGContextRef> ctx = CGBitmapContextCreate(im.bits(),"),
    ("painting/qcoregraphics_p.h",
     "Q_GUI_EXPORT std::optional<vImage_CGImageFormat> qt_mac_cgImageFormatForImage",
     "// The returned format owns its colorSpace; the caller must release it.\n"
     "Q_GUI_EXPORT std::optional<vImage_CGImageFormat> qt_mac_cgImageFormatForImage"),
)
_MACOS_COLORSPACE_SOURCE_PATCH = "        " + _MACOS_COLORSPACE_MARKER + "\n" + "".join(
    "        replace_in_file(self, os.path.join(self.source_folder, 'qtbase', 'src', 'gui', "
    + repr(path) + "), " + repr(before) + ", " + repr(after) + ", strict=True)\n"
    for path, before, after in _MACOS_COLORSPACE_REPLACEMENTS
)


def _patch_macos_colorspace(text: str) -> str:
    if _MACOS_COLORSPACE_MARKER in text:
        return text
    if text.count(_FFMPEG_SOURCE_ANCHOR) != 1:
        raise RuntimeError("Qt recipe source patch anchor changed")
    return text.replace(_FFMPEG_SOURCE_ANCHOR,
                        _FFMPEG_SOURCE_ANCHOR + _MACOS_COLORSPACE_SOURCE_PATCH)


_QUICK_BATCH_ORDERS_MARKER = "# f4: balance Qt Quick batch root order reservations"
_QUICK_BATCH_ORDERS_BEFORE = (
    "                BatchRootInfo *info = batchRootInfo(e->root);\n"
    "                info->availableOrders++;\n"
)
_QUICK_BATCH_ORDERS_AFTER = (
    "                BatchRootInfo *info = batchRootInfo(e->root);\n"
    "                // Mirror the ancestor reservations in Updater::visitGeometryNode().\n"
    "                while (info) {\n"
    "                    ++info->availableOrders;\n"
    "                    info = info->parentRoot ? batchRootInfo(info->parentRoot) : nullptr;\n"
    "                }\n"
)
_QUICK_BATCH_ORDERS_SOURCE_PATCH = (
    "        " + _QUICK_BATCH_ORDERS_MARKER + "\n"
    "        replace_in_file(\n"
    "            self,\n"
    "            os.path.join(self.source_folder, 'qtdeclarative', 'src', 'quick',\n"
    "                         'scenegraph', 'coreapi', 'qsgbatchrenderer.cpp'),\n"
    f"            {_QUICK_BATCH_ORDERS_BEFORE!r},\n"
    f"            {_QUICK_BATCH_ORDERS_AFTER!r},\n"
    "            strict=True,\n"
    "        )\n"
)


def _patch_quick_batch_orders(text: str) -> str:
    if _QUICK_BATCH_ORDERS_MARKER in text:
        return text
    if text.count(_FFMPEG_SOURCE_ANCHOR) != 1:
        raise SystemExit("unexpected Qt recipe: batch root source patch anchor is absent or ambiguous")
    return text.replace(_FFMPEG_SOURCE_ANCHOR,
                        _FFMPEG_SOURCE_ANCHOR + _QUICK_BATCH_ORDERS_SOURCE_PATCH)


_QUICK_BLOCKED_OPACITY_MARKER = "# f4: ignore opacity outside Qt Quick's renderer tree"
_QUICK_BLOCKED_OPACITY_BEFORE = (
    "        // Force a batch rebuild if this includes an opacity change\n"
    "        if (state & QSGNode::DirtyOpacity)\n"
    "            m_rebuild |= FullRebuild;\n"
)
_QUICK_BLOCKED_OPACITY_AFTER = (
    "        // A blocked ancestor can keep this node out of the renderer entirely.\n"
    "        // Only an existing subtree has batches whose opacity must be rebuilt.\n"
    "        if ((state & QSGNode::DirtyOpacity) && sn)\n"
    "            m_rebuild |= FullRebuild;\n"
)
_QUICK_BLOCKED_OPACITY_SOURCE_PATCH = (
    "        " + _QUICK_BLOCKED_OPACITY_MARKER + "\n"
    "        replace_in_file(\n"
    "            self,\n"
    "            os.path.join(self.source_folder, 'qtdeclarative', 'src', 'quick',\n"
    "                         'scenegraph', 'coreapi', 'qsgbatchrenderer.cpp'),\n"
    f"            {_QUICK_BLOCKED_OPACITY_BEFORE!r},\n"
    f"            {_QUICK_BLOCKED_OPACITY_AFTER!r},\n"
    "            strict=True,\n"
    "        )\n"
)


def _patch_quick_blocked_opacity(text: str) -> str:
    if _QUICK_BLOCKED_OPACITY_MARKER in text:
        return text
    if text.count(_FFMPEG_SOURCE_ANCHOR) != 1:
        raise SystemExit("unexpected Qt recipe: blocked opacity source patch anchor is absent or ambiguous")
    return text.replace(_FFMPEG_SOURCE_ANCHOR,
                        _FFMPEG_SOURCE_ANCHOR + _QUICK_BLOCKED_OPACITY_SOURCE_PATCH)


_QUICK_STABLE_INTERVALS_MARKER = "# f4: retain unchanged Qt Quick batches with owned order intervals"


def _patch_quick_stable_intervals(text: str) -> str:
    replacements = runpy.run_path(
        str(Path(__file__).with_name("qt_quick_stable_intervals.py"))
    )["REPLACEMENTS"]
    # Export the data into the recipe itself. Source preparation may happen on
    # another machine without this repository or its helper files available.
    source_patch = (
        "        " + _QUICK_STABLE_INTERVALS_MARKER + "\n"
        "        from conan.tools.files import load, save\n"
        "        _f4_quick_path = os.path.join(self.source_folder, 'qtdeclarative',\n"
        "                                     'src', 'quick', 'scenegraph', 'coreapi',\n"
        "                                     'qsgbatchrenderer.cpp')\n"
        "        _f4_quick_source = load(self, _f4_quick_path)\n"
        f"        for _f4_before, _f4_after in {replacements!r}:\n"
        "            if _f4_quick_source.count(_f4_before) != 1:\n"
        "                raise RuntimeError('Qt Quick stable intervals source anchor changed or is ambiguous')\n"
        "            _f4_quick_source = _f4_quick_source.replace(_f4_before, _f4_after)\n"
        "        save(self, _f4_quick_path, _f4_quick_source)\n"
    )
    if _QUICK_STABLE_INTERVALS_MARKER in text:
        start_marker = "        " + _QUICK_STABLE_INTERVALS_MARKER + "\n"
        end_marker = "        save(self, _f4_quick_path, _f4_quick_source)\n"
        if text.count(start_marker) != 1 or text.count(end_marker) != 1:
            raise SystemExit("unexpected Qt recipe: stable intervals refresh anchors are absent or ambiguous")
        start = text.index(start_marker)
        end = text.index(end_marker) + len(end_marker)
        if end <= start:
            raise SystemExit("unexpected Qt recipe: stable intervals refresh anchors are out of order")
        return text[:start] + source_patch + text[end:]
    predecessors = (_QUICK_BATCH_ORDERS_SOURCE_PATCH, _QUICK_BLOCKED_OPACITY_SOURCE_PATCH)
    if any(text.count(part) != 1 for part in predecessors):
        raise SystemExit("unexpected Qt recipe: stable intervals predecessor anchor is absent or ambiguous")
    end = max(text.index(part) + len(part) for part in predecessors)
    return text[:end] + source_patch + text[end:]


_QUICK_BUFFER_POOL_FIT_MARKER = "# f4: prefer pooled buffers with sufficient capacity"
_QUICK_BUFFER_POOL_FIT_BEFORE = '''                if (!buffer->buf
                    || (testBuffer->size() >= expectedSize && testBuffer->size() < buffer->buf->size())'''
_QUICK_BUFFER_POOL_FIT_AFTER = '''                if (!buffer->buf
                    || (testBuffer->size() >= expectedSize && buffer->buf->size() < expectedSize)
                    || (testBuffer->size() >= expectedSize && testBuffer->size() < buffer->buf->size())'''
_QUICK_BUFFER_POOL_FIT_SOURCE_PATCH = (
    "        " + _QUICK_BUFFER_POOL_FIT_MARKER + "\n"
    "        from conan.tools.files import load, save\n"
    "        _f4_pool_path = os.path.join(self.source_folder, 'qtdeclarative',\n"
    "                                    'src', 'quick', 'scenegraph', 'coreapi', 'qsgbatchrenderer.cpp')\n"
    "        _f4_pool_source = load(self, _f4_pool_path)\n"
    f"        _f4_pool_before = {_QUICK_BUFFER_POOL_FIT_BEFORE!r}\n"
    f"        _f4_pool_after = {_QUICK_BUFFER_POOL_FIT_AFTER!r}\n"
    "        if _f4_pool_source.count(_f4_pool_before) != 1:\n"
    "            raise RuntimeError('Qt Quick buffer-pool fit source anchor changed or is ambiguous')\n"
    "        save(self, _f4_pool_path, _f4_pool_source.replace(_f4_pool_before, _f4_pool_after))\n"
)


def _patch_quick_buffer_pool_fit(text: str) -> str:
    if _QUICK_BUFFER_POOL_FIT_MARKER in text:
        return text
    if text.count(_FFMPEG_SOURCE_ANCHOR) != 1:
        raise SystemExit("unexpected Qt recipe: source patch anchor is absent or ambiguous")
    return text.replace(_FFMPEG_SOURCE_ANCHOR,
                        _FFMPEG_SOURCE_ANCHOR + _QUICK_BUFFER_POOL_FIT_SOURCE_PATCH)


_QUICK_UPLOAD_POLICY_MARKER = "# f4: retain vector batches and bound initial upload work"


def _quick_upload_policy_source_patch() -> str:
    replacements = runpy.run_path(
        str(Path(__file__).with_name("qt_quick_upload_policy.py"))
    )["REPLACEMENTS"]
    return (
        "        " + _QUICK_UPLOAD_POLICY_MARKER + "\n"
        "        from conan.tools.files import load, save\n"
        "        _f4_upload_sources = []\n"
        f"        for _f4_name, _f4_changes in {replacements!r}:\n"
        "            _f4_path = os.path.join(self.source_folder, 'qtdeclarative', 'src',\n"
        "                                    'quick', 'scenegraph', 'coreapi', _f4_name)\n"
        "            _f4_source = load(self, _f4_path)\n"
        "            for _f4_before, _f4_after in _f4_changes:\n"
        "                if _f4_source.count(_f4_before) != 1:\n"
        "                    raise RuntimeError('Qt Quick upload policy source anchor changed or is ambiguous')\n"
        "                _f4_source = _f4_source.replace(_f4_before, _f4_after)\n"
        "            _f4_upload_sources.append((_f4_path, _f4_source))\n"
        "        for _f4_path, _f4_source in _f4_upload_sources:\n"
        "            save(self, _f4_path, _f4_source)\n"
    )


def _patch_quick_upload_policy(text: str) -> str:
    if _QUICK_UPLOAD_POLICY_MARKER in text:
        start_marker = "        " + _QUICK_UPLOAD_POLICY_MARKER + "\n"
        end_marker = ("        for _f4_path, _f4_source in _f4_upload_sources:\n"
                      "            save(self, _f4_path, _f4_source)\n")
        if text.count(start_marker) != 1 or text.count(end_marker) != 1:
            raise SystemExit("unexpected Qt recipe: upload policy refresh anchor is absent or ambiguous")
        start = text.index(start_marker)
        end = text.index(end_marker) + len(end_marker)
        if end <= start:
            raise SystemExit("unexpected Qt recipe: upload policy refresh anchors are out of order")
        return text[:start] + _quick_upload_policy_source_patch() + text[end:]
    predecessors = (
        _QUICK_BUFFER_POOL_FIT_SOURCE_PATCH,
        "        save(self, _f4_quick_path, _f4_quick_source)\n",
    )
    if (_QUICK_STABLE_INTERVALS_MARKER not in text
            or any(text.count(part) != 1 for part in predecessors)):
        raise SystemExit("unexpected Qt recipe: upload policy predecessor anchor is absent or ambiguous")
    end = max(text.index(part) + len(part) for part in predecessors)
    return text[:end] + _quick_upload_policy_source_patch() + text[end:]


_QUICK_CHILD_WINDOW_STACKING_MARKER = "# f4: skip child-window stacking walks without native children"
_QUICK_CHILD_WINDOW_STACKING_BEFORE = '''    if (!item) {
        qCDebug(lcQuickWindow) << "Updating child window stacking order for" << q;
'''
_QUICK_CHILD_WINDOW_STACKING_AFTER = '''    if (!item) {
        bool hasChildWindow = false;
        for (QWindow *childWindow : QGuiApplication::allWindows()) {
            if (childWindow->parent() == q) {
                hasChildWindow = true;
                break;
            }
        }
        if (!hasChildWindow)
            return;
        qCDebug(lcQuickWindow) << "Updating child window stacking order for" << q;
'''
_QUICK_CHILD_WINDOW_STACKING_SOURCE_PATCH = (
    "        " + _QUICK_CHILD_WINDOW_STACKING_MARKER + "\n"
    "        from conan.tools.files import load, save\n"
    "        _f4_window_path = os.path.join(self.source_folder, 'qtdeclarative',\n"
    "                                      'src', 'quick', 'items', 'qquickwindow.cpp')\n"
    "        _f4_window_source = load(self, _f4_window_path)\n"
    f"        _f4_window_before = {_QUICK_CHILD_WINDOW_STACKING_BEFORE!r}\n"
    f"        _f4_window_after = {_QUICK_CHILD_WINDOW_STACKING_AFTER!r}\n"
    "        if _f4_window_source.count(_f4_window_before) != 1:\n"
    "            raise RuntimeError('Qt Quick child-window stacking source anchor changed or is ambiguous')\n"
    "        save(self, _f4_window_path, _f4_window_source.replace(_f4_window_before, _f4_window_after))\n"
)


def _patch_quick_child_window_stacking(text: str) -> str:
    if _QUICK_CHILD_WINDOW_STACKING_MARKER in text:
        return text
    if text.count(_FFMPEG_SOURCE_ANCHOR) != 1:
        raise SystemExit("unexpected Qt recipe: source patch anchor is absent or ambiguous")
    return text.replace(_FFMPEG_SOURCE_ANCHOR,
                        _FFMPEG_SOURCE_ANCHOR + _QUICK_CHILD_WINDOW_STACKING_SOURCE_PATCH)


_QUICK_ANIMATION_CLOCK_MARKER = "# f4: expose precise render-animation callback clock"


def _quick_animation_clock_source_patch() -> str:
    replacements = runpy.run_path(
        str(Path(__file__).with_name("qt_quick_animation_clock.py"))
    )["REPLACEMENTS"]
    return (
        "        " + _QUICK_ANIMATION_CLOCK_MARKER + "\n"
        "        from conan.tools.files import load, save\n"
        "        _f4_clock_sources = []\n"
        f"        for _f4_name, _f4_changes in {replacements!r}:\n"
        "            _f4_path = os.path.join(self.source_folder, 'qtdeclarative', 'src',\n"
        "                                    'quick', 'scenegraph', _f4_name)\n"
        "            _f4_source = load(self, _f4_path)\n"
        "            for _f4_before, _f4_after in _f4_changes:\n"
        "                if _f4_source.count(_f4_before) != 1:\n"
        "                    raise RuntimeError('Qt Quick animation clock source anchor changed or is ambiguous')\n"
        "                _f4_source = _f4_source.replace(_f4_before, _f4_after)\n"
        "            _f4_clock_sources.append((_f4_path, _f4_source))\n"
        "        for _f4_path, _f4_source in _f4_clock_sources:\n"
        "            save(self, _f4_path, _f4_source)\n"
    )


def _patch_quick_animation_clock(text: str) -> str:
    block = _quick_animation_clock_source_patch()
    if _QUICK_ANIMATION_CLOCK_MARKER in text:
        begin = "        " + _QUICK_ANIMATION_CLOCK_MARKER + "\n"
        end = "        for _f4_path, _f4_source in _f4_clock_sources:\n            save(self, _f4_path, _f4_source)\n"
        if text.count(begin) != 1 or text.count(end) != 1:
            raise SystemExit("unexpected Qt recipe: animation clock refresh anchors are ambiguous")
        start, stop = text.index(begin), text.index(end) + len(end)
        if stop <= start:
            raise SystemExit("unexpected Qt recipe: animation clock refresh anchors are out of order")
        return text[:start] + block + text[stop:]
    end = "        for _f4_path, _f4_source in _f4_upload_sources:\n            save(self, _f4_path, _f4_source)\n"
    if text.count(end) != 1:
        raise SystemExit("unexpected Qt recipe: animation clock predecessor anchor is absent or ambiguous")
    return text.replace(end, end + block)


_QML_SIGNAL_MASK_MARKER = "# f4: reduce conservative QML signal-mask collisions"


def _qml_signal_mask_source_patch() -> str:
    replacements = runpy.run_path(
        str(Path(__file__).with_name("qt_qml_signal_policy.py"))
    )["REPLACEMENTS"]
    return (
        "        " + _QML_SIGNAL_MASK_MARKER + "\n"
        "        from conan.tools.files import load, save\n"
        "        _f4_signal_path = os.path.join(self.source_folder, 'qtdeclarative',\n"
        "                                       'src', 'qml', 'qml', 'qqmlengine.cpp')\n"
        "        _f4_signal_source = load(self, _f4_signal_path)\n"
        f"        for _f4_before, _f4_after in {replacements!r}:\n"
        "            if _f4_signal_source.count(_f4_before) != 1:\n"
        "                raise RuntimeError('Qt QML signal-mask source anchor changed or is ambiguous')\n"
        "            _f4_signal_source = _f4_signal_source.replace(_f4_before, _f4_after)\n"
        "        save(self, _f4_signal_path, _f4_signal_source)\n"
    )


def _patch_qml_signal_mask(text: str) -> str:
    block = _qml_signal_mask_source_patch()
    if _QML_SIGNAL_MASK_MARKER in text:
        begin = "        " + _QML_SIGNAL_MASK_MARKER + "\n"
        end = "        save(self, _f4_signal_path, _f4_signal_source)\n"
        if text.count(begin) != 1 or text.count(end) != 1:
            raise SystemExit("unexpected Qt recipe: QML signal-mask refresh anchors are ambiguous")
        start, stop = text.index(begin), text.index(end) + len(end)
        if stop <= start:
            raise SystemExit("unexpected Qt recipe: QML signal-mask refresh anchors are out of order")
        return text[:start] + block + text[stop:]
    if text.count(_FFMPEG_SOURCE_ANCHOR) != 1:
        raise SystemExit("unexpected Qt recipe: source patch anchor is absent or ambiguous")
    return text.replace(_FFMPEG_SOURCE_ANCHOR, _FFMPEG_SOURCE_ANCHOR + block)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("recipe", type=Path)
    parser.add_argument(
        "--linux-audio",
        action="store_true",
        help="require the Linux PulseAudio backend for Qt Multimedia",
    )
    args = parser.parse_args()

    text = args.recipe.read_text(encoding="utf-8")
    patched = _patch_quick_package_guard(
        _patch_host_path(
            _patch_ffmpeg(
                _patch_md4c(_patch_freetype(text)),
                linux_audio=args.linux_audio,
            )
        )
    )
    if args.linux_audio:
        patched = _patch_linux_audio(patched)
        print(
            "[FIX] Qt Linux audio recipe bridges WrapPulseAudio to "
            "Conan's pulseaudio::pulse target"
        )
    patched = _patch_macos_colorspace(patched)
    patched = _patch_windows_hdr_coordinates(patched)
    print("[FIX:mac-color-space] Qt recipe retains native image color spaces")
    patched = _patch_quick_batch_orders(patched)
    print("[FIX:quick-batch-orders] Qt Quick returns ancestor render-order reservations")
    patched = _patch_quick_blocked_opacity(patched)
    print("[FIX:quick-blocked-opacity] Qt Quick ignores opacity outside its renderer tree")
    patched = _patch_quick_stable_intervals(patched)
    print("[FIX:quick-stable-intervals] Qt Quick retains unchanged batches and owns free order intervals")
    patched = _patch_quick_buffer_pool_fit(patched)
    print("[FIX:quick-buffer-pool-fit] Qt Quick selects reusable buffers before growing undersized ones")
    patched = _patch_quick_upload_policy(patched)
    patched = _patch_quick_animation_clock(patched)
    print("[FIX:quick-upload-policy] Qt Quick retains vector batches and bounds initial upload work")
    patched = _patch_quick_child_window_stacking(patched)
    print("[FIX:quick-child-window-stacking] Qt Quick skips scene walks without native child windows")
    patched = _patch_qml_signal_mask(patched)
    print("[FIX:qml-signal-mask] Qt QML reduces unobserved cross-thread signal forwarding")
    if patched != text:
        args.recipe.write_text(patched, encoding="utf-8")


if __name__ == "__main__":
    main()
