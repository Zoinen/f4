# Search-first command focus shortcuts

Ctrl+E from panel focus never reached command history; Tab from command focus
was consumed without returning focus to the panel. Handle both shortcuts at
the panels-frame focus-routing boundary, using SetCommandLineFocus and the
existing HistoryUp implementation. Preserve raw-terminal and modified-Tab paths.

Regression checks: TestSearchFirstCtrlEActivatesCommandHistory and
TestSearchFirstCommandEnterPolicyAndTab fail before the fix and pass afterward.
Existing HistoryAndPromptFocusColors and MouseFocusAndInactiveCursor failures
were verified with the production change removed and are unrelated.
