package cloudfox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

var (
	ErrAuthenticationRequired = errors.New("cloudfox: authentication is required")
	ErrReadOnlyObject         = errors.New("cloudfox: object is read-only")
	ErrUnsupportedOperation   = errors.New("cloudfox: operation is not supported")
	ErrRemoteObjectChanged    = errors.New("cloudfox: remote object changed while it was open")
)

func requestedByteRange(size int64, bufferLength int, off int64) (end int64, count int, err error) {
	if off < 0 {
		return 0, 0, os.ErrInvalid
	}
	if bufferLength == 0 {
		return off, 0, nil
	}
	if off >= size {
		return 0, 0, io.EOF
	}
	remaining := size - off
	count = bufferLength
	if int64(count) > remaining {
		count = int(remaining)
	}
	return off + int64(count) - 1, count, nil
}

func parseContentRange(value string) (start, end, total int64, err error) {
	value = strings.TrimSpace(value)
	if len(value) < len("bytes ") || !strings.EqualFold(value[:len("bytes ")], "bytes ") {
		return 0, 0, 0, fmt.Errorf("cloudfox: invalid Content-Range %q", value)
	}
	rangeAndTotal := strings.Split(strings.TrimSpace(value[len("bytes "):]), "/")
	if len(rangeAndTotal) != 2 {
		return 0, 0, 0, fmt.Errorf("cloudfox: invalid Content-Range %q", value)
	}
	bounds := strings.Split(rangeAndTotal[0], "-")
	if len(bounds) != 2 {
		return 0, 0, 0, fmt.Errorf("cloudfox: invalid Content-Range %q", value)
	}
	start, startErr := strconv.ParseInt(strings.TrimSpace(bounds[0]), 10, 64)
	end, endErr := strconv.ParseInt(strings.TrimSpace(bounds[1]), 10, 64)
	total, totalErr := strconv.ParseInt(strings.TrimSpace(rangeAndTotal[1]), 10, 64)
	if startErr != nil || endErr != nil || totalErr != nil || start < 0 || end < start || total <= end {
		return 0, 0, 0, fmt.Errorf("cloudfox: invalid Content-Range %q", value)
	}
	return start, end, total, nil
}

func validateContentRange(value string, start, end, total int64) error {
	gotStart, gotEnd, gotTotal, err := parseContentRange(value)
	if err != nil || gotStart != start || gotEnd != end || gotTotal != total {
		return fmt.Errorf("cloudfox: Content-Range %q does not match requested bytes %d-%d/%d", value, start, end, total)
	}
	return nil
}

func strongETag(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 || strings.HasPrefix(strings.ToUpper(value), "W/") || value[0] != '"' || value[len(value)-1] != '"' {
		return ""
	}
	// RFC 9110 entity tags are one quoted opaque value. Reject wildcards,
	// unquoted tokens, lists, controls, and embedded quotes before relying on
	// the tag as an If-Match snapshot pin for arbitrary range reads.
	for i := 1; i < len(value)-1; i++ {
		ch := value[i]
		if ch == '"' || ch < 0x21 || (ch > 0x7e && ch < 0x80) {
			return ""
		}
	}
	return value
}

// cacheETag accepts either a strong entity-tag or a syntactically valid weak
// entity-tag. Weak validators cannot pin byte ranges with If-Match, but they
// can identify a complete representation in the session download cache.
func cacheETag(value string) string {
	value = strings.TrimSpace(value)
	if strongETag(value) != "" {
		return value
	}
	if strings.HasPrefix(value, "W/") {
		opaque := value[2:]
		if opaque == strings.TrimSpace(opaque) && strongETag(opaque) != "" {
			return value
		}
	}
	return ""
}

func canonicalCacheETag(value string) string {
	value = cacheETag(value)
	if strings.HasPrefix(value, "W/") {
		return value[2:]
	}
	return value
}

func weakETagEqual(first, second string) bool {
	first = canonicalCacheETag(first)
	second = canonicalCacheETag(second)
	return first != "" && first == second
}

// providerTempReader gives a VFS random access to a response that had to be
// materialized locally. Closing it always removes the private temporary file.
type providerTempReader struct {
	*os.File
	path string
	size int64
	once sync.Once
	err  error
}

func newProviderTempReader(f *os.File, path string, size int64) *providerTempReader {
	return &providerTempReader{File: f, path: path, size: size}
}

