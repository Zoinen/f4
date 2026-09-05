package fishplus

import (
	"math/rand"
	"testing"
)

func fillDeterministicBytes(t *testing.T, seed int64, dst []byte) *rand.Rand {
	t.Helper()
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- fixed seeds make binary transfer fixtures reproducible; the bytes have no security purpose.
	n, err := rng.Read(dst)
	if err != nil {
		t.Fatalf("generate deterministic fixture: %v", err)
	}
	if n != len(dst) {
		t.Fatalf("generated %d deterministic fixture bytes, want %d", n, len(dst))
	}
	return rng
}
