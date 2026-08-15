import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import analyze_navigation_benchmark as analyzer


class NavigationBenchmarkAnalyzerTests(unittest.TestCase):
    def test_runner_action_owns_metadata_and_late_events_are_bounded(self):
        events = [
            {
                "event": "qt.gallery.runner.action",
                "monotonicNs": 100,
                "actionSequence": 23,
                "direction": "enter",
                "warmup": False,
                "cycle": 10,
            },
            {
                "event": "qt.gallery.runner.transition-complete",
                "monotonicNs": 200,
                "actionSequence": 23,
                "direction": "enter",
                "warmup": False,
            },
            {
                "event": "qt.gallery.qml.host.panel.changed",
                "monotonicNs": 250,
                "direction": "leave",
                "warmup": True,
                "cycle": 11,
            },
        ]

        metadata = analyzer.trace_metadata(events)
        self.assertEqual(metadata["direction"], "enter")
        self.assertIs(metadata["warmup"], False)
        self.assertEqual(metadata["cycle"], 10)

        bounded = analyzer.bounded_transition_events(events, next_action_ns=300)
        self.assertEqual(
            [event["event"] for event in bounded],
            [
                "qt.gallery.runner.action",
                "qt.gallery.runner.transition-complete",
            ],
        )


if __name__ == "__main__":
    unittest.main()
