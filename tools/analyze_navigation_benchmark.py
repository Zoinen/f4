#!/usr/bin/env python3
"""Analyze correlated F4 folder-navigation benchmark trace events.

Both the Go core and Qt host emit compact JSON after the
``F4_NAV_BENCHMARK_TRACE`` marker.  Emission can be deferred, so this parser
always orders records by their shared monotonic nanosecond clock rather than
by line order.
"""

from __future__ import annotations

import argparse
import json
import math
import statistics
import sys
from collections import defaultdict
from pathlib import Path
from typing import Any, Iterable


MARKER = "F4_NAV_BENCHMARK_TRACE "


def percentile(values: list[float], percent: float) -> float:
    if not values:
        return math.nan
    ordered = sorted(values)
    position = (len(ordered) - 1) * percent / 100.0
    lower = math.floor(position)
    upper = math.ceil(position)
    if lower == upper:
        return ordered[lower]
    weight = position - lower
    return ordered[lower] * (1.0 - weight) + ordered[upper] * weight


def milliseconds(nanoseconds: int | float) -> float:
    return float(nanoseconds) / 1_000_000.0


def parse_events(path: Path) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    with path.open("r", encoding="utf-8", errors="replace") as source:
        for line_number, line in enumerate(source, 1):
            marker = line.find(MARKER)
            if marker < 0:
                continue
            payload = line[marker + len(MARKER) :].strip()
            try:
                event = json.loads(payload)
            except json.JSONDecodeError as error:
                print(
                    f"warning: invalid trace JSON at {path}:{line_number}: {error}",
                    file=sys.stderr,
                )
                continue
            if not isinstance(event, dict) or "event" not in event:
                continue
            event["_line"] = line_number
            try:
                event["monotonicNs"] = int(event["monotonicNs"])
            except (KeyError, TypeError, ValueError):
                continue
            trace_id = event.get("benchmarkTraceId")
            if trace_id is not None:
                event["benchmarkTraceId"] = str(trace_id)
            events.append(event)
    return events


def attach_sequence_trace_ids(events: list[dict[str, Any]]) -> None:
    """Attach IDs to pre-decode Qt events using their process-local sequence."""
    by_sequence: dict[tuple[Any, Any], str] = {}
    for event in events:
        trace_id = event.get("benchmarkTraceId")
        sequence = event.get("sequence")
        if trace_id and sequence is not None:
            by_sequence[(event.get("pid"), sequence)] = trace_id
    for event in events:
        if event.get("benchmarkTraceId"):
            continue
        sequence = event.get("sequence")
        if sequence is None:
            continue
        trace_id = by_sequence.get((event.get("pid"), sequence))
        if trace_id:
            event["benchmarkTraceId"] = trace_id


TRACE_METADATA_KEYS = (
    "direction",
    "cycle",
    "measuredCycle",
    "transition",
    "warmup",
    "fromPath",
    "toPath",
    "side",
)


def runner_action_event(
    events: Iterable[dict[str, Any]],
) -> dict[str, Any] | None:
    actions = [
        event
        for event in events
        if event.get("event") == "qt.gallery.runner.action"
    ]
    return min(
        actions,
        key=lambda event: (event["monotonicNs"], event.get("_line", 0)),
        default=None,
    )


def trace_metadata(events: Iterable[dict[str, Any]]) -> dict[str, Any]:
    """Return transition identity owned by the runner action.

    QML trace emission is deferred. A callback carrying an old trace ID can
    therefore run after the runner has advanced to the next transition and
    contain that *next* transition's direction or warmup fields. The action is
    the immutable boundary record, so later events must never overwrite it.
    """
    event_list = list(events)
    action = runner_action_event(event_list)
    if action is not None:
        return {
            key: action[key]
            for key in TRACE_METADATA_KEYS
            if key in action and action[key] not in (None, "")
        }

    # Keep the helper useful for non-runner diagnostic traces.
    keys = (
        *TRACE_METADATA_KEYS,
    )
    result: dict[str, Any] = {}
    for event in event_list:
        for key in keys:
            if key in event and event[key] not in (None, ""):
                result[key] = event[key]
    return result


