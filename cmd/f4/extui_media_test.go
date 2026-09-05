package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type countingMediaVFS struct {
	*vfs.NullVFS
	mu           sync.Mutex
	data         []byte
	item         vfs.VFSItem
	profile      vfs.ReadAccessProfile
	storage      vfs.StorageClass
	backingPath  string
	openCount    int
	readCount    int
	readOffsets  []int64
	closeCount   int
	openGate     chan struct{}
	openStarted  chan struct{}
	openOnce     sync.Once
	gate         chan struct{}
	started      chan struct{}
	startOnce    sync.Once
	failReadOnce bool
	failReadErr  error
	handleSlots  chan struct{}
}

func newCountingMediaVFS(data []byte) *countingMediaVFS {
	return &countingMediaVFS{
		NullVFS: vfs.NewNullVFS(0), data: append([]byte(nil), data...),
		item:    vfs.VFSItem{Name: "image.jpg", Size: int64(len(data)), SizeKnown: true, Revision: "revision-1"},
		profile: vfs.ReadAccessNativeRange, storage: vfs.StorageClassNetwork,
	}
}

func (f *countingMediaVFS) GetPath() string { return "/gallery" }
func (f *countingMediaVFS) Join(parts ...string) string {
	return path.Join(parts...)
}
func (f *countingMediaVFS) Abs(candidate string) (string, error) {
	if path.IsAbs(candidate) {
		return path.Clean(candidate), nil
	}
	return path.Join(f.GetPath(), candidate), nil
}
func (f *countingMediaVFS) Stat(context.Context, string) (vfs.VFSItem, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.item, nil
}
func (f *countingMediaVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, ReadAccess: f.profile, StorageClass: f.storage}
}
func (f *countingMediaVFS) Clone() vfs.VFS { return f }
func (f *countingMediaVFS) Open(ctx context.Context, _ string) (vfs.ReadAtCloser, error) {
	f.mu.Lock()
	f.openCount++
	gate := f.openGate
	started := f.openStarted
	if started != nil {
		f.openOnce.Do(func() { close(started) })
	}
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.handleSlots != nil {
		select {
		case f.handleSlots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &countingMediaReader{owner: f, slotHeld: f.handleSlots != nil}, nil
}

type countingMediaReader struct {
	owner    *countingMediaVFS
	mu       sync.Mutex
	offset   int64
	closed   bool
	slotHeld bool
}

type rotatingSessionMediaVFS struct {
	*countingMediaVFS
	session         any
	cloneCount      int
	cloneCloseCount int
}

func newRotatingSessionMediaVFS(data []byte) *rotatingSessionMediaVFS {
	return &rotatingSessionMediaVFS{countingMediaVFS: newCountingMediaVFS(data), session: new(int)}
}

func (f *rotatingSessionMediaVFS) SessionKey() any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.session
}

func (f *rotatingSessionMediaVFS) Clone() vfs.VFS {
	f.mu.Lock()
	f.cloneCount++
	session := f.session
	f.mu.Unlock()
	return &rotatingSessionMediaClone{countingMediaVFS: f.countingMediaVFS, owner: f, session: session}
}

type rotatingSessionMediaClone struct {
	*countingMediaVFS
	owner   *rotatingSessionMediaVFS
	session any
	once    sync.Once
}

func (f *rotatingSessionMediaClone) SessionKey() any { return f.session }
func (f *rotatingSessionMediaClone) Close() error {
	f.once.Do(func() {
		f.owner.mu.Lock()
		f.owner.cloneCloseCount++
		f.owner.mu.Unlock()
	})
	return nil
}

type closeDrainMediaState struct {
	mu              sync.Mutex
	started         chan struct{}
	cancelObserved  chan struct{}
	allowFinish     chan struct{}
	finished        chan struct{}
	startOnce       sync.Once
	cancelOnce      sync.Once
	allowFinishOnce sync.Once
	finishOnce      sync.Once
	opening         int
	cloneCloseCount int
	closeDuringOpen bool
}

type closeDrainMediaVFS struct {
	*countingMediaVFS
	state *closeDrainMediaState
	clone bool
}

func newCloseDrainMediaVFS(data []byte) *closeDrainMediaVFS {
	return &closeDrainMediaVFS{
		countingMediaVFS: newCountingMediaVFS(data),
		state: &closeDrainMediaState{
			started:        make(chan struct{}),
			cancelObserved: make(chan struct{}),
			allowFinish:    make(chan struct{}),
			finished:       make(chan struct{}),
		},
	}
}

func (f *closeDrainMediaVFS) Clone() vfs.VFS {
	return &closeDrainMediaVFS{
		countingMediaVFS: f.countingMediaVFS,
		state:            f.state,
		clone:            true,
	}
}

func (f *closeDrainMediaVFS) Open(ctx context.Context, _ string) (vfs.ReadAtCloser, error) {
	f.state.mu.Lock()
	f.state.opening++
	f.state.startOnce.Do(func() { close(f.state.started) })
	f.state.mu.Unlock()

	<-ctx.Done()
	f.state.cancelOnce.Do(func() { close(f.state.cancelObserved) })
	<-f.state.allowFinish

	f.state.mu.Lock()
	f.state.opening--
	f.state.finishOnce.Do(func() { close(f.state.finished) })
	f.state.mu.Unlock()
	return nil, ctx.Err()
}

func (f *closeDrainMediaVFS) Close() error {
	if !f.clone {
		return nil
	}
	f.state.mu.Lock()
	f.state.cloneCloseCount++
	if f.state.opening != 0 {
		f.state.closeDuringOpen = true
	}
	f.state.mu.Unlock()
	return nil
}

func (f *closeDrainMediaVFS) allowOpenToFinish() {
	f.state.allowFinishOnce.Do(func() { close(f.state.allowFinish) })
}

func (r *countingMediaReader) Size() int64 { return int64(len(r.owner.data)) }
func (r *countingMediaReader) ReadAccessProfile() vfs.ReadAccessProfile {
	if r.owner.backingPath != "" {
		return vfs.ReadAccessMaterializeOnce
	}
	return r.owner.profile
}
func (r *countingMediaReader) LocalPath() (string, bool) {
	return r.owner.backingPath, r.owner.backingPath != ""
}
func (r *countingMediaReader) ReadAt(ctx context.Context, dst []byte, offset int64) (int, error) {
	r.owner.mu.Lock()
	r.owner.readCount++
	r.owner.readOffsets = append(r.owner.readOffsets, offset)
	gate := r.owner.gate
	started := r.owner.started
	failRead := r.owner.failReadOnce
	failErr := r.owner.failReadErr
	if failRead {
		r.owner.failReadOnce = false
	}
	if started != nil {
		r.owner.startOnce.Do(func() { close(started) })
	}
	r.owner.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if failRead {
		if failErr == nil {
			failErr = errors.New("simulated transport read failure")
		}
		return 0, failErr
	}
	if offset >= int64(len(r.owner.data)) {
		return 0, io.EOF
	}
	n := copy(dst, r.owner.data[offset:])
	if n < len(dst) {
		return n, io.EOF
	}
	return n, nil
}
func (r *countingMediaReader) Read(ctx context.Context, dst []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n, err := r.ReadAt(ctx, dst, r.offset)
	r.offset += int64(n)
	return n, err
}
func (r *countingMediaReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	if r.slotHeld {
		<-r.owner.handleSlots
		r.slotHeld = false
	}
	r.owner.mu.Lock()
	r.owner.closeCount++
	r.owner.mu.Unlock()
	return nil
}

func registerCountingMediaSource(t *testing.T, broker *extUiMediaBroker, filesystem *countingMediaVFS) extUiImageSourceDescriptor {
	t.Helper()
	descriptor := broker.Register(mediaSourceRegistration{
		PanelID: "panel-1", CatalogVersion: 1, FS: filesystem,
		Path: "/gallery/image.jpg", Item: filesystem.item,
	})
	if descriptor.ResourceID == "" {
		t.Fatal("broker did not issue a resource id")
	}
	broker.CommitPanel("panel-1", 1, []string{descriptor.ResourceID})
	return descriptor
}

func TestExtUiMediaBrokerCoalescesRangeAndIsolatesCancellation(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("0123456789abcdef"))
	filesystem.gate = make(chan struct{})
	filesystem.started = make(chan struct{})
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := registerCountingMediaSource(t, broker, filesystem)

	type result struct {
		data []byte
		err  error
	}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	first := make(chan result, 1)
	second := make(chan result, 1)
	go func() {
		data, _, err := broker.ReadRange(firstCtx, descriptor.ResourceID, 0, 8)
		first <- result{data: data, err: err}
	}()
	<-filesystem.started
	go func() {
		data, _, err := broker.ReadRange(context.Background(), descriptor.ResourceID, 0, 8)
		second <- result{data: data, err: err}
	}()

	resource := broker.resources[descriptor.ResourceID]
	deadline := time.Now().Add(time.Second)
	for {
		resource.mu.Lock()
		waiters := 0
		for _, flight := range resource.rangeFlights {
			waiters = flight.waiters
		}
		resource.mu.Unlock()
		if waiters == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second request did not join the shared range read")
		}
		time.Sleep(time.Millisecond)
	}
	cancelFirst()
	close(filesystem.gate)

	if got := <-first; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v", got.err)
	}
	if got := <-second; got.err != nil || string(got.data) != "01234567" {
		t.Fatalf("surviving waiter = %q, %v", got.data, got.err)
	}
	data, _, err := broker.ReadRange(context.Background(), descriptor.ResourceID, 2, 3)
	if err != nil || string(data) != "234" {
		t.Fatalf("contained cache read = %q, %v", data, err)
	}
	filesystem.mu.Lock()
	defer filesystem.mu.Unlock()
	if filesystem.openCount != 1 || filesystem.readCount != 1 {
		t.Fatalf("opens/reads = %d/%d, want 1/1", filesystem.openCount, filesystem.readCount)
	}
}

