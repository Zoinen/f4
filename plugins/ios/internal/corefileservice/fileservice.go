package corefileservice

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	goios "github.com/danielpaulus/go-ios/ios"
	"github.com/danielpaulus/go-ios/ios/xpc"
	"github.com/google/uuid"
)

const (
	ControlServiceName = "com.apple.coredevice.fileservice.control"
	DataServiceName    = "com.apple.coredevice.fileservice.data"

	// MaxFileSize matches the 32-bit size field in the data-service protocol
	// while retaining the upstream defensive limit.
	MaxFileSize uint32 = 1 << 30

	defaultReceiveTimeout = 30 * time.Second
)

// ErrTimeout means neither FileService response stream answered before the
// receive deadline. The connection must be discarded after this error because
// one of the outstanding reads may still receive a late response.
var ErrTimeout = errors.New("timed out waiting for a response from the device")

// DeviceError is an error sent by the device in an EncodedError response.
type DeviceError struct {
	Description string
	Encoded     interface{}
}

func (e *DeviceError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("device error: %s", e.Description)
	}
	return fmt.Sprintf("device error: %+v", e.Encoded)
}

// Domain selects the device-side FileService container.
type Domain uint64

const (
	DomainAppDataContainer Domain = iota + 1
	DomainAppGroupDataContainer
	DomainTemporary
	DomainRootStaging
	DomainSystemCrashLogs
)

// controlConnection is the testable subset of the RemoteXPC connection used
// by FileService. Replies to a directory request can arrive on either stream.
type controlConnection interface {
	Send(map[string]interface{}, ...uint32) error
	ReceiveOnClientServerStream() (map[string]interface{}, error)
	ReceiveOnServerClientStream() (map[string]interface{}, error)
	Close() error
}

type receiveResult struct {
	response map[string]interface{}
	err      error
}

// Connection is one serialized CoreDevice FileService session.
type Connection struct {
	mu sync.Mutex

	conn       controlConnection
	device     goios.DeviceEntry
	sessionID  string
	domain     Domain
	identifier string

	receiveTimeout time.Duration
	pendingControl chan receiveResult
	// pendingList holds the client-to-server stream read left behind when a
	// directory request finishes on the control stream. Reusing it on the next
	// ListDirectory is essential: starting a second reader would let either
	// goroutine steal the next response and permanently desynchronise the
	// connection.
	pendingList chan receiveResult
}

// New connects to FileService and creates a session for domain.
func New(device goios.DeviceEntry, domain Domain, identifier string) (*Connection, error) {
	conn, err := goios.ConnectToXpcServiceTunnelIface(device, ControlServiceName)
	if err != nil {
		return nil, fmt.Errorf("New: failed to connect to file service: %w", err)
	}
	c := &Connection{
		conn: conn, device: device, domain: domain, identifier: identifier,
		receiveTimeout: defaultReceiveTimeout,
	}
	if err := c.createSession(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("New: failed to create session: %w", err)
	}
	return c, nil
}

func (c *Connection) createSession() error {
	request := map[string]interface{}{
		"Cmd":        "CreateSession",
		"Domain":     uint64(c.domain),
		"Identifier": c.identifier,
		"Session":    "",
		"User":       "mobile",
	}
	if err := c.conn.Send(request, xpc.HeartbeatRequestFlag); err != nil {
		return fmt.Errorf("createSession: failed to send request: %w", err)
	}
	response, err := c.receiveControl()
	if err != nil {
		return fmt.Errorf("createSession: failed to receive response: %w", err)
	}
	if err := extractError(response); err != nil {
		return fmt.Errorf("createSession: %w", err)
	}
	sessionID, ok := response["NewSessionID"].(string)
	if !ok {
		return fmt.Errorf("createSession: missing or invalid NewSessionID in response (got: %+v)", response)
	}
	c.sessionID = sessionID
	return nil
}

// receiveControl consumes a read left pending by a successful directory list
// before starting another reader on the server-to-client stream.
func (c *Connection) receiveControl() (map[string]interface{}, error) {
	if c.pendingControl != nil {
		result := <-c.pendingControl
		c.pendingControl = nil
		return result.response, result.err
	}
	return c.conn.ReceiveOnServerClientStream()
}

// ListDirectory waits on both FileService response streams. Successful lists
// normally arrive on the client-to-server stream, while device errors arrive
// on the control stream.
func (c *Connection) ListDirectory(path string) ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	request := map[string]interface{}{
		"Cmd":         "RetrieveDirectoryList",
		"MessageUUID": uuid.New().String(),
		"Path":        path,
		"SessionID":   c.sessionID,
	}
	if err := c.conn.Send(request, xpc.HeartbeatRequestFlag); err != nil {
		return nil, fmt.Errorf("ListDirectory: failed to send request: %w", err)
	}

	listCh := c.pendingList
	if listCh == nil {
		listCh = make(chan receiveResult, 1)
		conn := c.conn
		go func() {
			response, err := conn.ReceiveOnClientServerStream()
			listCh <- receiveResult{response: response, err: err}
		}()
	}
	c.pendingList = listCh

	controlCh := c.pendingControl
	if controlCh == nil {
		controlCh = make(chan receiveResult, 1)
		conn := c.conn
		go func() {
			response, err := conn.ReceiveOnServerClientStream()
			controlCh <- receiveResult{response: response, err: err}
		}()
	}
	c.pendingControl = controlCh

	timer := time.NewTimer(c.receiveTimeout)
	defer timer.Stop()
	select {
	case result := <-listCh:
		c.pendingList = nil
		if result.err != nil {
			return nil, fmt.Errorf("ListDirectory: failed to receive response: %w", result.err)
		}
		if err := extractError(result.response); err != nil {
			return nil, fmt.Errorf("ListDirectory: %w", err)
		}
		return parseFileList(result.response)
	case result := <-controlCh:
		c.pendingControl = nil
		if result.err != nil {
			return nil, fmt.Errorf("ListDirectory: failed to receive control response: %w", result.err)
		}
		if err := extractError(result.response); err != nil {
			return nil, fmt.Errorf("ListDirectory: %w", err)
		}
		return nil, fmt.Errorf("ListDirectory: unexpected control response without file list (got: %+v)", result.response)
	case <-timer.C:
		return nil, fmt.Errorf("ListDirectory: no response within %v: %w", c.receiveTimeout, ErrTimeout)
	}
}

