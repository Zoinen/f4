// Package androidfs implements access to Android devices through the local
// Android Debug Bridge server.
package androidfs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultADBAddress is the smart-socket address used by a local ADB server.
	DefaultADBAddress = "127.0.0.1:5037"

	maxADBServiceLength = 0xffff
	maxShellPacket      = 16 << 20
	shellWriteChunk     = 64 << 10
)

// Device describes one transport reported by host:devices-l.
type Device struct {
	Serial      string
	State       string
	Product     string
	Model       string
	Device      string
	TransportID string
}

// Online reports whether adbd considers the transport ready for services.
func (d Device) Online() bool { return d.State == "device" }

// ServiceError is a FAIL reply from the ADB smart socket.
type ServiceError struct {
	Service string
	Message string
}

func (e *ServiceError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("adb service %q failed", e.Service)
	}
	return fmt.Sprintf("adb service %q failed: %s", e.Service, e.Message)
}

// DialContextFunc and the following option hooks make Server testable without
// an installed ADB server or executable.
type DialContextFunc func(context.Context, string, string) (net.Conn, error)
type ADBStarterFunc func(context.Context, string) error
type ADBLookupFunc func() (string, error)
type ADBRestarterFunc func(context.Context, string) error

// ServerOption customizes an ADB smart-socket client.
type ServerOption func(*Server)

// WithADBAddress changes the smart-socket address.
func WithADBAddress(address string) ServerOption {
	return func(s *Server) {
		if address != "" {
			s.address = address
		}
	}
}

// WithADBDialer supplies the TCP dialer used by the server client.
func WithADBDialer(dial DialContextFunc) ServerOption {
	return func(s *Server) {
		if dial != nil {
			s.dialContext = dial
		}
	}
}

// WithADBStarter supplies the function that starts an installed ADB server.
func WithADBStarter(start ADBStarterFunc) ServerOption {
	return func(s *Server) {
		if start != nil {
			s.startServer = start
		}
	}
}

// WithADBLookup supplies ADB executable discovery.
func WithADBLookup(lookup ADBLookupFunc) ServerOption {
	return func(s *Server) {
		if lookup != nil {
			s.lookupADB = lookup
		}
	}
}

// WithADBRestarter supplies the command used to restart the local ADB daemon.
// It is primarily useful for tests and custom ADB installations.
func WithADBRestarter(restart ADBRestarterFunc) ServerOption {
	return func(s *Server) {
		if restart != nil {
			s.restartServer = restart
		}
	}
}

// Server talks directly to the local ADB server. It starts an installed adb
// executable on the first failed connection, but never downloads or bundles
// platform-tools.
type Server struct {
	address       string
	dialContext   DialContextFunc
	startServer   ADBStarterFunc
	lookupADB     ADBLookupFunc
	restartServer ADBRestarterFunc
	startMu       sync.Mutex
}

// NewServer creates a client for the conventional local ADB smart socket.
func NewServer(options ...ServerOption) *Server {
	dialer := &net.Dialer{}
	s := &Server{
		address:       DefaultADBAddress,
		dialContext:   dialer.DialContext,
		startServer:   startInstalledADB,
		lookupADB:     findADBExecutable,
		restartServer: restartInstalledADB,
	}
	for _, option := range options {
		if option != nil {
			option(s)
		}
	}
	return s
}

// Address returns the smart-socket address used by this client.
func (s *Server) Address() string { return s.address }

// Devices returns the transports known to the local ADB server, including
// offline and unauthorized devices. Older servers are supported through the
// host:devices fallback.
func (s *Server) Devices(ctx context.Context) ([]Device, error) {
	payload, err := s.hostQuery(ctx, "host:devices-l")
	if err != nil {
		var serviceErr *ServiceError
		if !errors.As(err, &serviceErr) {
			return nil, err
		}
		payload, err = s.hostQuery(ctx, "host:devices")
		if err != nil {
			return nil, err
		}
	}
	return parseDevices(payload)
}

