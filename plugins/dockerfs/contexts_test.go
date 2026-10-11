package dockerfs

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestListContexts(t *testing.T) {
	cfg := t.TempDir()
	if got := listContexts(cfg); got != nil {
		t.Fatalf("no store: %v", got)
	}
	if got := listContexts(""); got != nil {
		t.Fatalf("no directory: %v", got)
	}
	writeContext(t, cfg, "zeta", "tcp://10.0.0.1:2375", false, nil)
	writeContext(t, cfg, "my ctx", "unix:///run/x.sock", false, nil)
	writeContext(t, cfg, "default", "unix:///run/y.sock", false, nil)
	// Not usable: no docker endpoint, broken JSON, a directory that is not the hash of the name, a stray file.
	writeFile(t, filepath.Join(cfg, "contexts", "meta", contextID("k8s"), "meta.json"), []byte(`{"Name":"k8s","Endpoints":{"kubernetes":{"Host":"x"}}}`))
	writeFile(t, filepath.Join(cfg, "contexts", "meta", contextID("junk"), "meta.json"), []byte(`{`))
	writeFile(t, filepath.Join(cfg, "contexts", "meta", "abc", "meta.json"), []byte(`{"Name":"liar","Endpoints":{"docker":{"Host":"tcp://h:1"}}}`))
	writeFile(t, filepath.Join(cfg, "contexts", "meta", "stray.txt"), []byte("x"))
	want := []string{"my ctx", "zeta"}
	if got := listContexts(cfg); !reflect.DeepEqual(got, want) {
		t.Fatalf("listContexts = %v, want %v", got, want)
	}
}

func TestContextPanelURIs(t *testing.T) {
	cli, _ := fakeDaemon(t, testFS())
	p := uriProvider{
		open:        func() (*client, error) { return nil, errors.New("the default daemon must not be used") },
		openContext: func(string) func() (*client, error) { return func() (*client, error) { return cli, nil } },
	}
	ctx := context.Background()
	got, err := p.OpenURI(ctx, nil, "docker://my%20ctx/web/etc")
	if err != nil {
		t.Fatal(err)
	}
	v := got.(*dockerVFS)
	defer func() { _ = v.Close() }()
	if v.GetPath() != "docker://my%20ctx/web/etc" || v.plainPath() != "/web/etc" {
		t.Fatalf("path %q / %q", v.GetPath(), v.plainPath())
	}
	file := v.Join(v.GetPath(), "hostname")
	if file != "docker://my%20ctx/web/etc/hostname" || v.Dir(file) != "docker://my%20ctx/web/etc" || v.Base(file) != "hostname" {
		t.Fatalf("Join/Dir/Base: %q %q %q", file, v.Dir(file), v.Base(file))
	}
	if !v.IsAbs(file) || !v.IsAbs("/x") || v.IsAbs("docker:///x") {
		t.Fatal("IsAbs")
	}
	if abs, _ := v.Abs(file); abs != "/web/etc/hostname" {
		t.Fatalf("Abs = %q", abs)
	}
	if it, err := v.Stat(ctx, file); err != nil || it.Size != 4 {
		t.Fatalf("Stat = %+v, %v", it, err)
	}
	if title := v.PanelTitle(v.GetPath()); title != "Docker(my ctx):web/etc" {
		t.Fatalf("title %q", title)
	}
	if clone := v.Clone().(*dockerVFS); clone.GetPath() != v.GetPath() {
		t.Fatalf("clone path %q", clone.GetPath())
	}
	// The context's root, written with and without the slash.
	for _, raw := range []string{"docker://my%20ctx", "docker://my%20ctx/"} {
		root, err := p.OpenURI(ctx, nil, raw)
		if err != nil || !root.IsAtRoot() || root.GetPath() != "docker://my%20ctx/" {
			t.Fatalf("%s: %v", raw, err)
		}
	}
	if r, _ := p.OpenURI(ctx, nil, "docker://my%20ctx"); r.(*dockerVFS).PanelTitle("/") != "Docker(my ctx)" {
		t.Fatal("root title")
	}
	if _, err := p.OpenURI(ctx, nil, "docker://%zz/web"); err == nil {
		t.Fatal("a bad escape should be refused")
	}
	if _, err := (uriProvider{open: p.open}).OpenURI(ctx, nil, "docker://x/web"); err == nil {
		t.Fatal("a context address without context support should be refused")
	}
}

func TestContextOpenerUsesTheStore(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("DOCKER_CONFIG", cfg)
	writeContext(t, cfg, "lab", "tcp://127.0.0.1:1", false, nil)
	cli, err := contextOpener("lab")()
	if err != nil {
		t.Fatal(err)
	}
	cli.close()
	if _, err := contextOpener("gone")(); err == nil {
		t.Fatalf("a missing context: %v", err)
	}
	v := newContextVFS("lab")
	if v.GetPath() != "docker://lab/" {
		t.Fatalf("GetPath = %q", v.GetPath())
	}
	_ = v.Close()
	if contextDriveName("lab") != "Docker (lab)" {
		t.Fatal(contextDriveName("lab"))
	}
}