func TestExtUiMediaBrokerReopensHandleAfterReadFailure(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("0123456789abcdef"))
	filesystem.failReadOnce = true
	filesystem.failReadErr = errors.New("transport connection lost")
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := registerCountingMediaSource(t, broker, filesystem)

	if _, _, err := broker.ReadRange(
		context.Background(), descriptor.ResourceID, 0, 4); err == nil {
		t.Fatal("first read unexpectedly succeeded")
	}
	data, _, err := broker.ReadRange(
		context.Background(), descriptor.ResourceID, 0, 4)
	if err != nil || string(data) != "0123" {
		t.Fatalf("retry read = %q, %v", data, err)
	}

	filesystem.mu.Lock()
	opens, closes, reads := filesystem.openCount, filesystem.closeCount, filesystem.readCount
	filesystem.mu.Unlock()
	if opens != 2 || closes < 1 || reads != 2 {
		t.Fatalf("handle lifecycle = opens:%d closes:%d reads:%d, want 2, >=1, 2", opens, closes, reads)
	}
}

func TestExtUiMediaBrokerNativeRangeDoesNotExhaustScarceHandles(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("0123456789abcdef"))
	filesystem.handleSlots = make(chan struct{}, 2)
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()

	ids := make([]string, 0, 3)
	for index := range 3 {
		descriptor := broker.Register(mediaSourceRegistration{
			PanelID: "panel-1", CatalogVersion: 1, FS: filesystem,
			Path: fmt.Sprintf("/gallery/image-%d.jpg", index), Item: filesystem.item,
		})
		ids = append(ids, descriptor.ResourceID)
	}
	broker.CommitPanel("panel-1", 1, ids)

	for index, id := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		data, _, readErr := broker.ReadRange(ctx, id, 0, 4)
		cancel()
		if readErr != nil || string(data) != "0123" {
			t.Fatalf("range %d = %q, %v", index, data, readErr)
		}
	}

	filesystem.mu.Lock()
	opens, closes := filesystem.openCount, filesystem.closeCount
	filesystem.mu.Unlock()
	broker.mu.Lock()
	openHandles := broker.openHandles
	broker.mu.Unlock()
	if opens != 3 || closes != 3 || openHandles != 0 {
		t.Fatalf("scarce handle lifecycle = opens:%d closes:%d broker:%d, want 3/3/0",
			opens, closes, openHandles)
	}
}

