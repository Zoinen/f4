# Issue #260 solution review

## Candidate 1: keep the current argument and environment checks

- Keeps the startup path unchanged.
- Fails after sudo authentication because the elevated dispatcher inherits
  `F4_ASKPASS_PARENT` and is mistaken for the askpass helper.
- Rejected.

## Candidate 2: remove the askpass environment check

- Would let the dispatcher start, but the standalone askpass invocation would
  start the full f4 UI instead of returning a password to sudo.
- Rejected.

## Candidate 3: dispatch explicit sudo mode before askpass mode (chosen)

- Parses `--sudo-dispatcher` before looking at `F4_ASKPASS_PARENT`, so the
  authenticated root process always starts the IPC dispatcher.
- Leaves the environment-based askpass helper path intact for sudo's separate
  password request process.
- Covers both separate-value and equals-value argument forms.

## Three-pass review

### Pass 1: correctness

- The dispatcher path is selected before any mount, UI, or askpass startup.
- A missing dispatcher value is ignored rather than consuming an unrelated
  argument.
- The existing askpass environment path remains available to sudo.

### Pass 2: compatibility and failure modes

- Normal f4 startup and mount commands do not contain the dispatcher flag and
  follow their existing paths.
- The change is platform-neutral at argument parsing time; the platform-specific
  dispatcher implementation remains responsible for elevation behavior.
- The root dispatcher no longer waits for a second password prompt or leaves
  the client blocked while its socket is absent.

### Pass 3: scope and regression risk

- The fix is limited to startup mode selection and does not alter file access,
  history ordering, or VFS behavior.
- Unit tests cover both accepted dispatcher syntaxes and invalid or unrelated
  arguments.
- Native Linux execution is unavailable on this Windows x64 host; Linux
  compilation and the platform-specific unit suite are used for validation.
