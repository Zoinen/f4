package plughost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type directoryPreviewVFS struct {
	*countingMediaVFS
	entries []vfs.VFSItem
	readErr error
	late    bool
}

type blockedDirectoryVFS struct {
	*directoryPreviewVFS
	session any
	entered chan struct{}
	resume  chan struct{}
}

func (f *blockedDirectoryVFS) Clone() vfs.VFS  { return f }
func (f *blockedDirectoryVFS) SessionKey() any { return f.session }
func (f *blockedDirectoryVFS) ReadDir(_ context.Context, _ string, consume func([]vfs.VFSItem)) error {
	f.entered <- struct{}{}
	<-f.resume // Deliberately emulate a provider that ignores cancellation.
	consume(f.entries)
	return nil
}

func TestDirectoryPreviewAdmissionSurvivesCancellation(t *testing.T) {
	b, err := NewExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	session := new(int)
	makeSource := func(panel string, identity any) (*blockedDirectoryVFS, string) {
		f := &blockedDirectoryVFS{directoryPreviewVFS: &directoryPreviewVFS{countingMediaVFS: newCountingMediaVFS(nil)}, session: identity, entered: make(chan struct{}, 4), resume: make(chan struct{})}
		d := b.RegisterDirectory(MediaSourceRegistration{PanelID: panel, CatalogVersion: 1, FS: f, Path: "/gallery/album", Item: vfs.VFSItem{Name: "album", IsDir: true}})
		b.CommitDirectoryPanel(panel, 1, []string{d.ResourceID})
		return f, d.ResourceID
	}
	a, aid := makeSource("a", session)
	bfs, bid := makeSource("b", session) // A different VFS instance, same remote session.
	c, cid := makeSource("c", new(int))
	defer close(a.resume)
	defer close(bfs.resume)
	defer close(c.resume)
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { _, _, err := b.EnumerateDirectoryPreview(ctx, aid); result <- err }()
	select {
	case <-a.entered:
	case <-time.After(time.Second):
		t.Fatal("first request did not start")
	}
	cancel()
	queuedCtx, queuedCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer queuedCancel()
	if _, _, err := b.EnumerateDirectoryPreview(queuedCtx, bid); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("same session must stay occupied: %v", err)
	}
	select {
	case <-bfs.entered:
		t.Fatal("canceled provider released its worker too soon")
	default:
	}
	otherCtx, otherCancel := context.WithCancel(context.Background())
	defer otherCancel()
	go func() { _, lease, _ := b.EnumerateDirectoryPreview(otherCtx, cid); b.Release(cid, lease) }()
	select {
	case <-c.entered:
	case <-time.After(time.Second):
		t.Fatal("independent source starved")
	}
	if len(b.directoryPreviews.lane) != 2 {
		t.Fatal("underlying operations must retain both slots")
	}
	b.CommitDirectoryPanel("a", 2, nil)
	select {
	case <-result:
		t.Fatal("operation completed before underlying provider returned")
	default:
	}
}

