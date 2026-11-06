import copy
import math
import unittest

from wheel_curve_analysis import analyze, pair_error, position


class WheelCurveAnalysisTests(unittest.TestCase):
    def setUp(self):
        self.curve = dict(timeNs=1, job=2, revision=3, animationMs=25,
                          beforeStartMs=0, beforeFrom=0, afterStartMs=0, afterFrom=0,
                          destination=100, durationMs=100, curve='OutSine', motionIndex=0,
                          continuous=False)
        self.before = dict(timeNs=1, job=2, revision=3, animationMs=25,
                           geometryOffset=0, visualY=position(self.curve, 25),
                           continuous=False)
        self.before.update(renderedY=-self.before['visualY'], requestedY=self.before['visualY'])
        self.next_curve = dict(self.curve, timeNs=2, animationMs=31.25, motionIndex=1)
        self.after = dict(self.before, timeNs=2, animationMs=31.25,
                          visualY=position(self.curve, 31.25), requestedY=position(self.curve, 31.25))
        self.after['renderedY'] = -self.after['visualY']

    def error(self):
        return pair_error(self.before, self.after, self.curve, self.next_curve, 6_250_000, 1.75)

    def test_supported_curve_is_admitted_and_clock_jitter_is_measured(self):
        self.assertLess(self.error()['displayClockModelErrorPx'], 1e-10)
        self.after['animationMs'] += 1
        self.next_curve['animationMs'] += 1
        self.after['visualY'] = self.after['requestedY'] = position(self.curve, 32.25)
        self.after['renderedY'] = -self.after['visualY']
        self.assertGreater(self.error()['displayClockModelErrorPx'], 2)
        self.assertLess(self.error()['nativeRequestedErrorPx'], 1e-10)

    def test_coverage_hold_is_an_error_sample_not_an_exclusion(self):
        self.after['visualY'] = self.before['visualY']
        self.after['renderedY'] = -self.after['visualY']
        result = self.error()
        self.assertTrue(result['coverageLimited'])
        self.assertGreater(result['nativeCameraErrorPx'], 2)

    def test_continuation_after_coverage_reset_is_used(self):
        self.curve.update(afterStartMs=25, afterFrom=self.before['visualY'])
        self.next_curve.update(beforeStartMs=25, beforeFrom=self.before['visualY'])
        self.after['visualY'] = self.after['requestedY'] = position(self.curve, 31.25)
        self.after['renderedY'] = -self.after['visualY']
        self.assertLess(self.error()['nativeCameraErrorPx'], 1e-10)

    def test_retarget_unsupported_curve_and_geometry_remain_excluded(self):
        self.after['revision'] = self.next_curve['revision'] = 4
        self.assertEqual(self.error()['excluded'], 'input-revision')
        self.after['revision'] = self.next_curve['revision'] = 3
        self.curve['curve'] = self.next_curve['curve'] = 'Custom'
        self.assertEqual(self.error()['excluded'], 'unsupported-curve')
        self.curve['curve'] = self.next_curve['curve'] = 'OutSine'
        self.after['geometryOffset'] = 1
        self.assertEqual(self.error()['excluded'], 'geometry-change')

    def test_invalid_clock_parameters_and_commit_join_fail_closed(self):
        self.curve['durationMs'] = 0
        with self.assertRaises(ValueError):
            position(self.curve, 30)
        self.curve['durationMs'] = 100
        self.next_curve['timeNs'] = 9
        with self.assertRaises(ValueError):
            self.error()
        self.next_curve['timeNs'] = 2
        self.curve['destination'] = math.nan
        with self.assertRaises(ValueError):
            position(self.curve, 30)

    def test_indexed_trace_join_retains_missing_samples(self):
        motion = dict(overflow=False, count=2, samples=[self.before, self.after])
        curves = dict(overflow=False, joinErrors=0, count=2,
                      samples=[self.curve, self.next_curve])
        pairs = [dict(beforePose=self.before, afterPose=self.after, displayDeltaNs=6_250_000,
                      excluded='wheel-curve')]
        self.assertEqual(analyze(motion, curves, pairs, 1.75)['admittedWheelPairs'], 1)
        curves['samples'].pop()
        curves['count'] = 1
        result = analyze(motion, curves, pairs, 1.75)
        self.assertEqual(result['admittedWheelPairs'], 0)
        self.assertEqual(result['exclusionsByReason'], {'missing-curve-sample': 1})
        curves['samples'].append(copy.deepcopy(self.curve))
        curves['count'] = 2
        with self.assertRaises(ValueError):
            analyze(motion, curves, pairs, 1.75)


if __name__ == '__main__':
    unittest.main()
