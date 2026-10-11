package k8sfs

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/unxed/f4/vfs"
	"golang.org/x/net/websocket"
)

// fakeCluster is an API server with two namespaces, one pod of two containers
// and a tiny file system behind exec.
func fakeCluster(t *testing.T) *restClient {
	t.Helper()
	cli, _ := fakeClusterState(t)
	return cli
}

// clusterState records what the fake container was asked to change.
type clusterState struct {
	mu    sync.Mutex
	ran   [][]string
	files map[string]string
	dirs  map[string]bool
}

func (s *clusterState) file(p string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.files[p]
	return c, ok
}

func (s *clusterState) commands() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]string(nil), s.ran...)
}

// change carries out the writing commands the panel sends; ok is false for one
// it does not know, or one that should fail.
func (s *clusterState) change(cmd []string) (handled, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(cmd) == 0 {
		return false, false
	}
	switch {
	case cmd[0] == "mkdir" && len(cmd) == 3:
		s.dirs[cmd[2]] = true
		return true, true
	case cmd[0] == "rm" && len(cmd) == 4:
		if cmd[3] == "/fail" {
			return true, false
		}
		delete(s.files, cmd[3])
		return true, true
	case cmd[0] == "mv" && len(cmd) == 4:
		s.files[cmd[3]] = s.files[cmd[2]]
		delete(s.files, cmd[2])
		return true, true
	case cmd[0] == "sh" && len(cmd) == 6 && cmd[1] == "-c":
		data, err := base64.StdEncoding.DecodeString(cmd[4])
		if err != nil {
			return true, false
		}
		if strings.Contains(cmd[2], ">>") {
			s.files[cmd[5]] += string(data)
		} else {
			s.files[cmd[5]] = string(data)
		}
		return true, true
	}
	return false, false
}

func fakeClusterState(t *testing.T) (*restClient, *clusterState) {
	t.Helper()
	state := &clusterState{files: map[string]string{}, dirs: map[string]bool{}}
	const token = "s3cret"
	mux := http.NewServeMux()
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "forbidden: no token"})
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/v1/namespaces", authed(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[{"metadata":{"name":"default"}},{"metadata":{"name":"kube-system"}}]}`)
	}))
	mux.HandleFunc("/api/v1/namespaces/default/pods", authed(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[{"metadata":{"name":"web","creationTimestamp":"2026-01-02T03:04:05Z"},`+
			`"spec":{"containers":[{"name":"app"},{"name":"sidecar"}]},"status":{"phase":"Running"}}]}`)
	}))
	mux.HandleFunc("/api/v1/namespaces/nowhere/pods", authed(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"namespace not found"}`)
	}))
	commands := map[string]string{
		"ls -1ApL -- /":     "bin/\netc/\nhello.txt\nlnk/\n",
		"ls -1ApL -- /etc":  "hostname\n",
		"cat -- /hello.txt": "hello\n",
		"stat -c %F|%s|%Y|%a|%n -- /bin /etc /hello.txt /lnk": "directory|4096|1700000000|755|/bin\n" +
			"directory|4096|1700000000|755|/etc\nregular file|6|1700000000|644|/hello.txt\nsymbolic link|3|1700000000|777|/lnk\n",
		"stat -c %F|%s|%Y|%a|%n -- /etc":          "directory|4096|1700000000|755|/etc\n",
		"stat -c %F|%s|%Y|%a|%n -- /etc/hostname": "regular file|4|1700000001|600|/etc/hostname\n",
		"stat -c %F|%s|%Y|%a|%n -- /hello.txt":    "regular file|6|1700000000|644|/hello.txt\n",
		"stat -c %F|%s|%Y|%a|%n -- /lnk":          "symbolic link|3|1700000000|777|/lnk\n",
		"test -d /lnk":                            "",
	}
	exec := websocket.Server{
		Handshake: func(c *websocket.Config, r *http.Request) error {
			c.Protocol = []string{execSubprotocol}
			return nil
		},
		Handler: func(ws *websocket.Conn) {
			req := ws.Request()
			args := req.URL.Query()["command"]
			state.mu.Lock()
			state.ran = append(state.ran, args)
			state.mu.Unlock()
			if handled, fine := state.change(args); handled {
				if fine {
					_ = websocket.Message.Send(ws, append([]byte{3}, `{"status":"Success"}`...))
				} else {
					_ = websocket.Message.Send(ws, append([]byte{2}, "denied"...))
					_ = websocket.Message.Send(ws, append([]byte{3}, `{"status":"Failure","message":"exit code 1"}`...))
				}
				return
			}
			cmd := strings.Join(args, " ")
			out, ok := commands[cmd]
			if !ok {
				_ = websocket.Message.Send(ws, append([]byte{2}, "no such file"...))
				_ = websocket.Message.Send(ws, append([]byte{3}, `{"status":"Failure","message":"command terminated with exit code 1"}`...))
				return
			}
			if out != "" {
				_ = websocket.Message.Send(ws, append([]byte{1}, out...))
			}
			_ = websocket.Message.Send(ws, append([]byte{3}, `{"status":"Success"}`...))
		},
	}
	mux.Handle("/api/v1/namespaces/default/pods/web/exec", authed(exec.ServeHTTP))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return newRESTClient(&apiEndpoint{server: srv.URL, token: token}), state
}