func (r *providerTempReader) Read(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.File.Read(p)
}

func (r *providerTempReader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.File.ReadAt(p, off)
}

func (r *providerTempReader) Size() int64 { return r.size }
func (r *providerTempReader) ReadAccessProfile() vfs.ReadAccessProfile {
	return vfs.ReadAccessMaterializeOnce
}

// LocalPath exposes a read-only backing file lease to consumers such as the
// archive plugin. The path remains valid only until this reader is closed.
func (r *providerTempReader) LocalPath() (string, bool) {
	return r.path, r.path != ""
}

func (r *providerTempReader) Close() error {
	r.once.Do(func() {
		r.err = r.File.Close()
		if err := os.Remove(r.path); r.err == nil && err != nil && !errors.Is(err, os.ErrNotExist) {
			r.err = err
		}
	})
	return r.err
}

// detach closes the current descriptor but leaves the private file on disk.
// It transfers cleanup ownership to a session cache.
func (r *providerTempReader) detach() (string, int64, error) {
	var path string
	r.once.Do(func() {
		path = r.path
		r.path = ""
		r.err = r.File.Close()
	})
	return path, r.size, r.err
}

// providerSpoolWriter is used by APIs that require a replayable body or a
// known Content-Length. Close is the commit point and must propagate the
// remote upload error to the editor/file-operation caller.
type providerSpoolWriter struct {
	ctx       context.Context
	file      *os.File
	path      string
	name      string
	upload    func(context.Context, *os.File, int64) error
	mu        sync.Mutex
	closed    bool
	closeDone chan struct{}
	writeErr  error
}

func (*providerSpoolWriter) TransferProgressManaged() bool { return true }

func newProviderSpoolWriter(ctx context.Context, name string, upload func(context.Context, *os.File, int64) error) (*providerSpoolWriter, error) {
	f, err := os.CreateTemp("", "f4-cloudfox-upload-*")
	if err != nil {
		return nil, err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return nil, err
	}
	return &providerSpoolWriter{ctx: ctx, file: f, path: f.Name(), name: name, upload: upload, closeDone: make(chan struct{})}, nil
}

func (w *providerSpoolWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if err := w.ctx.Err(); err != nil {
		w.writeErr = err
		return 0, err
	}
	n, err := w.file.Write(p)
	if err != nil {
		w.writeErr = err
	}
	return n, err
}

func (w *providerSpoolWriter) Close() error {
	w.mu.Lock()
	if w.closed {
		done := w.closeDone
		w.mu.Unlock()
		<-done
		w.mu.Lock()
		err := w.writeErr
		w.mu.Unlock()
		return err
	}
	w.closed = true
	w.mu.Unlock()

	err := w.finishClose()
	w.mu.Lock()
	if err != nil {
		w.writeErr = err
	}
	result := w.writeErr
	close(w.closeDone)
	w.mu.Unlock()
	return result
}

