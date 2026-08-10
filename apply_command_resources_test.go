package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

type remoteApplyResourceVFS struct {
	*vfs.OSVFS
	removed []string
}

type encodedRemoteApplyResourceVFS struct {
	*remoteApplyResourceVFS
	called bool
}

type permissionFailingApplyResourceVFS struct {
	*remoteApplyResourceVFS
}

type privateCommandFileApplyResourceVFS struct {
	*remoteApplyResourceVFS
	called bool
}

type blockingRemoveApplyResourceVFS struct {
	*remoteApplyResourceVFS
	entered  chan struct{}
	unblock  chan struct{}
	once     sync.Once
	finished chan struct{}
	finish   sync.Once
}

type blockingCreateApplyResourceVFS struct {
	*remoteApplyResourceVFS
	entered chan struct{}
	unblock chan struct{}
	once    *sync.Once
}

type sameInstanceBlockingCreateApplyResourceVFS struct {
	*blockingCreateApplyResourceVFS
}

type multiBlockingRemoveApplyResourceVFS struct {
	*remoteApplyResourceVFS
	entered chan struct{}
	unblock chan struct{}
}

type blockingPermissionFailingApplyResourceVFS struct {
	*blockingRemoveApplyResourceVFS
}

type capturingAttributesApplyResourceVFS struct {
	*remoteApplyResourceVFS
	attributes vfs.VFSItem
}

func (r *permissionFailingApplyResourceVFS) Clone() vfs.VFS { return r }
func (r *permissionFailingApplyResourceVFS) GetCapabilities() vfs.VFSCapabilities {
	capabilities := r.remoteApplyResourceVFS.GetCapabilities()
	capabilities.HasUnixPermissions = true
	return capabilities
}
func (r *permissionFailingApplyResourceVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return errors.New("chmod denied")
}

func (r *privateCommandFileApplyResourceVFS) Clone() vfs.VFS { return r }
func (r *privateCommandFileApplyResourceVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return errors.New("post-create chmod must not be used")
}
func (r *privateCommandFileApplyResourceVFS) CreatePrivateCommandFile(ctx context.Context, path string) (io.WriteCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.called = true
	return os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
}

func (r *blockingRemoveApplyResourceVFS) Clone() vfs.VFS { return r }

func (r *blockingCreateApplyResourceVFS) Clone() vfs.VFS {
	return &blockingCreateApplyResourceVFS{
		remoteApplyResourceVFS: r.remoteApplyResourceVFS,
		entered:                r.entered,
		unblock:                r.unblock,
		once:                   r.once,
	}
}

func (r *sameInstanceBlockingCreateApplyResourceVFS) Clone() vfs.VFS { return r }

func (r *blockingCreateApplyResourceVFS) Create(_ context.Context, path string) (io.WriteCloser, error) {
	r.once.Do(func() { close(r.entered) })
	<-r.unblock
	return r.OSVFS.Create(context.Background(), path)
}

func (r *multiBlockingRemoveApplyResourceVFS) Clone() vfs.VFS { return r }

func (r *multiBlockingRemoveApplyResourceVFS) Remove(context.Context, string) error {
	r.entered <- struct{}{}
	<-r.unblock
	return nil
}

func (r *blockingRemoveApplyResourceVFS) Remove(context.Context, string) error {
	r.once.Do(func() { close(r.entered) })
	<-r.unblock
	r.finish.Do(func() { close(r.finished) })
	return nil
}

func (r *blockingPermissionFailingApplyResourceVFS) Clone() vfs.VFS { return r }
func (r *blockingPermissionFailingApplyResourceVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return errors.New("chmod denied")
}
func (r *blockingPermissionFailingApplyResourceVFS) GetCapabilities() vfs.VFSCapabilities {
	capabilities := r.blockingRemoveApplyResourceVFS.GetCapabilities()
	capabilities.HasUnixPermissions = true
	return capabilities
}

func (r *capturingAttributesApplyResourceVFS) Clone() vfs.VFS { return r }
func (r *capturingAttributesApplyResourceVFS) GetCapabilities() vfs.VFSCapabilities {
	capabilities := r.remoteApplyResourceVFS.GetCapabilities()
	capabilities.HasUnixPermissions = true
	return capabilities
}
func (r *capturingAttributesApplyResourceVFS) SetAttributes(_ context.Context, _ string, item vfs.VFSItem) error {
	r.attributes = item
	return nil
}

