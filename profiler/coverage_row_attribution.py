"""Join render coverage blockers to queue lifetimes, retaining unmatched holds.

Construction intervals include suspended incubation time; they are not CPU time.
"""
import argparse
from bisect import bisect_right
from collections import Counter, defaultdict
import json
from pathlib import Path


def stage_at(life, time_ns):
    if life.get('cancelNs', float('inf')) <= time_ns:
        return 'canceled'
    if life.get('failed') and life.get('readyNs', float('inf')) <= time_ns:
        return 'failed'
    for name in ('publish', 'take', 'ready', 'start'):
        if life.get(name + 'Ns', float('inf')) <= time_ns:
            return {'publish': 'published', 'take': 'taken',
                    'ready': 'ready', 'start': 'constructing'}[name]
    return 'queued'


def attribute(probe, holds, lifetimes, start_ns, tolerance=1e-4):
    if probe['overflow'] or probe['wrongWriter']:
        raise ValueError('Incomplete or multi-writer coverage trace')
    frames_by_job = defaultdict(list)
    for frame in probe['frames']:
        if frame['endNs'] < frame['beginNs']:
            raise ValueError('Reversed coverage interval')
        first, count = frame['firstRow'], frame['rowCount']
        if first < 0 or count < 0 or first + count > len(probe['rows']):
            raise ValueError('Coverage row range outside trace')
        frames_by_job[frame['job']].append(frame)
    for frames in frames_by_job.values():
        frames.sort(key=lambda frame: frame['endNs'])
    times = {job: [frame['endNs'] for frame in frames]
             for job, frames in frames_by_job.items()}
    lives_by_row = defaultdict(list)
    for life in lifetimes:
        lives_by_row[life['owner'], life['index']].append(life)
    result = []
    for hold in holds:
        item = {'holdNs': hold['timeNs'],
                'relativeMs': (hold['timeNs'] - start_ns) / 1e6,
                'blockers': []}
        result.append(item)
        job = format(int(hold['job']), 'x')
        position = bisect_right(times.get(job, []), hold['timeNs']) - 1
        if position < 0:
            item['unmatched'] = 'no-prior-sync'
            continue
        frame = frames_by_job[job][position]
        # The last probe snapshot cannot be extrapolated beyond its capture window.
        if hold['timeNs'] > probe['startedNs'] + probe['captureWindowMs'] * 1e6:
            item['unmatched'] = 'outside-probe-window'
            continue
        item['syncNs'] = frame['endNs']
        item['snapshotAgeMs'] = (hold['timeNs'] - frame['endNs']) / 1e6
        if (abs(frame['maximum'] - hold['coverageMaximum']) > tolerance
                or abs(frame['minimum'] - hold['minimum']) > tolerance
                or frame['anchorReady'] != hold['anchorReady']):
            item['unmatched'] = 'coverage-state-mismatch'
            continue
        requested = hold['requestedY']
        direction = ('anchor' if not frame['anchorReady'] else
                     'forward' if requested > frame['maximum'] + tolerance else
                     'backward' if requested < frame['minimum'] - tolerance else None)
        if direction is None:
            item['unmatched'] = 'request-within-coverage'
            continue
        item['direction'] = direction
        rows = probe['rows'][frame['firstRow']:frame['firstRow'] + frame['rowCount']]
        for row in rows:
            _, index, serial, flags, prepared, pointer, top, bottom, *_ = row
            if prepared:
                continue
            viewport_top = frame['viewportTop']
            viewport_bottom = viewport_top + frame['viewportHeight']
            if direction == 'forward':
                blocks = (top >= viewport_bottom and
                          abs(frame['logicalY'] + top - viewport_bottom
                              - frame['maximum']) <= tolerance)
            elif direction == 'backward':
                blocks = (bottom <= viewport_top and
                          abs(frame['logicalY'] + bottom - viewport_top
                              - frame['minimum']) <= tolerance)
            else:
                blocks = bottom > viewport_top and top < viewport_bottom
            if not blocks:
                continue
            candidates = [life for life in lives_by_row[frame['layout'], index]
                          if life['requestNs'] <= frame['endNs']]
            active = [life for life in candidates
                      if min(life.get('takeNs', float('inf')),
                             life.get('cancelNs', float('inf'))) > frame['endNs']]
            if len(active) > 1:
                raise ValueError('Overlapping queue lifetimes for one row')
            blocker = {'index': index, 'flags': flags, 'assignedSerial': serial,
                       'assignedItem': pointer}
            item['blockers'].append(blocker)
            if not active:
                blocker['stage'] = 'previous-terminal' if candidates else 'never-requested'
                continue
            life = active[0]
            blocker['serial'] = life['serial']
            blocker['stage'] = stage_at(life, frame['endNs'])
            # Publication on the GUI thread does not prove that the renderer has
            # synchronized the newly assigned scene node. Preserve both clocks.
            blocker['stageAtHeldRender'] = stage_at(life, hold['timeNs'])
            for stage in ('request', 'start', 'ready', 'take', 'publish', 'cancel'):
                if stage + 'Ns' in life:
                    blocker[stage + 'RelativeMs'] = (life[stage + 'Ns'] - start_ns) / 1e6
        if not item['blockers']:
            item['unmatched'] = 'no-geometric-blocker'
    return {'holds': result, 'holdCount': len(result),
            'matchedHolds': sum('unmatched' not in item for item in result),
            'unmatchedByReason': dict(Counter(item['unmatched'] for item in result if 'unmatched' in item)),
            'blockerOccurrencesByStage': dict(Counter(blocker['stage'] for item in result for blocker in item['blockers'])),
            'blockerOccurrencesAtHeldRender': dict(Counter(blocker.get('stageAtHeldRender', blocker['stage']) for item in result for blocker in item['blockers'])),
            'cpuResidencyProven': False, 'fullPerformanceAccepted': False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('capture', type=Path)
    parser.add_argument('case')
    args = parser.parse_args()
    def read(name):
        return json.loads((args.capture / name).read_text())
    if not read('native-foreground-admission.json')['acceptedNativeForegroundScope']:
        raise ValueError('Native foreground scope rejected')
    timeline = read('requested-row-timeline.json')
    if timeline['orphanEvents'] or timeline['unfinishedRequests']:
        raise ValueError('Incomplete queue lifetimes')
    result = attribute(read('coverage-readiness.json'),
                       read(args.case + '-motion-analysis.json')['coverageHolds'],
                       timeline['lifetimes'], read(args.case + '.json')['startNs'])
    (args.capture / 'coverage-row-attribution.json').write_text(json.dumps(result, indent=2))
    print(json.dumps({key: value for key, value in result.items() if key != 'holds'}, indent=2))


if __name__ == '__main__':
    main()
