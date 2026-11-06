"""Model supported wheel curves against recorded camera poses, retaining exclusions.

This checks native scene-matrix motion and display timing, not optical pixels.
Retargeted and unsupported segments remain unverified, never zero-error samples.
"""
from collections import Counter
import math

from requested_row_timeline import distribution


def position(curve, animation_ms, continuation=True):
    values = [animation_ms, curve['durationMs'], curve['destination']]
    prefix = 'after' if continuation else 'before'
    start, origin = curve[prefix+'StartMs'], curve[prefix+'From']
    values.extend([start, origin])
    if not all(math.isfinite(v) for v in values) or curve['durationMs'] <= 0:
        raise ValueError('Invalid curve parameters')
    progress = min(1, max(0, (animation_ms-start)/curve['durationMs']))
    if curve['curve'] == 'Linear':
        eased = progress
    elif curve['curve'] == 'OutSine':
        eased = math.sin(progress*math.pi/2)
    elif curve['curve'] == 'OutCubic':
        eased = 1-(1-progress)**3
    else:
        raise ValueError('Unsupported curve: '+curve['curve'])
    return origin+(curve['destination']-origin)*eased


def pair_error(before, after, curve_before, curve_after, display_delta_ns, dpr):
    if not math.isfinite(dpr) or dpr <= 0 or display_delta_ns <= 0:
        raise ValueError('Invalid DPR or display interval')
    for pose, curve in [(before, curve_before), (after, curve_after)]:
        if any(pose[key] != curve[key] for key in ('timeNs', 'job', 'revision', 'animationMs')):
            raise ValueError('Curve and camera samples do not identify the same commit')
        if pose['continuous'] != curve['continuous']:
            raise ValueError('Curve and camera motion modes differ')
        if not all(math.isfinite(pose[k]) for k in
                   ('animationMs', 'geometryOffset', 'visualY', 'renderedY', 'requestedY')):
            raise ValueError('Non-finite camera pose')
    if before['continuous'] or after['continuous']:
        return dict(excluded='continuous-motion')
    if before['job'] != after['job']:
        return dict(excluded='job-change')
    if before['revision'] != after['revision']:
        return dict(excluded='input-revision')
    if (before['geometryOffset'] != after['geometryOffset']
            or abs(before['renderedY']+before['visualY']
                   -after['renderedY']-after['visualY']) > .002):
        return dict(excluded='geometry-change')
    if any(c['curve'] not in {'Linear', 'OutSine', 'OutCubic'}
           for c in [curve_before, curve_after]):
        return dict(excluded='unsupported-curve')
    if (curve_before['afterStartMs'] != curve_after['beforeStartMs']
            or curve_before['afterFrom'] != curve_after['beforeFrom']
            or any(curve_before[k] != curve_after[k]
                   for k in ('durationMs', 'destination', 'curve'))):
        return dict(excluded='segment-change')
    if after['animationMs'] < before['animationMs']:
        raise ValueError('Animation clock reversed within a segment')
    expected_requested = position(curve_before, after['animationMs'])
    expected_display = position(curve_before, before['animationMs']+display_delta_ns/1e6)
    actual_delta = -(after['renderedY']-before['renderedY'])
    return dict(nativeRequestedErrorPx=abs(after['requestedY']-expected_requested)*dpr,
                nativeCameraErrorPx=abs(after['visualY']-expected_requested)*dpr,
                displayClockModelErrorPx=abs(actual_delta
                    -(expected_display-before['visualY']))*dpr,
                coverageLimited=abs(after['requestedY']-after['visualY']) > .001,
                animationDeltaMs=after['animationMs']-before['animationMs'],
                displayDeltaMs=display_delta_ns/1e6)


def analyze(motion, curves, pairs, dpr):
    if motion['overflow'] or curves['overflow'] or curves['joinErrors']:
        raise ValueError('Incomplete or incorrectly joined curve trace')
    if motion['count'] != len(motion['samples']) or curves['count'] != len(curves['samples']):
        raise ValueError('Trace count mismatch')
    keys, used = {}, set()
    for curve in curves['samples']:
        index = curve['motionIndex']
        if type(index) is not int or index in used or not 0 <= index < len(motion['samples']):
            raise ValueError('Invalid or duplicate motion index')
        used.add(index)
        pose = motion['samples'][index]
        key = tuple(pose[k] for k in ('timeNs', 'job', 'revision', 'animationMs'))
        if key in keys or key != tuple(curve[k] for k in ('timeNs', 'job', 'revision', 'animationMs')):
            raise ValueError('Ambiguous or mismatched curve join')
        keys[key] = curve
    rows, exclusions = [], []
    for pair in pairs:
        poses = pair['beforePose'], pair['afterPose']
        lookup = [keys.get(tuple(p[k] for k in ('timeNs', 'job', 'revision', 'animationMs')))
                  for p in poses]
        result = (dict(excluded='missing-curve-sample') if any(c is None for c in lookup)
                  else pair_error(*poses, *lookup, pair['displayDeltaNs'], dpr))
        result.update({k: pair[k] for k in ('relativeMs', 'present', 'frame', 'displayDeltaNs')
                       if k in pair})
        (exclusions if 'excluded' in result else rows).append(result)
    return dict(pairCount=len(pairs), admittedWheelPairs=len(rows), excludedPairs=exclusions,
                exclusionsByReason=dict(Counter(p['excluded'] for p in exclusions)),
                unpairedMotionSamples=len(motion['samples'])-len(used),
                nativeRequestedErrorPx=distribution([p['nativeRequestedErrorPx'] for p in rows]),
                nativeCameraErrorPx=distribution([p['nativeCameraErrorPx'] for p in rows]),
                displayClockModelErrorPx=distribution([p['displayClockModelErrorPx'] for p in rows]),
                admittedCoverageLimitedPairs=sum(p['coverageLimited'] for p in rows),
                admittedRows=rows, nativeModelOnly=True, opticalMotionVerified=False,
                fullPerformanceAccepted=False)
