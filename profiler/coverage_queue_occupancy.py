"""Inspect bounded root occupancy when an attributed coverage blocker is queued.

This joins recorded lifetimes, not QML Creating ranges. It does not infer queue
priority, CPU residency, or a free-slot interval from a single sampled instant.
"""
import argparse
from collections import Counter, defaultdict
import json
from pathlib import Path

from coverage_row_attribution import stage_at


def analyze(probe, coverage, lifetimes, root_limit):
    if root_limit < 1 or probe['overflow'] or probe['wrongWriter']:
        raise ValueError('Invalid root limit or incomplete coverage trace')
    frames = defaultdict(list)
    for frame in probe['frames']:
        frames[frame['endNs']].append(frame)
    owners = defaultdict(list)
    keys = set()
    for life in lifetimes:
        key = life['owner'], life['serial']
        if key in keys:
            raise ValueError('Duplicate owner/serial lifetime')
        keys.add(key)
        owners[life['owner']].append(life)

    def occupied(owner, time_ns):
        result = []
        for life in owners[owner]:
            if life.get('startNs', float('inf')) > time_ns:
                continue
            terminal = min(life.get('takeNs', float('inf')),
                           life.get('cancelNs', float('inf')),
                           life.get('readyNs', float('inf'))
                           if life.get('failed') else float('inf'))
            if terminal <= time_ns:
                continue
            item = dict(index=life['index'], serial=life['serial'],
                        stage=stage_at(life, time_ns),
                        constructionAgeMs=(time_ns-life['startNs'])/1e6)
            if item['stage'] == 'ready':
                item['readyAgeMs'] = (time_ns-life['readyNs'])/1e6
            result.append(item)
        if len(result) > root_limit:
            raise ValueError('Recorded occupancy exceeds supplied root limit')
        return result

    rows, unmatched = [], []
    for hold in coverage['holds']:
        if 'unmatched' in hold:
            unmatched.append(dict(holdNs=hold['holdNs'], reason=hold['unmatched']))
            continue
        matches = frames[hold['syncNs']]
        if len(matches) != 1:
            unmatched.append(dict(holdNs=hold['holdNs'], reason='ambiguous-sync-owner'))
            continue
        owner = matches[0]['layout']
        for blocker in hold['blockers']:
            if blocker['stage'] != 'queued':
                continue
            lives = [life for life in owners[owner]
                     if life['serial'] == blocker['serial']
                     and life['index'] == blocker['index']]
            if len(lives) != 1 or stage_at(lives[0], hold['syncNs']) != 'queued':
                raise ValueError('Queued blocker does not match recorded lifetime')
            life = lives[0]
            if life['requestNs'] > hold['syncNs']:
                raise ValueError('Blocker was requested after the readiness snapshot')
            at_sync = occupied(owner, hold['syncNs'])
            at_render = occupied(owner, hold['holdNs'])
            rows.append(dict(holdNs=hold['holdNs'], syncNs=hold['syncNs'],
                             owner=owner, index=life['index'], serial=life['serial'],
                             requestAgeAtSyncMs=(hold['syncNs']-life['requestNs'])/1e6,
                             stageAtHeldRender=stage_at(life, hold['holdNs']),
                             capacityAtSync='full' if len(at_sync) == root_limit else 'spare',
                             capacityAtHeldRender='full' if len(at_render) == root_limit else 'spare',
                             occupantsAtSync=at_sync, occupantsAtHeldRender=at_render))
    return dict(rootLimit=root_limit, queuedBlockerOccurrences=len(rows),
                capacityAtSync=dict(Counter(row['capacityAtSync'] for row in rows)),
                capacityAtHeldRender=dict(Counter(row['capacityAtHeldRender'] for row in rows)),
                unmatchedHolds=unmatched, rows=rows, queuePriorityProven=False,
                cpuResidencyProven=False, fullPerformanceAccepted=False)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('capture', type=Path)
    parser.add_argument('--root-limit', type=int, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    def read(name):
        return json.loads((args.capture/name).read_text())
    assert read('native-foreground-admission.json')['acceptedNativeForegroundScope']
    census = read('catalog-census.json')
    if census['overflow']:
        raise ValueError('Incomplete lifecycle trace')
    timeline = read('requested-row-timeline.json')
    if timeline['orphanEvents']:
        raise ValueError('Lifecycle trace contains stages without a request')
    if any(count > args.root_limit for count in
           timeline['maximumConstructingOrUnclaimedReadyByOwner'].values()):
        raise ValueError('Supplied limit differs from the recorded maximum occupancy')
    result = analyze(read('coverage-readiness.json'), read('coverage-row-attribution.json'),
                     timeline['lifetimes'], args.root_limit)
    args.output.write_text(json.dumps(result, indent=2))
    print(json.dumps({k: v for k, v in result.items() if k != 'rows'}, indent=2))


if __name__ == '__main__':
    main()
