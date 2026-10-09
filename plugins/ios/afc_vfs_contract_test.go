package iosfs

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"testing"

	"github.com/unxed/f4/plugins/ios/internal/afcproto"
	"github.com/unxed/f4/vfs"
)

func TestAFCVFSPathIdentityAndReadOnlyContracts(t *testing.T) {
	session := newAFCSession("device", func(context.Context) (io.ReadWriteCloser, error) { return nil, errors.New("unused") })
	v := &AFCVFS{session: session, key: "device", title: "Phone", path: "/"}
	if !v.IsAtRoot() || v.GetPath() != "/" || v.GetTitle() != "ios:device" {
		t.Fatal("unexpected AFC identity")
	}
	if v.PanelTitle("/") != "Phone/" || v.PanelTitle("/Documents") != "Phone/Documents" {
		t.Errorf("panel titles: root=%q child=%q", v.PanelTitle("/"), v.PanelTitle("/Documents"))
	}
	if !v.IsAbs("/x") || v.IsAbs("x") || v.SessionKey() != session || !v.CanReconnect() {
		t.Error("unexpected AFC session/path contract")
	}
	if _, err := v.Abs("/../escape"); err == nil {
		t.Error("Abs accepted a path escaping the domain")
	}
	if got, err := v.Abs("Documents"); err != nil || got != "/Documents" {
		t.Errorf("Abs relative=(%q,%v)", got, err)
	}
	if err := v.SetPathOptimistic("Documents"); err != nil || v.GetPath() != "/Documents" {
		t.Errorf("SetPathOptimistic path=%q err=%v", v.GetPath(), err)
	}
	if err := v.SetPathOptimistic("../escape"); err == nil {
		t.Error("SetPathOptimistic accepted an escaping path")
	}
	if err := v.SetPathOptimistic("/Documents//Library"); err != nil || v.GetPath() != "/Documents/Library" {
		t.Errorf("clean optimistic path=%q err=%v", v.GetPath(), err)
	}
	if v.Join("/Documents", "file") != "/Documents/file" || v.Base("/Documents/file") != "file" || v.Dir("/Documents/file") != "/Documents" {
		t.Error("AFC path helpers returned unexpected values")
	}
	if got := v.GetCapabilities(); got != (vfs.VFSCapabilities{HasServerSideMove: true, HasRandomAccess: true}) {
		t.Errorf("writable capabilities=%#v", got)
	}
	if _, err := v.Search(context.Background(), "/", "x"); !errors.Is(err, ErrSearchUnsupported) {
		t.Errorf("Search error=%v", err)
	}
	if _, err := v.Stat(context.Background(), "/../escape"); err == nil {
		t.Error("Stat accepted an escaping path")
	}
	if got, ok := v.CachedPanelInfo(vfs.PanelInfoRequest{}); ok || !got.Authoritative || len(got.Sections) == 0 {
		t.Errorf("baseline panel info=(%#v,%v)", got, ok)
	}

	clone := v.Clone().(*AFCVFS)
	if clone.GetPath() != "/Documents/Library" || clone.SessionKey() != session {
		t.Error("Clone did not preserve AFC view state")
	}
	if err := clone.Close(); err != nil {
		t.Fatal(err)
	}
	if err := v.Close(); err != nil || v.Close() != nil {
		t.Errorf("idempotent Close returned %v", err)
	}
}

func TestAFCVFSReadOnlyAndMutationPathGuards(t *testing.T) {
	ctx := context.Background()
	readOnly := &AFCVFS{readOnly: true, session: newAFCSession("ro", nil), path: "/"}
	if _, err := readOnly.Create(ctx, "/file"); !errors.Is(err, ErrReadOnlyDomain) {
		t.Errorf("Create read-only=%v", err)
	}
	if err := readOnly.MkDir(ctx, "/dir"); !errors.Is(err, ErrReadOnlyDomain) {
		t.Errorf("MkDir read-only=%v", err)
	}
	if err := readOnly.Remove(ctx, "/file"); !errors.Is(err, ErrReadOnlyDomain) {
		t.Errorf("Remove read-only=%v", err)
	}
	if err := readOnly.Rename(ctx, "/a", "/b"); !errors.Is(err, ErrReadOnlyDomain) {
		t.Errorf("Rename read-only=%v", err)
	}
	if err := readOnly.SetAttributes(ctx, "/file", vfs.VFSItem{}); !errors.Is(err, ErrReadOnlyDomain) {
		t.Errorf("SetAttributes read-only=%v", err)
	}
	if err := readOnly.Close(); err != nil {
		t.Fatal(err)
	}

	writable := &AFCVFS{session: newAFCSession("rw", nil), path: "/"}
	defer func() { _ = writable.Close() }()
	if _, err := writable.Create(ctx, "/"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("Create root=%v", err)
	}
	for _, name := range []string{"MkDir", "Remove", "Rename"} {
		var err error
		switch name {
		case "MkDir":
			err = writable.MkDir(ctx, "/")
		case "Remove":
			err = writable.Remove(ctx, "/")
		case "Rename":
			err = writable.Rename(ctx, "/", "/new")
		}
		if err == nil {
			t.Errorf("%s root unexpectedly succeeded", name)
		}
	}
	if err := writable.SetAttributes(ctx, "/file", vfs.VFSItem{}); !errors.Is(err, errors.ErrUnsupported) {
		t.Errorf("zero-time SetAttributes=%v", err)
	}
	writable.SetVirtualRoot(nil, CapabilityApplications)
	if err := writable.Remove(ctx, "/[Applications]"); !errors.Is(err, ErrSelectorReadOnly) {
		t.Errorf("selector mutation=%v", err)
	}
	if !writable.SessionLost(afcproto.ErrConnectionLost) || writable.SessionLost(errors.New("ordinary")) {
		t.Error("SessionLost classification mismatch")
	}
}