func parseFileList(response map[string]interface{}) ([]string, error) {
	raw, ok := response["FileList"]
	if !ok {
		return nil, errors.New("ListDirectory: missing FileList in response")
	}
	values, ok := raw.([]interface{})
	if !ok {
		return nil, errors.New("ListDirectory: FileList is not an array")
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if name, ok := value.(string); ok {
			result = append(result, name)
		}
	}
	return result, nil
}

// PullFile requests a file on the control service and streams its data to
// writer through a short-lived raw FileService data connection.
func (c *Connection) PullFile(path string, writer io.Writer) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	request := map[string]interface{}{
		"Cmd": "RetrieveFile", "Path": path, "SessionID": c.sessionID,
	}
	if err := c.conn.Send(request, xpc.HeartbeatRequestFlag); err != nil {
		return fmt.Errorf("PullFile: failed to send request: %w", err)
	}
	response, err := c.receiveControl()
	if err != nil {
		return fmt.Errorf("PullFile: failed to receive response: %w", err)
	}
	if err := extractError(response); err != nil {
		return fmt.Errorf("PullFile: %w", err)
	}
	responseToken, ok := response["Response"].(uint64)
	if !ok {
		return errors.New("PullFile: missing or invalid Response token")
	}
	fileID, ok := response["NewFileID"].(uint64)
	if !ok {
		return errors.New("PullFile: missing or invalid NewFileID")
	}
	if err := c.downloadFileData(responseToken, fileID, writer); err != nil {
		return fmt.Errorf("PullFile: failed to download file data: %w", err)
	}
	return nil
}

func (c *Connection) downloadFileData(responseToken, fileID uint64, writer io.Writer) error {
	conn, err := goios.ConnectToServiceTunnelIface(c.device, DataServiceName)
	if err != nil {
		return fmt.Errorf("downloadFileData: failed to connect to data service: %w", err)
	}
	defer func() { _ = conn.Close() }() // Download-connection cleanup is best effort.

	request := make([]byte, 40)
	copy(request[:8], "rwb!FILE")
	binary.BigEndian.PutUint64(request[8:16], responseToken)
	binary.BigEndian.PutUint64(request[24:32], fileID)
	if err := writeFull(conn, request); err != nil {
		return fmt.Errorf("downloadFileData: failed to send wire request: %w", err)
	}
	if err := receiveFileDataToWriter(conn, writer); err != nil {
		return fmt.Errorf("downloadFileData: failed to receive file data: %w", err)
	}
	return nil
}

func receiveFileDataToWriter(conn io.Reader, writer io.Writer) error {
	var header [40]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return fmt.Errorf("receiveFileDataToWriter: failed to read header: %w", err)
	}
	if string(header[:8]) != "rwb!FILE" {
		return fmt.Errorf("receiveFileDataToWriter: invalid data-service magic %q", header[:8])
	}
	fileSize := binary.BigEndian.Uint32(header[36:40])
	if fileSize > MaxFileSize {
		return fmt.Errorf("receiveFileDataToWriter: file size %d exceeds maximum allowed size %d", fileSize, MaxFileSize)
	}

	const chunkSize = 256 << 10
	buffer := make([]byte, chunkSize)
	remaining := int64(fileSize)
	for remaining > 0 {
		count := min(remaining, int64(len(buffer)))
		n, err := io.ReadFull(conn, buffer[:count])
		if err != nil {
			return fmt.Errorf("receiveFileDataToWriter: failed to read chunk: %w", err)
		}
		if err := writeFull(writer, buffer[:n]); err != nil {
			return fmt.Errorf("receiveFileDataToWriter: failed to write chunk: %w", err)
		}
		remaining -= int64(n)
	}
	return nil
}

func writeFull(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if n > 0 {
			data = data[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// Close closes the control connection. It serializes with protocol operations
// so no response reader is started after shutdown begins.
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.Close()
}

func extractError(response map[string]interface{}) error {
	encoded, ok := response["EncodedError"]
	if !ok || encoded == nil {
		return nil
	}
	deviceErr := &DeviceError{Encoded: encoded}
	if values, ok := encoded.(map[string]interface{}); ok {
		deviceErr.Description, _ = values["LocalizedDescription"].(string)
	}
	return deviceErr
}
