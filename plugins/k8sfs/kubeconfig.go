package k8sfs

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var (
	errNoKubeconfig = errors.New("k8sfs: no kubeconfig found")
	errUnsupported  = errors.New("k8sfs: unsupported kubeconfig")
)

// kubeconfig is the part of a kubeconfig file that a client needs.
type kubeconfig struct {
	CurrentContext string `yaml:"current-context"`
	Clusters       []struct {
		Name    string `yaml:"name"`
		Cluster struct {
			Server                string `yaml:"server"`
			CertificateAuthority  string `yaml:"certificate-authority"`
			CertificateAuthorityD string `yaml:"certificate-authority-data"`
			InsecureSkipTLSVerify bool   `yaml:"insecure-skip-tls-verify"`
		} `yaml:"cluster"`
	} `yaml:"clusters"`
	Users []struct {
		Name string `yaml:"name"`
		User struct {
			Token                 string         `yaml:"token"`
			TokenFile             string         `yaml:"tokenFile"`
			ClientCertificate     string         `yaml:"client-certificate"`
			ClientCertificateData string         `yaml:"client-certificate-data"`
			ClientKey             string         `yaml:"client-key"`
			ClientKeyData         string         `yaml:"client-key-data"`
			Exec                  *execSpec      `yaml:"exec"`
			AuthProvider          map[string]any `yaml:"auth-provider"`
		} `yaml:"user"`
	} `yaml:"users"`
	Contexts []struct {
		Name    string `yaml:"name"`
		Context struct {
			Cluster string `yaml:"cluster"`
			User    string `yaml:"user"`
		} `yaml:"context"`
	} `yaml:"contexts"`
}

// execSpec is a kubeconfig "exec" credential plugin: a program that prints the
// credentials (the way gke-gcloud-auth-plugin or aws eks get-token do).
type execSpec struct {
	APIVersion string   `yaml:"apiVersion"`
	Command    string   `yaml:"command"`
	Args       []string `yaml:"args"`
	Env        []struct {
		Name  string `yaml:"name"`
		Value string `yaml:"value"`
	} `yaml:"env"`
	InstallHint string `yaml:"installHint"`
}

// apiEndpoint is what a kubeconfig context resolves to.
type apiEndpoint struct {
	server  string // https://host:port
	token   string
	tls     *tls.Config
	exec    *execSpec // credentials to fetch from a helper program, see resolveExec
	baseDir string    // where a relative helper path is resolved from
}

// kubeconfigPath is where the config lives: the first entry of KUBECONFIG, or
// ~/.kube/config.
func kubeconfigPath() (string, error) {
	if env := os.Getenv("KUBECONFIG"); env != "" {
		for _, p := range filepath.SplitList(env) {
			if p != "" {
				return p, nil
			}
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", errNoKubeconfig
	}
	return filepath.Join(home, ".kube", "config"), nil
}

// loadEndpoint reads the kubeconfig and resolves its current context.
// Credentials that need a helper program (exec plugins, cloud auth providers)
// are refused with a message that names the reason rather than sent without
// credentials.
func loadEndpoint(configPath string) (*apiEndpoint, error) {
	return loadEndpointFor(configPath, "")
}

// loadEndpointFor is loadEndpoint for a named context ("" is the current one).
func loadEndpointFor(configPath, contextName string) (*apiEndpoint, error) {
	raw, err := os.ReadFile(configPath) // #nosec G304 -- the user's own kubeconfig
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w (%s)", errNoKubeconfig, configPath)
		}
		return nil, err
	}
	ep, err := parseEndpointFor(raw, filepath.Dir(configPath), contextName)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), execTimeout)
	defer cancel()
	if err := ep.resolveExec(ctx); err != nil {
		return nil, err
	}
	return ep, nil
}

func parseEndpoint(raw []byte, baseDir string) (*apiEndpoint, error) {
	return parseEndpointFor(raw, baseDir, "")
}

