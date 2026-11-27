"""Regression tests for Qt's Linux Multimedia recipe patch."""

from __future__ import annotations

import ast
import json
import importlib.util
import os
import pathlib
import runpy
import shutil
import subprocess
import tempfile
import textwrap
import types
import unittest
from unittest.mock import patch


SCRIPT = pathlib.Path(__file__).with_name("patch-qt-dependencies.py")
SPEC = importlib.util.spec_from_file_location("patch_qt_dependencies", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
patcher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(patcher)


class MacOSDynamicHdrDependencyTests(unittest.TestCase):
    def test_video_deadline_patch_is_self_contained_and_applied_once(self):
        old = "class Recipe:\n    def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR
        recipe = patcher._patch_macos_video_presentation(old)
        self.assertEqual(patcher._patch_macos_video_presentation(recipe), recipe)
        payload = (SCRIPT.parent / "patches" / "qt-6.11.1-macos-video-presentation.patch").read_text()
        instance = types.SimpleNamespace(source_folder="/qt")
        calls = []
        # Run the injected fragment as Conan would from its own recipe cache.
        namespace = {"self": instance, "os": os}
        body = recipe.split(patcher._FFMPEG_SOURCE_ANCHOR, 1)[1]
        with patch.dict("sys.modules", {"conan.tools.files": types.SimpleNamespace(
                patch=lambda _, **kwargs: calls.append(kwargs))}):
            exec(textwrap.dedent(body), namespace)
        self.assertEqual(calls, [dict(base_path=os.path.join("/qt", "qtmultimedia"),
                                      strip=1, patch_string=payload)])

    def test_qt_uses_the_matching_ffmpeg_reference_only_on_macos(self):
        recipe = "def requirements(self):\n" + patcher._FFMPEG_REQUIREMENT_PATCH + "            pass\n"
        patched = patcher._patch_macos_ffmpeg_requirement(recipe)
        self.assertEqual(patcher._patch_macos_ffmpeg_requirement(patched), patched)
        namespace = {}
        exec(patched, namespace)
        for os_name, expected in (("Macos", "ffmpeg/7.1.5@f4/macos"),
                                  ("Linux", "ffmpeg/7.1.5"), ("Windows", "ffmpeg/7.1.5")):
            calls = []
            options = types.SimpleNamespace(get_safe={"with_ffmpeg": True}.get)
            instance = types.SimpleNamespace(settings=types.SimpleNamespace(os=os_name),
                                             requires=calls.append, options=options)
            namespace["requirements"](instance)
            self.assertEqual(calls, [expected])

    def test_qt_uses_the_windows_msvc_ffmpeg_recipe(self):
        recipe = "def requirements(self):\n" + patcher._FFMPEG_REQUIREMENT_PATCH + "            pass\n"
        patched = patcher._patch_windows_ffmpeg_requirement(recipe)
        self.assertEqual(patcher._patch_windows_ffmpeg_requirement(patched), patched)
        namespace = {}
        exec(patched, namespace)
        for os_name, expected in (("Windows", "ffmpeg/7.1.5@f4/windows"),
                                  ("Linux", "ffmpeg/7.1.5"), ("Macos", "ffmpeg/7.1.5")):
            calls = []
            options = types.SimpleNamespace(get_safe={"with_ffmpeg": True}.get)
            instance = types.SimpleNamespace(settings=types.SimpleNamespace(os=os_name),
                                             requires=calls.append, options=options)
            namespace["requirements"](instance)
            self.assertEqual(calls, [expected])

    def test_windows_ffmpeg_exporter_exposes_generated_pkg_config_files(self):
        exporter = runpy.run_path(str(SCRIPT.with_name("patch-ffmpeg-windows-recipe.py")))
        recipe = (
            "from conan.tools.gnu import PkgConfigDeps\n"
            "class Recipe:\n"
            "    def generate(self):\n"
            "        deps = PkgConfigDeps(self)\n"
            "        deps.generate()\n"
            "    def build(self):\n"
            "        autotools = Autotools(self)\n"
            "        autotools.configure()\n"
        )
        patched = exporter["patch_recipe"](recipe)
        self.assertEqual(exporter["patch_recipe"](patched), patched)
        self.assertIn('"libopenjp2.pc"', patched)
        self.assertIn('os.environ["PKG_CONFIG_PATH"] = os.pathsep.join(pkg_config_paths)', patched)
        self.assertEqual(patched.count(exporter["MARKER"]), 1)

    def test_ffmpeg_applies_the_exported_patch_from_outside_its_source_folder(self):
        exporter = runpy.run_path(str(SCRIPT.with_name("patch-ffmpeg-macos-recipe.py")))
        recipe = ('class Recipe:\n'
                  '    package_type = "library"\n'
                  '    def source(self):\n'
                  '        get(self, **self.conan_data["sources"][self.version], strip_root=True)\n')
        patched = exporter["patch_recipe"](recipe)
        self.assertEqual(exporter["patch_recipe"](patched), patched)
        namespace = {"os": os, "get": lambda *args, **kwargs: None}
        exec(patched, namespace)
        instance = namespace["Recipe"]()
        self.assertEqual(instance.exports_sources, exporter["PATCH"])
        instance.version = "7.1.5"
        instance.conan_data = {"sources": {instance.version: {}}}
        with tempfile.TemporaryDirectory() as directory:
            instance.export_sources_folder = directory
            source = pathlib.Path(directory, "src")
            source.mkdir()
            instance.source_folder = str(source)
            patch_path = pathlib.Path(directory, instance.exports_sources)
            patch_path.write_text("exported patch", encoding="utf-8")
            applied = []
            def apply(_recipe, patch_file):
                applied.append(pathlib.Path(patch_file).read_text(encoding="utf-8"))
            with patch.dict("sys.modules", {"conan.tools.files": types.SimpleNamespace(patch=apply)}):
                instance.source()
            self.assertEqual(applied, ["exported patch"])


class WindowsHdrCoordinatesPatchTests(unittest.TestCase):
    def test_source_patch_maps_the_screen_origin_once(self):
        recipe = "def source(self):\n        apply_conandata_patches(self)\n"
        patched = patcher._patch_windows_hdr_coordinates(recipe)
        self.assertEqual(patcher._patch_windows_hdr_coordinates(patched), patched)
        source = ('#include "qdxgihdrinfo_p.h"\n'
                  '    QRect wr = w->geometry();\n'
                  '    wr = QRect(wr.topLeft() * w->devicePixelRatio(), wr.size() * w->devicePixelRatio());\n')
        def replace_in_file(_recipe, _path, before, after):
            nonlocal source
            self.assertEqual(source.count(before), 1)
            source = source.replace(before, after)
        namespace = dict(os=__import__("os"), replace_in_file=replace_in_file,
                         apply_conandata_patches=lambda _: None)
        exec(patched, namespace)
        namespace["source"](type("Recipe", (), {"source_folder": ""})())
        self.assertIn("QHighDpi::toNativePixels(w->geometry(), w)", source)
        self.assertNotIn("wr.topLeft() * w->devicePixelRatio()", source)

    def test_ambiguous_recipe_is_rejected(self):
        for recipe in ("", "        apply_conandata_patches(self)\n" * 2):
            with self.subTest(recipe=recipe), self.assertRaises(SystemExit):
                patcher._patch_windows_hdr_coordinates(recipe)


class WindowsQtWebEngineToolchainTests(unittest.TestCase):
    def test_pristine_pdf_recipe_includes_the_toolchain_patch(self):
        recipe = (
            '        "with_gstreamer": [True, False],\n'
            '        "with_gstreamer": False,\n'
            '        self.no_copy_source = not (self.settings_build.os == "Windows" and self.options.get_safe("qtwebengine"))\n'
            '            self.tool_requires(f"qt/{self.version}")\n'
            "        apply_conandata_patches(self)\n"
            '        tc.variables["BUILD_qtrepotools"] = "OFF"\n'
            "        for module in self._get_module_tree:\n"
            "            if not getattr(self.options, module):\n"
            '        if self.options.get_safe("qtwebengine") and qt_quick_enabled:\n'
        )
        patched = patcher._patch_pdf(recipe)
        self.assertIn(patcher._WINDOWS_QWE_SOURCE_MARKER, patched)
        self.assertIn("qwe_setup_toolchain.py", patched)
        self.assertEqual(patcher._patch_pdf(patched), patched)

    def test_pdf_recipe_adds_the_nested_toolchain_patch_once(self):
        recipe = (
            '        "with_pdf": [True, False],\n'
            "    def source(self):\n"
            "        apply_conandata_patches(self)\n"
        )
        patched = patcher._patch_pdf(recipe)
        self.assertIn(patcher._WINDOWS_QWE_SOURCE_MARKER, patched)
        self.assertEqual(patcher._patch_pdf(patched), patched)

    def test_reusable_qt_recipe_gate_requires_the_validated_windows_patch(self):
        bootstrap = (SCRIPT.parent / "patch-qt-recipe.sh").read_text(encoding="utf-8")
        self.assertIn(patcher._WINDOWS_QWE_SOURCE_MARKER, bootstrap)
        self.assertNotIn(
            "# f4: clear inherited MSVC environment before QtWebEngine GN target switch",
            bootstrap,
        )

    def test_source_patch_matches_runner_environment_and_reports_vcvars_errors(self):
        qwe_source = (
            "import os\n"
            "import re\n"
            "import subprocess\n"
            "def _LoadEnvFromBat(args):\n"
            "  \"\"\"Given a bat command, runs it and returns env vars set by it.\"\"\"\n"
            "  env = {}\n"
            "  args = args[:]\n"
            "  args.extend(('&&', 'set'))\n"
            + patcher._WINDOWS_QWE_POPEN_ANCHOR
            + "  variables, _ = popen.communicate()\n"
            + "  if popen.returncode != 0:\n"
            + "    raise Exception('\"%s\" failed with error %d' % (args, popen.returncode))\n"
            + "  output = variables.decode(errors='ignore')\n"
            + "  for line in output.splitlines():\n"
            + "    if re.match(\"^(.*)=(.*)$\", line):\n"
            + "      var, setting = line.split('=', 1)\n"
            + "      env[var] = setting\n"
            + "  return env\n"
        )
        recipe = (
            "def source(self):\n"
            "        apply_conandata_patches(self)\n"
            + patcher._windows_qwe_toolchain_source_patch()
        )
        writes = []

        with tempfile.TemporaryDirectory() as source_folder:
            qwe_path = (
                pathlib.Path(source_folder)
                / "qtwebengine"
                / "src"
                / "3rdparty"
                / "chromium"
                / "build"
                / "toolchain"
                / "win"
                / "qwe_setup_toolchain.py"
            )
            qwe_path.parent.mkdir(parents=True)
            qwe_path.write_text(qwe_source)

            def replace_in_file(_recipe, path, before, after, *, strict):
                self.assertTrue(strict)
                self.assertEqual(path, str(qwe_path))
                source = qwe_path.read_text()
                self.assertEqual(source.count(before), 1)
                writes.append(after)
                qwe_path.write_text(source.replace(before, after))

            namespace = {
                "os": os,
                "apply_conandata_patches": lambda _recipe: None,
                "replace_in_file": replace_in_file,
            }
            exec(recipe, namespace)
            instance = types.SimpleNamespace(
                settings_build=types.SimpleNamespace(os="Windows"),
                source_folder=source_folder,
            )
            namespace["source"](instance)
            namespace["source"](instance)
            self.assertEqual(
                writes,
                [patcher._WINDOWS_QWE_POPEN_PATCH, patcher._WINDOWS_QWE_ERROR_PATCH],
            )

            qwe_namespace = {}
            exec(qwe_path.read_text(), qwe_namespace)
            captured = {}

            class Process:
                returncode = 0

                @staticmethod
                def communicate():
                    return b"PATH=C:/VS/bin\r\nINCLUDE=C:/VS/include\r\n", None

            def fake_popen(args, **kwargs):
                captured["args"] = args
                captured.update(kwargs)
                return Process()

            initial_environment = {
                "PATH": "C:/tools",
                "SYSTEMROOT": "C:/Windows",
                "VSCMD_ARG_TGT_ARCH": "x64",
                "VSINSTALLDIR": "C:/VS",
                "INCLUDE": "C:/old/include",
                "LIB": "C:/old/lib",
                "LIBPATH": "C:/old/libpath",
            }
            with patch.dict(os.environ, initial_environment, clear=True), patch.object(
                subprocess, "Popen", fake_popen
            ):
                result = qwe_namespace["_LoadEnvFromBat"](["vcvarsall.bat", "amd64_x86"])

            self.assertEqual(result, {"PATH": "C:/VS/bin", "INCLUDE": "C:/VS/include"})
            self.assertEqual(captured["args"], ["vcvarsall.bat", "amd64_x86", "&&", "set"])
            self.assertTrue(captured["shell"])
            self.assertEqual(captured["env"], {
                "PATH": "C:/tools",
                "SYSTEMROOT": "C:/Windows",
                "VSINSTALLDIR": "C:/VS",
                "INCLUDE": "C:/old/include",
                "LIB": "C:/old/lib",
                "LIBPATH": "C:/old/libpath",
            })

            class FailedProcess:
                returncode = 255

                @staticmethod
                def communicate():
                    return b"vcvarsall: x86 tools unavailable", None

            with patch.object(subprocess, "Popen", lambda *_args, **_kwargs: FailedProcess()):
                with self.assertRaisesRegex(
                    Exception, "vcvarsall: x86 tools unavailable"
                ):
                    qwe_namespace["_LoadEnvFromBat"](["vcvarsall.bat", "amd64_x86"])

    def test_source_patch_is_skipped_outside_windows(self):
        recipe = "def source(self):\n" + patcher._windows_qwe_toolchain_source_patch()
        namespace = {"os": os}
        exec(recipe, namespace)
        instance = types.SimpleNamespace(
            settings_build=types.SimpleNamespace(os="Linux"), source_folder="/conan/src"
        )
        namespace["source"](instance)


class QtPdfStaticJpegTests(unittest.TestCase):
    def test_pdfium_reuses_the_static_qt_jpeg_package(self):
        recipe = "\n".join((
            '        "with_gstreamer": [True, False],',
            '        "with_gstreamer": False,',
            '        self.no_copy_source = not (self.settings_build.os == "Windows" and self.options.get_safe("qtwebengine"))',
            '            self.tool_requires(f"qt/{self.version}")',
            '        apply_conandata_patches(self)',
            '        tc.variables["BUILD_qtrepotools"] = "OFF"',
            '        for module in self._get_module_tree:',
            '            if not getattr(self.options, module):',
            '        if self.options.get_safe("qtwebengine") and qt_quick_enabled:',
        )) + "\n"

        patched = patcher._patch_pdf(recipe)

        self.assertIn("QT_FEATURE_webengine_qt_libjpeg ON", patched)
        self.assertIn("target_link_libraries(Pdf PRIVATE JPEG::JPEG)", patched)
        self.assertIn("JPEG_INCLUDES ${pdfJpegIncludes}", patched)
        self.assertEqual(patcher._patch_pdf(patched), patched)


class Md4cRequirementPatchTests(unittest.TestCase):
    def test_pins_md4c_before_incompatible_06(self):
        recipe = (
            "def requirements(self):\n"
            "        if self.options.get_safe(\"with_md4c\", False):\n"
            + patcher._MD4C_REQUIREMENT_ANCHOR
        )
        patched = patcher._patch_md4c(recipe)

        self.assertIn(patcher._MD4C_REQUIREMENT_MARKER, patched)
        ast.parse(patched)
        self.assertEqual(patcher._patch_md4c(patched), patched)

    def test_missing_or_ambiguous_requirement_is_rejected(self):
        for recipe in ("", patcher._MD4C_REQUIREMENT_ANCHOR * 2):
            with self.subTest(recipe=recipe), self.assertRaisesRegex(
                SystemExit, "md4c requirement"
            ):
                patcher._patch_md4c(recipe)


class QuickUploadPolicyPatchTests(unittest.TestCase):
    def test_portable_cache_keys_include_embedded_order_scratch_source(self):
        workflow = SCRIPT.parent.parent / ".github" / "workflows" / "build.yml"
        keys = [line for line in workflow.read_text(encoding="utf-8").splitlines()
                if "hashFiles(" in line and "'ci/qt_quick_upload_policy.py'" in line]
        self.assertTrue(keys)
        for key in keys:
            self.assertIn("'ci/qt_quick_order_scratch.h'", key)

    def test_order_scratch_storage_semantics_and_allocation_bound(self):
        configured_compiler = os.environ.get("CXX", "")
        compiler = (shutil.which("cl") or shutil.which(configured_compiler)
                    or shutil.which("g++") or shutil.which("clang++"))
        if compiler is None:
            self.skipTest("A C++ compiler is required for the portable scratch-storage regression")
        source = SCRIPT.parent / "tests" / "qt_quick_order_scratch_test.cpp"
        with tempfile.TemporaryDirectory() as temporary:
            directory = pathlib.Path(temporary)
            if pathlib.Path(compiler).stem.lower() == "cl":
                executable = directory / "scratch.exe"
                command = [compiler, "/nologo", "/std:c++20", "/O2", "/EHsc", "/MT",
                           str(source), "/Fe:" + str(executable), "/Fo:" + str(directory / "scratch.obj")]
            else:
                executable = directory / "scratch"
                command = [compiler, "-std=c++20", "-O2", "-pthread", str(source), "-o", str(executable)]
            compiled = subprocess.run(command, cwd=directory, capture_output=True, text=True)
            self.assertEqual(compiled.returncode, 0, compiled.stdout + compiled.stderr)
            result = subprocess.run([str(executable)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            evidence = json.loads(result.stdout)
            self.assertEqual(evidence["lookupComparisons"], 1048576)
            self.assertEqual(evidence["steadyStateSnapshots"], 4000)
            self.assertEqual(evidence["steadyStateAllocations"], 0)

    def predecessors(self):
        recipe = "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR
        for apply in (patcher._patch_quick_batch_orders,
                      patcher._patch_quick_blocked_opacity,
                      patcher._patch_quick_stable_intervals,
                      patcher._patch_quick_buffer_pool_fit):
            recipe = apply(recipe)
        return recipe

    def test_recipe_is_self_contained_idempotent_and_ordered(self):
        recipe = patcher._patch_quick_upload_policy(self.predecessors())
        ast.parse(recipe)
        self.assertEqual(patcher._patch_quick_upload_policy(recipe), recipe)
        self.assertNotIn("artifacts/", recipe)
        self.assertNotIn("qt_quick_upload_policy.py", recipe)
        self.assertGreater(recipe.index(patcher._QUICK_UPLOAD_POLICY_MARKER),
                           recipe.index("save(self, _f4_quick_path, _f4_quick_source)"))
        self.assertGreater(recipe.index(patcher._QUICK_UPLOAD_POLICY_MARKER),
                           recipe.index(patcher._QUICK_BUFFER_POOL_FIT_MARKER))

    def test_missing_or_ambiguous_predecessors_are_rejected(self):
        for recipe in ("def source(self):\n    pass\n", self.predecessors() * 2):
            with self.subTest(recipe=recipe), self.assertRaisesRegex(SystemExit, "predecessor anchor"):
                patcher._patch_quick_upload_policy(recipe)

    def inputs(self):
        changes = runpy.run_path(str(SCRIPT.with_name("qt_quick_upload_policy.py")))["REPLACEMENTS"]
        return changes, {name: "\n// source interval boundary\n".join(before for before, _ in pairs)
                         for name, pairs in changes}

    def execute(self, sources):
        self.saved = {}
        def load(_recipe, path):
            return sources[pathlib.Path(path).name]
        def save(_recipe, path, value):
            self.saved[pathlib.Path(path).name] = value
        body = patcher._quick_upload_policy_source_patch().replace(
            "        from conan.tools.files import load, save\n", "")
        namespace = dict(os=__import__("os"), load=load, save=save)
        exec("def source(self):\n" + body, namespace)
        namespace["source"](type("Recipe", (), {"source_folder": ""})())

    def test_source_patch_exports_both_capability_and_renderer(self):
        changes, sources = self.inputs()
        self.execute(sources)
        self.assertEqual(set(self.saved), {name for name, _ in changes})
        self.assertIn("F4VectorGateFrame f4GateFrame", self.saved["qsgbatchrenderer.cpp"])
        self.assertIn("previousOpaque=f4UnmergedPositions", self.saved["qsgbatchrenderer.cpp"])
        self.assertIn("QScopedValueRollback<int *> paddingScope", self.saved["qsgbatchrenderer.cpp"])
        self.assertIn("f4GateFrame.finish()", self.saved["qsgbatchrenderer.cpp"])
        self.assertIn("f4.vectorUploadGate.v2", self.saved["qsgrendererinterface.cpp"])

    def test_source_drift_saves_neither_file(self):
        changes, original = self.inputs()
        for name, pairs in changes:
            for value in ("upstream changed", original[name] + pairs[0][0]):
                sources = dict(original)
                sources[name] = value
                with self.subTest(name=name), self.assertRaisesRegex(RuntimeError, "source anchor"):
                    self.execute(sources)
                self.assertEqual(self.saved, {})

    def test_upload_policy_retains_bounded_geometry_working_set(self):
        _, sources = self.inputs()
        self.execute(sources)
        renderer = self.saved['qsgbatchrenderer.cpp']
        self.assertIn('const quint32 DEFAULT_BUFFER_POOL_SIZE_LIMIT = 16 * 1024 * 1024;', renderer)
        self.assertNotIn('const quint32 DEFAULT_BUFFER_POOL_SIZE_LIMIT = 2 * 1024 * 1024;', renderer)

    def test_unmerged_order_snapshots_reuse_bounded_thread_local_storage(self):
        _, sources = self.inputs()
        self.execute(sources)
        renderer = self.saved['qsgbatchrenderer.cpp']
        self.assertIn('namespace F4OrderScratch', renderer)
        self.assertIn('thread_local std::array<Slot<Key>, maximumLeases>', renderer)
        self.assertIn('maximumElements = 16384', renderer)
        self.assertIn('maximumLeases = 4', renderer)
        self.assertIn('F4OrderScratch::Lease<Element *> storage', renderer)
        self.assertIn('sequence.storage = F4OrderScratch::Lease<Element *>::acquire', renderer)
        self.assertIn('previous.at(oldLow+i)!=current.at(newLow+i)', renderer)
        self.assertIn('if (!sequence.storage)', renderer)
        self.assertNotIn('F4_PROFILE_', renderer)

    def test_small_uploads_preserve_large_pooled_buffers(self):
        _, sources = self.inputs()
        self.execute(sources)
        renderer = self.saved['qsgbatchrenderer.cpp']
        self.assertIn('buffer->size <= 8192', renderer)
        self.assertIn('qMax(quint32(4096), buffer->size * 8)', renderer)
        self.assertIn('bufferPool->isEmpty() || !f4PoolUsable', renderer)
        self.assertIn('testBuffer->size() > f4MaxPoolCapacity', renderer)
        self.assertNotIn('F4_PROFILE_SMALL_FIT_DISABLE', renderer)

    def test_curve_preprocessing_preserves_texture_and_custom_callbacks(self):
        _, sources = self.inputs()
        self.execute(sources)
        renderer = self.saved["qsgrenderer.cpp"]
        self.assertNotIn("QSGCurveStrokeNode", renderer)
        self.assertIn("typeid(*node)==typeid(QSGCurveFillNode)", renderer)
        self.assertIn("fillTextureProvider()!=nullptr", renderer)
        self.assertIn("!(state & QSGNode::DirtyNodeRemoved)", renderer)
        self.assertIn("state & QSGNode::DirtyMaterial", renderer)
        self.assertIn("defined(__cpp_rtti) || defined(_CPPRTTI)", renderer)
        self.assertNotIn("F4_PROFILE_", renderer)
        self.assertNotIn("F4RenderEntryScope", renderer)

    def test_visible_svg_admission_tracks_geometry_membership(self):
        _, sources = self.inputs()
        self.execute(sources)
        renderer = self.saved["qsgbatchrenderer.cpp"]
        self.assertIn("QSet<QSGNode *> knownNodes", renderer)
        self.assertIn("!f4GateKnowsBatch(gate.data(), batch)", renderer)
        self.assertIn("f4ForgetKnownVectorNode(node->sgNode)", renderer)
        self.assertIn("f4RememberKnownVectorNodes(gate.data(), current.value(gate.data()))", renderer)
        self.assertNotIn("F4_TRACE_VISIBLE_GATES", renderer)
        self.assertNotIn("F4_PROFILE_BUFFER_CONTENTS", renderer)

    def test_existing_recipe_refreshes_upload_policy_payload(self):
        current = patcher._patch_quick_upload_policy(self.predecessors())
        old = current.replace('DEFAULT_BUFFER_POOL_SIZE_LIMIT = 16 *',
                              'DEFAULT_BUFFER_POOL_SIZE_LIMIT = 2 *')
        self.assertNotEqual(old, current)
        suffix = '\n        # unrelated following patch\n'
        self.assertEqual(patcher._patch_quick_upload_policy(old + suffix), current + suffix)

    def test_existing_recipe_refresh_rejects_missing_or_duplicate_boundaries(self):
        current = patcher._patch_quick_upload_policy(self.predecessors())
        for bad in (current + current,
                    current.replace('        for _f4_path, _f4_source in _f4_upload_sources:', ''),
                    current.replace('        ' + patcher._QUICK_UPLOAD_POLICY_MARKER, patcher._QUICK_UPLOAD_POLICY_MARKER)):
            with self.subTest(recipe=bad), self.assertRaisesRegex(SystemExit, 'refresh anchor'):
                patcher._patch_quick_upload_policy(bad)


class QuickBufferPoolFitPatchTests(unittest.TestCase):
    def test_recipe_is_idempotent_self_contained_and_composes(self):
        recipe = "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR
        recipe = patcher._patch_quick_batch_orders(recipe)
        recipe = patcher._patch_quick_blocked_opacity(recipe)
        recipe = patcher._patch_quick_stable_intervals(recipe)
        recipe = patcher._patch_quick_child_window_stacking(recipe)
        patched = patcher._patch_quick_buffer_pool_fit(recipe)
        ast.parse(patched)
        self.assertEqual(patcher._patch_quick_buffer_pool_fit(patched), patched)
        self.assertIn(patcher._QUICK_STABLE_INTERVALS_MARKER, patched)
        self.assertIn(patcher._QUICK_CHILD_WINDOW_STACKING_MARKER, patched)
        self.assertNotIn("artifacts/", patched)

    def execute(self, source):
        self.saved = []
        def load(_recipe, path):
            self.assertEqual(pathlib.Path(path).as_posix(),
                "qtdeclarative/src/quick/scenegraph/coreapi/qsgbatchrenderer.cpp")
            return source
        def save(_recipe, path, value):
            self.saved.append(value)
        body = patcher._QUICK_BUFFER_POOL_FIT_SOURCE_PATCH.replace(
            "        from conan.tools.files import load, save\n", "")
        namespace = dict(os=__import__("os"), load=load, save=save)
        exec("def source(self):\n" + body, namespace)
        namespace["source"](type("Recipe", (), {"source_folder": ""})())
        return self.saved[0]

    def test_source_patch_preserves_the_rest_of_selection(self):
        original = "select() {\n" + patcher._QUICK_BUFFER_POOL_FIT_BEFORE + "\n    remainingSelection();\n}\n"
        expected = original.replace(patcher._QUICK_BUFFER_POOL_FIT_BEFORE,
                                    patcher._QUICK_BUFFER_POOL_FIT_AFTER)
        self.assertEqual(self.execute(original), expected)
        self.assertEqual(len(self.saved), 1)

    def test_source_drift_is_rejected_without_saving(self):
        for source in ("changed", patcher._QUICK_BUFFER_POOL_FIT_BEFORE * 2):
            with self.subTest(source=source):
                with self.assertRaisesRegex(RuntimeError, "source anchor changed or is ambiguous"):
                    self.execute(source)
                self.assertEqual(self.saved, [])

    def test_recipe_anchor_is_required_and_unambiguous(self):
        for recipe in ("def source(self):\n    pass\n",
                       "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR * 2):
            with self.subTest(recipe=recipe), self.assertRaisesRegex(SystemExit, "source patch anchor"):
                patcher._patch_quick_buffer_pool_fit(recipe)


class QuickChildWindowStackingPatchTests(unittest.TestCase):
    def test_recipe_is_idempotent_and_composes_with_existing_patches(self):
        recipe = "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR
        recipe = patcher._patch_quick_batch_orders(recipe)
        recipe = patcher._patch_quick_blocked_opacity(recipe)
        recipe = patcher._patch_quick_stable_intervals(recipe)
        patched = patcher._patch_quick_child_window_stacking(recipe)
        ast.parse(patched)
        self.assertEqual(patcher._patch_quick_child_window_stacking(patched), patched)
        self.assertIn(patcher._QUICK_STABLE_INTERVALS_MARKER, patched)
        self.assertNotIn("artifacts/", patched)

    def execute(self, source):
        self.saved = []
        def load(_recipe, path):
            self.assertEqual(pathlib.Path(path).as_posix(),
                             "qtdeclarative/src/quick/items/qquickwindow.cpp")
            return source
        def save(_recipe, path, value):
            self.saved.append(value)
        body = patcher._QUICK_CHILD_WINDOW_STACKING_SOURCE_PATCH.replace(
            "        from conan.tools.files import load, save\n", "")
        namespace = dict(os=__import__("os"), load=load, save=save)
        exec("def source(self):\n" + body, namespace)
        namespace["source"](type("Recipe", (), {"source_folder": ""})())
        return self.saved[0]

    def test_source_patch_retains_the_existing_stacking_walk(self):
        original = ("void update() {\n" + patcher._QUICK_CHILD_WINDOW_STACKING_BEFORE
                    + "        item = contentItem;\n    }\n    walkChildren(item);\n}\n")
        expected = original.replace(patcher._QUICK_CHILD_WINDOW_STACKING_BEFORE,
                                    patcher._QUICK_CHILD_WINDOW_STACKING_AFTER)
        self.assertEqual(self.execute(original), expected)
        self.assertEqual(len(self.saved), 1)

    def test_source_drift_is_rejected_without_saving(self):
        for source in ("changed", patcher._QUICK_CHILD_WINDOW_STACKING_BEFORE * 2):
            with self.subTest(source=source):
                with self.assertRaisesRegex(RuntimeError, "source anchor changed or is ambiguous"):
                    self.execute(source)
                self.assertEqual(self.saved, [])

    def test_recipe_anchor_is_required_and_unambiguous(self):
        for recipe in ("def source(self):\n    pass\n",
                       "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR * 2):
            with self.subTest(recipe=recipe), self.assertRaisesRegex(SystemExit, "source patch anchor"):
                patcher._patch_quick_child_window_stacking(recipe)


class QuickBatchRootOrderPatchTests(unittest.TestCase):
    def test_recipe_patch_is_idempotent_and_valid_python(self) -> None:
        recipe = "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR
        patched = patcher._patch_quick_batch_orders(recipe)
        ast.parse(patched)
        self.assertEqual(patcher._patch_quick_batch_orders(patched), patched)
        self.assertIn(patcher._QUICK_BATCH_ORDERS_MARKER, patched)

    def test_source_patch_balances_every_ancestor_reservation(self) -> None:
        source = patcher._QUICK_BATCH_ORDERS_BEFORE

        def replace_in_file(_recipe, path, before, after, strict):
            nonlocal source
            self.assertEqual(pathlib.Path(path).as_posix(),
                             "qtdeclarative/src/quick/scenegraph/coreapi/qsgbatchrenderer.cpp")
            self.assertTrue(strict)
            self.assertEqual(source.count(before), 1)
            source = source.replace(before, after)

        recipe = type("Recipe", (), {"source_folder": ""})()
        namespace = {"os": __import__("os"), "replace_in_file": replace_in_file}
        exec("def source(self):\n" + patcher._QUICK_BATCH_ORDERS_SOURCE_PATCH, namespace)
        namespace["source"](recipe)
        self.assertEqual(source, patcher._QUICK_BATCH_ORDERS_AFTER)
        self.assertIn("info->parentRoot", source)

    def test_unknown_recipe_source_anchor_is_rejected(self) -> None:
        with self.assertRaisesRegex(SystemExit, "source patch anchor"):
            patcher._patch_quick_batch_orders("def source(self):\n    pass\n")


class QuickBlockedOpacityPatchTests(unittest.TestCase):
    def test_recipe_patch_is_idempotent_and_composes_with_order_fix(self) -> None:
        recipe = "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR
        recipe = patcher._patch_quick_batch_orders(recipe)
        patched = patcher._patch_quick_blocked_opacity(recipe)
        ast.parse(patched)
        self.assertEqual(patcher._patch_quick_blocked_opacity(patched), patched)
        self.assertIn(patcher._QUICK_BATCH_ORDERS_MARKER, patched)
        self.assertIn(patcher._QUICK_BLOCKED_OPACITY_MARKER, patched)

    def test_strict_source_patch_preserves_existing_subtree_rebuild(self) -> None:
        source = patcher._QUICK_BLOCKED_OPACITY_BEFORE

        def replace_in_file(_recipe, path, before, after, strict):
            nonlocal source
            self.assertEqual(pathlib.Path(path).as_posix(),
                             "qtdeclarative/src/quick/scenegraph/coreapi/qsgbatchrenderer.cpp")
            self.assertTrue(strict)
            self.assertEqual(source.count(before), 1)
            source = source.replace(before, after)

        recipe = type("Recipe", (), {"source_folder": ""})()
        namespace = {"os": __import__("os"), "replace_in_file": replace_in_file}
        exec("def source(self):\n" + patcher._QUICK_BLOCKED_OPACITY_SOURCE_PATCH, namespace)
        namespace["source"](recipe)
        self.assertEqual(source, patcher._QUICK_BLOCKED_OPACITY_AFTER)
        self.assertIn("&& sn", source)
        self.assertIn("m_rebuild |= FullRebuild", source)

    def test_unknown_recipe_anchor_is_rejected(self) -> None:
        with self.assertRaisesRegex(SystemExit, "source patch anchor"):
            patcher._patch_quick_blocked_opacity("def source(self):\n    pass\n")


class QuickStableIntervalsPatchTests(unittest.TestCase):
    def recipe(self):
        text = "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR
        text = patcher._patch_quick_batch_orders(text)
        return patcher._patch_quick_blocked_opacity(text)

    def test_recipe_is_idempotent_self_contained_and_ordered(self):
        patched = patcher._patch_quick_stable_intervals(self.recipe())
        ast.parse(patched)
        self.assertEqual(patcher._patch_quick_stable_intervals(patched), patched)
        marker = patched.index(patcher._QUICK_STABLE_INTERVALS_MARKER)
        self.assertGreater(marker, patched.index(patcher._QUICK_BATCH_ORDERS_MARKER))
        self.assertGreater(marker, patched.index(patcher._QUICK_BLOCKED_OPACITY_MARKER))
        self.assertNotIn("run_path", patched)
        self.assertNotIn("artifacts/", patched)

    def test_missing_predecessor_or_ambiguous_anchor_is_rejected(self):
        for text in ("def source(self):\n    pass\n",
                     "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR,
                     self.recipe() + patcher._QUICK_BATCH_ORDERS_SOURCE_PATCH):
            with self.subTest(text=text), self.assertRaisesRegex(SystemExit, "stable intervals"):
                patcher._patch_quick_stable_intervals(text)

    def test_existing_recipe_refreshes_lazy_batch_root_initialization(self):
        current = patcher._patch_quick_stable_intervals(self.recipe())
        old = current.replace("batchRootInfo(parent)", "parent->rootInfo()")
        self.assertNotEqual(old, current)
        self.assertEqual(patcher._patch_quick_stable_intervals(old), current)

    def test_existing_recipe_refresh_rejects_ambiguous_boundaries(self):
        current = patcher._patch_quick_stable_intervals(self.recipe())
        end = "        save(self, _f4_quick_path, _f4_quick_source)\n"
        for broken in (current + current, current.replace(end, "")):
            with self.subTest(recipe=broken), self.assertRaisesRegex(
                    SystemExit, "stable intervals refresh anchors"):
                patcher._patch_quick_stable_intervals(broken)

    def execute(self, source):
        saved = []
        def replace_in_file(_recipe, path, before, after, strict):
            nonlocal source
            self.assertTrue(strict)
            self.assertEqual(source.count(before), 1)
            source = source.replace(before, after)
        def load(_recipe, path):
            self.assertEqual(pathlib.Path(path).as_posix(),
                             "qtdeclarative/src/quick/scenegraph/coreapi/qsgbatchrenderer.cpp")
            return source
        def save(_recipe, path, value):
            saved.append(value)
        # Execute the actual generated body without depending on a Conan install.
        # Only the local import is supplied by this test; replacement logic is real.
        body = patcher._patch_quick_stable_intervals(self.recipe()).replace(
            "        from conan.tools.files import load, save\n", "")
        namespace = dict(os=__import__("os"), load=load, save=save,
                         replace_in_file=replace_in_file, apply_conandata_patches=lambda _: None)
        exec(body, namespace)
        recipe = type("Recipe", (), {"source_folder": ""})()
        namespace["source"](recipe)
        self.assertEqual(len(saved), 1)
        return saved[0]

    def fixture(self):
        replacements = runpy.run_path(str(SCRIPT.with_name("qt_quick_stable_intervals.py")))["REPLACEMENTS"]
        self.assertIn("#define DECLARE_DEBUG_VAR(variable) " + chr(92) + "\n", replacements[0][0])
        source = "\n// fixture gap\n".join(before for before, _ in replacements)
        # The retained patch consumes the earlier ancestor-accounting correction.
        source = source.replace(patcher._QUICK_BATCH_ORDERS_AFTER,
                                patcher._QUICK_BATCH_ORDERS_BEFORE)
        source += "\n" + patcher._QUICK_BLOCKED_OPACITY_BEFORE
        return source, replacements

    def test_source_composition_retains_batches_and_balances_intervals(self):
        source, replacements = self.fixture()
        expected = source.replace(patcher._QUICK_BATCH_ORDERS_BEFORE,
                                  patcher._QUICK_BATCH_ORDERS_AFTER).replace(
                                      patcher._QUICK_BLOCKED_OPACITY_BEFORE,
                                      patcher._QUICK_BLOCKED_OPACITY_AFTER)
        for before, after in replacements:
            self.assertEqual(expected.count(before), 1)
            expected = expected.replace(before, after)
        self.assertEqual(self.execute(source), expected)
        self.assertIn("f4PendingClips.remove(this)", expected)

    def test_changed_or_duplicate_source_anchor_is_rejected_before_save(self):
        source, replacements = self.fixture()
        before = replacements[0][0]
        for broken in (source.replace(before, "// upstream changed\n"), source + before):
            with self.subTest(duplicate=broken.endswith(before)), self.assertRaisesRegex(
                    RuntimeError, "Qt Quick stable intervals source anchor"):
                self.execute(broken)


class MacColorSpaceRecipePatchTests(unittest.TestCase):
    def test_recipe_patch_is_idempotent_and_valid_python(self) -> None:
        recipe = "def source(self):\n" + patcher._FFMPEG_SOURCE_ANCHOR
        patched = patcher._patch_macos_colorspace(recipe)
        ast.parse(patched)
        self.assertEqual(patcher._patch_macos_colorspace(patched), patched)
        self.assertIn("CGColorSpaceRetain(colorSpace)", patched)
        self.assertEqual(patched.count("QCFType<CGColorSpaceRef> colorSpace = cgImageFormat->colorSpace"), 3)

    def test_source_patch_applies_to_all_consumers(self) -> None:
        sources: dict[str, str] = {}
        for path, before, _ in patcher._MACOS_COLORSPACE_REPLACEMENTS:
            sources[path] = sources.get(path, "") + before + "\n"

        def replace_in_file(_recipe, path, before, after, strict):
            relative = pathlib.Path(path).relative_to("qtbase/src/gui").as_posix()
            self.assertTrue(strict)
            self.assertEqual(sources[relative].count(before), 1)
            sources[relative] = sources[relative].replace(before, after)

        recipe = type("Recipe", (), {"source_folder": ""})()
        code = "def source(self):\n" + patcher._MACOS_COLORSPACE_SOURCE_PATCH
        namespace = {"os": __import__("os"), "replace_in_file": replace_in_file}
        exec(code, namespace)
        namespace["source"](recipe)
        for path, _, after in patcher._MACOS_COLORSPACE_REPLACEMENTS:
            self.assertIn(after, sources[path])


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

    def test_linux_arm64_cfi_generate_patch_is_idempotent_and_targets_gcc11(self) -> None:
        recipe = (
            "class Recipe:\n"
            "    def source(self):\n"
            "        apply_conandata_patches(self)\n"
            "    def generate(self):\n"
            "        pass\n"
        )
        patched = patcher._patch_linux_arm64_cfi(recipe)
        ast.parse(patched)
        self.assertEqual(patcher._patch_linux_arm64_cfi(patched), patched)
        self.assertIn(patcher._LINUX_ARM64_CFI_MARKER, patched)
        source_method = patched.split("    def generate(self):", 1)[0]
        self.assertNotIn("self.settings", source_method)

        def replace_in_file(_recipe, path, before, after, *, strict):
            self.assertTrue(strict)
            source = pathlib.Path(path).read_text()
            self.assertIn(before, source)
            pathlib.Path(path).write_text(source.replace(before, after, 1))

        namespace = {"os": os, "replace_in_file": replace_in_file}
        exec("class Recipe:\n    def generate(self):\n" +
             patcher._LINUX_ARM64_CFI_GENERATE_PATCH, namespace)

        with tempfile.TemporaryDirectory() as tmp:
            source_root = pathlib.Path(tmp) / "qtwebengine" / "src"
            core_path = source_root / "core" / "CMakeLists.txt"
            pdf_path = source_root / "pdf" / "CMakeLists.txt"
            core_path.parent.mkdir(parents=True)
            pdf_path.parent.mkdir(parents=True)
            core_path.write_text(patcher._LINUX_ARM64_CFI_CMAKE_BEFORE)
            pdf_path.write_text("        set(gnArgArg ${gnArgArg})\n" +
                                patcher._LINUX_ARM64_PDF_CFI_ANCHOR)

            instance = namespace["Recipe"]()
            instance.settings = types.SimpleNamespace(os="Linux", arch="armv8")
            instance.source_folder = tmp
            instance.generate()

            core_source = core_path.read_text()
            self.assertIn(patcher._LINUX_ARM64_CFI_MARKER, core_source)
            self.assertIn('arm_control_flow_integrity="none"', core_source)
            self.assertNotIn('arm_control_flow_integrity="standard"', core_source)
            pdf_source = pdf_path.read_text()
            self.assertIn(patcher._LINUX_ARM64_PDF_CFI_MARKER, pdf_source)
            self.assertIn('if(LINUX AND arch STREQUAL "aarch64" AND', pdf_source)
            self.assertNotIn("cpu STREQUAL", pdf_source)
            self.assertIn('list(APPEND gnArgArg arm_control_flow_integrity="none")',
                          pdf_source)

            # A resumed Conan build may run generate() over a source tree already
            # patched by an earlier recipe revision. Both patches must be stable.
            instance.generate()
            self.assertEqual(core_path.read_text(), core_source)
            self.assertEqual(pdf_path.read_text(), pdf_source)

            # Upgrade the output of the pre-marker patch used by the interrupted
            # CI run, then confirm subsequent retries remain idempotent.
            core_path.write_text(patcher._LINUX_ARM64_CFI_CMAKE_LEGACY_AFTER)
            instance.generate()
            upgraded_core = core_path.read_text()
            self.assertIn(patcher._LINUX_ARM64_CFI_MARKER, upgraded_core)
            self.assertIn('arm_control_flow_integrity="none"', upgraded_core)
            instance.generate()
            self.assertEqual(core_path.read_text(), upgraded_core)

    def test_linux_arm64_cfi_patch_rejects_an_unknown_generate_anchor(self) -> None:
        for recipe in ("class Recipe:\n    def source(self):\n        pass\n",
                       "class Recipe:\n    def generate(self):\n        pass\n"
                       "    def generate(self):\n        pass\n"):
            with self.subTest(recipe=recipe), self.assertRaisesRegex(
                SystemExit, "Linux ARM64 generate patch anchor"
            ):
                patcher._patch_linux_arm64_cfi(recipe)

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
