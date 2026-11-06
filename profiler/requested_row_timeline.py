"""Attribute diagnostic row queue lifetimes; elapsed waits are not CPU time."""
import argparse
from collections import Counter, defaultdict
import json
from pathlib import Path


REASONS = {1: 'identity-replaced', 2: 'outside-plan', 3: 'preempted',
           4: 'pool-satisfied', 5: 'cancel-all'}
STAGES = {65: 'start', 66: 'ready', 67: 'take', 68: 'cancel', 70: 'publish'}


def distribution(values):
    values = sorted(values)
    if not values:
        return {'n': 0}
    def percentile(p):
        position = (len(values) - 1) * p
        low = int(position)
        high = min(low + 1, len(values) - 1)
        return values[low] + (values[high] - values[low]) * (position - low)
    return {'n': len(values), 'p50': percentile(.5), 'p95': percentile(.95),
            'p99': percentile(.99), 'max': values[-1]}


def summarize(rows, input_ns, navigation_ns=None):
    lives, orphans = {}, []
    counts = Counter()
    active = defaultdict(set)
    maximum = Counter()
    for raw in sorted(rows, key=lambda row: row[0]):
        begin, end, owner, kind, index, serial, detail = raw
        if kind not in {64, 65, 66, 67, 68, 69, 70}:
            continue
        if end < begin:
            raise ValueError('Reversed diagnostic span')
        counts[kind] += 1
        if kind == 69:
            continue
        key = owner, serial
        if kind == 64:
            if key in lives:
                raise ValueError('Duplicate request serial within an owner')
            phase = ('startup' if navigation_ns is not None and begin < navigation_ns
                     else 'pre-input' if begin < input_ns else 'input')
            lives[key] = {'owner': owner, 'serial': serial, 'index': index,
                          'requestNs': begin, 'requestPhase': phase}
            continue
        if key not in lives:
            orphans.append(raw)
            continue
        life = lives[key]
        if life['index'] != index:
            raise ValueError('Request serial changed row index')
        stage = STAGES[kind]
        if stage + 'Ns' in life:
            raise ValueError('Duplicate lifecycle stage')
        if begin < life['requestNs']:
            raise ValueError('Stage precedes request')
        if ('cancelNs' in life or 'takeNs' in life) and kind != 70:
            raise ValueError('Stage follows terminal request state')
        life[stage + 'Ns'] = begin
        if kind == 65:
            active[owner].add(serial)
            maximum[owner] = max(maximum[owner], len(active[owner]))
        elif kind == 66:
            if 'startNs' not in life:
                raise ValueError('Ready without construction start')
            life['failed'] = bool(detail)
            if detail:
                active[owner].discard(serial)
        elif kind == 67:
            if 'readyNs' not in life or life.get('failed'):
                raise ValueError('Taken without successful completion')
            active[owner].discard(serial)
        elif kind == 68:
            life['cancelReason'] = REASONS.get(detail, 'unknown-' + str(detail))
            active[owner].discard(serial)
        elif kind == 70 and 'takeNs' not in life:
            raise ValueError('Published before taking the ready root')
    values = list(lives.values())
    canceled = [row for row in values if 'cancelNs' in row]
    timings = {}
    for name, first, second in [('queueWaitMs', 'request', 'start'),
                                ('constructionElapsedMs', 'start', 'ready'),
                                ('readyToTakeMs', 'ready', 'take'),
                                ('takeToPublicationMs', 'take', 'publish')]:
        durations = [(row[second + 'Ns'] - row[first + 'Ns']) / 1e6
                     for row in values if first + 'Ns' in row and second + 'Ns' in row]
        if any(value < 0 for value in durations):
            raise ValueError('Reversed lifecycle stage order')
        timings[name] = distribution(durations)
    return {
        'requests': len(values), 'eventCounts': dict(counts),
        'canceledByReason': dict(Counter(row['cancelReason'] for row in canceled)),
        'canceledAfterStartByReason': dict(Counter(row['cancelReason'] for row in canceled
                                                   if 'startNs' in row)),
        'canceledReadyByReason': dict(Counter(row['cancelReason'] for row in canceled
                                            if 'readyNs' in row and not row.get('failed'))),
        'requestsByPhase': dict(Counter(row['requestPhase'] for row in values)),
        'maximumConstructingOrUnclaimedReadyByOwner': dict(maximum),
        'stillConstructedByOwner': {owner: len(serials) for owner, serials in active.items()},
        'unfinishedRequests': [row for row in values if 'takeNs' not in row and 'cancelNs' not in row],
        'timings': timings, 'orphanEvents': orphans, 'lifetimes': values,
        'cpuResidencyProven': False, 'fullPerformanceAccepted': False,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('capture', type=Path)
    parser.add_argument('case')
    args = parser.parse_args()
    def read(name):
        return json.loads((args.capture / name).read_text())
    assert read('native-foreground-admission.json')['acceptedNativeForegroundScope']
    census = read('catalog-census.json')
    assert census['overflow'] == 0, 'Incomplete trace'
    result = summarize(census['rows'], read(args.case + '.json')['startNs'],
                       read('entry-navigation.json')['navigationSentNs'])
    (args.capture / 'requested-row-timeline.json').write_text(json.dumps(result, indent=2))
    print(json.dumps({key: value for key, value in result.items()
                      if key not in {'lifetimes', 'unfinishedRequests', 'orphanEvents'}}, indent=2))


if __name__ == '__main__':
    main()
