package k8sfs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const twoContexts = `
apiVersion: v1
current-context: dev
contexts:
- name: prod
  context: {cluster: c2, user: u2}
- name: dev
  context: {cluster: c1, user: u1}
- name: dev
  context: {cluster: c1, user: u1}
- name: broken
  context: {user: u1}
clusters:
- name: c1
  cluster: {server: "https://dev.example:6443", insecure-skip-tls-verify: true}
- name: c2
  cluster: {server: "https://prod.example:6443", insecure-skip-tls-verify: true}
users:
- name: u1
  user: {token: devtok}
- name: u2
  user: {token: prodtok}
`

func writeKubeconfig(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestListContextsAndNamedEndpoint(t *testing.T) {
	p := writeKubeconfig(t, twoContexts)
	if got, want := listContexts(p), []string{"dev", "prod"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("listContexts = %v, want %v", got, want)
	}
	if got := listContexts(filepath.Join(t.TempDir(), "absent")); got != nil {
		t.Fatalf("absent file: %v", got)
	}
	if got := listContexts(writeKubeconfig(t, "{{{")); got != nil {
		t.Fatalf("not yaml: %v", got)
	}
	ep, err := loadEndpointFor(p, "prod")
	if err != nil || ep.server != "https://prod.example:6443" || ep.token != "prodtok" {
		t.Fatalf("prod: %+v, %v", ep, err)
	}
	ep, err = loadEndpointFor(p, "")
	if err != nil || ep.token != "devtok" {
		t.Fatalf("current: %+v, %v", ep, err)
	}
	if _, err := loadEndpointFor(p, "nope"); !errors.Is(err, errUnsupported) || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("unknown context: %v", err)
	}
}

func TestContextPanelURIs(t *testing.T) {
	cli := fakeCluster(t)
	p := uriProvider{
		open:        func() (*restClient, error) { return nil, errors.New("the default cluster must not be used") },
		openContext: func(string) func() (*restClient, error) { return func() (*restClient, error) { return cli, nil } },
	}
	ctx := context.Background()
	got, err := p.OpenURI(ctx, nil, "k8s://my%20ctx/default/web/app/etc")
	if err != nil {
		t.Fatal(err)
	}
	v := got.(*k8sVFS)
	defer func() { _ = v.Close() }()
	if v.GetPath() != "k8s://my%20ctx/default/web/app/etc" || v.plainPath() != "/default/web/app/etc" {
		t.Fatalf("path %q / %q", v.GetPath(), v.plainPath())
	}
	file := v.Join(v.GetPath(), "hostname")
	if file != "k8s://my%20ctx/default/web/app/etc/hostname" || v.Dir(file) != "k8s://my%20ctx/default/web/app/etc" || v.Base(file) != "hostname" {
		t.Fatalf("Join/Dir/Base: %q %q %q", file, v.Dir(file), v.Base(file))
	}
	if !v.IsAbs(file) || !v.IsAbs("/x") || v.IsAbs("k8s:///x") {
		t.Fatal("IsAbs")
	}
	if abs, _ := v.Abs(file); abs != "/default/web/app/etc/hostname" {
		t.Fatalf("Abs = %q", abs)
	}
	if title := v.PanelTitle(v.GetPath()); title != "Kubernetes(my ctx):default/web/app/etc" {
		t.Fatalf("title %q", title)
	}
	if clone := v.Clone().(*k8sVFS); clone.GetPath() != v.GetPath() {
		t.Fatalf("clone path %q", clone.GetPath())
	}
	for _, raw := range []string{"k8s://my%20ctx", "k8s://my%20ctx/"} {
		root, err := p.OpenURI(ctx, nil, raw)
		if err != nil || !root.IsAtRoot() || root.GetPath() != "k8s://my%20ctx/" {
			t.Fatalf("%s: %v", raw, err)
		}
		if root.(*k8sVFS).PanelTitle("/") != "Kubernetes(my ctx)" {
			t.Fatal("root title")
		}
	}
	if _, err := p.OpenURI(ctx, nil, "k8s://%zz/default"); err == nil {
		t.Fatal("a bad escape should be refused")
	}
	if _, err := (uriProvider{open: p.open}).OpenURI(ctx, nil, "k8s://x/default"); err == nil {
		t.Fatal("a context address without context support should be refused")
	}
}

func TestContextOpenerUsesTheKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", writeKubeconfig(t, twoContexts))
	cli, err := contextOpener("prod")()
	if err != nil || cli == nil {
		t.Fatalf("prod: %v", err)
	}
	if _, err := contextOpener("gone")(); err == nil {
		t.Fatal("an unknown context should fail")
	}
	v := newContextVFS("prod")
	if v.GetPath() != "k8s://prod/" {
		t.Fatalf("GetPath = %q", v.GetPath())
	}
	_ = v.Close()
	if contextDriveName("prod") != "Kubernetes (prod)" {
		t.Fatal(contextDriveName("prod"))
	}
}

func TestInClusterEndpoint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if _, err := inClusterEndpoint(dir); err == nil {
		t.Fatal("outside a cluster it should fail")
	}
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.0.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")
	if _, err := inClusterEndpoint(dir); err == nil {
		t.Fatal("a missing token should fail")
	}
	if err := os.WriteFile(filepath.Join(dir, "token"), []byte("sa-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := inClusterEndpoint(dir); err == nil {
		t.Fatal("a missing CA should fail")
	}
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), []byte("not pem"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := inClusterEndpoint(dir); !errors.Is(err, errUnsupported) {
		t.Fatalf("a bad CA: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ca.crt"), selfSignedPEM(t), 0o600); err != nil {
		t.Fatal(err)
	}
	ep, err := inClusterEndpoint(dir)
	if err != nil || ep.server != "https://10.0.0.1:443" || ep.token != "sa-token" || ep.tls.RootCAs == nil {
		t.Fatalf("in-cluster: %+v, %v", ep, err)
	}
}

// selfSignedPEM is a throwaway CA certificate in PEM.
func selfSignedPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "f4 test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}
