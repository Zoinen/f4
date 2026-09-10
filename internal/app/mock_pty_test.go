package app

import (
	io "io"
	sync "sync"
	time "time"
)

type mockPty struct {
	mu      sync.Mutex
	written []byte
	closed  bool
}

func (m *mockPty) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = append(m.written, b...)
	return len(b), nil
}

func (m *mockPty) String() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return string(m.written)
}

func (m *mockPty) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.written = nil
}

func (m *mockPty) Read(b []byte) (int, error) {
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

func (m *mockPty) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockPty) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *mockPty) SetSize(cols, rows int) {}

func (m *mockPty) Wait() error { return nil }

func (m *mockPty) Run(name string, args ...string) error { return nil }

func (m *mockPty) IsBusy() bool { return false }
