package dockerfs

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// defaultSocket is where a Docker daemon listens unless DOCKER_HOST says
// otherwise.
const defaultSocket = "/var/run/docker.sock"

var errUnsupportedHost = errors.New("dockerfs: unsupported DOCKER_HOST")

// client speaks the Docker Engine HTTP API with nothing but the standard
// library: a unix socket (or a plain tcp:// address) and a handful of
// endpoints. No Docker SDK is linked, so the plugin adds no dependency and
// nothing to the binary beyond this file.
type client struct {
	http *http.Client
	base string
}

// clientFromEnv reads DOCKER_HOST the way the docker CLI does. Unset, it uses
// the system socket and, failing that, the rootless one under XDG_RUNTIME_DIR.
// On Windows the default is Docker Desktop's named pipe. DOCKER_CONTEXT, DOCKER_TLS_VERIFY/DOCKER_CERT_PATH and the CLI's current
// context are honoured too (see endpoint.go); ssh:// is not supported and the
// error says so instead of dialing something else.
func clientFromEnv() (*client, error) {
	ep, err := resolveEndpoint(os.Getenv)
	if err != nil {
		return nil, err
	}
	return newClientTLS(ep.host, ep.tls)
}

func defaultSocketPath() string {
	if _, err := os.Stat(defaultSocket); err == nil {
		return defaultSocket
	}
	if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
		rootless := filepath.Join(dir, "docker.sock")
		if _, err := os.Stat(rootless); err == nil { // #nosec G703 -- a fixed name under the user's own runtime dir
			return rootless
		}
	}
	return defaultSocket
}

func newClient(host string) (*client, error) { return newClientTLS(host, nil) }

