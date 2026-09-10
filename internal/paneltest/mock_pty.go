package paneltest

import (
	"io"
	"sync"
	"time"
)

// MockPty is a terminal.PtyBackend that records what was Written to it and
// never produces input. internal/terminal keeps its own copy for its own tests:
// a mock is not scaffolding worth sharing across a package boundary, and the
// two are free to drift apart with their subjects. This one is shared only
// because building a panels frame needs it and two packages build one.
type MockPty struct {
	mu sync.Mutex
	// Written is everything the frame sent to this backend.
	Written []byte
	closed  bool
}

func (m *MockPty) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Written = append(m.Written, b...)
	return len(b), nil
}

func (m *MockPty) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return string(m.Written)
}

func (m *MockPty) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Written = nil
}

func (m *MockPty) Read(b []byte) (int, error) {
	for {
		m.mu.Lock()
		closed := m.closed
		m.mu.Unlock()
		if closed {
			return 0, io.EOF
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (m *MockPty) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *MockPty) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *MockPty) SetSize(cols, rows int)                {}
func (m *MockPty) Wait() error                           { return nil }
func (m *MockPty) Run(name string, args ...string) error { return nil }
func (m *MockPty) IsBusy() bool                          { return false }
