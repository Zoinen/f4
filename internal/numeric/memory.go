package numeric

import (
	"runtime/debug"
)

// ReleaseHeavyMemory forces GC and releases OS memory when heavy sessions (>50MB) close.
func ReleaseHeavyMemory(sizeBytes int64) {
	if sizeBytes > 50*1024*1024 {
		debug.FreeOSMemory()
	}
}
