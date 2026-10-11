package dockerfs

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pki is a throwaway CA with a server and a client certificate.
type pki struct {
	caPEM, serverCert, serverKey, clientCert, clientKey []byte
}

func newPKI(t *testing.T) pki {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "f4 test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	leaf := func(serial int64, usage x509.ExtKeyUsage) (certPEM, keyPEM []byte) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: "f4 test leaf"},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
			KeyUsage:     x509.KeyUsageDigitalSignature,
			ExtKeyUsage:  []x509.ExtKeyUsage{usage},
			IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		keyDER, err := x509.MarshalECPrivateKey(key)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	}
	var p pki
	p.caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	p.serverCert, p.serverKey = leaf(2, x509.ExtKeyUsageServerAuth)
	p.clientCert, p.clientKey = leaf(3, x509.ExtKeyUsageClientAuth)
	return p
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// writeClientFiles puts ca.pem, cert.pem and key.pem in dir.
func (p pki) writeClientFiles(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "ca.pem"), p.caPEM)
	writeFile(t, filepath.Join(dir, "cert.pem"), p.clientCert)
	writeFile(t, filepath.Join(dir, "key.pem"), p.clientKey)
}

// tlsDaemon is a daemon that insists on a client certificate signed by the CA.
func (p pki) tlsDaemon(t *testing.T) *httptest.Server {
	t.Helper()
	pair, err := tls.X509KeyPair(p.serverCert, p.serverKey)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(p.caPEM)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("OK"))
	}))
	srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{pair},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
		MinVersion:   tls.VersionTLS12,
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

func tcpHost(srv *httptest.Server) string {
	return "tcp://" + strings.TrimPrefix(srv.URL, "https://")
}

func ping() func(*client) error {
	return func(c *client) error {
		resp, err := c.do(context.Background(), "GET", "/_ping", nil)
		if err != nil {
			return err
		}
		_ = resp.Body.Close()
		return nil
	}
}

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func dial(t *testing.T, env map[string]string) error {
	t.Helper()
	ep, err := resolveEndpoint(envOf(env))
	if err != nil {
		return err
	}
	c, err := newClientTLS(ep.host, ep.tls)
	if err != nil {
		return err
	}
	defer c.close()
	return ping()(c)
}

// writeContext stores a docker context the way the CLI does.
func writeContext(t *testing.T, cfgDir, name, host string, skip bool, p *pki) {
	t.Helper()
	meta := `{"Name":"` + name + `","Endpoints":{"docker":{"Host":"` + host + `","SkipTLSVerify":` + map[bool]string{true: "true", false: "false"}[skip] + `}}}`
	writeFile(t, filepath.Join(cfgDir, "contexts", "meta", contextID(name), "meta.json"), []byte(meta))
	if p != nil {
		p.writeClientFiles(t, filepath.Join(cfgDir, "contexts", "tls", contextID(name), "docker"))
	}
}

func TestEnvTLS(t *testing.T) {
	p := newPKI(t)
	srv := p.tlsDaemon(t)
	certs := t.TempDir()
	p.writeClientFiles(t, certs)
	env := map[string]string{
		"DOCKER_HOST":       tcpHost(srv),
		"DOCKER_TLS_VERIFY": "1",
		"DOCKER_CERT_PATH":  certs,
		"DOCKER_CONFIG":     t.TempDir(),
	}
	if err := dial(t, env); err != nil {
		t.Fatalf("mutual TLS through DOCKER_CERT_PATH: %v", err)
	}
	// Without the client certificate the daemon refuses.
	bare := t.TempDir()
	writeFile(t, filepath.Join(bare, "ca.pem"), p.caPEM)
	env["DOCKER_CERT_PATH"] = bare
	if err := dial(t, env); err == nil {
		t.Fatal("a daemon that wants a client certificate accepted none")
	}
	// A wrong CA is not trusted.
	other := newPKI(t)
	wrong := t.TempDir()
	other.writeClientFiles(t, wrong)
	env["DOCKER_CERT_PATH"] = wrong
	if err := dial(t, env); err == nil {
		t.Fatal("a server signed by an unknown CA was trusted")
	}
	// A broken ca.pem is reported, not ignored.
	broken := t.TempDir()
	writeFile(t, filepath.Join(broken, "ca.pem"), []byte("not a certificate"))
	env["DOCKER_CERT_PATH"] = broken
	if _, err := resolveEndpoint(envOf(env)); err == nil {
		t.Fatal("a ca.pem without a certificate was accepted")
	}
}