func listAll(t *testing.T, v *k8sVFS, p string) map[string]vfs.VFSItem {
	t.Helper()
	got := map[string]vfs.VFSItem{}
	if err := v.ReadDir(context.Background(), p, func(items []vfs.VFSItem) {
		for _, it := range items {
			got[it.Name] = it
		}
	}); err != nil {
		t.Fatalf("ReadDir(%q): %v", p, err)
	}
	return got
}

func TestK8sVFSBrowsesCluster(t *testing.T) {
	cli := fakeCluster(t)
	v := newK8sVFS(func() (*restClient, error) { return cli, nil })
	defer func() { _ = v.Close() }()

	if ns := listAll(t, v, "/"); len(ns) != 2 || !ns["default"].IsDir || !ns["kube-system"].IsDir {
		t.Fatalf("namespaces: %v", ns)
	}
	pods := listAll(t, v, "/default")
	if len(pods) != 1 || !pods["web"].IsDir || pods["web"].MTime.Year() != 2026 {
		t.Fatalf("pods: %+v", pods)
	}
	if cs := listAll(t, v, "/default/web"); len(cs) != 2 || !cs["app"].IsDir || !cs["sidecar"].IsDir {
		t.Fatalf("containers: %v", cs)
	}

	root := listAll(t, v, "/default/web/app")
	if len(root) != 4 || !root["bin"].IsDir || !root["etc"].IsDir || !root["lnk"].IsDir {
		t.Fatalf("container root: %v", root)
	}
	if hello := root["hello.txt"]; hello.IsDir || hello.Size != 6 || !hello.SizeKnown {
		t.Fatalf("hello.txt: %+v", hello)
	}
	if !root["lnk"].IsSymlink {
		t.Fatalf("lnk should be a symlink to a folder: %+v", root["lnk"])
	}
	etc := listAll(t, v, "/default/web/app/etc")
	if it := etc["hostname"]; it.Size != 4 || it.UnixMode != 0o600 {
		t.Fatalf("hostname: %+v", it)
	}
}