// parseEndpointFor resolves the named context of a kubeconfig; an empty name is
// the current context (or the only one).
func parseEndpointFor(raw []byte, baseDir, ctxName string) (*apiEndpoint, error) {
	var cfg kubeconfig
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("%w: %v", errUnsupported, err)
	}
	named := ctxName != ""
	if ctxName == "" {
		ctxName = cfg.CurrentContext
	}
	if ctxName == "" && len(cfg.Contexts) == 1 {
		ctxName = cfg.Contexts[0].Name
	}
	var clusterName, userName string
	for _, c := range cfg.Contexts {
		if c.Name == ctxName {
			clusterName, userName = c.Context.Cluster, c.Context.User
		}
	}
	if clusterName == "" {
		if named {
			return nil, fmt.Errorf("%w: context %q is not defined", errUnsupported, ctxName)
		}
		return nil, fmt.Errorf("%w: no current context", errUnsupported)
	}
	ep := &apiEndpoint{}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	found := false
	for _, c := range cfg.Clusters {
		if c.Name != clusterName {
			continue
		}
		found = true
		u, err := url.Parse(c.Cluster.Server)
		if err != nil || u.Host == "" {
			return nil, fmt.Errorf("%w: bad server %q", errUnsupported, c.Cluster.Server)
		}
		ep.server = strings.TrimRight(c.Cluster.Server, "/")
		tlsCfg.InsecureSkipVerify = c.Cluster.InsecureSkipTLSVerify // #nosec G402 -- the kubeconfig asked for it
		ca, err := fileOrData(baseDir, c.Cluster.CertificateAuthority, c.Cluster.CertificateAuthorityD)
		if err != nil {
			return nil, err
		}
		if ca != nil {
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(ca) {
				return nil, fmt.Errorf("%w: the cluster CA is not PEM", errUnsupported)
			}
			tlsCfg.RootCAs = pool
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: cluster %q is not defined", errUnsupported, clusterName)
	}
	for _, u := range cfg.Users {
		if u.Name != userName {
			continue
		}
		if len(u.User.AuthProvider) > 0 {
			return nil, fmt.Errorf("%w: user %q uses the removed auth-provider mechanism; use an exec credential plugin, a token or a client certificate", errUnsupported, userName)
		}
		if u.User.Exec != nil {
			if u.User.Exec.Command == "" {
				return nil, fmt.Errorf("%w: user %q has an exec section without a command", errUnsupported, userName)
			}
			ep.exec, ep.baseDir = u.User.Exec, baseDir
		}
		ep.token = u.User.Token
		if ep.token == "" && u.User.TokenFile != "" {
			tok, err := os.ReadFile(resolve(baseDir, u.User.TokenFile)) // #nosec G304 -- named by the user's kubeconfig
			if err != nil {
				return nil, err
			}
			ep.token = strings.TrimSpace(string(tok))
		}
		certPEM, err := fileOrData(baseDir, u.User.ClientCertificate, u.User.ClientCertificateData)
		if err != nil {
			return nil, err
		}
		keyPEM, err := fileOrData(baseDir, u.User.ClientKey, u.User.ClientKeyData)
		if err != nil {
			return nil, err
		}
		if certPEM != nil && keyPEM != nil {
			pair, err := tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				return nil, fmt.Errorf("%w: client certificate: %v", errUnsupported, err)
			}
			tlsCfg.Certificates = []tls.Certificate{pair}
		}
	}
	ep.tls = tlsCfg
	return ep, nil
}

func resolve(baseDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(baseDir, p)
}

// fileOrData reads a credential given either as a file name or as inline
// base64, and returns nil when neither is set.
func fileOrData(baseDir, file, data string) ([]byte, error) {
	if data != "" {
		out, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("%w: bad base64 credential: %v", errUnsupported, err)
		}
		return out, nil
	}
	if file != "" {
		return os.ReadFile(resolve(baseDir, file)) // #nosec G304 -- named by the user's kubeconfig
	}
	return nil, nil
}

// execTimeout bounds the credential helper.
const execTimeout = 30 * time.Second

// resolveExec runs the credential helper the kubeconfig names, once, and takes
// the token or client certificate it prints. The helper is what the user's own
// kubeconfig asks to run, as with kubectl; it gets no stdin and no terminal, so
// one that needs to prompt fails with its own message.
func (ep *apiEndpoint) resolveExec(ctx context.Context) error {
	spec := ep.exec
	if spec == nil {
		return nil
	}
	command := spec.Command
	if strings.ContainsAny(command, `/\`) {
		command = resolve(ep.baseDir, command)
	}
	info, _ := json.Marshal(map[string]any{
		"apiVersion": spec.APIVersion, "kind": "ExecCredential", "spec": map[string]any{"interactive": false},
	})
	cmd := exec.CommandContext(ctx, command, spec.Args...) // #nosec G204 -- the user's own kubeconfig names the helper, as it does for kubectl
	cmd.Env = append(os.Environ(), "KUBERNETES_EXEC_INFO="+string(info))
	for _, e := range spec.Env {
		cmd.Env = append(cmd.Env, e.Name+"="+e.Value)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		hint := strings.TrimSpace(stderr.String())
		if hint == "" {
			hint = spec.InstallHint
		}
		return fmt.Errorf("k8sfs: the credential helper %q failed: %w: %s", spec.Command, err, hint)
	}
	var cred struct {
		Status struct {
			Token                 string `json:"token"`
			ClientCertificateData string `json:"clientCertificateData"`
			ClientKeyData         string `json:"clientKeyData"`
		} `json:"status"`
	}
	if err := json.Unmarshal(out, &cred); err != nil {
		return fmt.Errorf("k8sfs: the credential helper %q printed something that is not an ExecCredential: %w", spec.Command, err)
	}
	if t := cred.Status.Token; t != "" {
		ep.token = t
	}
	if cred.Status.ClientCertificateData != "" && cred.Status.ClientKeyData != "" {
		pair, err := tls.X509KeyPair([]byte(cred.Status.ClientCertificateData), []byte(cred.Status.ClientKeyData))
		if err != nil {
			return fmt.Errorf("k8sfs: the credential helper %q gave a bad client certificate: %w", spec.Command, err)
		}
		ep.tls.Certificates = []tls.Certificate{pair}
	}
	if ep.token == "" && len(ep.tls.Certificates) == 0 {
		return fmt.Errorf("k8sfs: the credential helper %q gave neither a token nor a client certificate", spec.Command)
	}
	return nil
}