func TestDirectoryPreviewRejectsChangedChild(t *testing.T) {
	b, err := NewExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	f := &directoryPreviewVFS{countingMediaVFS: newCountingMediaVFS(nil), entries: []vfs.VFSItem{{Name: "photo.jpg", Revision: "old"}}}
	d := b.RegisterDirectory(MediaSourceRegistration{PanelID: "p", CatalogVersion: 1, FS: f, Path: "/gallery/album", Item: vfs.VFSItem{Name: "album", IsDir: true}})
	b.CommitDirectoryPanel("p", 1, []string{d.ResourceID})
	_, listing, err := b.EnumerateDirectoryPreview(context.Background(), d.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	f.entries[0].Revision = "new"
	if _, _, err := b.ResolveDirectoryPreview(context.Background(), d.ResourceID, listing, []string{"photo.jpg"}); !errors.Is(err, errMediaSourceChanged) {
		t.Fatalf("changed child: %v", err)
	}
	b.Release(d.ResourceID, listing)
}

func TestDirectoryPreviewWireLeaseOwnership(t *testing.T) {
	server, err := newExtUiMediaServer()
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	f := &directoryPreviewVFS{countingMediaVFS: newCountingMediaVFS(nil), entries: []vfs.VFSItem{{Name: "image.jpg"}}}
	d := server.broker.RegisterDirectory(MediaSourceRegistration{PanelID: "p", CatalogVersion: 1, FS: f, Path: "/gallery/album", Item: vfs.VFSItem{Name: "album", IsDir: true}})
	server.broker.CommitDirectoryPanel("p", 1, []string{d.ResourceID})
	conn := dialExtUiMediaTestServer(t, server)
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	send := func(message map[string]any) map[string]any {
		t.Helper()
		if err := extUiMediaSendMessage(conn, message); err != nil {
			t.Fatal(err)
		}
		response, err := extUiMediaReadMessage(conn)
		if err != nil {
			t.Fatal(err)
		}
		if !ExtUiBool(response, "ok") {
			t.Fatalf("response: %#v", response)
		}
		return response
	}
	listing := send(map[string]any{"type": "request", "requestId": "list", "op": "enumerateDirectoryPreview", "resourceId": d.ResourceID})
	listingID := extUiString(listing, "leaseId")
	if listingID == "" || len(listing["entries"].([]any)) != 1 {
		t.Fatalf("listing: %#v", listing)
	}
	send(map[string]any{"type": "ack", "requestId": "list", "resourceId": d.ResourceID, "leaseId": listingID})
	// The existing ACK protocol is idempotent, including after transport reconnect.
	send(map[string]any{"type": "ack", "requestId": "list", "resourceId": d.ResourceID, "leaseId": listingID})
	preview := send(map[string]any{"type": "request", "requestId": "resolve", "op": "resolveDirectoryPreview", "resourceId": d.ResourceID, "listingLeaseId": listingID, "names": []string{"image.jpg"}})
	previewID := extUiString(preview, "leaseId")
	if previewID == "" {
		t.Fatal("missing preview lease")
	}
	_ = conn.Close() // Unacknowledged children must be reclaimed.
	deadline := time.Now().Add(time.Second)
	for {
		server.broker.mu.Lock()
		remaining := len(server.broker.directoryPreviews.leases)
		server.broker.mu.Unlock()
		if remaining == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disconnected provisional lease leaked")
		}
		time.Sleep(time.Millisecond)
	}
}

func (f *directoryPreviewVFS) Clone() vfs.VFS { return f }
func (f *directoryPreviewVFS) Stat(_ context.Context, candidate string) (vfs.VFSItem, error) {
	for _, item := range f.entries {
		if f.Join("/gallery/album", item.Name) == candidate {
			return item, nil
		}
	}
	return vfs.VFSItem{}, errors.New("missing child")
}
func (f *directoryPreviewVFS) ReadDir(ctx context.Context, _ string, consume func([]vfs.VFSItem)) error {
	consume(f.entries)
	if f.late {
		consume(f.entries)
	}
	if f.readErr != nil {
		return f.readErr
	}
	return ctx.Err()
}

func TestDirectoryPreviewFirst200Files(t *testing.T) {
	f := &directoryPreviewVFS{countingMediaVFS: newCountingMediaVFS(nil), late: true}
	for i := range 500 {
		f.entries = append(f.entries, vfs.VFSItem{Name: fmt.Sprintf("dir%d", i), IsDir: true})
		f.entries = append(f.entries, vfs.VFSItem{Name: fmt.Sprintf("file%d.txt", i), IsHidden: i%2 == 0})
	}
	f.entries = append(f.entries[:400], append([]vfs.VFSItem{{Name: "too-late.jpg"}}, f.entries[400:]...)...)
	items, err := readDirectoryPreview(context.Background(), f, "/gallery/album")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 200 {
		t.Fatalf("got %d files", len(items))
	}
	for i, item := range items {
		if item.Name != fmt.Sprintf("file%d.txt", i) {
			t.Fatalf("item %d: %q", i, item.Name)
		}
	}
}

