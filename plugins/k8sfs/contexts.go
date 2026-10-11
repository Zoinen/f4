package k8sfs

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxContexts bounds how many kubeconfig contexts get a drive of their own.
const maxContexts = 64

// maxKubeconfigBytes bounds the kubeconfig read to list its contexts.
const maxKubeconfigBytes = 4 << 20

// listContexts names the contexts of the kubeconfig that point at a cluster and
// a user, sorted. An absent or unreadable file is an empty list.
func listContexts(configPath string) []string {
	f, err := os.Open(configPath) // #nosec G304 -- the user's own kubeconfig
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxKubeconfigBytes+1))
	if err != nil || len(data) > maxKubeconfigBytes {
		return nil
	}
	var cfg kubeconfig
	if yaml.Unmarshal(data, &cfg) != nil {
		return nil
	}
	var names []string
	seen := map[string]bool{}
	for _, c := range cfg.Contexts {
		if c.Name == "" || c.Context.Cluster == "" || seen[c.Name] {
			continue
		}
		seen[c.Name] = true
		names = append(names, c.Name)
	}
	sort.Strings(names)
	if len(names) > maxContexts {
		names = names[:maxContexts]
	}
	return names
}

// contextDriveName is what the drive menu calls the panel of one context.
func contextDriveName(name string) string { return driveName + " (" + name + ")" }

// contextPrefix is the URI head of the panel of one kubeconfig context.
func contextPrefix(name string) string { return uriPrefix + url.PathEscape(name) }

// contextOpener connects to the cluster of one kubeconfig context.
func contextOpener(name string) func() (*restClient, error) {
	return func() (*restClient, error) {
		p, err := kubeconfigPath()
		if err != nil {
			return nil, err
		}
		ep, err := loadEndpointFor(p, name)
		if err != nil {
			return nil, err
		}
		return newRESTClient(ep), nil
	}
}

// newContextVFS is the panel of one kubeconfig context.
func newContextVFS(name string) *k8sVFS {
	v := newK8sVFS(contextOpener(name))
	v.prefix = contextPrefix(name)
	return v
}

// inClusterDir is where a pod gets its service account.
const inClusterDir = "/var/run/secrets/kubernetes.io/serviceaccount"

// inClusterEndpoint is the configuration of code running in a pod: the API
// server from KUBERNETES_SERVICE_HOST/PORT, and the pod's service account token
// and CA from dir. Nothing is typed in or stored.
func inClusterEndpoint(dir string) (*apiEndpoint, error) {
	host, port := os.Getenv("KUBERNETES_SERVICE_HOST"), os.Getenv("KUBERNETES_SERVICE_PORT")
	if host == "" || port == "" {
		return nil, errors.New("k8sfs: not running in a cluster")
	}
	tok, err := os.ReadFile(filepath.Join(dir, "token")) // #nosec G304 -- the pod's own service account
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(filepath.Join(dir, "ca.crt")) // #nosec G304 -- the pod's own service account
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("%w: the service account CA is not PEM", errUnsupported)
	}
	return &apiEndpoint{
		server: "https://" + net.JoinHostPort(host, port),
		token:  strings.TrimSpace(string(tok)),
		tls:    &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: pool},
	}, nil
}
