# Coverage task: `internal/netproxy`

Origin: § 22.3 custom task. Codecov main was 61.06% overall and 63.59% for
`internal/netproxy` (228 lines) at task start.

The tests exercise proxy setting defaults and descriptions, environment proxy
resolution including the nil-request guard, invalid encrypted-secret handling,
and rejection of non-TCP traffic by the HTTP CONNECT dialer. All cases are
local and do not open an external connection.