func TestDirectoryPreviewLocalChildrenUseOrdinaryMedia(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "image.jpg"), []byte("image-data"), 0600); err != nil {
		t.Fatal(err)
	}
	fs := vfs.NewOSVFS(root)
	b, err := NewExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	d := b.RegisterDirectory(MediaSourceRegistration{PanelID: "local", CatalogVersion: 1, FS: fs, Path: root, Item: vfs.VFSItem{Name: filepath.Base(root), IsDir: true}})
	b.CommitDirectoryPanel("local", 1, []string{d.ResourceID})
	items, listing, err := b.EnumerateDirectoryPreview(context.Background(), d.ResourceID)
	if err != nil || len(items) != 1 {
		t.Fatalf("local enumeration: %v %v", items, err)
	}
	images, lease, err := b.ResolveDirectoryPreview(context.Background(), d.ResourceID, listing, []string{"image.jpg"})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Release(d.ResourceID, lease)
	data, _, err := b.ReadRange(context.Background(), images[0].Source.ResourceID, 0, 10)
	if err != nil || string(data) != "image-data" {
		t.Fatalf("local media: %q %v", data, err)
	}
}

func TestDirectoryPreviewEOFAndErrors(t *testing.T) {
	f := &directoryPreviewVFS{countingMediaVFS: newCountingMediaVFS(nil), entries: []vfs.VFSItem{{Name: "photo.jpg"}}}
	items, err := readDirectoryPreview(context.Background(), f, "/gallery/album")
	if err != nil || len(items) != 1 {
		t.Fatalf("EOF: %v %v", items, err)
	}
	f.readErr = errors.New("listing unavailable")
	if items, err = readDirectoryPreview(context.Background(), f, "/gallery/album"); err == nil || len(items) != 0 {
		t.Fatalf("incomplete result: %v %v", items, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = readDirectoryPreview(ctx, f, "/gallery/album"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestDirectoryPreviewLeasesAndRefresh(t *testing.T) {
	b, err := NewExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	f := &directoryPreviewVFS{countingMediaVFS: newCountingMediaVFS([]byte("image")), entries: []vfs.VFSItem{{Name: "image.jpg", Size: 5, SizeKnown: true, Revision: "r1"}}}
	d := b.RegisterDirectory(MediaSourceRegistration{PanelID: "left", CatalogVersion: 1, SourceEpoch: 1, FS: f, Path: "/gallery/album", Item: vfs.VFSItem{Name: "album", IsDir: true}})
	if d.ResourceID == "" {
		t.Fatal("missing directory authority")
	}
	if _, _, err := b.EnumerateDirectoryPreview(context.Background(), d.ResourceID); err == nil {
		t.Fatal("uncommitted directory was readable")
	}
	b.CommitDirectoryPanel("left", 1, []string{d.ResourceID})
	_, listing, err := b.EnumerateDirectoryPreview(context.Background(), d.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.ResolveDirectoryPreview(context.Background(), d.ResourceID, listing, []string{"outside.jpg"}); err == nil {
		t.Fatal("accepted child outside captured list")
	}
	images, lease, err := b.ResolveDirectoryPreview(context.Background(), d.ResourceID, listing, []string{"image.jpg"})
	if err != nil || len(images) != 1 {
		t.Fatalf("resolve: %v %v", images, err)
	}
	id := images[0].Source.ResourceID
	b.CommitPanel("left", 2, nil)
	r, err := b.acquire(id)
	if err != nil {
		t.Fatalf("main catalog revoked preview: %v", err)
	}
	r.finish()
	b.operationWG.Done()
	b.CommitPanel("right", 1, []string{id})
	b.Release(d.ResourceID, lease)
	r, err = b.acquire(id)
	if err != nil {
		t.Fatalf("preview release revoked other panel: %v", err)
	}
	r.finish()
	b.operationWG.Done()
	_, stale, err := b.EnumerateDirectoryPreview(context.Background(), d.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	b.CommitDirectoryPanel("left", 2, nil)
	if _, _, err := b.ResolveDirectoryPreview(context.Background(), d.ResourceID, stale, []string{"image.jpg"}); err == nil {
		t.Fatal("refresh accepted old lease")
	}
	b.CommitPanel("right", 2, nil)
	if _, err := b.acquire(id); err == nil {
		t.Fatal("child resource leaked")
	}
}
