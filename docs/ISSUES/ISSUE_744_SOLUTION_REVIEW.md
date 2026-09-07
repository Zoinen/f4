# Issue #744 solution review

## Evidence

The failed merged-run showed two independent problems. The missing
`math/rand` import in `cmd/f4/arkanoid_test.go` was already isolated and
merged through PR #743. The remaining cross-platform failures were the two
NetFox agent-forwarding tests: the security change now requires a known-hosts
file, but those tests created temporary homes without populating one.

## Candidate solutions

1. **Chosen: make the test fixture satisfy the production security contract.**
   The local SSH server already has a generated host key; write that key to
   the temporary home's `~/.ssh/known_hosts` before calling either dialer.
   This preserves strict host-key verification and keeps the tests isolated.
2. **Rejected: weaken `DialSSH` when running tests or allow unknown hosts.**
   That would hide the security regression and test a different authentication
   path from production.
3. **Rejected: depend on the runner's real `~/.ssh/known_hosts`.** CI runners
   and developer machines differ, and the tests would become nondeterministic
   or expose unrelated user configuration.

## Three review passes

- **Coverage:** both direct SSH and FISH+ dialer tests now install the generated
  server key in the exact temporary home they configure.
- **Safety:** host-key verification remains strict; no global `InsecureIgnoreHostKey`
  callback or production fallback is introduced.
- **Portability:** the fixture uses the existing `writeKnownHosts` helper and
  `knownhosts.Normalize`, so Linux, Windows, and macOS all use the same
  address format.

## Decision

Fix the tests, not the SSH security behavior. The compile failure is already
fixed in merged PR #743; this change addresses the remaining deterministic
CI failure exposed by the same run.
