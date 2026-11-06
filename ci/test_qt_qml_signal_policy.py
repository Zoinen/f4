"""Source and recipe contracts for Qt's wider conservative QML signal mask."""
import ast
import importlib.util
import os
from pathlib import Path
import runpy
import types
import unittest

SCRIPT = Path(__file__).with_name('patch-qt-dependencies.py')
SPEC = importlib.util.spec_from_file_location('signal_mask_patcher', SCRIPT)
patcher = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(patcher)
CHANGES = runpy.run_path(str(SCRIPT.with_name('qt_qml_signal_policy.py')))['REPLACEMENTS']


class QmlSignalMaskPolicyTests(unittest.TestCase):
    def execute(self, source):
        self.saved = []
        body = patcher._qml_signal_mask_source_patch().replace(
            '        from conan.tools.files import load, save\n', '')
        recipe = 'def apply(self):\n' + body
        namespace = dict(os=os, load=lambda _self, _path: source,
                         save=lambda _self, path, text: self.saved.append((path, text)))
        exec(compile(recipe, '<signal-mask-recipe>', 'exec'), namespace)
        namespace['apply'](types.SimpleNamespace(source_folder='/qt-source'))

    def fixture(self):
        return '\n// distinct source intervals\n'.join(old for old, _ in CHANGES)

    def test_recipe_self_contained_and_idempotent(self):
        original = 'def source(self):\n' + patcher._FFMPEG_SOURCE_ANCHOR
        recipe = patcher._patch_qml_signal_mask(original)
        ast.parse(recipe)
        self.assertEqual(recipe, patcher._patch_qml_signal_mask(recipe))
        self.assertNotIn('artifacts/', recipe)
        self.assertNotIn('qt_qml_signal_policy.py', recipe)
        self.assertNotIn('F4_PROFILE_', recipe)

    def test_reversible_source_and_existing_prefix(self):
        original = self.fixture()
        self.execute(original)
        self.assertEqual(len(self.saved), 1)
        path, patched = self.saved[0]
        self.assertEqual(Path(path).as_posix(), '/qt-source/qtdeclarative/src/qml/qml/qqmlengine.cpp')
        self.assertIn('QAtomicInteger<quint64> masks[8]', patched)
        self.assertIn('list->connectionMask.storeRelaxed(', patched)
        self.assertIn('delete static_cast<F4SignalNotifyList *>(list)', patched)
        for old, new in reversed(CHANGES):
            self.assertEqual(patched.count(new), 1)
            patched = patched.replace(new, old)
        self.assertEqual(patched, original)

    def test_changed_or_ambiguous_source_is_not_saved(self):
        for old, _ in CHANGES:
            for source in (self.fixture().replace(old, '// changed anchor'), self.fixture() + '\n' + old):
                with self.subTest(anchor=old), self.assertRaisesRegex(RuntimeError, 'anchor'):
                    self.execute(source)
                self.assertEqual(self.saved, [])

    def test_refreshes_existing_recipe_block(self):
        original = 'def source(self):\n' + patcher._FFMPEG_SOURCE_ANCHOR
        recipe = patcher._patch_qml_signal_mask(original)
        stale = recipe.replace('masks[8]', 'masks[4]')
        self.assertNotEqual(stale, recipe)
        self.assertEqual(patcher._patch_qml_signal_mask(stale), recipe)

    def test_invalid_recipe_anchors_rejected(self):
        for recipe in ('def source(self):\n    pass\n', patcher._FFMPEG_SOURCE_ANCHOR * 2,
                       patcher._QML_SIGNAL_MASK_MARKER):
            with self.subTest(recipe=recipe), self.assertRaises(SystemExit):
                patcher._patch_qml_signal_mask(recipe)

    def test_portable_cache_keys_include_source_policy(self):
        workflow = SCRIPT.parent.parent / '.github/workflows/build.yml'
        keys = [line for line in workflow.read_text(encoding='utf8').splitlines()
                if 'key: portable-conan-packages-' in line and 'hashFiles(' in line]
        self.assertEqual(len(keys), 6)
        for key in keys:
            self.assertIn("'ci/qt_qml_signal_policy.py'", key)


if __name__ == '__main__':
    unittest.main()
