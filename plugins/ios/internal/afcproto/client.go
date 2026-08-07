package afcproto

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Client serializes complete AFC request/response exchanges. An AFC connection
// cannot safely multiplex requests, so all Files opened by a Client share this
// serialization boundary.
type Client struct {
	conn io.ReadWriteCloser

	mu        sync.Mutex
	packetNum uint64

	stateMu   sync.Mutex
	terminal  error
	closeOnce sync.Once
	closeErr  error
}

func New(conn io.ReadWriteCloser) *Client {
	if conn == nil {
		panic("afcproto: nil connection")
	}
	return &Client{conn: conn}
}

func (c *Client) Close() error {
	c.stateMu.Lock()
	if c.terminal == nil {
		c.terminal = ErrClosed
	}
	c.stateMu.Unlock()
	c.closeConnection()
	return c.closeErr
}

func (c *Client) List(ctx context.Context, name string) ([]string, error) {
	clean, err := cleanPath("readdir", name, false)
	if err != nil {
		return nil, err
	}
	resp, err := c.exchange(ctx, opReadDir, pathBytes(clean), nil, opData)
	if err != nil {
		return nil, pathError("readdir", name, err)
	}
	entries, err := parseList(resp.payload)
	if err != nil {
		return nil, pathError("readdir", name, c.protocolFailure(err))
	}
	return entries, nil
}

func (c *Client) Stat(ctx context.Context, name string) (FileInfo, error) {
	return c.GetFileInfo(ctx, name)
}

func (c *Client) GetFileInfo(ctx context.Context, name string) (FileInfo, error) {
	clean, err := cleanPath("stat", name, false)
	if err != nil {
		return FileInfo{}, err
	}
	resp, err := c.exchange(ctx, opGetFileInfo, pathBytes(clean), nil, opData)
	if err != nil {
		return FileInfo{}, pathError("stat", name, err)
	}
	values, err := parseDictionary(resp.payload)
	if err != nil {
		return FileInfo{}, pathError("stat", name, c.protocolFailure(err))
	}
	info, err := fileInfoFromDictionary(clean, values)
	if err != nil {
		return FileInfo{}, pathError("stat", name, c.protocolFailure(err))
	}
	return info, nil
}

func (c *Client) DeviceInfo(ctx context.Context) (DeviceInfo, error) {
	resp, err := c.exchange(ctx, opGetDeviceInfo, nil, nil, opData)
	if err != nil {
		return DeviceInfo{}, fmt.Errorf("afc device info: %w", err)
	}
	values, err := parseDictionary(resp.payload)
	if err != nil {
		return DeviceInfo{}, c.protocolFailure(err)
	}
	info := DeviceInfo{Model: values["Model"], Values: values}
	for key, dst := range map[string]*uint64{
		"FSTotalBytes": &info.TotalBytes,
		"FSFreeBytes":  &info.FreeBytes,
		"FSBlockSize":  &info.BlockSize,
	} {
		value := values[key]
		if value == "" {
			continue
		}
		n, parseErr := strconv.ParseUint(value, 10, 64)
		if parseErr != nil {
			return DeviceInfo{}, c.protocolFailure(fmt.Errorf("%w: invalid %s %q", ErrProtocol, key, value))
		}
		*dst = n
	}
	return info, nil
}

func (c *Client) Open(ctx context.Context, name string, mode Mode) (*File, error) {
	clean, err := cleanPath("open", name, false)
	if err != nil {
		return nil, err
	}
	if !mode.valid() {
		return nil, pathError("open", name, fs.ErrInvalid)
	}

	var size int64
	info, statErr := c.GetFileInfo(ctx, clean)
	switch {
	case statErr == nil && info.IsDir():
		return nil, pathError("open", name, ErrIsDirectory)
	case statErr == nil:
		size = info.Size
	case mode.creates() && errors.Is(statErr, fs.ErrNotExist):
		size = 0
	case statErr != nil:
		return nil, statErr
	}
	if mode.truncates() {
		size = 0
	}

	header := append(putUint64s(uint64(mode)), pathBytes(clean)...)
	resp, err := c.exchange(ctx, opFileOpen, header, nil, opFileOpenResult)
	if err != nil {
		return nil, pathError("open", name, err)
	}
	if len(resp.headerPayload) != 8 {
		return nil, pathError("open", name, c.protocolFailure(fmt.Errorf("%w: invalid open result length %d", ErrProtocol, len(resp.headerPayload))))
	}
	handle := binary.LittleEndian.Uint64(resp.headerPayload)
	if handle == 0 {
		return nil, pathError("open", name, c.protocolFailure(fmt.Errorf("%w: zero file handle", ErrProtocol)))
	}
	offset := int64(0)
	if mode.appends() {
		offset = size
	}
	return &File{client: c, path: clean, handle: handle, size: size, offset: offset}, nil
}