def bounded_transition_events(
    events: list[dict[str, Any]], next_action_ns: int | None = None
) -> list[dict[str, Any]]:
    """Drop callbacks emitted after this action's transition boundary."""
    action = runner_action_event(events)
    if action is None:
        return events

    action_ns = action["monotonicNs"]
    action_sequence = action.get("actionSequence")
    completions = [
        event
        for event in events
        if event.get("event") == "qt.gallery.runner.transition-complete"
        and event["monotonicNs"] >= action_ns
        and (
            action_sequence is None
            or event.get("actionSequence") is None
            or event.get("actionSequence") == action_sequence
        )
    ]
    completion_ns = min(
        (event["monotonicNs"] for event in completions),
        default=next_action_ns,
    )
    end_ns = completion_ns
    if next_action_ns is not None:
        end_ns = (
            next_action_ns
            if end_ns is None
            else min(end_ns, next_action_ns)
        )
    if end_ns is None:
        return events

    # A completion belongs to this transition; a next action is an exclusive
    # boundary when setup/warmup diagnostics have no completion record.
    include_end = bool(completions and end_ns == completion_ns)
    return [
        event
        for event in events
        if event["monotonicNs"] < end_ns
        or (include_end and event["monotonicNs"] == end_ns)
    ]


def format_stats(values_ns: list[float]) -> str:
    if not values_ns:
        return "-"
    values_ms = [milliseconds(value) for value in values_ns]
    return (
        f"n={len(values_ms):>3} mean={statistics.fmean(values_ms):8.3f} "
        f"p50={percentile(values_ms, 50):8.3f} "
        f"p90={percentile(values_ms, 90):8.3f} "
        f"p95={percentile(values_ms, 95):8.3f} "
        f"max={max(values_ms):8.3f} ms"
    )


