"""Interval unions for bounded GUI census spans, never CPU attribution."""


def covered_ns(spans, begin, end):
    if end < begin:
        raise ValueError('Reversed observation interval')
    clipped = sorted((max(a, begin), min(b, end)) for a, b in spans
                     if a < end and b > begin and b >= a)
    total = 0
    previous_end = begin
    for a, b in clipped:
        total += max(0, b - max(a, previous_end))
        previous_end = max(previous_end, b)
    return total
