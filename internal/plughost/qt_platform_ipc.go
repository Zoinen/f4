package plughost

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

const extUiPlatformServicesCapability = "macPlatformServicesV1"

// ErrPlatformServicesUnavailable indicates that the Qt host cannot service native requests.
var ErrPlatformServicesUnavailable = errors.New("native platform services are unavailable")

// PlatformError carries a native service error, including user cancellation.
type PlatformError struct {
	Code      string
	Message   string
	Cancelled bool
}

func (e *PlatformError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Code != "" {
		return e.Code
	}
	return "platform request failed"
}

// PlatformResponse is a decoded native service reply or watch event.
type PlatformResponse struct {
	RequestID string
	Operation string
	Payload   map[string]any
	Chunk     bool
	Final     bool
	Event     bool
	Error     *PlatformError
}

type platformPendingRequest struct {
	operation string
	responses chan PlatformResponse
}

// platformIPCClient multiplexes native requests over the authenticated extui
// loopback connection. The existing nonce handshake is the authorization
// boundary; request IDs provide cancellation and stale-response rejection.
type platformIPCClient struct {
	send    *extUiMessageSender
	enabled bool
	nextID  atomic.Uint64

	mu       sync.Mutex
	pending  map[string]*platformPendingRequest
	closed   bool
	closeErr error
}

func newPlatformIPCClient(send *extUiMessageSender, enabled bool) *platformIPCClient {
	return &platformIPCClient{
		send: send, enabled: enabled && send != nil,
		pending: make(map[string]*platformPendingRequest),
	}
}

func (c *platformIPCClient) Available() bool {
	if c == nil || !c.enabled {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return !c.closed
}

func (c *platformIPCClient) Request(
	ctx context.Context,
	operation string,
	payload map[string]any,
	onResponse func(PlatformResponse) error,
) error {
	if !c.Available() {
		return ErrPlatformServicesUnavailable
	}
	requestID := fmt.Sprintf("p-%d", c.nextID.Add(1))
	pending := &platformPendingRequest{
		operation: operation,
		responses: make(chan PlatformResponse, 256),
	}
	c.mu.Lock()
	if c.closed {
		err := c.closeErr
		c.mu.Unlock()
		if err == nil {
			err = ErrPlatformServicesUnavailable
		}
		return err
	}
	c.pending[requestID] = pending
	c.mu.Unlock()

	message := map[string]any{
		"type":      "platform_request",
		"requestId": requestID,
		"operation": operation,
		"payload":   payload,
	}
	if err := c.send.Send(message); err != nil {
		c.removeRequest(requestID)
		return err
	}

	for {
		select {
		case <-ctx.Done():
			if c.removeRequest(requestID) {
				_ = c.send.Send(map[string]any{
					"type":      "platform_cancel",
					"requestId": requestID,
					"operation": operation,
				})
			}
			return ctx.Err()
		case response, ok := <-pending.responses:
			if !ok {
				c.mu.Lock()
				err := c.closeErr
				c.mu.Unlock()
				if err == nil {
					err = ErrPlatformServicesUnavailable
				}
				return err
			}
			if onResponse != nil {
				if err := onResponse(response); err != nil {
					c.removeRequest(requestID)
					_ = c.send.Send(map[string]any{
						"type": "platform_cancel", "requestId": requestID,
						"operation": operation,
					})
					return err
				}
			}
			if response.Final {
				c.removeRequest(requestID)
				if response.Error != nil {
					return response.Error
				}
				return nil
			}
		}
	}
}

func (c *platformIPCClient) removeRequest(requestID string) bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	_, exists := c.pending[requestID]
	delete(c.pending, requestID)
	c.mu.Unlock()
	return exists
}

