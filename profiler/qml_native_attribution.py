"""Correlate native wheel preparation with QML ranges without summing nesting.

All times use native steady-clock nanoseconds after an explicitly validated
trace-clock offset. QML creation ranges may span suspended incubation; their
elapsed durations are never labeled CPU time.
"""

import argparse
import hashlib
import json
from pathlib import Path
import xml.etree.ElementTree as ET


def clipped_union_ns(intervals, start, end):
    tail = start
    total = 0
    for left, right in sorted(intervals):
        left, right = max(start, left), min(end, right)
        if right > max(tail, left):
            total += right - max(tail, left)
            tail = right
    return total


def contained_commits(rows, target):
    start, end, owner = target[:3]
    return [row for row in rows if row[3] == 34 and row[2] == owner
            and start <= row[0] <= row[1] <= end]


def summarize_target(rows, ranges, target, origin):
    commits = contained_commits(rows, target)
    phases = {}
    names = ('prepare', 'identity', 'visual', 'layout', 'finalize', 'endVisualUpdate')
    for kind, name in enumerate(names, 50):
        phases[name] = sum(row[4] for row in rows if row[3] == kind
                           and any(row[2] == commit[2]
                                   and commit[0] <= row[0] <= row[1] <= commit[1]
                                   for commit in commits)) / 1e6
    creation = [(row['start'], row['end']) for row in ranges
                if row['type'] == 'Creating']
    contained_creation = [(left, right) for left, right in creation
                          if target[0] <= left <= right <= target[1]]
    return {
        'relativeStartMs': (target[0] - origin) / 1e6,
        'durationMs': (target[1] - target[0]) / 1e6,
        'owner': target[2], 'containedCommitCount': len(commits),
        'commitMs': sum(v[1] - v[0] for v in commits) / 1e6,
        'phaseMs': phases,
        'qmlCreationElapsedUnionMs': clipped_union_ns(creation, *target[:2]) / 1e6,
        'containedQmlCreationUnionMs': clipped_union_ns(
            contained_creation, *target[:2]) / 1e6,
    }


def analyze(trace_path, census, packet, mapping):
    assert census['overflow'] == 0, 'Incomplete native census'
    assert abs(mapping['navigationInputDeltaDiscrepancyNs']) <= 10000
    offset = mapping['offsetNs']
    root = ET.parse(trace_path).getroot()
    definitions = {int(row.get('index')): {v.tag: v.text or '' for v in row}
                   for row in root.find('eventData')}
    ranges = []
    for row in root.find('profilerDataModel'):
        duration = int(row.get('duration', 0))
        if duration <= 0:
            continue
        start = int(row.get('startTime')) + offset
        ranges.append(dict(definitions[int(row.get('eventIndex'))],
                           start=start, end=start + duration))
    start, end = packet['startNs'], packet['endNs']
    targets = sorted([row for row in census['rows'] if row[3] == 38
                      and start <= row[0] <= row[1] <= end],
                     key=lambda v: v[1] - v[0], reverse=True)
    assert targets, 'No measured motion target preparation'
    results = [summarize_target(census['rows'], ranges, v, start) for v in targets]
    shaders = sorted([row for row in ranges if row['type'] == 'Creating'
                      and row.get('filename', '').endswith('GalleryThumbnailShaderLayer.qml')
                      and row.get('details') == 'QtQuick/ShaderEffect'
                      and start <= row['start'] <= row['end'] <= end],
                     key=lambda v: v['end'] - v['start'], reverse=True)
    shader = None
    if shaders:
        longest = shaders[0]
        nested_input = [row for row in ranges if row.get('details') == 'restartScroll'
                        and longest['start'] <= row['start'] <= row['end'] <= longest['end']]
        shader = {'relativeStartMs': (longest['start'] - start) / 1e6,
                  'elapsedMs': (longest['end'] - longest['start']) / 1e6,
                  'containedWheelCalls': len(nested_input),
                  'containedWheelElapsedUnionMs': clipped_union_ns(
                      [(v['start'], v['end']) for v in nested_input],
                      longest['start'], longest['end']) / 1e6,
                  'cpuTimeProven': False,
                  'explanation': 'Creation elapsed ranges can span suspended incubation.'}
    return {'diagnosticOnly': True, 'fullPerformanceAccepted': False,
            'inputSpanMs': (end - start) / 1e6, 'targetCount': len(results),
            'targets': results, 'largestShaderCreation': shader}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--trace', type=Path, required=True)
    parser.add_argument('--capture', type=Path, required=True)
    parser.add_argument('--mapping', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    receipt = json.loads((args.capture / 'qml-trace-receipt.json').read_text())
    assert receipt['clientExit'] == 0 and receipt['launcherExit'] == 0
    with args.trace.open('rb') as source:
        assert hashlib.file_digest(source, 'sha256').hexdigest() == receipt['traceSha256']
    census = json.loads((args.capture / 'catalog-census.json').read_text())
    packet = json.loads((args.capture / (receipt['case'] + '.json')).read_text())
    mapping = json.loads(args.mapping.read_text())['mapping']
    result = analyze(args.trace, census, packet, mapping)
    args.output.write_text(json.dumps(result, indent=2))
    print(json.dumps({k: v for k, v in result.items() if k != 'targets'}, indent=2))


if __name__ == '__main__':
    main()