// Abort discards the local spool without invoking upload. It is deliberately
// distinct from Close: Close is the remote commit point for this writer.
func (w *providerSpoolWriter) Abort() error {
	w.mu.Lock()
	if w.closed {
		done := w.closeDone
		w.mu.Unlock()
		<-done
		w.mu.Lock()
		err := w.writeErr
		w.mu.Unlock()
		return err
	}
	w.closed = true
	w.mu.Unlock()

	closeErr := w.file.Close()
	removeErr := os.Remove(w.path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	err := errors.Join(closeErr, removeErr)
	w.mu.Lock()
	if err != nil {
		w.writeErr = err
	}
	close(w.closeDone)
	w.mu.Unlock()
	return err
}

func (w *providerSpoolWriter) finishClose() error {
	defer func() { _ = os.Remove(w.path) }()
	defer func() { _ = w.file.Close() }()
	w.mu.Lock()
	writeErr := w.writeErr
	w.mu.Unlock()
	if writeErr != nil {
		return writeErr
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	if err := w.file.Sync(); err != nil {
		return err
	}
	st, err := w.file.Stat()
	if err != nil {
		return err
	}
	if _, err := w.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if reporter, ok := w.ctx.Value(vfs.ReporterKey).(vfs.TaskReporter); ok {
		reporter.UpdateTransfer("Uploading", w.name, 0, "", 0, "")
	}
	if err := w.upload(w.ctx, w.file, st.Size()); err != nil {
		return err
	}
	if reporter, ok := w.ctx.Value(vfs.ReporterKey).(vfs.TaskReporter); ok {
		reporter.UpdateTransfer("Uploading", w.name, 100, "", 100, "")
	}
	return nil
}

type providerProgressReader struct {
	r        io.Reader
	ctx      context.Context
	reporter vfs.TaskReporter
	action   string
	name     string
	total    int64
	read     int64
	started  bool
}

// providerOperationContext is cancelled when either the individual call or
// the lifetime of the open remote handle ends. It lets Close interrupt an
// in-flight HTTP range request without discarding per-read cancellation.
func providerOperationContext(call, lifetime context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(call)
	if lifetime.Err() != nil {
		cancel()
		return ctx, cancel
	}
	stop := context.AfterFunc(lifetime, cancel)
	return ctx, func() {
		stop()
		cancel()
	}
}

type providerSessionLifetimeContextKey struct{}

// providerDetachedCleanupContext detaches a best-effort cleanup from the
// caller's Cancel button while preserving plugin/session shutdown. Direct
// backend callers have no pooled lifetime value and receive only the timeout.
func providerDetachedCleanupContext(ctx context.Context, timeout time.Duration) (context.Context, func()) {
	base := context.WithoutCancel(ctx)
	stopLifetime := func() {}
	if lifetime, ok := ctx.Value(providerSessionLifetimeContextKey{}).(context.Context); ok && lifetime != nil {
		base, stopLifetime = providerOperationContext(base, lifetime)
	}
	cleanup, cancel := context.WithTimeout(base, timeout)
	return cleanup, func() {
		cancel()
		stopLifetime()
	}
}

func (r *providerProgressReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if r.reporter != nil && r.reporter.IsCancelled() {
		return 0, context.Canceled
	}
	if r.reporter != nil && !r.started {
		action := r.action
		if action == "" {
			action = "Uploading"
		}
		r.reporter.UpdateTransfer(action, r.name, 0, "", 0, "")
		r.started = true
	}
	n, err := r.r.Read(p)
	r.read += int64(n)
	if r.reporter != nil && r.total > 0 {
		pct := int(r.read * 100 / r.total)
		// Byte transmission is not the commit point for an upload. Reserve 100
		// for providerSpoolWriter after the server confirms success so a failed
		// request never looks completed in the UI.
		if pct >= 100 {
			pct = 99
		}
		action := r.action
		if action == "" {
			action = "Uploading"
		}
		r.reporter.UpdateTransfer(action, r.name, pct, "", pct, "")
	}
	return n, err
}

type providerHTTPError struct {
	StatusCode int
	Method     string
	URL        string
	Message    string
}

func (e *providerHTTPError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("cloudfox: %s failed with HTTP %d", e.Method, e.StatusCode)
	}
	return fmt.Sprintf("cloudfox: %s failed with HTTP %d: %s", e.Method, e.StatusCode, e.Message)
}

func mapProviderHTTPError(resp *http.Response, message string) error {
	if resp == nil {
		return errors.New("cloudfox: empty HTTP response")
	}
	method, requestURL := "request", ""
	if resp.Request != nil {
		if resp.Request.Method != "" {
			method = resp.Request.Method
		}
		if resp.Request.URL != nil {
			requestURL = resp.Request.URL.Redacted()
		}
	}
	base := &providerHTTPError{StatusCode: resp.StatusCode, Method: method, URL: requestURL, Message: message}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("%w: %w", os.ErrPermission, base)
	case http.StatusNotFound:
		return fmt.Errorf("%w: %w", os.ErrNotExist, base)
	case http.StatusConflict, http.StatusPreconditionFailed:
		return fmt.Errorf("%w: %w", os.ErrExist, base)
	default:
		return base
	}
}

// providerHTTPMutationError distinguishes a definitive client rejection from
// a response which may have been produced after the server committed a write.
// Retrying a 3xx, 408 or 5xx mutation automatically can duplicate or destroy
// data, so those responses carry the explicit unknown-state sentinel.
func providerHTTPMutationError(operation string, resp *http.Response, message string) error {
	err := mapProviderHTTPError(resp, message)
	if resp == nil || resp.StatusCode < 400 || resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 500 {
		return &vfs.UnknownOperationStateError{Operation: operation, Err: err}
	}
	return err
}