// newClientTLS is newClient with the TLS settings for tcp://, http:// and
// https:// addresses (nil: plain HTTP, except https://, which uses the
// system roots).
func newClientTLS(host string, tlsCfg *tls.Config) (*client, error) {
	u, err := url.Parse(host)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %v", errUnsupportedHost, host, err)
	}
	switch u.Scheme {
	case "unix":
		if runtime.GOOS == "windows" {
			return nil, fmt.Errorf("%w %q: unix sockets are not available here", errUnsupportedHost, host)
		}
		socket := u.Path
		if socket == "" {
			return nil, fmt.Errorf("%w %q: no socket path", errUnsupportedHost, host)
		}
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     30 * time.Second,
		}
		return &client{http: &http.Client{Transport: transport}, base: "http://docker"}, nil
	case "npipe":
		if runtime.GOOS != "windows" {
			return nil, fmt.Errorf("%w %q: named pipes exist only on Windows", errUnsupportedHost, host)
		}
		name := strings.ReplaceAll(u.Path, "/", `\`)
		if name == "" {
			return nil, fmt.Errorf("%w %q: no pipe name", errUnsupportedHost, host)
		}
		return &client{http: &http.Client{Transport: &pipeTransport{dial: openPipe(name)}}, base: "http://docker"}, nil
	case "tcp", "http", "https":
		if u.Host == "" {
			return nil, fmt.Errorf("%w %q: no address", errUnsupportedHost, host)
		}
		if tlsCfg == nil && u.Scheme != "https" {
			return &client{http: &http.Client{}, base: "http://" + u.Host}, nil
		}
		if tlsCfg == nil {
			tlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		transport := &http.Transport{
			TLSClientConfig:     tlsCfg,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     30 * time.Second,
		}
		return &client{http: &http.Client{Transport: transport}, base: "https://" + u.Host}, nil
	default:
		return nil, fmt.Errorf("%w %q: only unix://, npipe://, tcp:// and https:// are supported (ssh:// is not)", errUnsupportedHost, host)
	}
}

func (c *client) close() {
	if c == nil {
		return
	}
	c.http.CloseIdleConnections()
}

// containerInfo is the part of /containers/json this plugin shows.
type containerInfo struct {
	ID      string   `json:"Id"`
	Names   []string `json:"Names"`
	Image   string   `json:"Image"`
	State   string   `json:"State"`
	Status  string   `json:"Status"`
	Created int64    `json:"Created"`
}

// name is what the container is called in the panel: its first name without
// the leading slash the API puts on it, or the short id of an unnamed one.
func (ci containerInfo) name() string {
	for _, n := range ci.Names {
		if n = strings.TrimPrefix(n, "/"); n != "" {
			return n
		}
	}
	if len(ci.ID) > 12 {
		return ci.ID[:12]
	}
	return ci.ID
}

// pathStat is X-Docker-Container-Path-Stat: what the daemon knows about one
// path inside a container.
type pathStat struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Mode       uint32    `json:"mode"`
	MTime      time.Time `json:"mtime"`
	LinkTarget string    `json:"linkTarget"`
}

// The bits of Go's os.FileMode the daemon sends the mode as.
func (s pathStat) fileMode() os.FileMode { return os.FileMode(s.Mode) }
func (s pathStat) isDir() bool           { return s.fileMode().IsDir() }
func (s pathStat) isSymlink() bool       { return s.fileMode()&os.ModeSymlink != 0 }

func (c *client) do(ctx context.Context, method, endpoint string, query url.Values) (*http.Response, error) {
	return c.send(ctx, method, endpoint, query, "", nil)
}

// send is do with a request body.
func (c *client) send(ctx context.Context, method, endpoint string, query url.Values, contentType string, body io.Reader) (*http.Response, error) {
	target := c.base + endpoint
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s: %w", dockerText("Docker.Unreachable",
			"Cannot reach the Docker daemon (is it running, and is DOCKER_HOST right?)",
			"Нет связи с демоном Docker (запущен ли он, верна ли переменная DOCKER_HOST?)"), err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("%s: %w", apiMessage(resp), os.ErrNotExist)
	}
	return nil, fmt.Errorf("docker: %s: %s", resp.Status, apiMessage(resp))
}

// apiMessage pulls {"message": "..."} out of an error body, or falls back to
// the status text.
func apiMessage(resp *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	var msg struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &msg) == nil && msg.Message != "" {
		return msg.Message
	}
	if text := strings.TrimSpace(string(body)); text != "" {
		return text
	}
	return resp.Status
}

func (c *client) listContainers(ctx context.Context) ([]containerInfo, error) {
	resp, err := c.do(ctx, http.MethodGet, "/containers/json", url.Values{"all": {"1"}})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var out []containerInfo
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("docker: bad container list: %w", err)
	}
	return out, nil
}

// statPath asks about one path without transferring anything: a HEAD on the
// archive endpoint carries the answer in a header.
func (c *client) statPath(ctx context.Context, id, p string) (pathStat, error) {
	resp, err := c.do(ctx, http.MethodHead, "/containers/"+url.PathEscape(id)+"/archive", url.Values{"path": {p}})
	if err != nil {
		return pathStat{}, err
	}
	_ = resp.Body.Close()
	return parsePathStat(resp.Header.Get("X-Docker-Container-Path-Stat"))
}

func parsePathStat(header string) (pathStat, error) {
	if header == "" {
		return pathStat{}, errors.New("docker: the daemon sent no path stat")
	}
	raw, err := base64.StdEncoding.DecodeString(header)
	if err != nil {
		return pathStat{}, fmt.Errorf("docker: bad path stat: %w", err)
	}
	var st pathStat
	if err := json.Unmarshal(raw, &st); err != nil {
		return pathStat{}, fmt.Errorf("docker: bad path stat: %w", err)
	}
	return st, nil
}

// archive streams a path as a tar. The caller closes the body, which also
// stops the daemon reading the rest.
func (c *client) archive(ctx context.Context, id, p string) (io.ReadCloser, error) {
	resp, err := c.do(ctx, http.MethodGet, "/containers/"+url.PathEscape(id)+"/archive", url.Values{"path": {p}})
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// putArchive uploads a tar to a folder of the container. write fills the tar;
// it runs while the request is being sent, so nothing is held in memory.
func (c *client) putArchive(ctx context.Context, id, dir string, write func(*tar.Writer) error) error {
	pr, pw := io.Pipe()
	go func() {
		tw := tar.NewWriter(pw)
		err := write(tw)
		if err == nil {
			err = tw.Close()
		}
		_ = pw.CloseWithError(err)
	}()
	resp, err := c.send(ctx, http.MethodPut, "/containers/"+url.PathEscape(id)+"/archive",
		url.Values{"path": {dir}}, "application/x-tar", pr)
	if err != nil {
		_ = pr.CloseWithError(err)
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (c *client) postJSON(ctx context.Context, endpoint string, in, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	resp, err := c.send(ctx, http.MethodPost, endpoint, nil, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

var errExecFailed = errors.New("the command failed in the container")

// execTimeout bounds one rm or mv run inside a container.
const execTimeout = 60 * time.Second

// exec runs a command (no shell) in a running container and reports its exit
// status. The output is not collected: the daemon has no delete or rename for
// a container's files, so those are done with rm and mv, and a nonzero status
// is all there is to say about them.
func (c *client) exec(ctx context.Context, id string, cmd []string) error {
	var created struct {
		ID string `json:"Id"`
	}
	err := c.postJSON(ctx, "/containers/"+url.PathEscape(id)+"/exec",
		map[string]any{"Cmd": cmd, "AttachStdout": false, "AttachStderr": false}, &created)
	if err != nil {
		return err
	}
	if err := c.postJSON(ctx, "/exec/"+url.PathEscape(created.ID)+"/start", map[string]any{"Detach": true}, nil); err != nil {
		return err
	}
	deadline := time.Now().Add(execTimeout)
	for {
		resp, err := c.do(ctx, http.MethodGet, "/exec/"+url.PathEscape(created.ID)+"/json", nil)
		if err != nil {
			return err
		}
		var state struct {
			Running  bool `json:"Running"`
			ExitCode int  `json:"ExitCode"`
		}
		err = json.NewDecoder(resp.Body).Decode(&state)
		_ = resp.Body.Close()
		if err != nil {
			return err
		}
		if !state.Running {
			if state.ExitCode != 0 {
				return fmt.Errorf("%s %s: %w (exit status %d)", cmd[0], strings.Join(cmd[1:], " "), errExecFailed, state.ExitCode)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: timed out", cmd[0])
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
