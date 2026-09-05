//go:build darwin

package main

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestMacOSQueryVFSStreamsDuplicateNamesAndDelegatesLocalOperations(t *testing.T) {
	root := t.TempDir()
	dirOne := filepath.Join(root, "One")
	dirTwo := filepath.Join(root, "Two")
	if err := os.MkdirAll(dirOne, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dirTwo, 0o755); err != nil {
		t.Fatal(err)
	}
	pathOne := filepath.Join(dirOne, "photo.jpg")
	pathTwo := filepath.Join(dirTwo, "photo.jpg")
	if err := os.WriteFile(pathOne, []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathTwo, []byte("two"), 0o644); err != nil {
		t.Fatal(err)
	}

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
			"operation": "macos.query", "chunk": true, "final": false,
			"payload": map[string]any{"items": []any{map[string]any{
				"id": "first", "path": pathOne,
				"displayName": "photo.jpg — One", "size": int64(3),
				"sizeKnown": true, "isDir": false,
			}}},
		})
		client.handleResponse(map[string]any{
			"type": "platform_response", "requestId": requestID,
			"operation": "macos.query", "chunk": false, "final": true,
			"payload": map[string]any{"items": []any{map[string]any{
				"id": "second", "path": pathTwo,
				"displayName": "photo.jpg — Two", "size": int64(3),
				"sizeKnown": true, "isDir": false,
			}}},
		})
	}()

	query := newMacOSQueryVFS("macos://recents", "recents", "", "Recents", nil)
	query.client = client
	var items []vfs.VFSItem
	if err := query.ReadDir(context.Background(), query.GetPath(), func(chunk []vfs.VFSItem) {
		items = append(items, chunk...)
	}); err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(items) != 2 || items[0].Name == items[1].Name ||
		items[0].DisplayName != "photo.jpg — One" ||
		items[1].DisplayName != "photo.jpg — Two" {
		t.Fatalf("duplicate result identities/display names are wrong: %#v", items)
	}
	if filepath.Ext(items[0].Name) != ".jpg" || filepath.Ext(items[1].Name) != ".jpg" {
		t.Fatalf("synthetic names lost extensions: %#v", items)
	}
	rootItem, err := query.Stat(context.Background(), query.GetPath())
	if err != nil || !rootItem.IsDir || rootItem.DisplayName != "Recents" {
		t.Fatalf("query root Stat = %#v, %v", rootItem, err)
	}

	firstPath := query.Join(query.GetPath(), items[0].Name)
	if local, err := query.LocalPath(firstPath); err != nil || local != pathOne {
		t.Fatalf("LocalPath = %q, %v; want %q", local, err, pathOne)
	}
	if got := query.TransferName(firstPath, nil); got != "photo.jpg" {
		t.Fatalf("TransferName = %q", got)
	}
	reader, err := query.Open(context.Background(), firstPath)
	if err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, 3)
	n, err := reader.Read(context.Background(), buffer)
	_ = reader.Close()
	if err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if n != 3 || string(buffer) != "one" {
		t.Fatalf("Open read %d bytes %q", n, buffer)
	}
	if _, err := query.Create(context.Background(), "new.jpg"); !errors.Is(err, errMacOSQueryReadOnly) {
		t.Fatalf("Create returned %v", err)
	}
	if err := query.MkDir(context.Background(), "folder"); !errors.Is(err, errMacOSQueryReadOnly) {
		t.Fatalf("MkDir returned %v", err)
	}
	if err := query.Rename(context.Background(), firstPath,
		query.Join(query.GetPath(), "renamed.jpg")); err != nil {
		t.Fatalf("Rename failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dirOne, "renamed.jpg")); err != nil {
		t.Fatalf("renamed local item missing: %v", err)
	}
	secondPath := query.Join(query.GetPath(), items[1].Name)
	if err := query.Remove(context.Background(), secondPath); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if _, err := os.Stat(pathTwo); !os.IsNotExist(err) {
		t.Fatalf("deleted local item still exists: %v", err)
	}
}