func TestK8sVFSStatOpenAndPaths(t *testing.T) {
	cli := fakeCluster(t)
	v := newK8sVFS(func() (*restClient, error) { return cli, nil })
	defer func() { _ = v.Close() }()
	ctx := context.Background()

	if err := v.SetPath("/default/web/app"); err != nil {
		t.Fatal(err)
	}
	if v.GetPath() != "k8s:///default/web/app" || v.IsAtRoot() {
		t.Fatalf("path %q", v.GetPath())
	}
	if it, err := v.Stat(ctx, "hello.txt"); err != nil || it.IsDir || it.Size != 6 {
		t.Fatalf("Stat(hello.txt) = %+v, %v", it, err)
	}
	if it, err := v.Stat(ctx, "lnk"); err != nil || !it.IsDir || !it.IsSymlink {
		t.Fatalf("Stat(lnk) = %+v, %v", it, err)
	}
	for _, p := range []string{"/", "/default", "/default/web", "/default/web/app", "/default/web/sidecar"} {
		if it, err := v.Stat(ctx, p); err != nil || !it.IsDir {
			t.Fatalf("Stat(%q) = %+v, %v", p, it, err)
		}
	}
	for _, p := range []string{"/nope", "/default/ghost", "/default/web/ghost", "/default/web/app/missing"} {
		if _, err := v.Stat(ctx, p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Stat(%q): %v", p, err)
		}
	}
	if err := v.SetPath("/default/web/app/hello.txt"); !errors.Is(err, errNotADirectory) {
		t.Fatalf("SetPath on a file: %v", err)
	}

	f, err := v.Open(ctx, "/default/web/app/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, f.Size())
	if _, err := f.ReadAt(ctx, data, 0); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	_ = f.Close()
	if string(data) != "hello\n" {
		t.Fatalf("Open read %q", data)
	}
	if _, err := v.Open(ctx, "/default/web/app/lnk"); !errors.Is(err, errIsADirectory) {
		t.Fatalf("Open on a folder link: %v", err)
	}
	if _, err := v.Open(ctx, "/default/web"); !errors.Is(err, errIsADirectory) {
		t.Fatalf("Open on a pod: %v", err)
	}
	if _, err := v.Open(ctx, "/default/web/app/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open on a missing file: %v", err)
	}
	if got := v.PanelTitle("/default/web/app/etc"); got != "Kubernetes:default/web/app/etc" || v.PanelTitle("/") != "Kubernetes" {
		t.Fatalf("PanelTitle = %q", got)
	}
	if v.Clone().(*k8sVFS).GetPath() != "k8s:///default/web/app" {
		t.Fatal("clone lost the path")
	}
}

func TestK8sVFSErrorsAndReadOnly(t *testing.T) {
	cli := fakeCluster(t)
	v := newK8sVFS(func() (*restClient, error) { return cli, nil })
	ctx := context.Background()

	if err := v.ReadDir(ctx, "/nowhere", func([]vfs.VFSItem) {}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing namespace: %v", err)
	}
	if err := v.ReadDir(ctx, "/default/web/app/gone", func([]vfs.VFSItem) {}); err == nil {
		t.Fatal("listing a missing folder should fail")
	}

	_, createErr := v.Create(ctx, "/x")
	for i, err := range []error{v.MkDir(ctx, "/x"), v.Remove(ctx, "/x"), v.Rename(ctx, "/x", "/y"), v.SetAttributes(ctx, "/x", vfs.VFSItem{}), createErr} {
		if !errors.Is(err, os.ErrPermission) || err.Error() == "" {
			t.Errorf("mutation %d: %v", i, err)
		}
	}

	forbidden := newRESTClient(&apiEndpoint{server: cli.ep.server, token: "wrong"})
	if _, err := forbidden.listNamespaces(ctx); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("a wrong token: %v", err)
	}
	dead := newRESTClient(&apiEndpoint{server: "http://127.0.0.1:1"})
	if _, err := dead.listNamespaces(ctx); err == nil || !strings.Contains(err.Error(), "Kubernetes") {
		t.Fatalf("an unreachable server: %v", err)
	}
	if err := dead.exec(ctx, "a", "b", "c", []string{"ls"}, io.Discard); err == nil {
		t.Fatal("exec on an unreachable server should fail")
	}

	broken := newK8sVFS(func() (*restClient, error) { return nil, errNoKubeconfig })
	if err := broken.ReadDir(ctx, "/", nil); !errors.Is(err, errNoKubeconfig) {
		t.Fatalf("no kubeconfig: %v", err)
	}
	_ = v.Close()
	if _, err := v.clientFor(); err == nil {
		t.Fatal("a closed panel should not reconnect")
	}
}