func TestContextStore(t *testing.T) {
	p := newPKI(t)
	srv := p.tlsDaemon(t)
	cfg := t.TempDir()
	writeContext(t, cfg, "remote", tcpHost(srv), false, &p)
	writeFile(t, filepath.Join(cfg, "config.json"), []byte(`{"currentContext":"remote"}`))
	if err := dial(t, map[string]string{"DOCKER_CONFIG": cfg}); err != nil {
		t.Fatalf("the current context: %v", err)
	}
	if err := dial(t, map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_CONTEXT": "remote"}); err != nil {
		t.Fatalf("DOCKER_CONTEXT: %v", err)
	}
	// DOCKER_HOST beats the sticky current context.
	ep, err := resolveEndpoint(envOf(map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_HOST": "tcp://127.0.0.1:1"}))
	if err != nil || ep.host != "tcp://127.0.0.1:1" || ep.tls != nil {
		t.Fatalf("DOCKER_HOST with a current context = %+v, %v", ep, err)
	}
	// DOCKER_CONTEXT beats DOCKER_HOST.
	ep, err = resolveEndpoint(envOf(map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_HOST": "tcp://127.0.0.1:1", "DOCKER_CONTEXT": "remote"}))
	if err != nil || ep.host != tcpHost(srv) || ep.tls == nil {
		t.Fatalf("DOCKER_CONTEXT with DOCKER_HOST = %+v, %v", ep, err)
	}
	// The default context means "no context".
	ep, err = resolveEndpoint(envOf(map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_CONTEXT": "default", "DOCKER_HOST": "tcp://127.0.0.1:2"}))
	if err != nil || ep.host != "tcp://127.0.0.1:2" {
		t.Fatalf("the default context = %+v, %v", ep, err)
	}
	// A context that is not there is an error that names it.
	_, err = resolveEndpoint(envOf(map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_CONTEXT": "nope"}))
	if err == nil || !strings.Contains(err.Error(), `"nope"`) {
		t.Fatalf("missing context error = %v", err)
	}
}

func TestContextWithoutTLS(t *testing.T) {
	cfg := t.TempDir()
	writeContext(t, cfg, "plain", "tcp://10.0.0.1:2375", false, nil)
	ep, err := resolveEndpoint(envOf(map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_CONTEXT": "plain"}))
	if err != nil || ep.tls != nil {
		t.Fatalf("a context without TLS files = %+v, %v", ep, err)
	}
	writeContext(t, cfg, "lax", "tcp://10.0.0.1:2376", true, nil)
	ep, err = resolveEndpoint(envOf(map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_CONTEXT": "lax"}))
	if err != nil || ep.tls == nil || !ep.tls.InsecureSkipVerify {
		t.Fatalf("SkipTLSVerify = %+v, %v", ep, err)
	}
	writeFile(t, filepath.Join(cfg, "contexts", "meta", contextID("bad"), "meta.json"), []byte(`{"Endpoints":{}}`))
	if _, err := resolveEndpoint(envOf(map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_CONTEXT": "bad"})); err == nil {
		t.Fatal("a context without a docker endpoint was accepted")
	}
	writeFile(t, filepath.Join(cfg, "contexts", "meta", contextID("junk"), "meta.json"), []byte(`{`))
	if _, err := resolveEndpoint(envOf(map[string]string{"DOCKER_CONFIG": cfg, "DOCKER_CONTEXT": "junk"})); err == nil {
		t.Fatal("a context with broken JSON was accepted")
	}
}

func TestHTTPSSchemeUsesSystemRoots(t *testing.T) {
	c, err := newClient("https://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(c.base, "https://") {
		t.Fatalf("base = %q", c.base)
	}
	c.close()
	if _, err := newClient("ssh://user@host"); err == nil || !strings.Contains(err.Error(), "ssh://") {
		t.Fatalf("ssh error = %v", err)
	}
}
