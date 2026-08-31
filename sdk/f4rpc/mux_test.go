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
	go server.Serve()
	go client.Serve()

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

	go server.Serve()
	go client.Serve()

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

	go server.Serve()
	go client.Serve()

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
