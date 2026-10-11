package netfox

import "sync/atomic"

// FishPreferRemoteF4 makes a FISH+ connection over SSH try f4 itself as the
// server first (nativeRemoteCommand, fishplus.Server), falling back to the
// shell helper on a host without f4 (unxed/f4#1680). Off by default: on a host
// without f4 it costs one extra connection attempt before the fallback.
var FishPreferRemoteF4 atomic.Bool

// nativeRemoteCommand is what the client runs on the peer in place of a shell.
const nativeRemoteCommand = "f4 --fish-server"

// fishRemoteF4Option is the key of the per-site setting in NetFoxConfig.Options
// ("true" when the site prefers f4 on the peer), set by the checkbox of the
// fish+ connection dialog.
const fishRemoteF4Option = "RemoteF4"
