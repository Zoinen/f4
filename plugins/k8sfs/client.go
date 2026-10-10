package k8sfs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/net/websocket"
)

// restClient talks to the Kubernetes API server with the standard library and
// golang.org/x/net/websocket (which f4 already depends on through x/net): no
// client-go, no kubectl.
type restClient struct {
	http *http.Client
	ep   *apiEndpoint
}

func newRESTClient(ep *apiEndpoint) *restClient {
	return &restClient{
		http: &http.Client{Transport: &http.Transport{
			TLSClientConfig:     ep.tls,
			MaxIdleConnsPerHost: 4,
			IdleConnTimeout:     30 * time.Second,
		}},
		ep: ep,
	}
}

func (c *restClient) close() {
	if c != nil {
		c.http.CloseIdleConnections()
	}
}

var errExecFailed = errors.New("the command failed in the container")

// podInfo is what the panel shows of a pod.
type podInfo struct {
	name       string
	phase      string
	created    time.Time
	containers []string
}

func (c *restClient) getJSON(ctx context.Context, endpoint string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.ep.server+endpoint, nil)
	if err != nil {
		return err
	}
	if c.ep.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.ep.token)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("%s: %w", k8sText("K8s.Unreachable",
			"Cannot reach the Kubernetes API server (check the kubeconfig and the network)",
			"Нет связи с API-сервером Kubernetes (проверьте kubeconfig и сеть)"), err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var status struct {
			Message string `json:"message"`
		}
		msg := strings.TrimSpace(string(body))
		if json.Unmarshal(body, &status) == nil && status.Message != "" {
			msg = status.Message
		}
		if resp.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%s: %w", msg, os.ErrNotExist)
		}
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
			return fmt.Errorf("kubernetes: %s: %s: %w", resp.Status, msg, os.ErrPermission)
		}
		return fmt.Errorf("kubernetes: %s: %s", resp.Status, msg)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *restClient) listNamespaces(ctx context.Context) ([]string, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name string `json:"name"`
			} `json:"metadata"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, "/api/v1/namespaces", &list); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list.Items))
	for _, it := range list.Items {
		names = append(names, it.Metadata.Name)
	}
	return names, nil
}

func (c *restClient) listPods(ctx context.Context, namespace string) ([]podInfo, error) {
	var list struct {
		Items []struct {
			Metadata struct {
				Name    string    `json:"name"`
				Created time.Time `json:"creationTimestamp"`
			} `json:"metadata"`
			Spec struct {
				Containers []struct {
					Name string `json:"name"`
				} `json:"containers"`
			} `json:"spec"`
			Status struct {
				Phase string `json:"phase"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := c.getJSON(ctx, "/api/v1/namespaces/"+url.PathEscape(namespace)+"/pods", &list); err != nil {
		return nil, err
	}
	pods := make([]podInfo, 0, len(list.Items))
	for _, it := range list.Items {
		p := podInfo{name: it.Metadata.Name, phase: it.Status.Phase, created: it.Metadata.Created}
		for _, ct := range it.Spec.Containers {
			p.containers = append(p.containers, ct.Name)
		}
		pods = append(pods, p)
	}
	return pods, nil
}

// execSubprotocol is the WebSocket subprotocol whose first byte of every frame
// names a channel (1 stdout, 2 stderr, 3 status).
const execSubprotocol = "v4.channel.k8s.io"

// maxStderr is how much of a command's stderr is kept for an error message.
const maxStderr = 2048

// exec runs a command (no shell) in a container and copies its stdout to out.
// A command that exits nonzero comes back as errExecFailed with the reason the
// API server gave, plus whatever it wrote to stderr.
func (c *restClient) exec(ctx context.Context, namespace, pod, container string, cmd []string, out io.Writer) error {
	q := url.Values{"container": {container}, "stdout": {"true"}, "stderr": {"true"}, "stdin": {"false"}, "tty": {"false"}}
	for _, a := range cmd {
		q.Add("command", a)
	}
	wsURL := "ws" + strings.TrimPrefix(c.ep.server, "http") + "/api/v1/namespaces/" +
		url.PathEscape(namespace) + "/pods/" + url.PathEscape(pod) + "/exec?" + q.Encode()
	cfg, err := websocket.NewConfig(wsURL, c.ep.server)
	if err != nil {
		return err
	}
	cfg.Protocol = []string{execSubprotocol}
	cfg.TlsConfig = c.ep.tls
	if c.ep.token != "" {
		cfg.Header.Set("Authorization", "Bearer "+c.ep.token)
	}
	ws, err := cfg.DialContext(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("kubernetes: exec: %w", err)
	}
	defer func() { _ = ws.Close() }()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			_ = ws.Close()
		case <-stop:
		}
	}()

	var stderr strings.Builder
	for {
		var frame []byte
		if err := websocket.Message.Receive(ws, &frame); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(frame) == 0 {
			continue
		}
		switch frame[0] {
		case 1:
			if _, err := out.Write(frame[1:]); err != nil {
				return err
			}
		case 2:
			if stderr.Len() < maxStderr {
				stderr.Write(frame[1:])
			}
		case 3:
			if err := statusError(frame[1:], cmd, stderr.String()); err != nil {
				return err
			}
		}
	}
}

// statusError reads the v4 status frame; nil means the command succeeded.
func statusError(raw []byte, cmd []string, stderr string) error {
	var st struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &st); err != nil || st.Status != "Failure" {
		return nil
	}
	detail := strings.TrimSpace(stderr)
	if detail == "" {
		detail = st.Message
	}
	return fmt.Errorf("%s: %w: %s", cmd[0], errExecFailed, detail)
}