func TestExecReportsAFailingCommand(t *testing.T) {
	cli := fakeCluster(t)
	var out strings.Builder
	err := cli.exec(context.Background(), "default", "web", "app", []string{"cat", "--", "/nope"}, &out)
	if !errors.Is(err, errExecFailed) || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("a failing command: %v", err)
	}
}

const sampleKubeconfig = `
apiVersion: v1
current-context: dev
contexts:
- name: dev
  context: {cluster: c1, user: u1}
clusters:
- name: c1
  cluster: {server: "https://k8s.example:6443/", insecure-skip-tls-verify: true}
users:
- name: u1
  user: {token: abc}
`

func TestParseEndpoint(t *testing.T) {
	ep, err := parseEndpoint([]byte(sampleKubeconfig), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ep.server != "https://k8s.example:6443" || ep.token != "abc" || !ep.tls.InsecureSkipVerify {
		t.Fatalf("endpoint: %+v", ep)
	}

	cases := map[string]string{
		"auth provider":        strings.Replace(sampleKubeconfig, "{token: abc}", "{auth-provider: {name: gcp}}", 1),
		"exec without command": strings.Replace(sampleKubeconfig, "{token: abc}", "{exec: {apiVersion: v1}}", 1),
		"no context":           strings.Replace(sampleKubeconfig, "current-context: dev", "current-context: zz", 1),
		"no cluster":           strings.Replace(sampleKubeconfig, "cluster: c1,", "cluster: zz,", 1),
		"bad server":           strings.Replace(sampleKubeconfig, "https://k8s.example:6443/", "nohost", 1),
		"bad CA data":          strings.Replace(sampleKubeconfig, "insecure-skip-tls-verify: true", "certificate-authority-data: '!!'", 1),
		"not yaml":             "{{{",
	}
	for name, cfg := range cases {
		if _, err := parseEndpoint([]byte(cfg), t.TempDir()); !errors.Is(err, errUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestExecCredential(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper here is a shell script")
	}
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper.sh")
	script := "#!/bin/sh\necho \"$KUBERNETES_EXEC_INFO\" | grep -q ExecCredential || exit 3\n" +
		"echo \"{\\\"status\\\":{\\\"token\\\":\\\"$TOKEN_FROM_ENV-$1\\\"}}\"\n"
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil { // #nosec G306 -- a test helper that must be executable
		t.Fatal(err)
	}
	cfg := strings.Replace(sampleKubeconfig, "{token: abc}",
		"{exec: {apiVersion: client.authentication.k8s.io/v1, command: ./helper.sh, args: [arg1], env: [{name: TOKEN_FROM_ENV, value: envtok}]}}", 1)
	ep, err := parseEndpoint([]byte(cfg), dir)
	if err != nil || ep.exec == nil || ep.token != "" {
		t.Fatalf("parse: %+v, %v", ep, err)
	}
	if err := ep.resolveExec(context.Background()); err != nil || ep.token != "envtok-arg1" {
		t.Fatalf("resolveExec: token %q, %v", ep.token, err)
	}

	cfgPath := filepath.Join(dir, "config")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if ep, err := loadEndpoint(cfgPath); err != nil || ep.token != "envtok-arg1" {
		t.Fatalf("loadEndpoint: %+v, %v", ep, err)
	}

	for name, script := range map[string]string{
		"exits nonzero":   "#!/bin/sh\necho boom >&2\nexit 2\n",
		"prints garbage":  "#!/bin/sh\necho nope\n",
		"prints nothing":  "#!/bin/sh\necho '{\"status\":{}}'\n",
		"bad certificate": "#!/bin/sh\necho '{\"status\":{\"clientCertificateData\":\"x\",\"clientKeyData\":\"y\"}}'\n",
	} {
		if err := os.WriteFile(helper, []byte(script), 0o700); err != nil { // #nosec G306 -- a test helper that must be executable
			t.Fatal(err)
		}
		bad, err := parseEndpoint([]byte(cfg), dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := bad.resolveExec(context.Background()); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if err := (&apiEndpoint{}).resolveExec(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLoadEndpointAndPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if _, err := loadEndpoint(cfgPath); !errors.Is(err, errNoKubeconfig) {
		t.Fatalf("a missing file: %v", err)
	}
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("filetoken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := strings.Replace(sampleKubeconfig, "{token: abc}", "{tokenFile: token}", 1)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	ep, err := loadEndpoint(cfgPath)
	if err != nil || ep.token != "filetoken" {
		t.Fatalf("loadEndpoint: %+v, %v", ep, err)
	}

	t.Setenv("KUBECONFIG", cfgPath+string(os.PathListSeparator)+filepath.Join(dir, "other"))
	if p, err := kubeconfigPath(); err != nil || p != cfgPath {
		t.Fatalf("kubeconfigPath = %q, %v", p, err)
	}
	t.Setenv("KUBECONFIG", "")
	if p, err := kubeconfigPath(); err != nil || !strings.HasSuffix(p, filepath.Join(".kube", "config")) {
		t.Fatalf("default kubeconfigPath = %q, %v", p, err)
	}
	t.Setenv("KUBECONFIG", cfgPath)
	if cli, err := openFromKubeconfig(); err != nil || cli == nil {
		t.Fatalf("openFromKubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", filepath.Join(dir, "missing"))
	if _, err := openFromKubeconfig(); !errors.Is(err, errNoKubeconfig) {
		t.Fatalf("openFromKubeconfig without a file: %v", err)
	}
}

func TestPluginAndHelpers(t *testing.T) {
	p := NewPlugin()
	if p.GetName() != "Kubernetes" || p.Close() != nil || p.Init(nil) == nil {
		t.Fatal("plugin identity")
	}
	loc := parseLocation("/default/web/app/etc/passwd")
	if loc.ns != "default" || loc.pod != "web" || loc.container != "app" || loc.inner != "/etc/passwd" || loc.depth != 3 {
		t.Fatalf("location %+v", loc)
	}
	if loc := parseLocation("/"); loc.depth != 0 || loc.inner != "/" {
		t.Fatalf("root location %+v", loc)
	}
	if _, ok := parseStatLine("garbage"); ok {
		t.Fatal("garbage is not a stat line")
	}
	if err := statusError([]byte(`{"status":"Success"}`), []string{"ls"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := statusError([]byte(`not json`), []string{"ls"}, ""); err != nil {
		t.Fatal(err)
	}
}

func TestK8sVFSWrites(t *testing.T) {
	cli, state := fakeClusterState(t)
	v := newK8sVFS(func() (*restClient, error) { return cli, nil })
	defer func() { _ = v.Close() }()
	ctx := context.Background()
	base := "/default/web/app"

	upload := func(name string, size int) {
		t.Helper()
		w, err := v.Create(ctx, base+name)
		if err != nil {
			t.Fatal(err)
		}
		payload := strings.Repeat("0123456789abcdef", size/16)
		for off := 0; off < len(payload); off += 5000 { // written in odd pieces
			if _, err := io.WriteString(w, payload[off:min(off+5000, len(payload))]); err != nil {
				t.Fatal(err)
			}
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("a second Close: %v", err)
		}
		if got, ok := state.file(name); !ok || got != payload {
			t.Fatalf("%s: uploaded %d bytes, want %d", name, len(got), len(payload))
		}
	}
	upload("/small", 16)
	upload("/multi", 40<<10) // four chunks
	upload("/exact", 3*uploadChunk)
	upload("/empty", 0)
	// The first command of an upload truncates, the later ones append.
	var redirects []string
	for _, c := range state.commands() {
		if c[0] == "sh" && c[5] == "/multi" {
			if strings.Contains(c[2], ">>") {
				redirects = append(redirects, ">>")
			} else {
				redirects = append(redirects, ">")
			}
		}
	}
	if len(redirects) != 4 || redirects[0] != ">" || redirects[1] != ">>" || redirects[3] != ">>" {
		t.Fatalf("redirects of the chunks: %v", redirects)
	}

	if err := v.MkDir(ctx, base+"/newdir"); err != nil {
		t.Fatal(err)
	}
	if !state.dirs["/newdir"] {
		t.Fatal("mkdir did not reach the container")
	}
	if err := v.Rename(ctx, base+"/small", base+"/renamed"); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.file("/small"); ok {
		t.Fatal("the old name should be gone")
	}
	if _, ok := state.file("/renamed"); !ok {
		t.Fatal("the new name should exist")
	}
	if err := v.Remove(ctx, base+"/renamed"); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.file("/renamed"); ok {
		t.Fatal("rm did not reach the container")
	}
	if err := v.Remove(ctx, base+"/fail"); !errors.Is(err, errExecFailed) || !strings.Contains(err.Error(), "denied") {
		t.Fatalf("a failing rm: %v", err)
	}

	// What it does not do.
	if err := v.Rename(ctx, base+"/multi", "/default/web/sidecar/multi"); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("a rename across containers: %v", err)
	}
	if _, err := v.Create(ctx, base); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Create on a container root: %v", err)
	}
	// A file over the cap is refused while it is written; closing after that
	// with a cancelled context sends nothing and cleans up.
	cctx, cancel := context.WithCancel(ctx)
	w, err := v.Create(cctx, base+"/huge")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(make([]byte, maxUploadSize)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); err == nil || !strings.Contains(err.Error(), "8 MiB") {
		t.Fatalf("writing past the cap: %v", err)
	}
	cancel()
	if err := w.Close(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Close with a cancelled context: %v", err)
	}
}

func TestURIProviderAndURIPaths(t *testing.T) {
	cli := fakeCluster(t)
	p := uriProvider{open: func() (*restClient, error) { return cli, nil }}
	ctx := context.Background()
	if p.Scheme() != "k8s" {
		t.Fatal(p.Scheme())
	}
	got, err := p.OpenURI(ctx, nil, "k8s:///default/web/app/etc")
	if err != nil {
		t.Fatal(err)
	}
	v := got.(*k8sVFS)
	defer func() { _ = v.Close() }()
	if v.GetPath() != "k8s:///default/web/app/etc" || v.plainPath() != "/default/web/app/etc" {
		t.Fatalf("path %q / %q", v.GetPath(), v.plainPath())
	}
	file := v.Join(v.GetPath(), "hostname")
	if file != "k8s:///default/web/app/etc/hostname" || v.Base(file) != "hostname" || v.Dir(file) != "k8s:///default/web/app/etc" {
		t.Fatalf("Join/Base/Dir: %q %q %q", file, v.Base(file), v.Dir(file))
	}
	if v.Join("/a", "b") != "/a/b" || v.Dir("/a/b") != "/a" || v.Join() != "" {
		t.Fatal("plain paths keep their form")
	}
	if !v.IsAbs("k8s:///x") || !v.IsAbs("/x") || v.IsAbs("x") {
		t.Fatal("IsAbs")
	}
	if abs, _ := v.Abs(file); abs != "/default/web/app/etc/hostname" {
		t.Fatalf("Abs(uri) = %q", abs)
	}
	if abs, _ := v.Abs("hostname"); abs != "/default/web/app/etc/hostname" {
		t.Fatalf("Abs(relative) = %q", abs)
	}
	if names := listAll(t, v, v.GetPath()); len(names) != 1 {
		t.Fatalf("ReadDir(uri): %v", names)
	}
	if root, err := p.OpenURI(ctx, nil, "K8S://"); err != nil || root.GetPath() != "k8s:///" {
		t.Fatalf("the root: %v", err)
	}
	if _, err := p.OpenURI(ctx, nil, "k8s:///nowhere"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing namespace: %v", err)
	}
	if _, err := p.OpenURI(ctx, nil, "k8s:///default/web/app/hello.txt"); !errors.Is(err, errNotADirectory) {
		t.Fatalf("a file: %v", err)
	}
	if _, err := p.OpenURI(ctx, nil, "ftp://x"); err == nil {
		t.Fatal("a foreign scheme should be refused")
	}
}
