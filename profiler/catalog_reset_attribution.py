"""Join requested-row cancel-all events to enclosing catalog applications."""
from collections import Counter


def attribute(census, lifetimes, input_ns, window_ns=500_000_000):
    if census['overflow']:
        raise ValueError('Incomplete catalog trace')
    end_ns = input_ns + window_ns
    scopes = [row for row in census['rows'] if row[3] == 8
              and row[0] < end_ns and row[1] >= input_ns]
    lives = {(row['owner'], row['serial']): row for row in lifetimes}
    if len(lives) != len(lifetimes):
        raise ValueError('Duplicate row lifetime')
    groups, unmatched = {}, []
    for begin, end, owner, kind, index, serial, reason in census['rows']:
        if kind != 68 or reason != 5 or not input_ns <= begin < end_ns:
            continue
        row = dict(owner=owner, index=index, serial=serial, relativeMs=(begin-input_ns)/1e6)
        life = lives.get((owner, serial))
        if life is None or life['index'] != index or life.get('cancelNs') != begin:
            unmatched.append(dict(row, reason='missing-lifetime'))
            continue
        matches = [i for i, scope in enumerate(scopes)
                   if scope[0] <= begin <= end <= scope[1]]
        if len(matches) != 1:
            unmatched.append(dict(row, reason='ambiguous-scope' if matches else 'no-scope'))
            continue
        i = matches[0]
        groups.setdefault(i, []).append(dict(row, started='startNs' in life))
    results = []
    for i, rows in sorted(groups.items(), key=lambda item: scopes[item[0]][0]):
        begin, end, owner, _, incoming, prior, equivalent = scopes[i]
        results.append(dict(catalogOwner=owner, relativeMs=(begin-input_ns)/1e6,
                            durationMs=(end-begin)/1e6, incomingEntries=incoming,
                            previousLogicalCount=prior, checkedEquivalent=bool(equivalent),
                            queued=sum(not row['started'] for row in rows),
                            started=sum(row['started'] for row in rows), rows=rows))
    return dict(matched=sum(len(group['rows']) for group in results),
                groups=results, unmatched=unmatched,
                unmatchedReasons=dict(Counter(row['reason'] for row in unmatched)),
                revisionIdentityProven=False, cpuResidencyProven=False)