func TestMacOSQueryVFSSpotlightTimestampDriftStillFeedsGalleryBroker(t *testing.T) {
	root := t.TempDir()
	realPath := filepath.Join(root, "tagged.PNG")
	content := []byte("real tagged image bytes")
	if err := os.WriteFile(realPath, content, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(realPath)
	if err != nil {
		t.Fatal(err)
	}

	query := newMacOSQueryVFS("macos://tag/Red", "tag", "Red", "Red", nil)
	// NSDate -> nanoseconds does not reproduce APFS' integer timestamp exactly.
	// The real IMG_0118.PNG differed by 44 ns, which used to make every gallery
	// range and materialization request fail as a stale source.
	entry, ok := query.decodeEntry(map[string]any{
		"id":          "red-image",
		"path":        realPath,
		"displayName": "tagged.PNG",
		"isDir":       false,
		"size":        info.Size(),
		"sizeKnown":   true,
		"mtimeNanos":  info.ModTime().UnixNano() + 44,
	})
	if !ok {
		t.Fatal("query result was not decoded")
	}
	query.entries[entry.item.Name] = entry
	logicalPath := query.Join(query.GetPath(), entry.item.Name)

	statItem, err := query.Stat(context.Background(), logicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if statItem.MTime.UnixNano() != entry.item.MTime.UnixNano() {
		t.Fatalf("equivalent Spotlight/APFS mtimes were not reconciled: listed=%d stat=%d",
			entry.item.MTime.UnixNano(), statItem.MTime.UnixNano())
	}

	broker, err := newExtUiMediaBroker()
	if err != nil {
		t.Fatal(err)
	}
	defer broker.Close()
	descriptor := broker.Register(mediaSourceRegistration{
		PanelID: "tag-panel", CatalogVersion: 1, SourceEpoch: 1,
		FS: query, Path: logicalPath, Item: entry.item,
	})
	if descriptor.ResourceID == "" {
		t.Fatal("gallery broker did not register the query result")
	}
	broker.CommitPanel("tag-panel", 1, []string{descriptor.ResourceID})

	rangeData, _, err := broker.ReadRange(
		context.Background(), descriptor.ResourceID, 0, len(content))
	if err != nil {
		t.Fatalf("gallery range read failed: %v", err)
	}
	if string(rangeData) != string(content) {
		t.Fatalf("gallery range read = %q, want %q", rangeData, content)
	}
	materialized, leaseID, size, _, err := broker.Materialize(
		context.Background(), descriptor.ResourceID)
	if err != nil {
		t.Fatalf("viewer materialization failed: %v", err)
	}
	defer broker.Release(descriptor.ResourceID, leaseID)
	if materialized != realPath || size != int64(len(content)) {
		t.Fatalf("viewer materialization = %q (%d), want %q (%d)",
			materialized, size, realPath, len(content))
	}
}

func TestMacOSQueryMTimeEquivalenceIsBounded(t *testing.T) {
	listed := time.Unix(1_786_461_349, 470_703_104)
	if !macOSQueryEquivalentMTime(listed, listed.Add(-44*time.Nanosecond)) {
		t.Fatal("NSDate/APFS conversion drift was rejected")
	}
	if macOSQueryEquivalentMTime(listed, listed.Add(2*time.Microsecond)) {
		t.Fatal("a genuine timestamp change was accepted")
	}
	if !macOSQueryEquivalentMTime(time.Time{}, listed) {
		t.Fatal("missing Spotlight timestamp did not retain its fallback version")
	}

	query := newMacOSQueryVFS("macos://tag/Red", "tag", "Red", "Red", nil)
	entry, ok := query.decodeEntry(map[string]any{
		"id": "missing-date", "path": "/tmp/missing-date.PNG",
		"displayName": "missing-date.PNG", "mtimeNanos": int64(0),
	})
	if !ok || !entry.item.MTime.IsZero() {
		t.Fatalf("missing Spotlight timestamp decoded as %#v", entry)
	}
}

func TestMacOSQueryVFSWatchUsesPlatformEventsAndCancels(t *testing.T) {
	core, host := net.Pipe()
	defer core.Close()
	defer host.Close()
	client := newPlatformIPCClient(&extUiMessageSender{w: core}, true)
	defer client.Close(nil)
	cancelSeen := make(chan struct{}, 1)
	go func() {
		request, err := extUiReadMessage(host)
		if err != nil {
			return
		}
		if extUiString(request, "operation") != "macos.watch" {
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
			"operation": "macos.watch", "final": false,
			"payload": map[string]any{"refresh": true},
		})
		if cancel, err := extUiReadMessage(host); err == nil &&
			extUiString(cancel, "type") == "platform_cancel" {
			cancelSeen <- struct{}{}
		}
	}()

	query := newMacOSQueryVFS("macos://recents", "recents", "", "Recents", nil)
	query.client = client
	ctx, cancel := context.WithCancel(context.Background())
	changed := make(chan struct{}, 1)
	done := make(chan error, 1)
	go func() {
		done <- query.WatchDirectory(ctx, query.GetPath(), func() {
			changed <- struct{}{}
			cancel()
		})
	}()
	select {
	case <-changed:
	case <-time.After(time.Second):
		t.Fatal("query watcher did not publish the platform event")
	}
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchDirectory returned %v", err)
	}
	select {
	case <-cancelSeen:
	case <-time.After(time.Second):
		t.Fatal("query watcher did not cancel its native request")
	}
}

