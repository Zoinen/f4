package semantic

import (
	"time"
)

func SemanticMTimeNanos(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixNano()
}