// RestartForAuthorization restarts the local ADB daemon. A transport-level
// reconnect can remove an unauthorized USB device from ADB until the daemon is
// restarted; restarting the daemon reliably recreates the transport and makes
// Android show the host-key confirmation prompt again.
func (s *Server) RestartForAuthorization(ctx context.Context) error {
	adbPath, err := s.lookupADB()
	if err != nil {
		return fmt.Errorf("adb: cannot restart for authorization: %w", err)
	}
	return s.restartServer(ctx, adbPath)
}

// Features returns the feature names advertised for serial. If the server
// cannot answer a transport-specific request, its global feature set is used.
func (s *Server) Features(ctx context.Context, serial string) (map[string]bool, error) {
	service := "host:features"
	if serial != "" {
		service = "host-serial:" + serial + ":features"
	}
	payload, err := s.hostQuery(ctx, service)
	if err != nil && serial != "" {
		var serviceErr *ServiceError
		if errors.As(err, &serviceErr) {
			payload, err = s.hostQuery(ctx, "host:features")
		}
	}
	if err != nil {
		return nil, err
	}
	features := make(map[string]bool)
	for _, feature := range strings.FieldsFunc(string(payload), func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t'
	}) {
		if feature != "" {
			features[feature] = true
		}
	}
	return features, nil
}

// OpenService selects serial and opens an arbitrary transport service. The
// returned stream owns its TCP connection and must be closed by the caller.
// Context cancellation interrupts the selection handshake; after the method
// returns, the stream has no deadline and lives until it is closed.
func (s *Server) OpenService(ctx context.Context, serial, service string) (io.ReadWriteCloser, error) {
	return s.openServiceConn(ctx, serial, service)
}

func (s *Server) openServiceConn(ctx context.Context, serial, service string) (net.Conn, error) {
	if serial == "" {
		return nil, errors.New("adb: device serial is empty")
	}
	if service == "" {
		return nil, errors.New("adb: service is empty")
	}
	conn, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	stop := interruptConnOnCancel(ctx, conn)
	ok := false
	defer func() {
		stop()
		if !ok {
			conn.Close()
		}
	}()
	if err := requestService(conn, "host:transport:"+serial); err != nil {
		return nil, errorAfterContext(ctx, err)
	}
	if err := requestService(conn, service); err != nil {
		return nil, errorAfterContext(ctx, err)
	}
	ok = true
	return conn, nil
}

// OpenShellV2 starts command without a pseudo-terminal and exposes the ADB
// shell-v2 byte stream as an ordinary full-duplex stream. Stderr and the exit
// code remain available through ShellStream methods and do not contaminate
// stdout (which is important for binary protocols such as FISH+).
func (s *Server) OpenShellV2(ctx context.Context, serial, command string) (*ShellStream, error) {
	if strings.IndexByte(command, 0) >= 0 {
		return nil, errors.New("adb shell: command contains NUL")
	}
	conn, err := s.openServiceConn(ctx, serial, "shell,v2,raw:"+command)
	if err != nil {
		return nil, err
	}
	return newShellStream(conn), nil
}

// OpenShellRaw opens a raw, non-PTY shell stream. shell-v2 is preferred; a
// legacy shell is used only when the device does not advertise shell_v2. In
// legacy mode stderr is merged into stdout and no exit status is available.
func (s *Server) OpenShellRaw(ctx context.Context, serial, command string) (io.ReadWriteCloser, error) {
	if strings.IndexByte(command, 0) >= 0 {
		return nil, errors.New("adb shell: command contains NUL")
	}
	features, err := s.Features(ctx, serial)
	if err != nil {
		return nil, err
	}
	if features["shell_v2"] {
		return s.OpenShellV2(ctx, serial, command)
	}
	return s.openServiceConn(ctx, serial, "shell:"+command)
}

// RunShell runs command to completion. A non-zero remote exit code is data,
// not a transport error. Legacy shells return exitCode -1 and combine all
// output in stdout because that protocol has no stderr or exit-code channels.
func (s *Server) RunShell(ctx context.Context, serial, command string) (stdout, stderr []byte, exitCode int, err error) {
	return s.runShell(ctx, serial, command, 0)
}