func TestMacOSQueryResultDirectoryUsesQueryAsImmediateParent(t *testing.T) {
	resultRoot := t.TempDir()
	child := filepath.Join(resultRoot, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatal(err)
	}
	query := newMacOSQueryVFS("macos://recents", "recents", "", "Recents", nil)
	query.entries["result"] = &macOSQueryEntry{
		item:     vfs.VFSItem{Name: "result", DisplayName: "Result", IsDir: true},
		realPath: resultRoot,
	}

	opened, err := (macOSQueryDirectoryProvider{}).Open(
		context.Background(), query, query.Join(query.GetPath(), "result"))
	if err != nil {
		t.Fatalf("opening local query directory failed: %v", err)
	}
	if opened.ParentVFS() != query || !opened.IsAtRoot() {
		t.Fatalf("opened VFS parent/root = %T, %v", opened.ParentVFS(), opened.IsAtRoot())
	}
	if err := opened.SetPath(child); err != nil {
		t.Fatalf("entering child failed: %v", err)
	}
	if opened.IsAtRoot() {
		t.Fatal("child directory was incorrectly treated as the query boundary")
	}
	if err := opened.SetPath(resultRoot); err != nil {
		t.Fatalf("returning to result root failed: %v", err)
	}
	if !opened.IsAtRoot() {
		t.Fatal("result directory no longer returns to the query VFS with ..")
	}
}

func TestMacOSAllTagsEntriesOpenDedicatedTagQueryVFS(t *testing.T) {
	allTags := newMacOSQueryVFS("macos://tags", "allTags", "", "All Tags…", nil)
	entry, ok := allTags.decodeEntry(map[string]any{
		"id": "tag-red", "section": "tags", "kind": "query",
		"label": "Red", "queryKind": "tag", "tag": "Red",
		"uri": "macos://tag/Red", "color": "#ff7b72",
	})
	if !ok {
		t.Fatal("synthetic tag directory was rejected")
	}
	if !entry.item.IsDir || entry.item.DisplayName != "Red" || !entry.item.NoExtension {
		t.Fatalf("tag item = %+v, want a display-only directory", entry.item)
	}
	allTags.entries[entry.item.Name] = entry
	fullPath := allTags.Join(allTags.GetPath(), entry.item.Name)
	provider := macOSQueryDirectoryProvider{}
	if !provider.CanOpen(context.Background(), allTags, fullPath) {
		t.Fatal("tag directory was not recognized by the query directory provider")
	}

	opened, err := provider.Open(context.Background(), allTags, fullPath)
	if err != nil {
		t.Fatalf("opening tag directory failed: %v", err)
	}
	tagQuery, ok := opened.(*macOSQueryVFS)
	if !ok {
		t.Fatalf("opened VFS type = %T, want *macOSQueryVFS", opened)
	}
	if tagQuery.uri != "macos://tag/Red" || tagQuery.kind != "tag" ||
		tagQuery.tag != "Red" || tagQuery.title != "Red" {
		t.Fatalf("opened tag query = uri %q, kind %q, tag %q, title %q",
			tagQuery.uri, tagQuery.kind, tagQuery.tag, tagQuery.title)
	}
	if tagQuery.ParentVFS() != allTags {
		t.Fatal("tag query does not return to the All Tags panel")
	}
}

func TestMacOSNetworkResultResolvesThroughNativeMountPayload(t *testing.T) {
	mountPoint := t.TempDir()
	core, host := net.Pipe()
	defer core.Close()
	defer host.Close()
	client := newPlatformIPCClient(&extUiMessageSender{w: core}, true)
	defer client.Close(nil)
	requestSeen := make(chan map[string]any, 1)
	go func() {
		request, err := extUiReadMessage(host)
		if err != nil {
			return
		}
		requestSeen <- request
		requestID := extUiString(request, "requestId")
		client.handleResponse(map[string]any{
			"type": "platform_response", "requestId": requestID,
			"operation": "macos.mount", "final": true,
			"payload": map[string]any{"mountPaths": []any{mountPoint}},
		})
	}()

	query := newMacOSQueryVFS("macos://network", "network", "", "Network", nil)
	query.client = client
	query.entries["studio-nas"] = &macOSQueryEntry{
		item:        vfs.VFSItem{Name: "studio-nas", DisplayName: "Studio NAS", IsDir: true},
		serviceName: "Studio NAS", serviceType: "_smb._tcp",
		serviceDomain: "local.", scheme: "smb",
	}
	opened, err := (macOSQueryDirectoryProvider{}).Open(
		context.Background(), query, query.Join(query.GetPath(), "studio-nas"))
	if err != nil {
		t.Fatalf("opening Bonjour result failed: %v", err)
	}
	if opened.GetPath() != mountPoint || opened.ParentVFS() != query || !opened.IsAtRoot() {
		t.Fatalf("opened VFS path/parent/root = %q, %T, %v",
			opened.GetPath(), opened.ParentVFS(), opened.IsAtRoot())
	}
	select {
	case request := <-requestSeen:
		if extUiString(request, "operation") != "macos.mount" {
			t.Fatalf("operation = %q", extUiString(request, "operation"))
		}
		payload := platformMessageMap(request["payload"])
		if platformAnyString(payload["serviceName"]) != "Studio NAS" ||
			platformAnyString(payload["serviceType"]) != "_smb._tcp" ||
			platformAnyString(payload["serviceDomain"]) != "local." ||
			platformAnyString(payload["scheme"]) != "smb" {
			t.Fatalf("native mount payload = %#v", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("native mount request was not sent")
	}
}
