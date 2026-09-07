package fishplus

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPatchAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanPatch() {
		t.Skip("this host cannot assemble a file remotely")
	}

	dir := t.TempDir()
	old := make([]byte, 200000)
	fillDeterministicBytes(t, 5, old)
	src := filepath.Join(dir, "an old file.bin")
	dst := filepath.Join(dir, "new one.bin")
	if err := os.WriteFile(src, old, 0600); err != nil {
		t.Fatal(err)
	}

	for _, mode := range WriteModes {
		if err := c.SetWriteMode(ctx, mode); err != nil {
			t.Logf("write backend %q unavailable here: %v", mode, err)
			continue
		}
		t.Run(mode, func(t *testing.T) {
			// The case the whole command exists for: one byte changes in the
			// middle of a large file and only that byte is sent.
			err := c.Patch(ctx, src, dst, []PatchSegment{
				Copy(0, 100000), Literal([]byte("X")), Copy(100001, 99999),
			})
			if err != nil {
				t.Fatalf("one byte edit: %v", err)
			}
			want := append(append(append([]byte{}, old[:100000]...), 'X'), old[100001:]...)
			got, err := os.ReadFile(dst)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("one byte edit produced %d bytes, want %d", len(got), len(want))
			}

			// Insertion, deletion and reordering at once, with a literal
			// larger than one chunk so that the splitting is exercised.
			blob := make([]byte, 70000)
			fillDeterministicBytes(t, 9, blob)
			err = c.Patch(ctx, src, dst, []PatchSegment{
				Copy(50000, 10000), Literal(blob), Copy(0, 1), Copy(199999, 1),
			})
			if err != nil {
				t.Fatalf("mixed segments: %v", err)
			}
			want = append([]byte{}, old[50000:60000]...)
			want = append(want, blob...)
			want = append(want, old[0], old[199999])
			if got, _ = os.ReadFile(dst); !bytes.Equal(got, want) {
				t.Fatalf("mixed segments produced %d bytes, want %d", len(got), len(want))
			}

			// A file made of nothing but new bytes, and an empty one.
			if err = c.Patch(ctx, src, dst, []PatchSegment{Literal([]byte("only new\n"))}); err != nil {
				t.Fatalf("literal only: %v", err)
			}
			if got, _ = os.ReadFile(dst); string(got) != "only new\n" {
				t.Errorf("literal only produced %q", got)
			}
			if err = c.Patch(ctx, src, dst, nil); err != nil {
				t.Fatalf("empty result: %v", err)
			}
			if got, _ = os.ReadFile(dst); len(got) != 0 {
				t.Errorf("empty result produced %q", got)
			}

			// A refusal has to take the payload off the wire, or the next
			// request would be parsed out of the middle of it. Only what the
			// session answers afterwards can show that it did.
			err = c.Patch(ctx, src, filepath.Join(dir, "no such dir", "x"),
				[]PatchSegment{Copy(0, 10), Literal([]byte("payload to be drained"))})
			if err == nil {
				t.Error("an uncreatable destination was accepted")
			}
			if err = c.Patch(ctx, filepath.Join(dir, "missing"), dst,
				[]PatchSegment{Literal([]byte("x"))}); err == nil {
				t.Error("a missing original was accepted")
			}
			if err := c.Session().Noop(ctx); err != nil {
				t.Fatalf("session out of sync after a refused patch: %v", err)
			}
		})
	}

	if err := c.Patch(ctx, src, src, []PatchSegment{Copy(0, 1)}); err == nil {
		t.Error("patching a file from itself was accepted")
	}
}
