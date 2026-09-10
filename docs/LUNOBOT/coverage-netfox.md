# Coverage task: `plugins/netfox`

Origin: § 22.3 custom task. Codecov main was 61.0% overall and 57.61% for
`plugins/netfox` (30 files, 4454 lines) at task start.

The package had untested pure helpers and URI validation branches in
`sftp_uri.go` and `proxy_dialog.go`. The tests exercise proxy mode mapping,
label padding, translated mode item construction, SFTP provider identity, and
malformed/hostless URI rejection without opening a network connection.
