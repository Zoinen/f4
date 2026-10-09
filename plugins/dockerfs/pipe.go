package dockerfs

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// A Windows named pipe (Docker Desktop's \\.\pipe\docker_engine) is opened
// here as a plain file. A file handle opened that way is synchronous: a read
// that is waiting holds up a write on the same handle, which is exactly what
// net/http's transport does (a reader goroutine per connection alongside the
// writer). So the pipe gets its own tiny transport that does strictly one
// thing at a time: write the request, then read the response, one connection
// per request. That needs no overlapped I/O and no third-party pipe library.

// pipeTransport is an http.RoundTripper over connections made by dial.
type pipeTransport struct {
	dial func() (io.ReadWriteCloser, error)
}

// pipeRetries and pipeRetryDelay cover ERROR_PIPE_BUSY: the daemon has a
// limited number of pipe instances waiting and makes a new one a moment after
// each connection.
const (
	pipeRetries    = 20
	pipeRetryDelay = 50 * time.Millisecond
)

func (t *pipeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var conn io.ReadWriteCloser
	var err error
	for attempt := 0; ; attempt++ {
		if conn, err = t.dial(); err == nil {
			break
		}
		if attempt >= pipeRetries || !strings.Contains(strings.ToLower(err.Error()), "busy") {
			return nil, err
		}
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(pipeRetryDelay):
		}
	}
	// A cancelled request closes the connection under its blocked read.
	stop := context.AfterFunc(req.Context(), func() { _ = conn.Close() })

	req = req.Clone(req.Context())
	req.Close = true // one request per connection: the daemon hangs up after the answer
	if err := req.Write(conn); err != nil {
		stop()
		_ = conn.Close()
		return nil, ctxErr(req.Context(), err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		stop()
		_ = conn.Close()
		return nil, ctxErr(req.Context(), err)
	}
	resp.Body = &pipeBody{ReadCloser: resp.Body, conn: conn, stop: stop}
	return resp, nil
}

func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// pipeBody closes the connection with the body.
type pipeBody struct {
	io.ReadCloser
	conn io.Closer
	stop func() bool
}

func (b *pipeBody) Close() error {
	b.stop()
	return errors.Join(b.ReadCloser.Close(), b.conn.Close())
}

// openPipe opens a named pipe by its path.
func openPipe(path string) func() (io.ReadWriteCloser, error) {
	return func() (io.ReadWriteCloser, error) {
		return os.OpenFile(path, os.O_RDWR, 0) // #nosec G304 G703 -- the pipe DOCKER_HOST names
	}
}
