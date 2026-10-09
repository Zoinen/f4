package editor

import "sync"

// setupCounter counts goroutines at work and lets a caller wait until none
// are.
type setupCounter struct {
	mu   sync.Mutex
	idle sync.Cond
	n    int
}

func newSetupCounter() *setupCounter {
	c := &setupCounter{}
	c.idle.L = &c.mu
	return c
}

// start is called before the goroutine is started, so that a wait that
// begins before the goroutine gets to run still waits for it.
func (c *setupCounter) start() {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
}

func (c *setupCounter) done() {
	c.mu.Lock()
	c.n--
	if c.n == 0 {
		c.idle.Broadcast()
	}
	c.mu.Unlock()
}

func (c *setupCounter) wait() {
	c.mu.Lock()
	for c.n > 0 {
		c.idle.Wait()
	}
	c.mu.Unlock()
}

// colorerSetups counts the goroutines that set up a Colorer session: an
// editor's highlighter, the editor background colour, and the viewer's text
// and window colorizers. Setting a session up writes the Radiola colour style
// into the configuration directory (ensureRadiolaSchema), and closing the
// editor or viewer first cancels the goroutine without stopping that write.
// Tests wait for them before their configuration directory is removed; the
// write used to land while t.TempDir's cleanup was removing it, which failed
// with "directory not empty".
var colorerSetups = newSetupCounter()
