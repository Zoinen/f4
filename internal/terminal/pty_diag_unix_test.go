//go:build !windows

package terminal

import (
	"testing"
)

func TestLogPTYDiagnostics(t *testing.T) {
	LogPTYDiagnostics()
}
