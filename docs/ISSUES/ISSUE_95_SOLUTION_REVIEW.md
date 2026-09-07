# Issue #95 solution review

## Problem

In the built-in terminal, pressing Tab after a command such as `cd "completion-f` did not offer names from the current panel directory. The existing path hint provider intentionally required `/` or `\\` in the token, so a relative name without a separator produced no candidates.

## Candidate solutions

1. Treat every separator-free command-line token as a path. This would make ordinary arguments such as `echo foo` list panel files and would change autocomplete behavior broadly.
2. Add a second autocomplete implementation in the terminal handler that scans the local filesystem. This would duplicate VFS listing, ranking, coloring, timeout, and replacement logic, and would not work consistently with remote VFS panels.
3. Reuse the existing VFS path-hint pipeline for separator-free arguments only when the command is `cd`, `chdir`, or `pushd`; preserve quote delimiters and keep the existing separator-based path behavior unchanged. **Chosen.**

## Three-pass review of the chosen solution

### Pass 1 — behavior

- `cd`, `chdir`, and `pushd` resolve a bare relative token against the active panel VFS path.
- Windows `cd /d <path>` remains supported by accepting option-only arguments before the path.
- Opening quotes remain in the replacement text, so `cd "prefix` stays a valid shell argument.
- Directories receive the VFS-appropriate separator and files remain selectable.
- Non-directory commands do not get unrelated panel-file suggestions.

### Pass 2 — compatibility and scope

- Existing separator-based completion continues through the original `pathHintItems` entry point.
- Remote and plugin VFS implementations still use their normal `ReadDir` and timeout behavior.
- Existing replacement spans, fuzzy ranking, panel source selection, colors, and history merging are untouched.
- The change is platform-neutral Go code; Windows native validation covers the reported path, and Linux amd64 test binaries compile successfully.

### Pass 3 — regression and failure modes

- Added tests cover quoted bare `cd` completion, replacement spans, and rejection for `echo`.
- Existing path-hint, command-line, and full repository tests pass.
- A native Windows terminal run showed the completion menu and accepted a directory with Tab without executing the command.
- If a VFS cannot list the directory, the existing timeout/error path still returns no suggestions rather than blocking indefinitely.

## Conclusion

Candidate 3 is the smallest general fix that addresses the reported Windows workflow while preserving the existing autocomplete contract and VFS abstraction.