// RunShellStream runs command and delivers stdout and stderr bytes live in the
// order their shell-v2 packets arrive. Callback chunks are bounded and valid
// only for the duration of the callback. Legacy shells are streamed through
// their already-merged byte channel; a randomized terminal marker recovers
// the status that their protocol does not carry.
func (s *Server) RunShellStream(ctx context.Context, serial, command string, cb func([]byte)) (exitCode int, err error) {
	if strings.IndexByte(command, 0) >= 0 {
		return -1, errors.New("adb shell: command contains NUL")
	}
	features, err := s.Features(ctx, serial)
	if err != nil {
		return -1, err
	}
	if !features["shell_v2"] {
		return s.runLegacyShellStream(ctx, serial, command, cb)
	}
	return s.runShellV2Stream(ctx, serial, command, cb)
}

func (s *Server) runLegacyShellStream(ctx context.Context, serial, command string, cb func([]byte)) (int, error) {
	marker, err := newLegacyShellStatusMarker()
	if err != nil {
		return -1, err
	}
	const statusVariable = "__f4_apply_status"
	wrapped := "sh -c " + quoteShellArg(command) + " </dev/null; " + statusVariable + "=$?; printf " +
		quoteShellArg(marker+"%u") + " \"$" + statusVariable + "\""
	conn, err := s.openServiceConn(ctx, serial, "shell:"+wrapped)
	if err != nil {
		return -1, err
	}
	defer conn.Close()
	stop := interruptConnOnCancel(ctx, conn)
	defer stop()

	output := newLegacyShellStatusParser(marker, cb)
	buffer := make([]byte, shellWriteChunk)
	for {
		n, readErr := conn.Read(buffer)
		if n > 0 {
			output.Write(buffer[:n])
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return output.Finish()
			}
			output.Flush()
			return -1, errorAfterContext(ctx, readErr)
		}
	}
}

const (
	legacyShellStatusMarkerPrefix = "__f4_status_"
	legacyShellStatusNonceBytes   = 16
	legacyShellStatusMaxDigits    = 10
)

func newLegacyShellStatusMarker() (string, error) {
	var nonce [legacyShellStatusNonceBytes]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("adb legacy shell: generate status marker: %w", err)
	}
	return legacyShellStatusMarkerPrefix + hex.EncodeToString(nonce[:]) + "__", nil
}

// legacyShellStatusParser withholds only enough trailing output to recognize
// the randomized status marker. Everything before that bounded tail remains
// live, while the marker itself never reaches the command transcript.
type legacyShellStatusParser struct {
	marker  []byte
	pending []byte
	cb      func([]byte)
}

func newLegacyShellStatusParser(marker string, cb func([]byte)) *legacyShellStatusParser {
	return &legacyShellStatusParser{marker: []byte(marker), cb: cb}
}

func (p *legacyShellStatusParser) Write(data []byte) {
	if len(data) == 0 {
		return
	}
	p.pending = append(p.pending, data...)
	keep := len(p.marker) + legacyShellStatusMaxDigits
	if len(p.pending) <= keep {
		return
	}
	emit := len(p.pending) - keep
	emitShellStreamChunks(p.pending[:emit], p.cb)
	copy(p.pending, p.pending[emit:])
	p.pending = p.pending[:keep]
}

func (p *legacyShellStatusParser) Finish() (int, error) {
	markerAt := bytes.LastIndex(p.pending, p.marker)
	if markerAt < 0 {
		p.Flush()
		return -1, errors.New("adb legacy shell: stream ended without a status marker")
	}
	status := p.pending[markerAt+len(p.marker):]
	code, err := strconv.ParseUint(string(status), 10, 8)
	if err != nil {
		p.Flush()
		return -1, fmt.Errorf("adb legacy shell: invalid status %q: %w", status, err)
	}
	emitShellStreamChunks(p.pending[:markerAt], p.cb)
	p.pending = nil
	return int(code), nil
}

func (p *legacyShellStatusParser) Flush() {
	emitShellStreamChunks(p.pending, p.cb)
	p.pending = nil
}