func TestExtUiMediaBrokerCanceledReadDoesNotBreakConcurrentRange(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("0123456789abcdef"))
	filesystem.gate = make(chan struct{})
	filesystem.started = make(chan struct{})
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := registerCountingMediaSource(t, broker, filesystem)

	type result struct {
		data []byte
		err  error
	}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	first := make(chan result, 1)
	second := make(chan result, 1)
	go func() {
		data, _, err := broker.ReadRange(firstCtx, descriptor.ResourceID, 0, 4)
		first <- result{data: data, err: err}
	}()
	<-filesystem.started
	go func() {
		data, _, err := broker.ReadRange(context.Background(), descriptor.ResourceID, 4, 4)
		second <- result{data: data, err: err}
	}()

	resource := broker.resources[descriptor.ResourceID]
	deadline := time.Now().Add(time.Second)
	for {
		resource.mu.Lock()
		flights := len(resource.rangeFlights)
		resource.mu.Unlock()
		if flights == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("concurrent range did not start")
		}
		time.Sleep(time.Millisecond)
	}
	// Give the second flight time to obtain the shared handle and queue behind
	// the first read's ioMu. The assertion is race-covered and deterministic
	// even if the scheduler instead lets it observe the detached handle later.
	time.Sleep(10 * time.Millisecond)
	cancelFirst()
	if got := <-first; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled range error = %v", got.err)
	}
	close(filesystem.gate)
	if got := <-second; got.err != nil || string(got.data) != "4567" {
		t.Fatalf("concurrent range = %q, %v", got.data, got.err)
	}
	filesystem.mu.Lock()
	opens, closes := filesystem.openCount, filesystem.closeCount
	filesystem.mu.Unlock()
	if opens != 2 || closes != 2 {
		t.Fatalf("handles after cancellation = opens:%d closes:%d, want 2/2", opens, closes)
	}
}

func TestExtUiMediaBrokerCoalescesOpenAcrossDifferentOperations(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("0123456789abcdef"))
	filesystem.openGate = make(chan struct{})
	filesystem.openStarted = make(chan struct{})
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := registerCountingMediaSource(t, broker, filesystem)

	type result struct {
		data []byte
		err  error
	}
	results := make(chan result, 2)
	go func() {
		data, _, err := broker.ReadRange(context.Background(), descriptor.ResourceID, 0, 2)
		results <- result{data: data, err: err}
	}()
	<-filesystem.openStarted
	go func() {
		data, _, err := broker.ReadRange(context.Background(), descriptor.ResourceID, 4, 2)
		results <- result{data: data, err: err}
	}()

	deadline := time.Now().Add(time.Second)
	for {
		resource := broker.resources[descriptor.ResourceID]
		resource.mu.Lock()
		activeFlights := len(resource.rangeFlights)
		resource.mu.Unlock()
		if activeFlights == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second operation did not wait on the shared open")
		}
		time.Sleep(time.Millisecond)
	}
	filesystem.mu.Lock()
	opensBeforeRelease := filesystem.openCount
	filesystem.mu.Unlock()
	if opensBeforeRelease != 1 {
		t.Fatalf("concurrent opens before release = %d, want 1", opensBeforeRelease)
	}
	close(filesystem.openGate)

	got := []result{<-results, <-results}
	for _, current := range got {
		if current.err != nil {
			t.Fatalf("range error = %v", current.err)
		}
	}
	filesystem.mu.Lock()
	opens := filesystem.openCount
	filesystem.mu.Unlock()
	if opens != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}
}

func TestExtUiMediaBrokerSharedOpenIsolatesWaiterCancellation(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("0123456789abcdef"))
	filesystem.openGate = make(chan struct{})
	filesystem.openStarted = make(chan struct{})
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := registerCountingMediaSource(t, broker, filesystem)

	type result struct {
		data []byte
		err  error
	}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	first := make(chan result, 1)
	second := make(chan result, 1)
	go func() {
		data, _, err := broker.ReadRange(firstCtx, descriptor.ResourceID, 0, 2)
		first <- result{data: data, err: err}
	}()
	<-filesystem.openStarted
	go func() {
		data, _, err := broker.ReadRange(context.Background(), descriptor.ResourceID, 4, 2)
		second <- result{data: data, err: err}
	}()

	resource := broker.resources[descriptor.ResourceID]
	deadline := time.Now().Add(time.Second)
	for {
		resource.mu.Lock()
		waiters := 0
		if resource.openFlight != nil {
			waiters = resource.openFlight.waiters
		}
		resource.mu.Unlock()
		if waiters == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("second operation did not join the shared open")
		}
		time.Sleep(time.Millisecond)
	}
	cancelFirst()
	if got := <-first; !errors.Is(got.err, context.Canceled) {
		t.Fatalf("canceled open waiter error = %v", got.err)
	}
	close(filesystem.openGate)
	if got := <-second; got.err != nil || string(got.data) != "45" {
		t.Fatalf("surviving open waiter = %q, %v", got.data, got.err)
	}
	filesystem.mu.Lock()
	opens, reads := filesystem.openCount, filesystem.readCount
	filesystem.mu.Unlock()
	if opens != 1 || reads != 1 {
		t.Fatalf("shared open counts = opens:%d reads:%d, want 1/1", opens, reads)
	}
}