func (c *platformIPCClient) handleResponse(message map[string]any) {
	if c == nil {
		return
	}
	requestID := extUiString(message, "requestId")
	c.mu.Lock()
	pending := c.pending[requestID]
	c.mu.Unlock()
	if pending == nil {
		return
	}
	messageType := extUiString(message, "type")
	operation := extUiString(message, "operation")
	response := PlatformResponse{
		RequestID: requestID,
		Operation: operation,
		Payload:   platformMessageMap(message["payload"]),
		Chunk:     ExtUiBool(message, "chunk"),
		Final:     ExtUiBool(message, "final"),
		Event:     messageType == "platform_event",
	}
	if rawError := platformMessageMap(message["error"]); len(rawError) != 0 {
		response.Error = &PlatformError{
			Code:      platformAnyString(rawError["code"]),
			Message:   platformAnyString(rawError["message"]),
			Cancelled: platformAnyBool(rawError["cancelled"]),
		}
	}
	protocolMessage := ""
	switch {
	case operation == "":
		protocolMessage = "platform response is missing its operation"
	case operation != pending.operation:
		protocolMessage = fmt.Sprintf(
			"platform response operation %q does not match request %q",
			operation, pending.operation)
	case messageType != "platform_response" && messageType != "platform_event":
		protocolMessage = fmt.Sprintf("unexpected platform message type %q", messageType)
	case response.Chunk && response.Final:
		protocolMessage = "platform response cannot be both a chunk and final"
	case messageType == "platform_response" && !response.Chunk && !response.Final:
		protocolMessage = "platform response is neither a chunk nor final"
	case messageType == "platform_event" && (response.Chunk || response.Final || response.Error != nil):
		protocolMessage = "platform event has response-only state"
	case response.Error != nil && !response.Final:
		protocolMessage = "platform error is not final"
	}
	if protocolMessage != "" {
		response.Chunk = false
		response.Final = true
		response.Event = false
		response.Error = &PlatformError{
			Code: "protocol", Message: protocolMessage,
		}
	}
	select {
	case pending.responses <- response:
	default:
		// A bounded native producer must not stall keyboard/mouse IPC if a
		// consumer disappears. Fail that request and cancel it explicitly.
		if c.removeRequest(requestID) {
			for {
				select {
				case <-pending.responses:
					continue
				default:
				}
				break
			}
			pending.responses <- PlatformResponse{
				RequestID: requestID,
				Operation: pending.operation,
				Final:     true,
				Error: &PlatformError{
					Code:    "buffer_overflow",
					Message: "native platform response exceeded the bounded request buffer",
				},
			}
			_ = c.send.Send(map[string]any{
				"type": "platform_cancel", "requestId": requestID,
				"operation": pending.operation,
			})
		}
	}
}

func (c *platformIPCClient) Close(err error) {
	if c == nil {
		return
	}
	if err == nil {
		err = ErrPlatformServicesUnavailable
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.closeErr = err
	pending := c.pending
	c.pending = make(map[string]*platformPendingRequest)
	c.mu.Unlock()
	for _, request := range pending {
		select {
		case request.responses <- PlatformResponse{
			Operation: request.operation,
			Final:     true,
			Error: &PlatformError{
				Code: "disconnected", Message: err.Error(),
			},
		}:
		default:
		}
	}
}

func platformMessageMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			out[fmt.Sprint(key)] = item
		}
		return out
	default:
		return nil
	}
}

func platformAnyString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}

func platformAnyBool(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case int8:
		return typed != 0
	case int64:
		return typed != 0
	case uint64:
		return typed != 0
	default:
		return false
	}
}

var activePlatformIPC struct {
	sync.RWMutex
	client *platformIPCClient
}

func setActivePlatformIPC(client *platformIPCClient) func() {
	activePlatformIPC.Lock()
	previous := activePlatformIPC.client
	activePlatformIPC.client = client
	activePlatformIPC.Unlock()
	return func() {
		activePlatformIPC.Lock()
		if activePlatformIPC.client == client {
			activePlatformIPC.client = previous
		}
		activePlatformIPC.Unlock()
	}
}

func currentPlatformIPC() *platformIPCClient {
	activePlatformIPC.RLock()
	client := activePlatformIPC.client
	activePlatformIPC.RUnlock()
	return client
}

// PlatformServices is the native request boundary used by views. The transport,
// authentication and request multiplexing remain private to the plugin host.
type PlatformServices interface {
	Available() bool
	Request(context.Context, string, map[string]any, func(PlatformResponse) error) error
}

// CurrentPlatformServices returns the active Qt host's native services, if any.
func CurrentPlatformServices() PlatformServices {
	client := currentPlatformIPC()
	if client == nil {
		return nil
	}
	return client
}