func (s *Server) runShellV2Stream(ctx context.Context, serial, command string, cb func([]byte)) (int, error) {
	conn, err := s.openServiceConn(ctx, serial, "shell,v2,raw:"+command)
	if err != nil {
		return -1, err
	}
	defer conn.Close()
	stop := interruptConnOnCancel(ctx, conn)
	defer stop()

	for {
		id, payload, readErr := readShellPacket(conn)
		if readErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return -1, ctxErr
			}
			if errors.Is(readErr, io.EOF) || errors.Is(readErr, io.ErrUnexpectedEOF) {
				return -1, fmt.Errorf("adb shell-v2: stream ended without an exit packet: %w", readErr)
			}
			return -1, readErr
		}
		switch id {
		case shellIDStdout, shellIDStderr:
			emitShellStreamChunks(payload, cb)
		case shellIDExit:
			code, parseErr := parseShellExit(payload)
			if parseErr != nil {
				return -1, parseErr
			}
			return code, nil
		case shellIDCloseStdin, shellIDWindowSize:
			// A command runner does not write stdin and uses a raw, non-PTY
			// service, so neither packet carries output.
		default:
			return -1, fmt.Errorf("adb shell-v2: unexpected packet id %d", id)
		}
	}
}

func emitShellStreamChunks(payload []byte, cb func([]byte)) {
	if cb == nil {
		return
	}
	for len(payload) > 0 {
		chunk := payload
		if len(chunk) > shellWriteChunk {
			chunk = chunk[:shellWriteChunk]
		}
		cb(chunk)
		payload = payload[len(chunk):]
	}
}

// RunShellLimited is RunShell with a client-side stdout bound. Device-info
// probes use it for commands such as getprop whose output is normally small,
// but whose size is controlled by the device rather than by f4. Reaching the
// limit closes the operation-scoped shell instead of letting a vendor command
// consume unbounded host memory.
func (s *Server) RunShellLimited(ctx context.Context, serial, command string, maxStdout int64) (stdout, stderr []byte, exitCode int, err error) {
	if maxStdout <= 0 {
		return nil, nil, -1, errors.New("adb shell: stdout limit must be positive")
	}
	return s.runShell(ctx, serial, command, maxStdout)
}

func (s *Server) runShell(ctx context.Context, serial, command string, maxStdout int64) (stdout, stderr []byte, exitCode int, err error) {
	if strings.IndexByte(command, 0) >= 0 {
		return nil, nil, -1, errors.New("adb shell: command contains NUL")
	}
	features, err := s.Features(ctx, serial)
	if err != nil {
		return nil, nil, -1, err
	}
	if !features["shell_v2"] {
		conn, openErr := s.openServiceConn(ctx, serial, "shell:"+command)
		if openErr != nil {
			return nil, nil, -1, openErr
		}
		defer conn.Close()
		stop := interruptConnOnCancel(ctx, conn)
		defer stop()
		out, readErr := readAllWithLimit(conn, maxStdout)
		if readErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				readErr = ctxErr
			}
		}
		return out, nil, -1, readErr
	}

	stream, openErr := s.OpenShellV2(ctx, serial, command)
	if openErr != nil {
		return nil, nil, -1, openErr
	}
	defer stream.Close()
	stop := stream.InterruptOnCancel(ctx)
	defer stop()
	out, readErr := readAllWithLimit(stream, maxStdout)
	if readErr != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			readErr = ctxErr
		}
		return out, stream.Stderr(), -1, readErr
	}
	code, present := stream.ExitCode()
	if !present {
		return out, stream.Stderr(), -1, errors.New("adb shell-v2: stream ended without an exit packet")
	}
	return out, stream.Stderr(), code, nil
}

func readAllWithLimit(r io.Reader, max int64) ([]byte, error) {
	if max <= 0 {
		return io.ReadAll(r)
	}
	out, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(out)) > max {
		return nil, fmt.Errorf("adb shell: stdout exceeds %d bytes", max)
	}
	return out, nil
}

func (s *Server) hostQuery(ctx context.Context, service string) ([]byte, error) {
	conn, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := interruptConnOnCancel(ctx, conn)
	defer stop()
	if err := requestService(conn, service); err != nil {
		return nil, errorAfterContext(ctx, err)
	}
	payload, err := readLengthPrefixed(conn, service)
	return payload, errorAfterContext(ctx, err)
}

