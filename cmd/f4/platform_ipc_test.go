package main

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestPlatformIPCRequestStreamsChunksAndFinal(t *testing.T) {
	core, host := net.Pipe()
	defer core.Close()
	defer host.Close()
	client := newPlatformIPCClient(&extUiMessageSender{w: core}, true)
	defer client.Close(nil)

	go func() {
		request, err := extUiReadMessage(host)
		if err != nil {
			return
		}
		requestID := extUiString(request, "requestId")
		client.handleResponse(map[string]any{
			"type": "platform_response", "requestId": requestID,
			"operation": "macos.locations", "chunk": true, "final": false,
			"payload": map[string]any{"items": []any{"first"}},
		})
		client.handleResponse(map[string]any{
			"type": "platform_response", "requestId": requestID,
			"operation": "macos.locations", "chunk": false, "final": true,
			"payload": map[string]any{"items": []any{"second"}},
		})
	}()

	var chunks []string
	err := client.Request(context.Background(), "macos.locations", nil,
		func(response platformResponse) error {
			for _, item := range response.Payload["items"].([]any) {
				chunks = append(chunks, item.(string))
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	if len(chunks) != 2 || chunks[0] != "first" || chunks[1] != "second" {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestPlatformIPCCancellationSendsNativeCancel(t *testing.T) {
	core, host := net.Pipe()
	defer core.Close()
	defer host.Close()
	client := newPlatformIPCClient(&extUiMessageSender{w: core}, true)
	defer client.Close(nil)
	cancelSeen := make(chan map[string]any, 1)
	go func() {
		request, err := extUiReadMessage(host)
		if err != nil || extUiString(request, "type") != "platform_request" {
			return
		}
		cancel, err := extUiReadMessage(host)
		if err == nil {
			cancelSeen <- cancel
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- client.Request(ctx, "macos.query", map[string]any{"kind": "recents"}, nil)
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Request returned %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled request did not return")
	}
	select {
	case message := <-cancelSeen:
		if extUiString(message, "type") != "platform_cancel" ||
			extUiString(message, "operation") != "macos.query" {
			t.Fatalf("unexpected cancellation message: %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("native cancellation message was not sent")
	}
}

func TestPlatformIPCRejectsUnavailableAndStaleResponses(t *testing.T) {
	if err := (*platformIPCClient)(nil).Request(
		context.Background(), "macos.query", nil, nil); !errors.Is(err, errPlatformServicesUnavailable) {
		t.Fatalf("nil client returned %v", err)
	}
	client := newPlatformIPCClient(nil, false)
	client.handleResponse(map[string]any{
		"type": "platform_response", "requestId": "stale", "final": true,
	})
	if err := client.Request(context.Background(), "macos.query", nil, nil); !errors.Is(err, errPlatformServicesUnavailable) {
		t.Fatalf("disabled client returned %v", err)
	}
}

func TestPlatformIPCStreamsEventsUntilCancellation(t *testing.T) {
	core, host := net.Pipe()
	defer core.Close()
	defer host.Close()
	client := newPlatformIPCClient(&extUiMessageSender{w: core}, true)
	defer client.Close(nil)
	cancelSeen := make(chan map[string]any, 1)
	go func() {
		request, err := extUiReadMessage(host)
		if err != nil {
			return
		}
		requestID := extUiString(request, "requestId")
		client.handleResponse(map[string]any{
			"type": "platform_response", "requestId": requestID,
			"operation": "macos.watch", "chunk": true, "final": false,
			"payload": map[string]any{"ready": true},
		})
		client.handleResponse(map[string]any{
			"type": "platform_event", "requestId": requestID,
			"operation": "macos.watch", "chunk": false, "final": false,
			"payload": map[string]any{"refresh": true},
		})
		cancel, err := extUiReadMessage(host)
		if err == nil {
			cancelSeen <- cancel
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	events := 0
	err := client.Request(ctx, "macos.watch", map[string]any{"kind": "recents"},
		func(response platformResponse) error {
			if response.Event {
				events++
				cancel()
			}
			return nil
		})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("watch request returned %v", err)
	}
	if events != 1 {
		t.Fatalf("received %d platform events, want 1", events)
	}
	select {
	case message := <-cancelSeen:
		if extUiString(message, "type") != "platform_cancel" ||
			extUiString(message, "operation") != "macos.watch" {
			t.Fatalf("unexpected watch cancellation: %#v", message)
		}
	case <-time.After(time.Second):
		t.Fatal("watch cancellation was not sent")
	}
}

func TestPlatformIPCRejectsMismatchedOperation(t *testing.T) {
	core, host := net.Pipe()
	defer core.Close()
	defer host.Close()
	client := newPlatformIPCClient(&extUiMessageSender{w: core}, true)
	defer client.Close(nil)
	go func() {
		request, err := extUiReadMessage(host)
		if err != nil {
			return
		}
		client.handleResponse(map[string]any{
			"type":      "platform_response",
			"requestId": extUiString(request, "requestId"),
			"operation": "macos.mount", "final": true,
		})
	}()

	err := client.Request(context.Background(), "macos.query", nil, nil)
	var structured *platformStructuredError
	if !errors.As(err, &structured) || structured.Code != "protocol" {
		t.Fatalf("mismatched response returned %v", err)
	}
}

func TestPlatformIPCRejectsMalformedResponse(t *testing.T) {
	core, host := net.Pipe()
	defer core.Close()
	defer host.Close()
	client := newPlatformIPCClient(&extUiMessageSender{w: core}, true)
	defer client.Close(nil)
	go func() {
		request, err := extUiReadMessage(host)
		if err != nil {
			return
		}
		client.handleResponse(map[string]any{
			"type":      "platform_response",
			"requestId": extUiString(request, "requestId"),
			"final":     true,
		})
	}()

	err := client.Request(context.Background(), "macos.query", nil, nil)
	var structured *platformStructuredError
	if !errors.As(err, &structured) || structured.Code != "protocol" {
		t.Fatalf("malformed response returned %v", err)
	}
}

func TestPlatformIPCBufferOverflowFailsRequest(t *testing.T) {
	var wire bytes.Buffer
	client := newPlatformIPCClient(&extUiMessageSender{w: &wire}, true)
	pending := &platformPendingRequest{
		operation: "macos.query",
		responses: make(chan platformResponse, 1),
	}
	client.pending["p-overflow"] = pending
	message := map[string]any{
		"type": "platform_response", "requestId": "p-overflow",
		"operation": "macos.query", "chunk": true, "final": false,
	}
	client.handleResponse(message)
	client.handleResponse(message)

	response := <-pending.responses
	if !response.Final || response.Error == nil || response.Error.Code != "buffer_overflow" {
		t.Fatalf("overflow response = %#v", response)
	}
	client.mu.Lock()
	_, stillPending := client.pending["p-overflow"]
	client.mu.Unlock()
	if stillPending {
		t.Fatal("overflowed request was not removed")
	}
	if wire.Len() == 0 {
		t.Fatal("overflow did not send platform_cancel")
	}
}

func TestPlatformIPCDisconnectFailsPendingRequest(t *testing.T) {
	core, host := net.Pipe()
	defer core.Close()
	defer host.Close()
	client := newPlatformIPCClient(&extUiMessageSender{w: core}, true)
	requestSeen := make(chan struct{})
	go func() {
		if _, err := extUiReadMessage(host); err == nil {
			close(requestSeen)
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- client.Request(context.Background(), "macos.query", nil, nil)
	}()
	select {
	case <-requestSeen:
	case <-time.After(time.Second):
		t.Fatal("pending request was not written")
	}
	client.Close(errors.New("native host disconnected"))

	select {
	case err := <-done:
		var structured *platformStructuredError
		if !errors.As(err, &structured) || structured.Code != "disconnected" {
			t.Fatalf("disconnect returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending request did not fail after disconnect")
	}
	if client.Available() {
		t.Fatal("disconnected platform client still reports available")
	}
}
