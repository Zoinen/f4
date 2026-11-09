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
import unittest


SCRIPT = pathlib.Path(__file__).with_name("patch-qt-dependencies.py")
SPEC = importlib.util.spec_from_file_location("patch_qt_dependencies", SCRIPT)
assert SPEC is not None and SPEC.loader is not None
patcher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(patcher)


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


class Md4cRequirementPatchTests(unittest.TestCase):
    def test_pins_md4c_before_incompatible_06(self):
        recipe = patcher._MD4C_REQUIREMENT_ANCHOR
        patched = patcher._patch_md4c(recipe)

        self.assertIn(patcher._MD4C_REQUIREMENT_MARKER, patched)
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
