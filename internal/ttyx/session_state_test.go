package ttyx

import (
	"github.com/jezek/xgb"
	"testing"
)

func TestSourceNames(t *testing.T) {
	tests := []struct {
		source Source
		want   string
	}{
		{SourceNone, "none"},
		{SourceWindowID, "WINDOWID"},
		{SourceProcess, "_NET_WM_PID"},
		{SourceActive, "_NET_ACTIVE_WINDOW"},
		{Source(99), "none"},
	}

	for _, test := range tests {
		if got := test.source.String(); got != test.want {
			t.Errorf("Source(%d).String() = %q, want %q", test.source, got, test.want)
		}
	}
}

func TestSessionQueriesCachedState(t *testing.T) {
	s := &Session{
		conn:    &xgb.Conn{},
		win:     17,
		source:  SourceProcess,
		alive:   true,
		focused: true,
		geom:    Rect{X: 10, Y: 20, W: 300, H: 200},
		geomOK:  true,
	}

	if !s.Alive() {
		t.Fatal("an active session with a connection must be alive")
	}
	if !s.Focused() {
		t.Fatal("a focused active session must report focus")
	}
	if got := s.Window(); got != 17 {
		t.Fatalf("Window: got %d, want 17", got)
	}
	if got := s.Source(); got != SourceProcess {
		t.Fatalf("Source: got %v, want %v", got, SourceProcess)
	}
	if got, err := s.Geometry(); err != nil || got != s.geom {
		t.Fatalf("Geometry: got %+v, %v, want %+v, nil", got, err, s.geom)
	}

	s.alive = false
	if s.Alive() {
		t.Fatal("a closed session must not report alive")
	}
	if s.Focused() {
		t.Fatal("a closed session must not report focus")
	}
}

func TestContainsPID(t *testing.T) {
	if !containsPID([]uint32{3, 5, 8}, 5) {
		t.Fatal("containsPID must find a matching process id")
	}
	if containsPID([]uint32{3, 5, 8}, 13) {
		t.Fatal("containsPID must reject an absent process id")
	}
}
