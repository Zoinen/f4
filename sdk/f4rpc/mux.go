package f4rpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/vmihailenco/msgpack/v5"
)

// Message is the generic envelope for all RPC traffic.
type Message struct {
	Type   int                `msgpack:"t"` // 0: Request, 1: Response
	ID     uint32             `msgpack:"i"`
	Method string             `msgpack:"m,omitempty"`
	Data   msgpack.RawMessage `msgpack:"d,omitempty"`
	Error  string             `msgpack:"e,omitempty"`
}

// Handler defines a callback for processing incoming requests.
type Handler func(data msgpack.RawMessage) (any, error)

// Session multiplexes concurrent requests and responses over an io.Reader and io.Writer.
type Session struct {
	// OnError is called when an asynchronous response cannot be sent and may be called from a serving goroutine.
	// It is nil by default; if set, it must be set before Serve is called and be safe to call from serving goroutines.
	OnError func(error)

	enc      *msgpack.Encoder
	dec      *msgpack.Decoder
	mu       sync.Mutex
	handlers map[string]Handler
	pending  map[uint32]chan callResult
	nextID   uint32
	closed   bool
	closeErr error
}

type callResult struct {
	msg *Message
	err error
}

// NewSession creates a new RPC session.
func NewSession(r io.Reader, w io.Writer) *Session {
	return &Session{
		enc:      msgpack.NewEncoder(w),
		dec:      msgpack.NewDecoder(r),
		handlers: make(map[string]Handler),
		pending:  make(map[uint32]chan callResult),
	}
}

// Register assigns a callback to a specific RPC method name.
func (s *Session) Register(method string, h Handler) {
	s.handlers[method] = h
}

// Call makes a synchronous RPC call to the remote endpoint.
func (s *Session) Call(method string, params any, result any) error {
	return s.CallContext(context.Background(), method, params, result)
}

// CallContext makes a synchronous RPC call which can be abandoned by the
// caller. Cancellation removes the request from the pending set immediately;
// a late response is safely ignored. The remote handler is not forcibly
// interrupted by this transport-level primitive, so protocols which support
// active cancellation should expose an explicit cancel method as well.
func (s *Session) CallContext(ctx context.Context, method string, params any, result any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	var rawParams msgpack.RawMessage
	if params != nil {
		b, err := msgpack.Marshal(params)
		if err != nil {
			return fmt.Errorf("marshal params error: %w", err)
		}
		rawParams = b
	}

	id := atomic.AddUint32(&s.nextID, 1)
	ch := make(chan callResult, 1)

	s.mu.Lock()
	if s.closed {
		err := s.closeErr
		if err == nil {
			err = io.ErrClosedPipe
		}
		s.mu.Unlock()
		return err
	}
	s.pending[id] = ch

	req := &Message{
		Type:   0,
		ID:     id,
		Method: method,
		Data:   rawParams,
	}

	err := s.enc.Encode(req)
	if err != nil {
		delete(s.pending, id)
		s.mu.Unlock()
		return fmt.Errorf("send request error: %w", err)
	}
	s.mu.Unlock()

	var response callResult
	select {
	case response = <-ch:
	case <-ctx.Done():
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
		return ctx.Err()
	}
	if response.err != nil {
		return response.err
	}
	resp := response.msg
	if resp == nil {
		return io.ErrUnexpectedEOF
	}
	if resp.Error != "" {
		return fmt.Errorf("rpc error: %s", resp.Error)
	}

	if result != nil && len(resp.Data) > 0 {
		if err := msgpack.Unmarshal(resp.Data, result); err != nil {
			return fmt.Errorf("unmarshal result error: %w", err)
		}
	}
	return nil
}

// Serve starts the blocking loop that reads incoming messages.
func (s *Session) Serve() error {
	for {
		var msg Message
		if err := s.dec.Decode(&msg); err != nil {
			serveErr := err
			if err != io.EOF {
				serveErr = fmt.Errorf("decode error: %w", err)
			}
			s.failPending(serveErr)
			if err == io.EOF {
				return nil
			}
			return serveErr
		}

		switch msg.Type {
		case 1: // Response
			s.mu.Lock()
			ch, ok := s.pending[msg.ID]
			if ok {
				delete(s.pending, msg.ID)
			}
			s.mu.Unlock()
			if ok {
				ch <- callResult{msg: &msg}
			}
		case 0: // Request
			go s.handleRequest(&msg)
		}
	}
}

// failPending marks the session closed and releases every caller waiting for
// a response. It is intentionally idempotent because both sides of a pipe can
// notice shutdown at nearly the same time.
func (s *Session) failPending(err error) {
	if err == nil {
		err = io.ErrClosedPipe
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.closeErr = err
	pending := s.pending
	s.pending = make(map[uint32]chan callResult)
	s.mu.Unlock()

	for _, ch := range pending {
		ch <- callResult{err: err}
	}
}

func (s *Session) handleRequest(req *Message) {
	s.mu.Lock()
	h, ok := s.handlers[req.Method]
	s.mu.Unlock()

	resp := &Message{
		Type: 1,
		ID:   req.ID,
	}

	if !ok {
		resp.Error = fmt.Sprintf("method %q not found", req.Method)
	} else {
		res, err := h(req.Data)
		if err != nil {
			resp.Error = err.Error()
		} else if res != nil {
			b, err := msgpack.Marshal(res)
			if err != nil {
				resp.Error = "failed to marshal response"
			} else {
				resp.Data = b
			}
		}
	}

	s.mu.Lock()
	var encodeErr error
	if !s.closed {
		encodeErr = s.enc.Encode(resp)
	}
	s.mu.Unlock()
	if encodeErr != nil && s.OnError != nil {
		s.OnError(fmt.Errorf("response %d was not sent: %w", req.ID, encodeErr))
	}
}

// ErrClosed reports whether err came from a session whose transport ended.
// Callers normally use this to decide whether a child process may be safely
// restarted; it deliberately does not classify context cancellation.
func ErrClosed(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.ErrUnexpectedEOF)
}