def compact_fields(event: dict[str, Any]) -> str:
    hidden = {
        "_line",
        "event",
        "monotonicNs",
        "benchmarkTraceId",
        "pid",
        "thread",
        "durationNs",
    }
    fields = []
    for key in sorted(event):
        if key in hidden:
            continue
        value = event[key]
        if isinstance(value, (dict, list)):
            value = json.dumps(value, separators=(",", ":"), sort_keys=True)
        fields.append(f"{key}={value}")
    return " ".join(fields)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("trace", type=Path, help="combined Go/Qt stderr trace")
    parser.add_argument(
        "--include-warmup",
        action="store_true",
        help="include warmup transitions in aggregate statistics",
    )
    parser.add_argument(
        "--timeline",
        action="append",
        default=[],
        metavar="TRACE_ID",
        help="print a complete ordered timeline (repeatable; 'all' prints all)",
    )
    args = parser.parse_args()

    events = parse_events(args.trace)
    attach_sequence_trace_ids(events)
    if not events:
        print(f"No {MARKER.strip()} events found in {args.trace}", file=sys.stderr)
        return 2

    grouped: dict[str, list[dict[str, Any]]] = defaultdict(list)
    for event in events:
        trace_id = event.get("benchmarkTraceId")
        if trace_id:
            grouped[trace_id].append(event)
    for trace_events in grouped.values():
        trace_events.sort(key=lambda item: (item["monotonicNs"], item["_line"]))

    ordered_actions = sorted(
        (
            event
            for event in events
            if event["event"] == "qt.gallery.runner.action"
            and event.get("benchmarkTraceId")
        ),
        key=lambda event: (event["monotonicNs"], event["_line"]),
    )
    next_action_by_trace: dict[str, int] = {}
    for index, action in enumerate(ordered_actions[:-1]):
        next_action_by_trace[str(action["benchmarkTraceId"])] = int(
            ordered_actions[index + 1]["monotonicNs"]
        )

    # Runner-level configuration IDs are useful diagnostics but are not one
    # enter/leave transition. Keep only IDs that include an action event.
    transitions: dict[str, list[dict[str, Any]]] = {}
    for trace_id, trace_events in grouped.items():
        if any(event["event"] == "qt.gallery.runner.action" for event in trace_events):
            transitions[trace_id] = bounded_transition_events(
                trace_events, next_action_by_trace.get(trace_id)
            )

    print(f"Trace file: {args.trace}")
    print(f"Parsed events: {len(events)}; correlated transition traces: {len(transitions)}")
    if not transitions:
        print("No completed runner action traces were found.", file=sys.stderr)
        return 3

    selected: dict[str, list[dict[str, Any]]] = {}
    metadata_by_trace: dict[str, dict[str, Any]] = {}
    for trace_id, trace_events in transitions.items():
        metadata = trace_metadata(trace_events)
        metadata_by_trace[trace_id] = metadata
        if args.include_warmup or not bool(metadata.get("warmup", False)):
            selected[trace_id] = trace_events

    counts: dict[str, int] = defaultdict(int)
    for trace_id in selected:
        direction = str(metadata_by_trace[trace_id].get("direction", "unknown"))
        counts[direction] += 1
    print(
        "Measured traces: "
        + ", ".join(f"{direction}={count}" for direction, count in sorted(counts.items()))
    )

    # Offset from the exact runner action timestamp exposes the end-to-end
    # critical path for every individual event. Repeated events in one trace
    # (cached/fresh scenes or duplicate renders) contribute separately.
    offsets: dict[tuple[str, str], list[float]] = defaultdict(list)
    durations: dict[tuple[str, str], list[float]] = defaultdict(list)
    transition_totals: dict[str, list[float]] = defaultdict(list)
    for trace_id, trace_events in selected.items():
        metadata = metadata_by_trace[trace_id]
        direction = str(metadata.get("direction", "unknown"))
        starts = [
            event for event in trace_events
            if event["event"] == "qt.gallery.runner.action"
        ]
        start_ns = starts[0]["monotonicNs"] if starts else trace_events[0]["monotonicNs"]
        for event in trace_events:
            # ipc.read.begin marks when Go started its blocking read for the
            # next message, which can legitimately predate the Qt action by a
            # whole preceding transition. It remains available in timelines,
            # but pre-action waits are not critical-path arrival offsets.
            if event["monotonicNs"] >= start_ns:
                offsets[(direction, event["event"])].append(
                    event["monotonicNs"] - start_ns
                )
            duration = event.get("durationNs")
            if isinstance(duration, (int, float)):
                durations[(direction, event["event"])].append(float(duration))
        completed = [
            event for event in trace_events
            if event["event"] == "qt.gallery.runner.transition-complete"
        ]
        if not completed:
            completed = [
                event for event in trace_events
                if event["event"] == "qt.gallery.frame-swapped"
            ]
        end_ns = completed[-1]["monotonicNs"] if completed else trace_events[-1]["monotonicNs"]
        transition_totals[direction].append(end_ns - start_ns)

    print("\nEnd-to-end transition totals")
    for direction, values in sorted(transition_totals.items()):
        print(f"  {direction:<10} {format_stats(values)}")

    print("\nEvent offsets from Qt runner action (critical-path arrival time)")
    for (direction, event_name), values in sorted(
        offsets.items(), key=lambda item: (item[0][0], percentile(item[1], 50), item[0][1])
    ):
        print(f"  {direction:<8} {event_name:<48} {format_stats(values)}")

    print("\nInstrumented self-durations")
    for (direction, event_name), values in sorted(
        durations.items(), key=lambda item: (item[0][0], -percentile(item[1], 50), item[0][1])
    ):
        print(f"  {direction:<8} {event_name:<48} {format_stats(values)}")

    requested_timelines = set(args.timeline)
    if "all" in requested_timelines:
        requested_timelines = set(selected)
    missing = requested_timelines.difference(transitions)
    for trace_id in sorted(missing):
        print(f"warning: timeline trace not found: {trace_id}", file=sys.stderr)
    for trace_id in sorted(requested_timelines.intersection(transitions)):
        trace_events = transitions[trace_id]
        metadata = metadata_by_trace[trace_id]
        action_events = [
            event for event in trace_events
            if event["event"] == "qt.gallery.runner.action"
        ]
        origin = (
            action_events[0]["monotonicNs"]
            if action_events else trace_events[0]["monotonicNs"]
        )
        print(f"\nTimeline {trace_id}: {json.dumps(metadata, sort_keys=True)}")
        previous = origin
        for event in trace_events:
            timestamp = event["monotonicNs"]
            duration = event.get("durationNs")
            duration_text = (
                f" self={milliseconds(duration):8.3f}ms"
                if isinstance(duration, (int, float)) else ""
            )
            print(
                f"  +{milliseconds(timestamp - origin):9.3f}ms "
                f"delta={milliseconds(timestamp - previous):8.3f}ms "
                f"{event['event']}{duration_text} {compact_fields(event)}"
            )
            previous = timestamp

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