func (s *Server) dial(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, initialErr := s.dialContext(ctx, "tcp", s.address)
	if initialErr == nil {
		return conn, nil
	}

	// Starts are serialized. Another caller may have started the daemon while
	// this one waited, so retry once under the lock before spawning a process.
	s.startMu.Lock()
	defer s.startMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if conn, err := s.dialContext(ctx, "tcp", s.address); err == nil {
		return conn, nil
	}
	adbPath, lookupErr := s.lookupADB()
	if lookupErr != nil {
		return nil, fmt.Errorf("adb: connect to %s: %w; cannot start server: %v", s.address, initialErr, lookupErr)
	}
	if err := s.startServer(ctx, adbPath); err != nil {
		return nil, fmt.Errorf("adb: connect to %s: %w; start server: %v", s.address, initialErr, err)
	}
	conn, err := s.dialContext(ctx, "tcp", s.address)
	if err != nil {
		return nil, fmt.Errorf("adb: connect to %s after starting server: %w", s.address, err)
	}
	return conn, nil
}

func requestService(rw io.ReadWriter, service string) error {
	if err := writeADBRequest(rw, service); err != nil {
		return err
	}
	var status [4]byte
	if _, err := io.ReadFull(rw, status[:]); err != nil {
		return fmt.Errorf("adb service %q: read status: %w", service, err)
	}
	switch string(status[:]) {
	case "OKAY":
		return nil
	case "FAIL":
		message, err := readLengthPrefixed(rw, service)
		if err != nil {
			return fmt.Errorf("adb service %q: read failure: %w", service, err)
		}
		return &ServiceError{Service: service, Message: string(message)}
	default:
		return fmt.Errorf("adb service %q: unexpected status %q", service, string(status[:]))
	}
}

func writeADBRequest(w io.Writer, service string) error {
	payload := []byte(service)
	if len(payload) > maxADBServiceLength {
		return fmt.Errorf("adb service name is too long: %d bytes", len(payload))
	}
	header := fmt.Sprintf("%04X", len(payload))
	if err := writeFull(w, []byte(header)); err != nil {
		return fmt.Errorf("adb service %q: write length: %w", service, err)
	}
	if err := writeFull(w, payload); err != nil {
		return fmt.Errorf("adb service %q: write request: %w", service, err)
	}
	return nil
}

func readLengthPrefixed(r io.Reader, service string) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, fmt.Errorf("adb service %q: read length: %w", service, err)
	}
	n, err := strconv.ParseUint(string(header[:]), 16, 16)
	if err != nil {
		return nil, fmt.Errorf("adb service %q: invalid length %q", service, string(header[:]))
	}
	payload := make([]byte, int(n))
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("adb service %q: read %d-byte payload: %w", service, n, err)
	}
	return payload, nil
}

func parseDevices(payload []byte) ([]Device, error) {
	lines := strings.Split(strings.ReplaceAll(string(payload), "\r\n", "\n"), "\n")
	devices := make([]Device, 0, len(lines))
	for lineNumber, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lineFields := strings.Fields(line)
		if len(lineFields) < 2 {
			return nil, fmt.Errorf("adb: malformed device record on line %d: %q", lineNumber+1, line)
		}
		serial := lineFields[0]
		fields := lineFields[1:]
		propertyAt := len(fields)
		for i, field := range fields {
			if strings.Contains(field, ":") {
				propertyAt = i
				break
			}
		}
		if propertyAt == 0 {
			return nil, fmt.Errorf("adb: missing device state on line %d: %q", lineNumber+1, line)
		}
		device := Device{Serial: serial, State: strings.Join(fields[:propertyAt], " ")}
		for _, property := range fields[propertyAt:] {
			key, value, ok := strings.Cut(property, ":")
			if !ok {
				continue
			}
			switch key {
			case "product":
				device.Product = value
			case "model":
				device.Model = value
			case "device":
				device.Device = value
			case "transport_id":
				device.TransportID = value
			}
		}
		devices = append(devices, device)
	}
	return devices, nil
}

