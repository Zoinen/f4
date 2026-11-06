package filemenu

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
)

// helperClient serializes requests over one process. Once a request has been
// sent, any transport failure is final: an uncertain verb is never replayed.
type helperClient struct {
	mu        sync.Mutex
	processMu sync.Mutex
	process   *helperProcess
	closed    bool
	command   func(context.Context) (*exec.Cmd, error)
}

type helperProcess struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	out     io.ReadCloser
	replies *bufio.Scanner
	done    chan struct{}
}

func (c *helperClient) close() {
	c.processMu.Lock()
	defer c.processMu.Unlock()
	c.closeLocked()
}

func (c *helperClient) shutdown() {
	c.processMu.Lock()
	defer c.processMu.Unlock()
	c.closed = true
	c.closeLocked()
}

func (c *helperClient) closeLocked() {
	if p := c.process; p != nil {
		_ = p.in.Close()
		_ = p.cmd.Process.Kill()
		<-p.done
		_ = p.out.Close()
		c.process = nil
	}
}

func (c *helperClient) start() (*helperProcess, error) {
	c.processMu.Lock()
	defer c.processMu.Unlock()
	if c.closed {
		return nil, fmt.Errorf("File menu client is closed")
	}
	if p := c.process; p != nil {
		select {
		case <-p.done:
			_ = p.in.Close()
			_ = p.out.Close()
			c.process = nil
		default:
			return p, nil
		}
	}
	cmd, err := c.command(context.Background())
	if err != nil {
		return nil, err
	}
	configureProcess(cmd)
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, writer, err := os.Pipe()
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	cmd.Stdout = writer
	if err = cmd.Start(); err != nil {
		_ = in.Close()
		_ = out.Close()
		_ = writer.Close()
		return nil, err
	}
	_ = writer.Close()
	p := &helperProcess{cmd: cmd, in: in, out: out, replies: bufio.NewScanner(out), done: make(chan struct{})}
	p.replies.Buffer(make([]byte, 4096), 4<<20)
	c.process = p
	go func() { _ = cmd.Wait(); close(p.done) }()
	return p, nil
}

func (c *helperClient) run(ctx context.Context, r Request) Result {
	return c.runWhen(ctx, r, nil)
}

func (c *helperClient) runWhen(ctx context.Context, r Request, current func() bool) Result {
	c.mu.Lock()
	defer c.mu.Unlock()
	if ctx.Err() != nil || (current != nil && !current()) {
		return Result{Outcome: Cancelled}
	}
	if err := validate(r); err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	p, err := c.start()
	if err != nil {
		return Result{Outcome: Unavailable, Error: err.Error()}
	}
	prepareProcessRequest(p.cmd, r)
	// Killing the process unblocks both pipe operations, including a hung shell
	// extension. Join the cancellation callback before another request starts.
	stopped := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { c.close(); close(stopped) })
	defer func() {
		if !stop() {
			<-stopped
		}
	}()
	if err = json.NewEncoder(p.in).Encode(r); err == nil {
		if p.replies.Scan() {
			var result Result
			err = json.Unmarshal(p.replies.Bytes(), &result)
			if err == nil {
				switch result.Outcome {
				case Cancelled, Invoked, Selected, Unavailable, Failed:
					return result
				default:
					err = fmt.Errorf("invalid outcome %q", result.Outcome)
				}
			}
		} else {
			err = p.replies.Err()
			if err == nil {
				err = io.ErrUnexpectedEOF
			}
		}
	}
	c.close()
	if ctx.Err() != nil {
		return Result{Outcome: Cancelled}
	}
	return Result{Outcome: Failed, Error: fmt.Sprintf("File menu helper: %v", err)}
}
