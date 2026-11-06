"""Measure slot occupancy between a row's first queued hold and admission."""
from collections import Counter, defaultdict

from requested_row_timeline import distribution


def summarize(queued_rows, lifetimes, root_limit):
    if root_limit < 1:
        raise ValueError('Invalid root limit')
    by_owner = defaultdict(dict)
    for life in lifetimes:
        owner, serial = life['owner'], life['serial']
        if serial in by_owner[owner]:
            raise ValueError('Duplicate lifetime')
        by_owner[owner][serial] = life

    first_hold = {}
    for row in queued_rows:
        key = row['owner'], row['serial']
        first_hold[key] = min(first_hold.get(key, row['syncNs']), row['syncNs'])

    results = []
    for (owner, serial), begin in first_hold.items():
        life = by_owner[owner][serial]
        end = life.get('startNs', life.get('cancelNs'))
        if end is None:
            continue
        if end < begin:
            raise ValueError('Queued lifetime ended before its hold')
        changes = Counter()
        occupancy = 0
        other_starts = 0
        for other in by_owner[owner].values():
            start = other.get('startNs')
            if start is None or start >= end:
                continue
            terminal = min(other.get('takeNs', float('inf')),
                           other.get('cancelNs', float('inf')),
                           other.get('readyNs', float('inf'))
                           if other.get('failed') else float('inf'))
            if terminal <= begin:
                continue
            if start <= begin:
                occupancy += 1
            else:
                changes[start] += 1
                other_starts += 1
            if terminal < end:
                changes[terminal] -= 1
        last, spare = begin, 0
        for time in sorted({*changes, end}):
            if not 0 <= occupancy <= root_limit:
                raise ValueError('Invalid occupancy in queue lifetime')
            if occupancy < root_limit:
                spare += time - last
            last = time
            occupancy += changes[time]
        results.append(dict(owner=owner, serial=serial, started='startNs' in life,
                            waitMs=(end-begin)/1e6, spareSlotMs=spare/1e6,
                            otherStarts=other_starts))

    started = [row for row in results if row['started']]
    return dict(rows=results, unterminated=len(first_hold)-len(results),
                admittedWaitMs=distribution([row['waitMs'] for row in started]),
                admittedSpareSlotMs=distribution([row['spareSlotMs'] for row in started]),
                admittedOtherStarts=distribution([row['otherStarts'] for row in started]))
