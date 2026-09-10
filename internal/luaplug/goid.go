package luaplug

import (
	"runtime"
	"strconv"
)

// goID returns the current goroutine's id.
//
// A Lua state may only ever be touched by one goroutine, but a native callback
// created through the FFI bridge is invoked by whichever goroutine made the
// native call. When that is the runtime's own worker, the callback has to run
// inline instead of being queued, or the worker deadlocks waiting on itself.
// Comparing goroutine ids is the only way to tell the two cases apart.
func goID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	line := buf[:n]

	const prefix = "goroutine "
	if len(line) < len(prefix) {
		return 0
	}
	line = line[len(prefix):]

	end := 0
	for end < len(line) && line[end] >= '0' && line[end] <= '9' {
		end++
	}
	id, err := strconv.ParseInt(string(line[:end]), 10, 64)
	if err != nil {
		return 0
	}
	return id
}
