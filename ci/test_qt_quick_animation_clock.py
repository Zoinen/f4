import ast
import importlib.util
from pathlib import Path
import runpy
import unittest

root=Path(__file__).parent
spec=importlib.util.spec_from_file_location('patcher',root/'patch-qt-dependencies.py')
patcher=importlib.util.module_from_spec(spec)
spec.loader.exec_module(patcher)
changes=runpy.run_path(str(root/'qt_quick_animation_clock.py'))['REPLACEMENTS']


class AnimationClockPatchTests(unittest.TestCase):
    def execute(self, sources):
        self.saved={}
        def load(recipe,path):
            return sources[Path(path).as_posix().split('scenegraph/')[1]]
        def save(recipe,path,value):
            self.saved[path]=value
        body=patcher._quick_animation_clock_source_patch().replace('        from conan.tools.files import load, save\n','')
        namespace=dict(load=load,save=save,os=__import__('os'))
        exec('def source(self):\n'+body,namespace)
        namespace['source'](type('Recipe',(),{'source_folder':''})())

    def test_strict_atomic_source_edits(self):
        original={name:'\n'.join(before for before,_ in pairs) for name,pairs in changes}
        self.execute(original)
        self.assertEqual(len(self.saved),3)
        self.assertIn('f4.animationClock.v1','\n'.join(self.saved.values()))
        controller=next(value for path,value in self.saved.items() if path.endswith('qquickanimatorcontroller.cpp'))
        self.assertIn('startedAnimations && QThread::currentThread() != m_window->thread()',controller)
        self.assertIn('QCoreApplication::sendPostedEvents(unified, QEvent::MetaCall)',controller)
        for name,pairs in changes:
            for bad in ('drift',original[name]+pairs[0][0]):
                sources=dict(original);sources[name]=bad
                with self.assertRaisesRegex(RuntimeError,'source anchor'):
                    self.execute(sources)
                self.assertEqual(self.saved,{})

    def test_recipe_order_refresh_and_idempotence(self):
        predecessor='        for _f4_path, _f4_source in _f4_upload_sources:\n            save(self, _f4_path, _f4_source)\n'
        recipe='def source(self):\n'+predecessor
        result=patcher._patch_quick_animation_clock(recipe)
        ast.parse(result)
        self.assertEqual(patcher._patch_quick_animation_clock(result),result)
        self.assertNotIn('artifacts/',result)
        self.assertGreater(result.index(patcher._QUICK_ANIMATION_CLOCK_MARKER),result.index(predecessor))
        for bad in ('',recipe+recipe):
            with self.assertRaisesRegex(SystemExit,'predecessor anchor'):
                patcher._patch_quick_animation_clock(bad)


if __name__=='__main__':
    unittest.main()