func TestExtUiMediaBrokerMaterializationUsesRangePrefixAndSingleFlight(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("0123456789abcdef"))
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := registerCountingMediaSource(t, broker, filesystem)
	if _, _, err := broker.ReadRange(context.Background(), descriptor.ResourceID, 0, 4); err != nil {
		t.Fatal(err)
	}

	type result struct {
		path, lease string
		err         error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() {
			local, lease, _, _, err := broker.Materialize(context.Background(), descriptor.ResourceID)
			results <- result{path: local, lease: lease, err: err}
		}()
	}
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("materialize errors = %v / %v", first.err, second.err)
	}
	if first.path == "" || first.path != second.path || first.lease == second.lease {
		t.Fatalf("materialization paths/leases = %#v / %#v", first, second)
	}
	data, err := os.ReadFile(first.path)
	if err != nil || !bytes.Equal(data, filesystem.data) {
		t.Fatalf("materialized data = %q, %v", data, err)
	}
	filesystem.mu.Lock()
	offsets := append([]int64(nil), filesystem.readOffsets...)
	opens := filesystem.openCount
	filesystem.mu.Unlock()
	if opens != 2 || !reflect.DeepEqual(offsets, []int64{0, 4}) {
		t.Fatalf("opens/offsets = %d/%v, want 2/[0 4]", opens, offsets)
	}

	broker.Release(descriptor.ResourceID, first.lease)
	broker.Release(descriptor.ResourceID, second.lease)
	broker.Release(descriptor.ResourceID, "")
	if _, err := os.Stat(first.path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("broker-owned spool was not removed: %v", err)
	}
	broker.mu.Lock()
	openHandles, rangeBytes, materializedBytes := broker.openHandles, broker.rangeCacheBytes, broker.ownMaterializationBytes
	broker.mu.Unlock()
	if openHandles != 0 || rangeBytes != 0 || materializedBytes != 0 {
		t.Fatalf("accounting after release = handles:%d range:%d materialized:%d", openHandles, rangeBytes, materializedBytes)
	}
}

