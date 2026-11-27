"""Ensure the Qt host and Qt recipe resolve the same platform FFmpeg ref."""

from __future__ import annotations

import ast
from pathlib import Path
import types
import unittest


ROOT = Path(__file__).resolve().parents[1]
CONANFILE = ROOT / "qt" / "host" / "conanfile.py"


def load_requirements():
    tree = ast.parse(CONANFILE.read_text(encoding="utf-8"))
    recipe = next(
        node for node in tree.body
        if isinstance(node, ast.ClassDef) and node.name == "F4QtHostConan"
    )
    method = next(
        node for node in recipe.body
        if isinstance(node, ast.FunctionDef) and node.name == "requirements"
    )
    module = ast.Module(body=[method], type_ignores=[])
    namespace = {}
    exec(compile(ast.fix_missing_locations(module), str(CONANFILE), "exec"), namespace)
    return namespace["requirements"]


class QtHostFFmpegReferenceTests(unittest.TestCase):
    def test_platforms_use_one_matching_ffmpeg_reference(self):
        requirements = load_requirements()
        expected = {
            "Macos": "ffmpeg/7.1.5@f4/macos",
            "Windows": "ffmpeg/7.1.5@f4/windows",
            "Linux": "ffmpeg/7.1.5",
        }

        for os_name, expected_reference in expected.items():
            with self.subTest(os=os_name):
                references = []
                host = types.SimpleNamespace(
                    settings=types.SimpleNamespace(os=os_name),
                    options=types.SimpleNamespace(
                        with_video_thumbnails=True,
                        with_ffmpeg_backend=True,
                    ),
                    requires=lambda reference, **_kwargs: references.append(reference),
                )

                requirements(host)

                self.assertEqual(
                    [reference for reference in references if reference.startswith("ffmpeg/")],
                    [expected_reference],
                )


if __name__ == "__main__":
    unittest.main()
