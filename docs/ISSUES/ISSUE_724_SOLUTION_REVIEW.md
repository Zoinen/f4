# Issue #724 solution review

## Problem

`plugins/netfox/ssh_dial.go` used `ssh.InsecureIgnoreHostKey`, so both SFTP
and FISH+ accepted any SSH server key. Because authentication can fall back to
a password, an active network attacker could impersonate the server and see
the password and session contents.

## Candidate solutions

1. Accept an unknown key once and write it to `known_hosts` (TOFU). This keeps
   first-use connections convenient, but silently trusting a key on the first
   connection does not protect the exact first connection described by the
   report.
2. Use the user's OpenSSH `known_hosts` database and reject both unknown and
   changed keys. This matches the trust model users already configure for SSH,
   works for SFTP and FISH+ through their shared dialer, and is safe through
   HTTP CONNECT and SOCKS5 proxies.
3. Add a new f4-specific host-key field to every NetFox site. This would allow
   explicit pinning but requires new UI, persistence, migration, and a second
   trust database without evidence that the project needs one.

## Chosen solution

Use `~/.ssh/known_hosts` (and `known_hosts2` when present) through
`golang.org/x/crypto/ssh/knownhosts`. A missing database is an explicit error;
there is no insecure fallback or interactive trust-on-first-use path. The
callback is created before any authentication methods are used, so no password
or agent operation begins until the server key is verified.

## Verification

- Unit-test accepted, unknown, and changed host keys with temporary OpenSSH
  known-hosts files.
- Run the full Go test suite and vet the NetFox package.
- Build and run the native Windows amd64 binary; verify that an SSH endpoint
  with a known key connects and an unknown/changed key is rejected before
  authentication.

— zoin-bot