func TestExtUiMediaBrokerReusesProviderLocalBacking(t *testing.T) {
	backing := path.Join(t.TempDir(), "cached.jpg")
	if err := os.WriteFile(backing, []byte("provider-cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	filesystem := newCountingMediaVFS([]byte("provider-cache"))
	filesystem.backingPath = backing
	filesystem.profile = vfs.ReadAccessMaterializeOnce
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := registerCountingMediaSource(t, broker, filesystem)
	local, lease, _, profile, err := broker.Materialize(context.Background(), descriptor.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if local != backing || profile != "materializeOnce" {
		t.Fatalf("local/profile = %q/%q", local, profile)
	}
	filesystem.mu.Lock()
	reads := filesystem.readCount
	filesystem.mu.Unlock()
	if reads != 0 {
		t.Fatalf("provider backing was copied through %d reads", reads)
	}
	broker.Release(descriptor.ResourceID, lease)
	broker.Release(descriptor.ResourceID, "")
	if _, err := os.Stat(backing); err != nil {
		t.Fatalf("provider-owned backing was removed: %v", err)
	}
}

func TestExtUiMediaBrokerRejectsResponseAfterCatalogInvalidation(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("stale-data"))
	filesystem.gate = make(chan struct{})
	filesystem.started = make(chan struct{})
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := registerCountingMediaSource(t, broker, filesystem)
	result := make(chan error, 1)
	go func() {
		_, _, err := broker.ReadRange(context.Background(), descriptor.ResourceID, 0, 5)
		result <- err
	}()
	<-filesystem.started
	broker.CommitPanel("panel-1", 2, nil)
	close(filesystem.gate)
	if err := <-result; !errors.Is(err, errMediaUnknownResource) {
		t.Fatalf("stale read error = %v", err)
	}
}

func dialExtUiMediaTestServer(t *testing.T, server *extUiMediaServer) net.Conn {
	t.Helper()
	conn, err := net.Dial("tcp", server.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	if err := extUiMediaSendMessage(conn, map[string]any{
		"type": "hello", "protocol": extUiMediaProtocolVersion, "nonce": server.nonce,
	}); err != nil {
		t.Fatal(err)
	}
	hello, err := extUiMediaReadMessage(conn)
	if err != nil {
		t.Fatal(err)
	}
	if extUiString(hello, "type") != "hello" || extUiInt(hello, "maxChunkSize") != extUiMediaMaxRangeSize {
		t.Fatalf("media hello = %#v", hello)
	}
	return conn
}

func TestExtUiMediaServerWireAndReconnect(t *testing.T) {
	server, err := newExtUiMediaServer()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	filesystem := newCountingMediaVFS([]byte("wire-protocol"))
	descriptor := registerCountingMediaSource(t, server.broker, filesystem)
	unauthorized, err := net.Dial("tcp", server.Endpoint())
	if err != nil {
		t.Fatal(err)
	}
	if err := extUiMediaSendMessage(unauthorized, map[string]any{
		"type": "hello", "protocol": extUiMediaProtocolVersion, "nonce": "wrong",
	}); err != nil {
		t.Fatal(err)
	}
	_ = unauthorized.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := extUiMediaReadMessage(unauthorized); err == nil {
		t.Fatal("media server accepted an invalid nonce")
	}
	_ = unauthorized.Close()

	conn := dialExtUiMediaTestServer(t, server)
	if err := extUiMediaSendMessage(conn, map[string]any{
		"type": "request", "requestId": "range-1", "op": "readRange",
		"resourceId": descriptor.ResourceID, "offset": int64(5), "length": 8,
	}); err != nil {
		t.Fatal(err)
	}
	response, err := extUiMediaReadMessage(conn)
	if err != nil {
		t.Fatal(err)
	}
	if !extUiBool(response, "ok") || string(response["data"].([]byte)) != "protocol" {
		t.Fatalf("range response = %#v", response)
	}
	if err := extUiMediaSendMessage(conn, map[string]any{
		"type": "request", "requestId": "materialize-ack", "op": "materialize",
		"resourceId": descriptor.ResourceID,
	}); err != nil {
		t.Fatal(err)
	}
	materialized, err := extUiMediaReadMessage(conn)
	if err != nil {
		t.Fatal(err)
	}
	leaseID := extUiString(materialized, "leaseId")
	if !extUiBool(materialized, "ok") || leaseID == "" || !server.broker.hasLease(descriptor.ResourceID, leaseID) {
		t.Fatalf("materialize response = %#v", materialized)
	}
	if err := extUiMediaSendMessage(conn, map[string]any{
		"type": "ack", "requestId": "materialize-ack",
		"resourceId": descriptor.ResourceID, "leaseId": leaseID,
	}); err != nil {
		t.Fatal(err)
	}
	ack, err := extUiMediaReadMessage(conn)
	if err != nil {
		t.Fatal(err)
	}
	if extUiString(ack, "type") != "ack" || !extUiBool(ack, "ok") || extUiString(ack, "leaseId") != leaseID {
		t.Fatalf("lease ack = %#v", ack)
	}
	_ = conn.Close()

	// The endpoint remains valid for a Qt host reconnect during the same ExtUI
	// session; acknowledged leases survive that transport reconnect.
	reconnected := dialExtUiMediaTestServer(t, server)
	if !server.broker.hasLease(descriptor.ResourceID, leaseID) {
		t.Fatal("acknowledged lease was released on disconnect")
	}
	if err := extUiMediaSendMessage(reconnected, map[string]any{
		"type": "release", "requestId": "release-ack",
		"resourceId": descriptor.ResourceID, "leaseId": leaseID,
	}); err != nil {
		t.Fatal(err)
	}
	releaseAck, err := extUiMediaReadMessage(reconnected)
	if err != nil {
		t.Fatal(err)
	}
	if extUiString(releaseAck, "type") != "releaseAck" ||
		extUiString(releaseAck, "requestId") != "release-ack" ||
		!extUiBool(releaseAck, "ok") {
		t.Fatalf("release ack = %#v", releaseAck)
	}
	deadline := time.Now().Add(time.Second)
	for server.broker.hasLease(descriptor.ResourceID, leaseID) {
		if time.Now().After(deadline) {
			t.Fatal("acknowledged lease was not explicitly released")
		}
		time.Sleep(time.Millisecond)
	}

	// A successful response which the client never acknowledges remains owned
	// by this connection and is reclaimed as soon as the connection disappears.
	if err := extUiMediaSendMessage(reconnected, map[string]any{
		"type": "request", "requestId": "materialize-orphan", "op": "materialize",
		"resourceId": descriptor.ResourceID,
	}); err != nil {
		t.Fatal(err)
	}
	orphaned, err := extUiMediaReadMessage(reconnected)
	if err != nil {
		t.Fatal(err)
	}
	orphanLease := extUiString(orphaned, "leaseId")
	if orphanLease == "" || !server.broker.hasLease(descriptor.ResourceID, orphanLease) {
		t.Fatalf("provisional response = %#v", orphaned)
	}
	_ = reconnected.Close()
	deadline = time.Now().Add(time.Second)
	for server.broker.hasLease(descriptor.ResourceID, orphanLease) {
		if time.Now().After(deadline) {
			t.Fatal("unacknowledged lease survived disconnect")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestExtUiMediaServerCloseCancelsAndDrainsBlockingOpen(t *testing.T) {
	server, err := newExtUiMediaServer()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	filesystem := newCloseDrainMediaVFS([]byte("blocking-open"))
	descriptor := server.broker.Register(mediaSourceRegistration{
		PanelID: "panel-1", CatalogVersion: 1, FS: filesystem,
		Path: "/gallery/image.jpg", Item: filesystem.item,
	})
	if descriptor.ResourceID == "" {
		t.Fatal("broker did not issue a resource id")
	}
	server.broker.CommitPanel("panel-1", 1, []string{descriptor.ResourceID})

	conn := dialExtUiMediaTestServer(t, server)
	defer conn.Close()
	if err := extUiMediaSendMessage(conn, map[string]any{
		"type": "request", "requestId": "blocking-open", "op": "readRange",
		"resourceId": descriptor.ResourceID, "offset": int64(0), "length": 4,
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-filesystem.state.started:
	case <-time.After(time.Second):
		t.Fatal("media request did not enter VFS Open")
	}
	defer filesystem.allowOpenToFinish()

	closed := make(chan error, 1)
	go func() { closed <- server.Close() }()
	select {
	case <-filesystem.state.cancelObserved:
	case <-time.After(time.Second):
		t.Fatal("media server Close did not cancel VFS Open")
	}
	select {
	case err := <-closed:
		filesystem.allowOpenToFinish()
		t.Fatalf("media server Close returned before canceled VFS Open was allowed to finish: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	filesystem.state.mu.Lock()
	closedBeforeDrain := filesystem.state.cloneCloseCount
	filesystem.state.mu.Unlock()
	if closedBeforeDrain != 0 {
		filesystem.allowOpenToFinish()
		t.Fatalf("owned VFS was closed %d time(s) while canceled Open was still draining", closedBeforeDrain)
	}

	filesystem.allowOpenToFinish()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("media server Close did not drain the blocked request")
	}

	select {
	case <-filesystem.state.finished:
	default:
		t.Fatal("media server Close returned before VFS Open finished")
	}
	filesystem.state.mu.Lock()
	closeCount := filesystem.state.cloneCloseCount
	closeDuringOpen := filesystem.state.closeDuringOpen
	filesystem.state.mu.Unlock()
	if closeDuringOpen || closeCount != 1 {
		t.Fatalf("owned VFS close count/during Open = %d/%t, want 1/false", closeCount, closeDuringOpen)
	}
}

func TestMediaVersionPrefersRevisionAndSemanticMetadataFingerprintTracksIt(t *testing.T) {
	item := vfs.VFSItem{Name: "image.jpg", Size: 10, SizeKnown: true, Revision: "opaque-revision"}
	if version, strength := mediaVersion(item, false, 7); version != item.Revision || strength != "strong" {
		t.Fatalf("version/strength = %q/%q", version, strength)
	}
	filesystem := newCountingMediaVFS([]byte("0123456789"))
	unknown := vfs.VFSItem{Name: "unknown.jpg"}
	unknownLocalVersion, localStrength := mediaSourceVersion(filesystem, unknown, true, 1, 7)
	if localStrength != "session" || unknownLocalVersion == "0:0" {
		t.Fatalf("unknown local tuple became persistent: %q (%s)", unknownLocalVersion, localStrength)
	}
	firstSessionVersion, strength := mediaSourceVersion(filesystem, unknown, false, 1, 7)
	secondSessionVersion, _ := mediaSourceVersion(filesystem, unknown, false, 99, 7)
	if strength != "session" || firstSessionVersion == secondSessionVersion {
		t.Fatalf("session versions reused across panel revisions: %q/%q (%s)", firstSessionVersion, secondSessionVersion, strength)
	}
	refreshedSessionVersion, _ := mediaSourceVersion(filesystem, unknown, false, 99, 8)
	if refreshedSessionVersion == secondSessionVersion {
		t.Fatalf("session version ignored source refresh epoch: %q", refreshedSessionVersion)
	}
	panel := &FileSystemPanel{vfs: filesystem, entries: []*fileEntry{{VFSItem: item}}}
	firstCatalog, firstMetadata, _ := panel.semanticFingerprints()
	panel.entries[0].Revision = "opaque-revision-2"
	secondCatalog, secondMetadata, _ := panel.semanticFingerprints()
	if firstCatalog != secondCatalog {
		t.Fatal("VFSItem.Revision unexpectedly changed the base catalog fingerprint")
	}
	if firstMetadata == secondMetadata {
		t.Fatal("metadata fingerprint ignored VFSItem.Revision")
	}
}

func TestExtUiMediaBrokerUnknownVersionRefreshInvalidatesAndRetires(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("catalog-scoped"))
	filesystem.item = vfs.VFSItem{Name: "image.jpg"}
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()

	register := func(revision int64) extUiImageSourceDescriptor {
		descriptor := broker.Register(mediaSourceRegistration{
			PanelID: "panel-1", CatalogVersion: revision, FS: filesystem,
			Path: "/gallery/image.jpg", Item: filesystem.item,
		})
		broker.CommitPanel("panel-1", revision, []string{descriptor.ResourceID})
		return descriptor
	}
	first := register(1)
	second := register(2)
	if first.ResourceID == second.ResourceID || first.Version == second.Version {
		t.Fatalf("catalog refresh reused unknown-version identity: %#v / %#v", first, second)
	}
	if _, _, err := broker.ReadRange(context.Background(), first.ResourceID, 0, 1); !errors.Is(err, errMediaUnknownResource) {
		t.Fatalf("old catalog resource read error = %v", err)
	}
	if ids := broker.resourceIDs(); !reflect.DeepEqual(ids, []string{second.ResourceID}) {
		t.Fatalf("registry after refresh = %v, want only %q", ids, second.ResourceID)
	}

	for revision := int64(3); revision <= 100; revision++ {
		second = register(revision)
		if ids := broker.resourceIDs(); len(ids) != 1 || ids[0] != second.ResourceID {
			t.Fatalf("registry grew at revision %d: %v", revision, ids)
		}
	}
	broker.CommitPanel("panel-1", 101, nil)
	if ids := broker.resourceIDs(); len(ids) != 0 {
		t.Fatalf("registry retained terminal resources: %v", ids)
	}
}

func TestExtUiMediaBrokerReclonesAfterSessionEpochChanges(t *testing.T) {
	filesystem := newRotatingSessionMediaVFS([]byte("session-data"))
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	first := broker.Register(mediaSourceRegistration{
		PanelID: "panel-1", CatalogVersion: 1, FS: filesystem,
		Path: "/gallery/image.jpg", Item: filesystem.item,
	})
	broker.CommitPanel("panel-1", 1, []string{first.ResourceID})

	filesystem.mu.Lock()
	filesystem.session = new(int)
	filesystem.mu.Unlock()
	second := broker.Register(mediaSourceRegistration{
		PanelID: "panel-1", CatalogVersion: 2, FS: filesystem,
		Path: "/gallery/image.jpg", Item: filesystem.item,
	})
	broker.CommitPanel("panel-1", 2, []string{second.ResourceID})
	if first.ResourceID == second.ResourceID {
		t.Fatalf("session epoch reused resource id %q", first.ResourceID)
	}
	filesystem.mu.Lock()
	clones, closes := filesystem.cloneCount, filesystem.cloneCloseCount
	filesystem.mu.Unlock()
	if clones != 2 || closes != 1 {
		t.Fatalf("session clones/closes = %d/%d, want 2/1", clones, closes)
	}
	broker.mu.Lock()
	leaseCount := len(broker.vfsLeases)
	broker.mu.Unlock()
	if leaseCount != 1 {
		t.Fatalf("VFS lease registry size = %d, want 1", leaseCount)
	}
}

func TestExtUiMediaBrokerSceneSweepAllowsReusedPanelIDAndLowerRevision(t *testing.T) {
	filesystem := newRotatingSessionMediaVFS([]byte("scene-lifecycle-data"))
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	restore := setActiveExtUiMediaBroker(broker)
	defer restore()

	const panelID = "reused-semantic-panel-id"
	legacyScene := func(panelIDs ...string) map[string]any {
		panels := make([]map[string]any, 0, len(panelIDs))
		for _, id := range panelIDs {
			panels = append(panels, map[string]any{"id": id})
		}
		return map[string]any{
			"screens": []map[string]any{{
				"frames": []map[string]any{{
					"kind": "shell", "panels": panels,
				}},
			}},
		}
	}
	exportScene := func(panelIDs ...string) {
		t.Helper()
		if scene := BuildAppSceneFromLegacy(nil, legacyScene(panelIDs...)); scene == nil {
			t.Fatal("semantic application adapter returned a nil scene")
		}
	}

	first := broker.Register(mediaSourceRegistration{
		PanelID: panelID, CatalogVersion: 99, FS: filesystem,
		Path: "/gallery/image.jpg", Item: filesystem.item,
	})
	broker.CommitPanel(panelID, 99, []string{first.ResourceID})
	exportScene(panelID)

	filesystem.mu.Lock()
	filesystem.session = new(int)
	filesystem.item.Revision = "revision-after-panel-reuse"
	secondItem := filesystem.item
	filesystem.mu.Unlock()
	second := broker.Register(mediaSourceRegistration{
		PanelID: panelID, CatalogVersion: 1, FS: filesystem,
		Path: "/gallery/image.jpg", Item: secondItem,
	})
	if first.ResourceID == second.ResourceID {
		t.Fatalf("reused panel source retained resource id %q", first.ResourceID)
	}
	// A newly allocated FileSystemPanel may reuse a pointer-derived SemanticID,
	// while its catalog revision restarts below the preceding panel's value.
	broker.CommitPanel(panelID, 1, []string{second.ResourceID})
	exportScene(panelID)
	if _, _, err := broker.ReadRange(context.Background(), first.ResourceID, 0, 1); !errors.Is(err, errMediaUnknownResource) {
		t.Fatalf("preceding panel resource read error = %v", err)
	}
	if data, _, err := broker.ReadRange(context.Background(), second.ResourceID, 0, 5); err != nil || string(data) != "scene" {
		t.Fatalf("replacement panel range = %q, %v", data, err)
	}

	filesystem.mu.Lock()
	clonesBeforeSweep, closesBeforeSweep := filesystem.cloneCount, filesystem.cloneCloseCount
	filesystem.mu.Unlock()
	if clonesBeforeSweep != 2 || closesBeforeSweep != 1 {
		t.Fatalf("replacement clone lifecycle = %d/%d, want 2/1", clonesBeforeSweep, closesBeforeSweep)
	}

	exportScene()
	if ids := broker.resourceIDs(); len(ids) != 0 {
		t.Fatalf("closed workspace retained media registry: %v", ids)
	}
	broker.mu.Lock()
	panelCount, leaseCount := len(broker.panels), len(broker.vfsLeases)
	broker.mu.Unlock()
	if panelCount != 0 || leaseCount != 0 {
		t.Fatalf("closed workspace retained panels/VFS leases = %d/%d", panelCount, leaseCount)
	}
	filesystem.mu.Lock()
	clonesAfterSweep, closesAfterSweep := filesystem.cloneCount, filesystem.cloneCloseCount
	filesystem.mu.Unlock()
	if clonesAfterSweep != 2 || closesAfterSweep != 2 {
		t.Fatalf("scene sweep clone lifecycle = %d/%d, want 2/2", clonesAfterSweep, closesAfterSweep)
	}
}

func TestExtUiMediaBrokerLargeCatalogAvoidsBelowThresholdSweeps(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("jpeg-probe-data"))
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()

	const catalogSize = 10_000
	ids := make([]string, 0, catalogSize)
	for index := range catalogSize {
		descriptor := broker.Register(mediaSourceRegistration{
			PanelID: "large-panel", CatalogVersion: 1, FS: filesystem,
			Path: fmt.Sprintf("/gallery/image-%05d.jpg", index), Item: filesystem.item,
		})
		ids = append(ids, descriptor.ResourceID)
	}
	broker.CommitPanel("large-panel", 1, ids)
	if data, _, err := broker.ReadRange(context.Background(), ids[0], 0, 4); err != nil || string(data) != "jpeg" {
		t.Fatalf("probe read = %q, %v", data, err)
	}

	broker.mu.Lock()
	visits := broker.globalSweepVisits
	openHandles := broker.openHandles
	rangeBytes := broker.rangeCacheBytes
	broker.mu.Unlock()
	if visits != 0 {
		t.Fatalf("below-threshold request visited %d catalog resources", visits)
	}
	if openHandles != 0 || rangeBytes != 4 {
		t.Fatalf("broker accounting handles/range = %d/%d, want 0/4", openHandles, rangeBytes)
	}
}

func TestExtUiMediaBrokerLargeCatalogChurnSweepsOnlyOpenCandidates(t *testing.T) {
	backing := path.Join(t.TempDir(), "provider-cache.jpg")
	if err := os.WriteFile(backing, []byte("provider-cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	filesystem := newCountingMediaVFS([]byte("provider-cache"))
	filesystem.backingPath = backing
	filesystem.profile = vfs.ReadAccessMaterializeOnce
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()

	const catalogSize = 10_000
	ids := make([]string, 0, catalogSize)
	for index := range catalogSize {
		descriptor := broker.Register(mediaSourceRegistration{
			PanelID: "large-panel", CatalogVersion: 1, FS: filesystem,
			Path: fmt.Sprintf("/gallery/image-%05d.jpg", index), Item: filesystem.item,
		})
		ids = append(ids, descriptor.ResourceID)
	}
	broker.CommitPanel("large-panel", 1, ids)

	type retainedLease struct {
		resourceID string
		leaseID    string
	}
	retained := make([]retainedLease, 0, extUiMediaMaxIdleHandles+1)
	for index := 0; index < extUiMediaMaxIdleHandles*2; index++ {
		local, leaseID, _, _, err := broker.Materialize(
			context.Background(), ids[index])
		if err != nil || local != backing || leaseID == "" {
			t.Fatalf("materialize %d = %q/%q, %v", index, local, leaseID, err)
		}
		retained = append(retained, retainedLease{
			resourceID: ids[index], leaseID: leaseID,
		})

		// Make the over-limit sweep deterministic even if the materialization
		// flight's deferred maintenance is still unwinding. All 65 handles are
		// leased here, so none is evictable until the oldest lease is released.
		broker.evictIdleHandles(nil)
		if len(retained) > extUiMediaMaxIdleHandles {
			oldest := retained[0]
			retained = retained[1:]
			broker.Release(oldest.resourceID, oldest.leaseID)
		}
	}

	broker.mu.Lock()
	visits := broker.globalSweepVisits
	openHandles := broker.openHandles
	candidateCount := len(broker.openHandleCandidates)
	broker.mu.Unlock()
	// A registry-wide implementation visits roughly 10k resources on every
	// post-limit image (>600k visits here). Candidate sweeps stay proportional
	// to the 64 retained provider handles, independent of catalog size.
	if visits >= catalogSize*5 {
		t.Fatalf("handle churn visited %d resources from a %d-entry catalog", visits, catalogSize)
	}
	if openHandles != extUiMediaMaxIdleHandles || candidateCount != openHandles {
		t.Fatalf("open handle accounting/candidates = %d/%d, want %d/%d",
			openHandles, candidateCount, extUiMediaMaxIdleHandles, extUiMediaMaxIdleHandles)
	}

	for _, current := range retained {
		broker.Release(current.resourceID, current.leaseID)
	}
	broker.CommitPanel("large-panel", 2, nil)
}

func TestSemanticStaticEntryRegistersBrokerSourceDescriptor(t *testing.T) {
	filesystem := newCountingMediaVFS([]byte("0123456789"))
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	restore := setActiveExtUiMediaBroker(broker)
	defer restore()
	panel := &FileSystemPanel{vfs: filesystem, entries: []*fileEntry{{VFSItem: filesystem.item}}}
	panel.updateSemanticRevisions()
	static := panel.semanticStaticPanelData("vfs")
	if len(static.entries) != 1 || static.entries[0].Source == nil {
		t.Fatalf("semantic entries = %#v", static.entries)
	}
	source := static.entries[0].Source
	if source.ResourceID == "" || source.SourceKey == "" || source.Version != filesystem.item.Revision ||
		source.VersionStrength != "strong" || source.AccessProfile != "nativeRange" || source.StorageClass != "network" {
		t.Fatalf("semantic source = %#v", source)
	}
}

func TestDeferredSemanticStaticCatalogRegistersOnlyImageSources(t *testing.T) {
	previousCapability := setExtUiPanelCatalogMetadataEnabled(true)
	t.Cleanup(func() { setExtUiPanelCatalogMetadataEnabled(previousCapability) })

	filesystem := newCountingMediaVFS([]byte("0123456789"))
	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	restore := setActiveExtUiMediaBroker(broker)
	defer restore()

	panel := &FileSystemPanel{
		vfs: filesystem,
		entries: []*fileEntry{
			{VFSItem: vfs.VFSItem{Name: "image.jpg", Size: 10, SizeKnown: true}},
			{VFSItem: vfs.VFSItem{Name: "notes.txt", Size: 20, SizeKnown: true}},
			{VFSItem: vfs.VFSItem{Name: "folder", IsDir: true}},
		},
	}
	panel.updateSemanticRevisions()
	static := panel.semanticStaticPanelData("vfs")
	if len(static.entries) != 3 {
		t.Fatalf("semantic entries = %#v", static.entries)
	}
	if static.entries[0].Source == nil || !static.entries[0].IsImage {
		t.Fatalf("image source was omitted: %#v", static.entries[0])
	}
	if static.entries[1].Source != nil || static.entries[1].IsImage {
		t.Fatalf("non-image source was registered: %#v", static.entries[1])
	}
	if static.entries[2].Source != nil {
		t.Fatalf("directory source was registered: %#v", static.entries[2])
	}
	if ids := broker.resourceIDs(); len(ids) != 1 {
		t.Fatalf("broker resources = %d, want one image resource", len(ids))
	}
}
