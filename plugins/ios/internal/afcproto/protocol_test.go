package afcproto

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type shortWriter struct {
	bytes.Buffer
	limit int
}

func (w *shortWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		p = p[:w.limit]
	}
	return w.Buffer.Write(p)
}

func TestWritePacketHandlesShortWrites(t *testing.T) {
	w := &shortWriter{limit: 3}
	if err := writePacket(w, 7, opFileWrite, []byte("head"), []byte("payload")); err != nil {
		t.Fatal(err)
	}
	p, err := readPacket(bytes.NewReader(w.Bytes()), 7)
	if err != nil {
		t.Fatal(err)
	}
	if p.header.Operation != opFileWrite || string(p.headerPayload) != "head" || string(p.payload) != "payload" {
		t.Fatalf("packet = %#v", p)
	}
}

type zeroWriter struct{}

func (zeroWriter) Write([]byte) (int, error) { return 0, nil }

func TestWriteFullRejectsNoProgress(t *testing.T) {
	if err := writeFull(zeroWriter{}, []byte("x")); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("error = %v", err)
	}
}
