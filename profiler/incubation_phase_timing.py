"""Join root initialization callbacks to their requested-row lifetimes."""
from requested_row_timeline import distribution


def summarize(census, lifetimes, input_ns):
    if census['overflow']:
        raise ValueError('Incomplete catalog census')
    by_key = {(row['owner'], row['serial']): row for row in lifetimes}
    if len(by_key) != len(lifetimes):
        raise ValueError('Duplicate lifetime')
    initialized = {}
    for begin, end, owner, kind, index, serial, _ in census['rows']:
        if kind != 71:
            continue
        key = owner, serial
        life = by_key.get(key)
        if (not life or key in initialized or life['index'] != index
                or not life.get('startNs', float('inf')) <= begin <= end
                <= min(life.get('readyNs', float('inf')),
                       life.get('cancelNs', float('inf')))):
            raise ValueError('Invalid root initialization interval')
        initialized[key] = begin, end

    completed = []
    for key, life in by_key.items():
        if life.get('failed') or 'readyNs' not in life:
            continue
        if key not in initialized:
            raise ValueError('Ready lifetime lacks initialization callback')
        begin, end = initialized[key]
        completed.append(dict(owner=key[0], serial=key[1], index=life['index'],
                              relativeStartMs=(life['startNs']-input_ns)/1e6,
                              preInitMs=(begin-life['startNs'])/1e6,
                              initializationMs=(end-begin)/1e6,
                              postInitMs=(life['readyNs']-end)/1e6,
                              totalMs=(life['readyNs']-life['startNs'])/1e6))
    during_input = [row for row in completed if row['relativeStartMs'] >= 0]
    return dict(completed=completed, duringInput={
        name: distribution([row[name] for row in during_input])
        for name in ('preInitMs', 'initializationMs', 'postInitMs', 'totalMs')})