func (r *encodedRemoteApplyResourceVFS) EncodeCommandListANSI(text []byte) ([]byte, error) {
	r.called = true
	return append([]byte("encoded:"), text...), nil
}

func (r *remoteApplyResourceVFS) Remove(ctx context.Context, path string) error {
	r.removed = append(r.removed, path)
	return r.OSVFS.Remove(ctx, path)
}

func (r *remoteApplyResourceVFS) Clone() vfs.VFS { return r }

func TestEncodeApplyCommandListPortableFormats(t *testing.T) {
	spec := ApplyCommandListFileSpec{Entries: []string{"a", "b c"}, QuoteEntries: true, Encoding: ApplyCommandListUTF8}
	if got := string(encodeApplyCommandList(spec, vfs.CommandDialectPOSIX)); got != "\"a\"\n\"b c\"\n" {
		t.Fatalf("UTF-8 list = %q", got)
	}
	if got := string(encodeApplyCommandList(spec, vfs.CommandDialectCmd)); got != "\"a\"\r\n\"b c\"\r\n" {
		t.Fatalf("cmd list = %q", got)
	}
	spec.Encoding = ApplyCommandListUTF8BOM
	if got := encodeApplyCommandList(spec, vfs.CommandDialectPOSIX); !bytes.HasPrefix(got, []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("UTF-8 BOM list = %x", got[:3])
	}
	spec.Encoding = ApplyCommandListUTF16LE
	got := encodeApplyCommandList(spec, vfs.CommandDialectPOSIX)
	if len(got) < 4 || binary.LittleEndian.Uint16(got[:2]) != 0xfeff || binary.LittleEndian.Uint16(got[2:4]) != '"' {
		t.Fatalf("UTF-16 list prefix = %x", got[:4])
	}
}

func TestMaterializeLocalApplyCommandResource(t *testing.T) {
	osvfs := vfs.NewOSVFS(t.TempDir())
	paths, cleanup, err := materializeApplyCommandResources(t.Context(), osvfs, osvfs.GetPath(), vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
		ID: 3, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"one"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if paths[3] == "" || cleanup == nil {
		t.Fatalf("paths=%v cleanup=%v", paths, cleanup != nil)
	}
	cleanup(false)
}

func TestMaterializeRemoteApplyCommandResourceUsesTargetVFS(t *testing.T) {
	dir := t.TempDir()
	target := &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)}
	paths, cleanup, err := materializeApplyCommandResources(t.Context(), target, dir, vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
		ID: 1, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"remote name"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := paths[1]
	if filepath.Dir(path) != dir {
		t.Fatalf("remote resource path = %q, want directory %q", path, dir)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "remote name\n" {
		t.Fatalf("remote resource data=%q err=%v", data, err)
	}
	cleanup(false)
	if len(target.removed) != 1 || target.removed[0] != path {
		t.Fatalf("removed = %v", target.removed)
	}
}

func TestMaterializeRemoteApplyCommandResourceUsesPrivateCreator(t *testing.T) {
	dir := t.TempDir()
	target := &privateCommandFileApplyResourceVFS{remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)}}
	paths, cleanup, err := materializeApplyCommandResources(t.Context(), target, dir, vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
		ID: 2, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"private"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := paths[2]
	if !target.called {
		t.Fatal("private command-file creator was not used")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); runtime.GOOS != "windows" && got != 0o600 {
		t.Fatalf("resource mode = %#o, want 0600", got)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "private\n" {
		t.Fatalf("private resource data=%q err=%v", data, err)
	}
	cleanup(false)
}

func TestMaterializeApplyResourceRejectsUnknownDialect(t *testing.T) {
	target := &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(t.TempDir())}
	_, _, err := materializeApplyCommandResources(t.Context(), target, target.GetPath(), vfs.CommandDialectUnknown, []ApplyCommandResourceRequest{{
		ID: 1, Kind: ApplyCommandListFileResource,
	}})
	if err == nil {
		t.Fatal("unknown command dialect accepted for a shell-quoted resource")
	}
}

