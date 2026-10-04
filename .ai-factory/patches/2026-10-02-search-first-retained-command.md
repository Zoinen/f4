# Search-first retained command routing

- Reproduced: plain Enter opened the selected panel item after returning focus from a nonempty command line; `TestSearchFirstRetainedCommandRunsFromPanel` failed before the fix.
- Autocomplete is a top-level vtui frame and consumes the physical grave key before the panels frame can toggle focus. The frame-manager filter now gives this one reserved key to the panels frame first; closing command focus also closes the suggestion menu.
- The Qt Gallery router no longer treats retained command text as ownership of every key. It still forwards Enter to Go, while unmodified spatial/page/edge keys remain in Gallery when the command edit is unfocused. Focused command editing and fast-find retain their existing ownership.
- Targeted Go and isolated Qt regression tests pass. The broader worktree suites currently have unrelated failures, including `TestEnterOnUnlistableDirectoryKeepsPanelInPlace` and an older Gallery selection expectation.
