from pathlib import Path
import os
import runpy
import tempfile
import textwrap
import types
import unittest


SCRIPT = Path(__file__).with_name("patch-ffmpeg-windows-recipe.py")


class PatchFFmpegWindowsRecipeTests(unittest.TestCase):
    def setUp(self):
        self.exporter = runpy.run_path(str(SCRIPT))

    def test_patch_is_idempotent_and_exposes_generated_pkg_config_folder(self):
        recipe = (
            "from conan.tools.gnu import PkgConfigDeps\n"
            "def generate(self):\n"
            "    deps = PkgConfigDeps(self)\n"
            "    deps.generate()\n"
            "def build(self):\n"
            '        autotools = Autotools(self)\n'
            "        autotools.configure()\n"
        )

        patched = self.exporter["patch_recipe"](recipe)
        self.assertEqual(self.exporter["patch_recipe"](patched), patched)
        self.assertIn(self.exporter["MARKER"], patched)
        self.assertLess(patched.index('os.environ["PKG_CONFIG_PATH"]'),
                        patched.index("autotools.configure()"))

    def test_build_prepends_generator_folder_and_keeps_existing_search_path(self):
        patch = textwrap.dedent(self.exporter["PATCH"])
        namespace = {"os": os, "is_msvc": lambda _recipe: True}
        observed = {}

        class FakeAutotools:
            def __init__(self, _recipe):
                pass

            def configure(self):
                observed["pkg_config_path"] = os.environ.get("PKG_CONFIG_PATH", "")

        namespace["Autotools"] = FakeAutotools
        exec("def build(self):\n" + textwrap.indent(patch, "    "), namespace)

        previous = os.environ.get("PKG_CONFIG_PATH")
        try:
            with tempfile.TemporaryDirectory() as generator_dir:
                Path(generator_dir, "libopenjp2.pc").write_text("Name: libopenjp2\n")
                os.environ["PKG_CONFIG_PATH"] = "existing-pkgconfig-path"
                recipe = types.SimpleNamespace(
                    generators_folder=generator_dir,
                    options=types.SimpleNamespace(with_openjpeg=True),
                )

                namespace["build"](recipe)

                self.assertEqual(
                    observed["pkg_config_path"].split(os.pathsep),
                    [os.path.abspath(generator_dir), "existing-pkgconfig-path"],
                )
        finally:
            if previous is None:
                os.environ.pop("PKG_CONFIG_PATH", None)
            else:
                os.environ["PKG_CONFIG_PATH"] = previous

    def test_missing_openjpeg_pc_file_fails_before_configure(self):
        patch = textwrap.dedent(self.exporter["PATCH"])
        namespace = {"os": os, "is_msvc": lambda _recipe: True}
        configured = []

        class FakeAutotools:
            def __init__(self, _recipe):
                pass

            def configure(self):
                configured.append(True)

        namespace["Autotools"] = FakeAutotools
        exec("def build(self):\n" + textwrap.indent(patch, "    "), namespace)

        with tempfile.TemporaryDirectory() as generator_dir:
            recipe = types.SimpleNamespace(
                generators_folder=generator_dir,
                options=types.SimpleNamespace(with_openjpeg=True),
            )
            with self.assertRaisesRegex(RuntimeError, "libopenjp2.pc"):
                namespace["build"](recipe)
        self.assertFalse(configured)


if __name__ == "__main__":
    unittest.main()
