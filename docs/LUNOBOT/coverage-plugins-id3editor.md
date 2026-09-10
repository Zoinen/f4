# Coverage: `plugins/id3editor`

## Scope

This change covers the ID3 editor plugin's command dispatch and dialog lifecycle:

- local-only, selection, and MP3-extension guards;
- opening a selected MP3 in the editor dialog;
- populating the editor controls and saving edited fields;
- closing the editor through Cancel;
- the existing ID3 v1/v2 comment and round-trip tests remain in place.

## Baseline

The package was selected from the Codecov report for merge `3ab39ef964ed065fbd9de1f81fde57e5f481dae6`: `plugins/id3editor` had 26.69% line coverage. The package contains real plugin logic; platform stubs and the actively claimed `internal/ttyx` package were excluded from this selection.

## Verification

No local Go build or test was run, per the current LUNOBOT instructions. Static checks are `gofmt` and `git diff --check`; GitHub Actions is the authoritative verification for this change.

The first CI run `34295042683` on commit `bd3bddc56d07014b2c808148b94bcdcf06850b8d` exposed a test assertion mistake: `vtui.Msg("ID3Editor.Title")` returned `{ID3Editor.Title}`. The assertion was corrected in `ef077a733dab4ea8204931e11722b6b35ecdc89a`.

The corrected CI run `34295694105` on that exact commit completed successfully with all 26 required jobs green; release jobs were skipped. The final PR-head run and post-merge Codecov result will be recorded here next.