func (c *Client) MkDir(ctx context.Context, name string) error {
	return c.pathMutation(ctx, "mkdir", name, opMakeDir)
}

func (c *Client) Remove(ctx context.Context, name string) error {
	return c.pathMutation(ctx, "remove", name, opRemovePath)
}

func (c *Client) RemoveAll(ctx context.Context, name string) error {
	return c.pathMutation(ctx, "removeall", name, opRemovePathAndContents)
}

func (c *Client) pathMutation(ctx context.Context, operation, name string, op opcode) error {
	clean, err := cleanPath(operation, name, true)
	if err != nil {
		return err
	}
	_, err = c.exchange(ctx, op, pathBytes(clean), nil, opStatus)
	return pathError(operation, name, err)
}

func (c *Client) Rename(ctx context.Context, oldName, newName string) error {
	oldClean, err := cleanPath("rename", oldName, true)
	if err != nil {
		return err
	}
	newClean, err := cleanPath("rename", newName, true)
	if err != nil {
		return err
	}
	header := append(pathBytes(oldClean), pathBytes(newClean)...)
	_, err = c.exchange(ctx, opRenamePath, header, nil, opStatus)
	if err != nil {
		return fmt.Errorf("rename %q to %q: %w", oldName, newName, err)
	}
	return nil
}

func (c *Client) SetModTime(ctx context.Context, name string, mtime time.Time) error {
	clean, err := cleanPath("chtimes", name, true)
	if err != nil {
		return err
	}
	seconds := mtime.Unix()
	nanosecond := int64(mtime.Nanosecond())
	if mtime.IsZero() || seconds < 0 || seconds > (math.MaxInt64-nanosecond)/int64(time.Second) {
		return pathError("chtimes", name, fs.ErrInvalid)
	}
	nanos := seconds*int64(time.Second) + nanosecond
	header := append(putUint64s(uint64(nanos)), pathBytes(clean)...)
	_, err = c.exchange(ctx, opSetFileModTime, header, nil, opStatus)
	return pathError("chtimes", name, err)
}

func (c *Client) exchange(ctx context.Context, operation opcode, headerPayload, payload []byte, expected ...opcode) (packet, error) {
	if ctx == nil {
		return packet{}, fs.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return packet{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.terminalError(); err != nil {
		return packet{}, err
	}
	if err := ctx.Err(); err != nil {
		return packet{}, err
	}

	watchDone := make(chan struct{})
	watchStopped := make(chan struct{})
	go func() {
		defer close(watchStopped)
		select {
		case <-ctx.Done():
			// A completed exchange disarms the watcher before returning its
			// connection to a pool. If completion and cancellation race, prefer
			// the completed response once watchDone has been closed.
			select {
			case <-watchDone:
				return
			default:
			}
			c.poison(ctx.Err())
		case <-watchDone:
		}
	}()
	defer func() {
		close(watchDone)
		<-watchStopped
	}()

	c.packetNum++
	number := c.packetNum
	if err := writePacket(c.conn, number, operation, headerPayload, payload); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			c.poison(ctxErr)
			return packet{}, c.terminalError()
		}
		return packet{}, c.transportFailure(err)
	}
	resp, err := readPacket(c.conn, number)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			c.poison(ctxErr)
			return packet{}, c.terminalError()
		}
		return packet{}, c.transportFailure(err)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		c.poison(ctxErr)
		return packet{}, c.terminalError()
	}

	if resp.header.Operation == opStatus {
		if len(resp.headerPayload) != 8 || len(resp.payload) != 0 {
			return packet{}, c.protocolFailure(fmt.Errorf("%w: malformed status response", ErrProtocol))
		}
		code := binary.LittleEndian.Uint64(resp.headerPayload)
		if code != 0 {
			statusErr := statusError(code)
			if errors.Is(statusErr, ErrProtocol) {
				return packet{}, c.transportFailure(statusErr)
			}
			if IsConnectionLost(statusErr) {
				c.poison(statusErr)
			}
			return packet{}, statusErr
		}
	}
	if resp.header.Operation == opData && len(resp.headerPayload) != 0 {
		return packet{}, c.protocolFailure(fmt.Errorf("%w: data response has a header payload", ErrProtocol))
	}
	if resp.header.Operation == opFileOpenResult && (len(resp.headerPayload) != 8 || len(resp.payload) != 0) {
		return packet{}, c.protocolFailure(fmt.Errorf("%w: malformed file-open response", ErrProtocol))
	}
	for _, want := range expected {
		if resp.header.Operation == want {
			return resp, nil
		}
	}
	return packet{}, c.protocolFailure(fmt.Errorf("%w: response opcode %#x not valid for request %#x", ErrProtocol, resp.header.Operation, operation))
}

