# Search-first autocomplete focus toggle in active workspace

- Reproduced in the running Qt app with search-first navigation and command autocomplete enabled: typing `cd` then pressing the physical grave key produced `cd\`` instead of returning focus to the panel.
- The event log showed active workspace index 1 and a top `vtui.AutoCompleteMenu`, while the bootstrap event filter inspected the startup `PanelsFrame` from workspace 0 (`CommandLineFocused=false`). The menu therefore received the key as text.
- The filter now resolves the active workspace's panels frame for each event, then lets that frame consume the reserved focus key before autocomplete.
- Added a two-workspace regression test confirming the inactive startup frame cannot consume the key, while the active frame closes autocomplete and preserves command text.
- Focused Go tests and `CGO_ENABLED=0` build pass. Live Qt test after restart: pressing grave with `cd` and autocomplete open returned focus to the panel without appending a backtick. The broader app/panel suites still have unrelated existing failures (menu-order expectations, history/focus tests, and an unlistable-directory test).