// ShellStream demultiplexes an ADB shell-v2 connection. Read yields stdout;
// Write sends stdin packets. One goroutine may read while another writes.
type ShellStream struct {
	conn net.Conn

	readMu    sync.Mutex
	writeMu   sync.Mutex
	stateMu   sync.RWMutex
	pending   []byte
	stderr    bytes.Buffer
	exitCode  int
	exited    bool
	closed    bool
	stdinEOF  bool
	readErr   error
	closeOnce sync.Once
}

func newShellStream(conn net.Conn) *ShellStream {
	return &ShellStream{conn: conn, exitCode: -1}
}

// InterruptOnCancel makes blocked stream I/O fail when ctx is cancelled. The
// returned function disarms the watcher and clears the temporary deadline.
// This is useful while a higher-level protocol performs a bounded handshake;
// callers should disarm it once that handshake succeeds.
func (s *ShellStream) InterruptOnCancel(ctx context.Context) func() {
	return interruptConnOnCancel(ctx, s.conn)
}

func (s *ShellStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s.readMu.Lock()
	defer s.readMu.Unlock()
	if len(s.pending) > 0 {
		n := copy(p, s.pending)
		s.pending = s.pending[n:]
		return n, nil
	}
	if s.readErr != nil {
		return 0, s.readErr
	}

	for {
		id, payload, err := readShellPacket(s.conn)
		if err != nil {
			s.stateMu.RLock()
			exited := s.exited
			s.stateMu.RUnlock()
			if errors.Is(err, io.EOF) && exited {
				err = io.EOF
			} else if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				err = fmt.Errorf("adb shell-v2: truncated stream: %w", err)
			}
			s.readErr = err
			return 0, err
		}
		switch id {
		case shellIDStdout:
			if len(payload) == 0 {
				continue
			}
			n := copy(p, payload)
			if n < len(payload) {
				s.pending = append(s.pending[:0], payload[n:]...)
			}
			return n, nil
		case shellIDStderr:
			s.stateMu.Lock()
			_, _ = s.stderr.Write(payload)
			s.stateMu.Unlock()
		case shellIDExit:
			code, parseErr := parseShellExit(payload)
			if parseErr != nil {
				s.readErr = parseErr
				return 0, parseErr
			}
			s.stateMu.Lock()
			s.exitCode = code
			s.exited = true
			s.stateMu.Unlock()
			s.readErr = io.EOF
			return 0, io.EOF
		case shellIDCloseStdin:
			s.stateMu.Lock()
			s.stdinEOF = true
			s.stateMu.Unlock()
		case shellIDWindowSize:
			// A raw, non-PTY service has no window to resize.
		default:
			s.readErr = fmt.Errorf("adb shell-v2: unexpected packet id %d", id)
			return 0, s.readErr
		}
	}
}

func (s *ShellStream) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.stateMu.RLock()
	closed, stdinEOF := s.closed, s.stdinEOF
	s.stateMu.RUnlock()
	if closed {
		return 0, net.ErrClosed
	}
	if stdinEOF {
		return 0, io.ErrClosedPipe
	}
	written := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > shellWriteChunk {
			chunk = chunk[:shellWriteChunk]
		}
		if err := writeShellPacket(s.conn, shellIDStdin, chunk); err != nil {
			return written, err
		}
		written += len(chunk)
		p = p[len(chunk):]
	}
	return written, nil
}

// CloseWrite tells adbd that no more stdin packets will be sent.
func (s *ShellStream) CloseWrite() error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.stateMu.RLock()
	if s.closed || s.stdinEOF {
		s.stateMu.RUnlock()
		return nil
	}
	s.stateMu.RUnlock()
	if err := writeShellPacket(s.conn, shellIDCloseStdin, nil); err != nil {
		return err
	}
	s.stateMu.Lock()
	s.stdinEOF = true
	s.stateMu.Unlock()
	return nil
}

func (s *ShellStream) Close() error {
	var err error
	s.closeOnce.Do(func() {
		s.stateMu.Lock()
		s.closed = true
		s.stateMu.Unlock()
		err = s.conn.Close()
	})
	return err
}

