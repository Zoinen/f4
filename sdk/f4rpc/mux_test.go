package f4rpc

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/vmihailenco/msgpack/v5"
)

func TestSession_CallAndServe(t *testing.T) {
	// Эмулируем стандартные потоки с помощью in-memory пайпов
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()

	client := NewSession(s2cR, c2sW)
	server := NewSession(c2sR, s2cW)

	// Регистрируем хендлер на сервере
	server.Register("Test.Echo", func(data msgpack.RawMessage) (any, error) {
		var msg string
		if err := msgpack.Unmarshal(data, &msg); err != nil {
			return nil, err
		}
		return msg + " (RPC)", nil
	})

	// Запускаем слушателей в фоне
	serverErr := make(chan error, 1)
	clientErr := make(chan error, 1)
	go func() { serverErr <- server.Serve() }()
	go func() { clientErr <- client.Serve() }()
	t.Cleanup(func() {
		if err := c2sW.Close(); err != nil {
			t.Errorf("close client pipe: %v", err)
		}
		if err := s2cW.Close(); err != nil {
			t.Errorf("close server pipe: %v", err)
		}
		if err := <-serverErr; err != nil {
			t.Errorf("server Serve: %v", err)
		}
		if err := <-clientErr; err != nil {
			t.Errorf("client Serve: %v", err)
		}
	})

	// Выполняем синхронный вызов с клиента на сервер
	var res string
	err := client.Call("Test.Echo", "Hello", &res)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}

	if res != "Hello (RPC)" {
		t.Errorf("Unexpected RPC response: %q", res)
	}
}

func TestSessionCallContextCancellationReleasesCaller(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	client := NewSession(reader, io.Discard)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := client.CallContext(ctx, "Blocked", nil, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CallContext error = %v, want context.Canceled", err)
	}
}

func TestSessionServeEOFReleasesPendingCalls(t *testing.T) {
	reader, writer := io.Pipe()
	client := NewSession(reader, io.Discard)
	done := make(chan error, 1)
	go func() { done <- client.Call("Blocked", nil, nil) }()
	time.Sleep(10 * time.Millisecond)
	_ = writer.Close()
	_ = client.Serve()
	select {
	case err := <-done:
		if !ErrClosed(err) {
			t.Fatalf("pending call error = %v, want closed transport", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending call was not released after EOF")
	}
}

func TestSession_MethodNotFound(t *testing.T) {
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()

	client := NewSession(s2cR, c2sW)
	server := NewSession(c2sR, s2cW)

	serverErr := make(chan error, 1)
	clientErr := make(chan error, 1)
	go func() { serverErr <- server.Serve() }()
	go func() { clientErr <- client.Serve() }()
	t.Cleanup(func() {
		if err := c2sW.Close(); err != nil {
			t.Errorf("close client pipe: %v", err)
		}
		if err := s2cW.Close(); err != nil {
			t.Errorf("close server pipe: %v", err)
		}
		if err := <-serverErr; err != nil {
			t.Errorf("server Serve: %v", err)
		}
		if err := <-clientErr; err != nil {
			t.Errorf("client Serve: %v", err)
		}
	})

	err := client.Call("Unknown.Method", nil, nil)
	if err == nil {
		t.Fatal("Expected error for unknown method, got nil")
	}
	if !strings.Contains(err.Error(), "method \"Unknown.Method\" not found") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestSession_Concurrency(t *testing.T) {
	c2sR, c2sW := io.Pipe()
	s2cR, s2cW := io.Pipe()

	client := NewSession(s2cR, c2sW)
	server := NewSession(c2sR, s2cW)

	server.Register("Ping", func(data msgpack.RawMessage) (any, error) {
		time.Sleep(10 * time.Millisecond) // Имитация задержки обработки
		return "Pong", nil
	})

	serverErr := make(chan error, 1)
	clientErr := make(chan error, 1)
	go func() { serverErr <- server.Serve() }()
	go func() { clientErr <- client.Serve() }()
	t.Cleanup(func() {
		if err := c2sW.Close(); err != nil {
			t.Errorf("close client pipe: %v", err)
		}
		if err := s2cW.Close(); err != nil {
			t.Errorf("close server pipe: %v", err)
		}
		if err := <-serverErr; err != nil {
			t.Errorf("server Serve: %v", err)
		}
		if err := <-clientErr; err != nil {
			t.Errorf("client Serve: %v", err)
		}
	})

	done := make(chan bool)
	for i := 0; i < 50; i++ {
		go func() {
			var res string
			err := client.Call("Ping", nil, &res)
			if err != nil || res != "Pong" {
				t.Errorf("Concurrent ping failed: err=%v, res=%s", err, res)
			}
			done <- true
		}()
	}

	// Ожидаем завершения всех 50 горутин
	timeout := time.After(2 * time.Second)
	for i := 0; i < 50; i++ {
		select {
		case <-done:
		case <-timeout:
			t.Fatal("Timeout waiting for concurrent calls to finish")
		}
	}
}

func TestSession_ResponseEncodeError(t *testing.T) {
	encodeErr := errors.New("encode failed")
	sess := NewSession(strings.NewReader(""), errorWriter{err: encodeErr})

	var got error
	sess.OnError = func(err error) { got = err }
	sess.handleRequest(&Message{ID: 42, Method: "missing"})

	if !errors.Is(got, encodeErr) {
		t.Fatalf("OnError error = %v, want %v", got, encodeErr)
	}
	if !strings.Contains(got.Error(), "response 42 was not sent") {
		t.Fatalf("OnError error = %q, want response ID", got)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
