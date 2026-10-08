"""Regression tests for Qt's Linux Multimedia recipe patch."""

from __future__ import annotations

import ast
import importlib.util
import pathlib
import shutil
import subprocess
import tempfile
import unittest


SCRIPT = pathlib.Path(__file__).with_name("patch-qt-dependencies.py")
SPEC = importlib.util.spec_from_file_location("patch_qt_dependencies", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
patcher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(patcher)


class LinuxAudioRecipePatchTests(unittest.TestCase):
    def recipe_skeleton(self) -> str:
        return "".join(
            (
                patcher._FFMPEG_OPTION_ANCHOR,
                patcher._FFMPEG_DEFAULT_ANCHOR,
                patcher._FFMPEG_REQUIREMENT_ANCHOR,
                patcher._FFMPEG_GENERATE_ANCHOR,
                patcher._FFMPEG_SOURCE_ANCHOR,
                patcher._FFMPEG_PACKAGE_INFO_ANCHOR,
                patcher._PULSEAUDIO_VALIDATION_ANCHOR,
            )
        )

    def test_linux_audio_keeps_qt_ffmpeg_gate_and_selects_pulseaudio(self) -> None:
        patched = patcher._patch_ffmpeg(self.recipe_skeleton(), linux_audio=True)
        patched = patcher._patch_linux_audio(patched)

        self.assertIn(patcher._FFMPEG_LINUX_AUDIO_NOTE, patched)
        self.assertNotIn(patcher._FFMPEG_LINUX_VIDEO_ONLY_RELAXATION, patched)
        self.assertNotIn(patcher._FFMPEG_LINUX_AUDIO_RELAXATION_MARKER, patched)
        self.assertIn(patcher._PULSEAUDIO_VALIDATION_PATCH, patched)
        self.assertIn(patcher._PULSEAUDIO_FEATURE_MARKER, patched)
        self.assertIn(patcher._ALSA_FEATURE_MARKER, patched)
        self.assertIn(patcher._PULSEAUDIO_SOURCE_PATCH_MARKER, patched)

    def test_linux_audio_patch_is_idempotent(self) -> None:
        patched = patcher._patch_ffmpeg(self.recipe_skeleton(), linux_audio=True)
        patched = patcher._patch_linux_audio(patched)

        self.assertEqual(
            patcher._patch_linux_audio(
                patcher._patch_ffmpeg(patched, linux_audio=True)
            ),
            patched,
        )

    def test_pulseaudio_source_recipe_patch_is_valid_python(self) -> None:
        ast.parse("def source(self):\n" + patcher._PULSEAUDIO_SOURCE_PATCH)

    def test_linux_audio_ignores_unrelated_package_info_alsa_guard(self) -> None:
        recipe = self.recipe_skeleton() + (
            '            if self.options.get_safe("with_libalsa", False):\n'
            '                multimedia_reqs.append("libalsa::libalsa")\n'
        )
        patched = patcher._patch_ffmpeg(recipe, linux_audio=True)
        patched = patcher._patch_linux_audio(patched)

        self.assertIn(patcher._PULSEAUDIO_VALIDATION_PATCH, patched)
        self.assertIn(
            '                multimedia_reqs.append("libalsa::libalsa")\n',
            patched,
        )

    def test_video_only_mode_remains_unchanged_for_other_platforms(self) -> None:
        patched = patcher._patch_ffmpeg(self.recipe_skeleton())

        self.assertIn(patcher._FFMPEG_LINUX_VIDEO_ONLY_RELAXATION, patched)
        self.assertNotIn(patcher._PULSEAUDIO_FEATURE_MARKER, patched)

    def test_audio_mode_rejects_a_video_only_recipe(self) -> None:
        legacy = patcher._patch_ffmpeg(self.recipe_skeleton())

        with self.assertRaisesRegex(SystemExit, "must preserve Qt's FFmpeg gate"):
            patcher._patch_linux_audio(legacy)

    def test_pulseaudio_finder_bridges_the_conan_target(self) -> None:
        finder = """if(TARGET WrapPulseAudio::WrapPulseAudio)
    set(WrapPulseAudio_FOUND ON)
    return()
endif()
find_package(PulseAudio QUIET)
if(PulseAudio_FOUND)
    set(WrapPulseAudio_FOUND 1)
endif()
if(WrapPulseAudio_FOUND AND NOT TARGET WrapPulseAudio::WrapPulseAudio)
    add_library(WrapPulseAudio::WrapPulseAudio INTERFACE IMPORTED)
    target_include_directories(WrapPulseAudio::WrapPulseAudio INTERFACE "${PULSEAUDIO_INCLUDE_DIR}")
    target_link_libraries(WrapPulseAudio::WrapPulseAudio INTERFACE "${PULSEAUDIO_LIBRARY}")
endif()
include(FindPackageHandleStandardArgs)
find_package_handle_standard_args(WrapPulseAudio REQUIRED_VARS
    PULSEAUDIO_LIBRARY PULSEAUDIO_INCLUDE_DIR WrapPulseAudio_FOUND)
"""
        patched = patcher._patch_pulseaudio_finder(finder)
        self.assertEqual(patcher._patch_pulseaudio_finder(patched), patched)

        cmake = shutil.which("cmake")
        if cmake is None:
            self.skipTest("CMake is unavailable for the configure-only finder fixture")

        with tempfile.TemporaryDirectory(prefix="f4-pulseaudio-finder-") as temp_dir:
            root = pathlib.Path(temp_dir)
            prefix = root / "prefix" / "lib" / "cmake" / "pulseaudio"
            prefix.mkdir(parents=True)
            (prefix / "pulseaudio-config.cmake").write_text(
                "add_library(pulseaudio::pulse INTERFACE IMPORTED)\n",
                encoding="utf-8",
            )
            (root / "CMakeLists.txt").write_text(
                """cmake_minimum_required(VERSION 3.15)
project(F4PulseAudioFinderFixture NONE)
set(CMAKE_MODULE_PATH "${CMAKE_CURRENT_LIST_DIR}")
set(pulseaudio_DIR "${CMAKE_CURRENT_LIST_DIR}/prefix/lib/cmake/pulseaudio")
set(CMAKE_DISABLE_FIND_PACKAGE_PulseAudio TRUE)
find_package(WrapPulseAudio REQUIRED)
if(NOT TARGET WrapPulseAudio::WrapPulseAudio)
    message(FATAL_ERROR "Qt PulseAudio wrapper target was not created")
endif()
get_target_property(_pulse_links WrapPulseAudio::WrapPulseAudio INTERFACE_LINK_LIBRARIES)
if(NOT "${_pulse_links}" STREQUAL "pulseaudio::pulse")
    message(FATAL_ERROR "Qt PulseAudio wrapper does not link Conan target: ${_pulse_links}")
endif()
""",
                encoding="utf-8",
            )

            finder_path = root / "FindWrapPulseAudio.cmake"
            finder_path.write_text(finder, encoding="utf-8")
            original_result = subprocess.run(
                [cmake, "-S", str(root), "-B", str(root / "build-original")],
                capture_output=True,
                check=False,
                text=True,
            )
            self.assertNotEqual(original_result.returncode, 0)

            finder_path.write_text(patched, encoding="utf-8")
            patched_result = subprocess.run(
                [cmake, "-S", str(root), "-B", str(root / "build-patched")],
                capture_output=True,
                check=False,
                text=True,
            )
            self.assertEqual(
                patched_result.returncode,
                0,
                patched_result.stdout + patched_result.stderr,
            )


if __name__ == "__main__":
    unittest.main()