// Stderr returns stderr received so far.
func (s *ShellStream) Stderr() []byte {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return bytes.Clone(s.stderr.Bytes())
}

// ExitCode returns the remote status after an exit packet has been read.
func (s *ShellStream) ExitCode() (int, bool) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.exitCode, s.exited
}

const (
	shellIDStdin      byte = 0
	shellIDStdout     byte = 1
	shellIDStderr     byte = 2
	shellIDExit       byte = 3
	shellIDCloseStdin byte = 4
	shellIDWindowSize byte = 5
)

func readShellPacket(r io.Reader) (byte, []byte, error) {
	var header [5]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}
	n := binary.LittleEndian.Uint32(header[1:])
	if n > maxShellPacket {
		return 0, nil, fmt.Errorf("adb shell-v2: packet is too large: %d bytes", n)
	}
	payload := make([]byte, int(n))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return header[0], payload, nil
}

func writeShellPacket(w io.Writer, id byte, payload []byte) error {
	if uint64(len(payload)) > uint64(^uint32(0)) {
		return fmt.Errorf("adb shell-v2: packet is too large: %d bytes", len(payload))
	}
	var header [5]byte
	header[0] = id
	binary.LittleEndian.PutUint32(header[1:], uint32(len(payload)))
	if err := writeFull(w, header[:]); err != nil {
		return fmt.Errorf("adb shell-v2: write packet header: %w", err)
	}
	if len(payload) > 0 {
		if err := writeFull(w, payload); err != nil {
			return fmt.Errorf("adb shell-v2: write packet body: %w", err)
		}
	}
	return nil
}

func parseShellExit(payload []byte) (int, error) {
	switch len(payload) {
	case 1:
		return int(payload[0]), nil
	case 4:
		return int(binary.LittleEndian.Uint32(payload)), nil
	default:
		return -1, fmt.Errorf("adb shell-v2: invalid exit packet length %d", len(payload))
	}
}

func interruptConnOnCancel(ctx context.Context, conn net.Conn) func() {
	if ctx == nil {
		ctx = context.Background()
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-stop:
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(stop)
			<-done
			_ = conn.SetDeadline(time.Time{})
		})
	}
}

func errorAfterContext(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	return err
}

func writeFull(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if n < 0 || n > len(payload) {
			return fmt.Errorf("invalid write count %d for %d-byte buffer", n, len(payload))
		}
		if n > 0 {
			payload = payload[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}

func findADBExecutable() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("F4_ADB_PATH")); configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			return "", fmt.Errorf("F4_ADB_PATH %q: %w", configured, err)
		}
		return path, nil
	}
	if path, err := exec.LookPath("adb"); err == nil {
		return path, nil
	}

	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	for _, variable := range []string{"ANDROID_SDK_ROOT", "ANDROID_HOME"} {
		root := strings.TrimSpace(os.Getenv(variable))
		if root == "" {
			continue
		}
		candidate := filepath.Join(root, "platform-tools", name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			if path, lookErr := exec.LookPath(candidate); lookErr == nil {
				return path, nil
			}
		}
	}
	return "", errors.New("adb executable not found (set F4_ADB_PATH or install Android SDK platform-tools)")
}

func startInstalledADB(ctx context.Context, adbPath string) error {
	command := exec.CommandContext(ctx, adbPath, "start-server")
	output, err := command.CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if len(message) > 4096 {
		message = message[:4096] + "..."
	}
	if message == "" {
		return err
	}
	return fmt.Errorf("%w: %s", err, message)
}

func restartInstalledADB(ctx context.Context, adbPath string) error {
	for _, action := range []string{"kill-server", "start-server"} {
		command := exec.CommandContext(ctx, adbPath, action)
		output, err := command.CombinedOutput()
		if err == nil {
			continue
		}
		message := strings.TrimSpace(string(output))
		if len(message) > 4096 {
			message = message[:4096] + "..."
		}
		if message == "" {
			return fmt.Errorf("adb %s: %w", action, err)
		}
		return fmt.Errorf("adb %s: %w: %s", action, err, message)
	}
	return nil
}