func (c *Client) protocolFailure(err error) error {
	if !errors.Is(err, ErrProtocol) {
		err = fmt.Errorf("%w: %v", ErrProtocol, err)
	}
	return c.transportFailure(err)
}

func (c *Client) transportFailure(cause error) error {
	err := &connectionError{cause: cause}
	c.poison(err)
	return err
}

func (c *Client) poison(cause error) {
	c.stateMu.Lock()
	if c.terminal == nil {
		if IsConnectionLost(cause) {
			c.terminal = cause
		} else {
			c.terminal = &connectionError{cause: cause}
		}
	}
	c.stateMu.Unlock()
	c.closeConnection()
}

func (c *Client) closeConnection() {
	c.closeOnce.Do(func() { c.closeErr = c.conn.Close() })
}

func (c *Client) terminalError() error {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.terminal
}

func parseList(payload []byte) ([]string, error) {
	values, err := parseNULTerminated(payload)
	if err != nil {
		return nil, err
	}
	entries := make([]string, 0, len(values))
	for _, entry := range values {
		if entry == "." || entry == ".." {
			continue
		}
		if !validEntryName(entry) {
			return nil, fmt.Errorf("%w: unsafe directory entry %q", ErrProtocol, entry)
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func parseDictionary(payload []byte) (map[string]string, error) {
	parts, err := parseNULTerminated(payload)
	if err != nil {
		return nil, err
	}
	if len(parts)%2 != 0 {
		return nil, fmt.Errorf("%w: dictionary has an unpaired key", ErrProtocol)
	}
	values := make(map[string]string, len(parts)/2)
	for i := 0; i < len(parts); i += 2 {
		if parts[i] == "" {
			return nil, fmt.Errorf("%w: dictionary contains an empty key", ErrProtocol)
		}
		if _, exists := values[parts[i]]; exists {
			return nil, fmt.Errorf("%w: dictionary contains duplicate key %q", ErrProtocol, parts[i])
		}
		values[parts[i]] = parts[i+1]
	}
	return values, nil
}

func parseNULTerminated(payload []byte) ([]string, error) {
	if len(payload) == 0 || payload[len(payload)-1] != 0 {
		return nil, fmt.Errorf("%w: unterminated AFC string list", ErrProtocol)
	}
	parts := strings.Split(string(payload), "\x00")
	for len(parts) != 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts, nil
}

func fileInfoFromDictionary(name string, values map[string]string) (FileInfo, error) {
	info := FileInfo{
		Name:       path.Base(name),
		Type:       FileType(values["st_ifmt"]),
		LinkTarget: values["st_linktarget"],
		Values:     values,
	}
	if value := values["st_size"]; value != "" {
		size, err := strconv.ParseInt(value, 10, 64)
		if err != nil || size < 0 {
			return FileInfo{}, fmt.Errorf("%w: invalid st_size %q", ErrProtocol, value)
		}
		info.Size = size
	}
	if value := values["st_mode"]; value != "" {
		mode, err := strconv.ParseUint(value, 0, 32)
		if err != nil {
			return FileInfo{}, fmt.Errorf("%w: invalid st_mode %q", ErrProtocol, value)
		}
		info.Mode = uint32(mode)
	}
	for key, dst := range map[string]*time.Time{"st_mtime": &info.ModTime, "st_birthtime": &info.BirthTime} {
		value := values[key]
		if value == "" {
			continue
		}
		nanos, err := strconv.ParseInt(value, 10, 64)
		if err != nil || nanos < 0 {
			return FileInfo{}, fmt.Errorf("%w: invalid %s %q", ErrProtocol, key, value)
		}
		*dst = time.Unix(0, nanos)
	}
	return info, nil
}

func pathError(operation, name string, err error) error {
	if err == nil {
		return nil
	}
	var existing *fs.PathError
	if errors.As(err, &existing) {
		return err
	}
	return &fs.PathError{Op: operation, Path: name, Err: err}
}
