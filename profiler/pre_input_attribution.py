"""Preserve cold-entry GUI work that precedes the first injected input.

Elapsed native ranges may nest. Union them instead of adding their durations.
Presents can contain old content; submission timing never proves first-paint
content readiness. Idle time preceding navigation is outside this scope.
"""

import argparse
import csv
from decimal import Decimal
import json
from pathlib import Path

from qml_native_attribution import clipped_union_ns


def summarize(rows, navigation_ns, input_ns, present_rows):
    if input_ns <= navigation_ns:
        raise ValueError('Navigation must precede input')
    if any(row[1] < row[0] for row in rows if row[3] == 34):
        raise ValueError('Invalid delegate commit interval')
    overlapping = [row for row in rows if row[3] == 34
                   and row[0] < input_ns and row[1] > navigation_ns]
    contained = [row for row in overlapping
                 if navigation_ns <= row[0] <= row[1] <= input_ns]
    straddling = [row for row in overlapping if row not in contained]
    submitted = []
    for row in present_rows:
        tick = int(Decimal(row['QPCTime']) * 1_000_000_000)
        if navigation_ns <= tick < input_ns:
            submitted.append((tick, row))
    displayed = [(tick + int(Decimal(row['msUntilDisplayed']) * 1_000_000), row)
                 for tick, row in submitted
                 if row['Dropped'] == '0' and Decimal(row['msUntilDisplayed']) > 0]
    largest = sorted(contained, key=lambda row: row[1] - row[0], reverse=True)
    return {
        'navigationToInputMs': (input_ns - navigation_ns) / 1e6,
        'overlappingDelegateCommits': len(overlapping),
        'fullyContainedDelegateCommits': len(contained),
        'straddlingDelegateCommits': len(straddling),
        'delegateCommitElapsedUnionMs': clipped_union_ns(
            [(row[0], row[1]) for row in overlapping], navigation_ns, input_ns) / 1e6,
        'largestContainedCommitMs': max(
            ((row[1] - row[0]) / 1e6 for row in contained), default=None),
        'largestContainedCommits': [
            {'relativeNavigationMs': (row[0] - navigation_ns) / 1e6,
             'durationMs': (row[1] - row[0]) / 1e6, 'owner': row[2]}
            for row in largest[:8]],
        'preInputSubmittedPresents': len(submitted),
        'preInputSubmittedDroppedPresents': sum(row['Dropped'] != '0' for _, row in submitted),
        'firstPostNavigationSubmissionDisplayedMs': (
            (min(tick for tick, _ in displayed) - navigation_ns) / 1e6
            if displayed else None),
        'firstVisibleContentProven': False,
        'cpuResidencyProven': False,
        'fullPerformanceAccepted': False,
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
    assert census['overflow'] == 0, 'Incomplete native census'
    navigation = read('entry-navigation.json')
    packet = read(args.case + '.json')
    with (args.capture / (args.case + '-target.csv')).open(newline='') as source:
        result = summarize(census['rows'], navigation['navigationSentNs'],
                           packet['startNs'], list(csv.DictReader(source)))
    (args.capture / 'pre-input-attribution.json').write_text(json.dumps(result, indent=2))
    print(json.dumps(result, indent=2))


if __name__ == '__main__':
    main()