func TestEncodeApplyCommandListUsesTargetPanelCodepage(t *testing.T) {
	target := &encodedRemoteApplyResourceVFS{remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(t.TempDir())}}
	spec := ApplyCommandListFileSpec{Entries: []string{"one"}, Encoding: ApplyCommandListANSI}
	got, err := encodeApplyCommandListForTarget(spec, vfs.CommandDialectPOSIX, target)
	if err != nil {
		t.Fatal(err)
	}
	if !target.called || string(got) != "encoded:one\n" {
		t.Fatalf("called=%v data=%q", target.called, got)
	}
}

func TestEncodeApplyCommandListRejectsUnrepresentableNames(t *testing.T) {
	target := vfs.NewOSVFS(t.TempDir())
	tests := []ApplyCommandListFileSpec{
		{Entries: []string{"one\ntwo"}, Encoding: ApplyCommandListUTF8},
		{Entries: []string{string([]byte{'b', 'a', 'd', 0xff})}, Encoding: ApplyCommandListUTF8BOM},
		{Entries: []string{string([]byte{'b', 'a', 'd', 0xff})}, Encoding: ApplyCommandListUTF16LE},
	}
	for _, spec := range tests {
		if _, err := encodeApplyCommandListForTarget(spec, vfs.CommandDialectPOSIX, target); err == nil {
			t.Fatalf("unrepresentable entries were accepted: %#v", spec.Entries)
		}
	}
}

func TestRemoteApplyCommandResourceRequestsOnlyPrivateMode(t *testing.T) {
	dir := t.TempDir()
	target := &capturingAttributesApplyResourceVFS{remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)}}
	_, cleanup, err := materializeApplyCommandResources(t.Context(), target, dir, vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
		ID: 7, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"private"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup(false)
	if target.attributes.UnixMode != 0o600 || target.attributes.Uid != -1 || target.attributes.Gid != -1 {
		t.Fatalf("attributes = %+v", target.attributes)
	}
	if !target.attributes.ATime.IsZero() || !target.attributes.MTime.IsZero() {
		t.Fatalf("resource chmod unexpectedly requested timestamps: %+v", target.attributes)
	}
}

func TestActiveApplyCommandResourceIsFlushed(t *testing.T) {
	osvfs := vfs.NewOSVFS(t.TempDir())
	paths, release, err := materializeApplyCommandResources(t.Context(), osvfs, osvfs.GetPath(), vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
		ID: 8, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"active"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	path := paths[8]
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	cleanupAllApplyCommandResources()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("active list file still exists: %v", err)
	}
	release(false)
}

func TestApplyCommandShutdownResourceCleanupIsBounded(t *testing.T) {
	dir := t.TempDir()
	target := &blockingRemoveApplyResourceVFS{
		remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)},
		entered:                make(chan struct{}),
		unblock:                make(chan struct{}),
		finished:               make(chan struct{}),
	}
	_, release, err := materializeApplyCommandResources(t.Context(), target, dir, vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
		ID: 9, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"blocked"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if cleanupAllApplyCommandResourcesWithin(25 * time.Millisecond) {
		t.Fatal("blocking remote cleanup unexpectedly completed")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bounded cleanup took %v", elapsed)
	}
	select {
	case <-target.entered:
	default:
		t.Fatal("remote cleanup was not attempted")
	}
	close(target.unblock)
	release(false)
	<-target.finished
}

func TestApplyCommandResourceRemovalsStartConcurrently(t *testing.T) {
	dir := t.TempDir()
	target := &multiBlockingRemoveApplyResourceVFS{
		remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)},
		entered:                make(chan struct{}, 3),
		unblock:                make(chan struct{}),
	}
	requests := make([]ApplyCommandResourceRequest, 3)
	for i := range requests {
		requests[i] = ApplyCommandResourceRequest{
			ID: i, Kind: ApplyCommandListFileResource,
			ListFile: ApplyCommandListFileSpec{Entries: []string{"blocked"}},
		}
	}
	_, release, err := materializeApplyCommandResources(t.Context(), target, dir, vfs.CommandDialectPOSIX, requests)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		release(false)
		close(done)
	}()
	for i := 0; i < len(requests); i++ {
		select {
		case <-target.entered:
		case <-time.After(time.Second):
			t.Fatalf("only %d of %d remote removals started concurrently", i, len(requests))
		}
	}
	close(target.unblock)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("parallel remote cleanup did not finish")
	}
}

func TestApplyCommandResourceFailureCleanupHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	blocking := &blockingRemoveApplyResourceVFS{
		remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)},
		entered:                make(chan struct{}),
		unblock:                make(chan struct{}),
		finished:               make(chan struct{}),
	}
	target := &blockingPermissionFailingApplyResourceVFS{blockingRemoveApplyResourceVFS: blocking}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, _, err := materializeApplyCommandResources(ctx, target, dir, vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
			ID: 10, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"blocked"}},
		}})
		result <- err
	}()
	select {
	case <-target.entered:
	case <-time.After(time.Second):
		t.Fatal("failed resource removal was not attempted")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("permission failure was ignored")
		}
	case <-time.After(time.Second):
		t.Fatal("failure cleanup ignored cancellation")
	}
	close(target.unblock)
	select {
	case <-target.finished:
	case <-time.After(time.Second):
		t.Fatal("failed resource removal did not finish after unblocking")
	}
}

func TestRemoteApplyCommandMaterializationHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	target := &blockingCreateApplyResourceVFS{
		remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)},
		entered:                make(chan struct{}),
		unblock:                make(chan struct{}),
		once:                   &sync.Once{},
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, _, err := materializeApplyCommandResources(ctx, target, dir, vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
			ID: 11, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"blocked"}},
		}})
		result <- err
	}()
	select {
	case <-target.entered:
	case <-time.After(time.Second):
		t.Fatal("remote resource creation did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("materialization error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("remote resource creation ignored cancellation")
	}
	if cleanupAllApplyCommandResourcesWithin(25 * time.Millisecond) {
		t.Fatal("shutdown cleanup lost track of the blocked remote materialization")
	}

	// The retained clone remains alive until the context-free transport call
	// returns, after which an abandoned resource is removed automatically.
	close(target.unblock)
	deadline := time.Now().Add(time.Second)
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("abandoned remote list file was not removed: %v", entries)
		}
		time.Sleep(time.Millisecond)
	}
	if !cleanupAllApplyCommandResourcesWithin(time.Second) {
		t.Fatal("remote materialization remained registered after it drained")
	}
}

func TestRemoteApplyCommandMaterializationShieldsSameInstanceClone(t *testing.T) {
	dir := t.TempDir()
	base := &blockingCreateApplyResourceVFS{
		remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)},
		entered:                make(chan struct{}),
		unblock:                make(chan struct{}),
		once:                   &sync.Once{},
	}
	target := &sameInstanceBlockingCreateApplyResourceVFS{blockingCreateApplyResourceVFS: base}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, _, err := materializeApplyCommandResources(ctx, target, dir, vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
			ID: 12, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"blocked"}},
		}})
		result <- err
	}()
	select {
	case <-target.entered:
	case <-time.After(time.Second):
		t.Fatal("same-instance remote resource creation did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("materialization error = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("same-instance remote resource creation ignored cancellation")
	}
	if cleanupAllApplyCommandResourcesWithin(25 * time.Millisecond) {
		t.Fatal("shutdown cleanup lost same-instance remote work")
	}
	close(target.unblock)
	if !cleanupAllApplyCommandResourcesWithin(time.Second) {
		t.Fatal("same-instance remote work did not drain")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("same-instance abandoned list file was not removed: %v", entries)
	}
}

func TestRemoteApplyCommandResourceRequiresPrivatePermissions(t *testing.T) {
	dir := t.TempDir()
	target := &permissionFailingApplyResourceVFS{remoteApplyResourceVFS: &remoteApplyResourceVFS{OSVFS: vfs.NewOSVFS(dir)}}
	_, _, err := materializeApplyCommandResources(t.Context(), target, dir, vfs.CommandDialectPOSIX, []ApplyCommandResourceRequest{{
		ID: 1, Kind: ApplyCommandListFileResource, ListFile: ApplyCommandListFileSpec{Entries: []string{"secret"}},
	}})
	if err == nil {
		t.Fatal("permission failure was ignored")
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed private resource was not removed: %v", entries)
	}
}
