"""Regression tests for Qt's Linux Multimedia recipe patch."""

from __future__ import annotations

import importlib.util
import pathlib
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

    def test_linux_audio_patch_is_idempotent(self) -> None:
        patched = patcher._patch_ffmpeg(self.recipe_skeleton(), linux_audio=True)
        patched = patcher._patch_linux_audio(patched)

        self.assertEqual(
            patcher._patch_linux_audio(
                patcher._patch_ffmpeg(patched, linux_audio=True)
            ),
            patched,
        )

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


if __name__ == "__main__":
    unittest.main()
