package dockerfs

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Where the connection settings come from. The plugin stores no credentials of
// its own: it reads the same places the docker CLI does, so a TLS key stays in
// the files the user already protects, and nothing secret is typed into f4 or
// written to its settings.
//
// Order, as in the CLI: DOCKER_CONTEXT, then DOCKER_HOST, then the current
// context of the CLI configuration (`docker context use`), then the local
// daemon.

// endpoint is one resolved daemon address with its TLS settings.
type endpoint struct {
	host string
	// tls is nil for a connection without TLS.
	tls *tls.Config
}

// maxConfigBytes bounds every file read from the docker configuration.
const maxConfigBytes = 1 << 20

// dockerConfigDir is DOCKER_CONFIG or ~/.docker.
func dockerConfigDir(getenv func(string) string) string {
	if dir := getenv("DOCKER_CONFIG"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".docker")
}

func readSmall(name string) ([]byte, error) {
	f, err := os.Open(name) // #nosec G304 -- a file of the user's own docker configuration
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxConfigBytes {
		return nil, fmt.Errorf("%s: too large for a docker configuration file", name)
	}
	return data, nil
}

// resolveEndpoint picks the daemon to talk to.
func resolveEndpoint(getenv func(string) string) (endpoint, error) {
	cfgDir := dockerConfigDir(getenv)
	name := strings.TrimSpace(getenv("DOCKER_CONTEXT"))
	if name == "" && strings.TrimSpace(getenv("DOCKER_HOST")) == "" {
		name = currentContext(cfgDir)
	}
	if name != "" && name != "default" {
		return contextEndpoint(cfgDir, name)
	}
	host := strings.TrimSpace(getenv("DOCKER_HOST"))
	if host == "" {
		if runtime.GOOS == "windows" {
			host = "npipe:////./pipe/docker_engine"
		} else {
			host = "unix://" + defaultSocketPath()
		}
	}
	return envEndpoint(host, getenv, cfgDir)
}

// currentContext is "currentContext" of config.json, empty when there is none.
func currentContext(cfgDir string) string {
	if cfgDir == "" {
		return ""
	}
	data, err := readSmall(filepath.Join(cfgDir, "config.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		CurrentContext string `json:"currentContext"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return ""
	}
	return cfg.CurrentContext
}

// envEndpoint is the DOCKER_HOST flavour: DOCKER_TLS_VERIFY turns TLS on, and
// the certificates are ca.pem, cert.pem and key.pem in DOCKER_CERT_PATH (or the
// configuration directory).
func envEndpoint(host string, getenv func(string) string, cfgDir string) (endpoint, error) {
	ep := endpoint{host: host}
	verify := getenv("DOCKER_TLS_VERIFY") != ""
	if !verify && !strings.HasPrefix(host, "https://") {
		return ep, nil
	}
	dir := getenv("DOCKER_CERT_PATH")
	if dir == "" {
		dir = cfgDir
	}
	cfg, _, err := loadTLS(dir, false)
	if err != nil {
		return endpoint{}, err
	}
	ep.tls = cfg
	return ep, nil
}

// contextEndpoint reads a docker context from the CLI's context store:
// contexts/meta/<sha256 of the name>/meta.json, with the TLS files under
// contexts/tls/<same hash>/docker/.
func contextEndpoint(cfgDir, name string) (endpoint, error) {
	if cfgDir == "" {
		return endpoint{}, fmt.Errorf("docker context %q: no docker configuration directory", name)
	}
	id := contextID(name)
	data, err := readSmall(filepath.Join(cfgDir, "contexts", "meta", id, "meta.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return endpoint{}, fmt.Errorf("docker context %q does not exist", name)
		}
		return endpoint{}, fmt.Errorf("docker context %q: %w", name, err)
	}
	var meta struct {
		Endpoints map[string]struct {
			Host          string `json:"Host"`
			SkipTLSVerify bool   `json:"SkipTLSVerify"`
		} `json:"Endpoints"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return endpoint{}, fmt.Errorf("docker context %q: %w", name, err)
	}
	dk, ok := meta.Endpoints["docker"]
	if !ok || dk.Host == "" {
		return endpoint{}, fmt.Errorf("docker context %q has no docker endpoint", name)
	}
	ep := endpoint{host: dk.Host}
	tlsDir := filepath.Join(cfgDir, "contexts", "tls", id, "docker")
	cfg, found, err := loadTLS(tlsDir, dk.SkipTLSVerify)
	if err != nil {
		return endpoint{}, fmt.Errorf("docker context %q: %w", name, err)
	}
	// Like the CLI: a context uses TLS when it has TLS files or asks to skip
	// verification; otherwise the address is used as it is.
	if found || dk.SkipTLSVerify {
		ep.tls = cfg
	}
	return ep, nil
}

// loadTLS builds a TLS configuration from ca.pem (trusted roots; the system
// roots when absent) and cert.pem with key.pem (the client certificate).
// found tells whether any of those files existed.
func loadTLS(dir string, skipVerify bool) (cfg *tls.Config, found bool, err error) {
	cfg = &tls.Config{MinVersion: tls.VersionTLS12}
	if skipVerify {
		cfg.InsecureSkipVerify = true // #nosec G402 -- explicit, from the user's docker settings
	}
	if dir == "" {
		return cfg, false, nil
	}
	if ca, rerr := readSmall(filepath.Join(dir, "ca.pem")); rerr == nil {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, false, fmt.Errorf("%s: no certificate in ca.pem", dir)
		}
		cfg.RootCAs = pool
		found = true
	}
	certFile, keyFile := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	_, certErr := os.Stat(certFile)
	_, keyErr := os.Stat(keyFile)
	if certErr == nil && keyErr == nil {
		pair, lerr := tls.LoadX509KeyPair(certFile, keyFile)
		if lerr != nil {
			return nil, false, fmt.Errorf("%s: client certificate: %w", dir, lerr)
		}
		cfg.Certificates = []tls.Certificate{pair}
		found = true
	}
	return cfg, found, nil
}

// contextID is the name of a context's directories in the store: the SHA-256 of
// its name in hex.
func contextID(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}
