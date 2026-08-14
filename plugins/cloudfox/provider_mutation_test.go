package cloudfox

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"google.golang.org/api/googleapi"
)

type mutationRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper mutationRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTripper(request)
}

type closeIdleTrackingTransport struct{ closed bool }

func (*closeIdleTrackingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("not used")
}

func (t *closeIdleTrackingTransport) CloseIdleConnections() { t.closed = true }

func TestHTTPBackendsCloseIdleConnections(t *testing.T) {
	for _, makeBackend := range []func(*http.Client) Backend{
		func(client *http.Client) Backend { return &webDAVBackend{client: client} },
		func(client *http.Client) Backend { return &yandexDiskBackend{client: client} },
		func(client *http.Client) Backend { return &s3Backend{httpClient: client} },
	} {
		transport := &closeIdleTrackingTransport{}
		if err := makeBackend(&http.Client{Transport: transport}).Close(); err != nil {
			t.Fatal(err)
		}
		if !transport.closed {
			t.Fatal("backend Close did not close idle HTTP connections")
		}
	}
}

func TestProviderSpoolWriterConcurrentCloseWaitsForCommit(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	commitErr := errors.New("remote commit failed")
	w, err := newProviderSpoolWriter(context.Background(), "item", func(context.Context, *os.File, int64) error {
		close(started)
		<-release
		return commitErr
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "payload"); err != nil {
		t.Fatal(err)
	}
	first := make(chan error, 1)
	go func() { first <- w.Close() }()
	<-started
	second := make(chan error, 1)
	go func() { second <- w.Close() }()
	select {
	case err := <-second:
		t.Fatalf("concurrent Close returned before commit finished: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-first; !errors.Is(err, commitErr) {
		t.Fatalf("first Close = %v", err)
	}
	if err := <-second; !errors.Is(err, commitErr) {
		t.Fatalf("second Close = %v", err)
	}
}

func TestProviderSpoolWriterAbortNeverInvokesCommit(t *testing.T) {
	commits := 0
	w, err := newProviderSpoolWriter(context.Background(), "item", func(context.Context, *os.File, int64) error {
		commits++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tempPath := w.path
	_, _ = io.WriteString(w, "partial source")
	if err := w.Abort(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close after Abort = %v", err)
	}
	if commits != 0 {
		t.Fatalf("Abort invoked %d remote commit(s)", commits)
	}
	if _, err := os.Stat(tempPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("aborted spool still exists: %v", err)
	}
}

func TestMutationHTTPResponsesDistinguishDefinitiveRejectionFromUnknownState(t *testing.T) {
	t.Parallel()
	request, err := http.NewRequest(http.MethodDelete, "https://storage.example/item", nil)
	if err != nil {
		t.Fatal(err)
	}
	serviceUnavailable := &http.Response{StatusCode: http.StatusServiceUnavailable, Request: request}
	if err := providerHTTPMutationError("remote delete", serviceUnavailable, "retry later"); !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("HTTP 503 mutation error = %v, want unknown state", err)
	}
	forbidden := &http.Response{StatusCode: http.StatusForbidden, Request: request}
	if err := providerHTTPMutationError("remote delete", forbidden, "denied"); errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("HTTP 403 mutation error = %v, want definitive rejection", err)
	}
	if err := googleMutationError("delete", &googleapi.Error{Code: http.StatusServiceUnavailable, Message: "retry later"}); !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("Google HTTP 503 mutation error = %v, want unknown state", err)
	}
	if err := googleMutationError("delete", &googleapi.Error{Code: http.StatusForbidden, Message: "denied"}); errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("Google HTTP 403 mutation error = %v, want definitive rejection", err)
	}
}

func TestTransportFailuresLeaveRemoteMutationStateUnknown(t *testing.T) {
	t.Parallel()
	lostResponse := errors.New("connection lost after request write")
	client := &http.Client{Transport: mutationRoundTripper(func(*http.Request) (*http.Response, error) {
		return nil, lostResponse
	})}
	davBase, err := url.Parse("https://dav.example/root")
	if err != nil {
		t.Fatal(err)
	}
	dav := &webDAVBackend{client: client, base: davBase, rootPath: "/"}
	yandex := &yandexDiskBackend{client: client, baseURL: "https://cloud-api.yandex.test/v1/disk", token: "token", root: "disk:/"}

	tests := []struct {
		name string
		err  error
	}{
		{"WebDAV delete", dav.mutation(context.Background(), http.MethodDelete, "/item", nil)},
		{"Yandex delete", yandex.mutation(context.Background(), http.MethodDelete, "/resources", url.Values{"path": {"disk:/item"}})},
		{"S3 delete", s3MutationError("delete", lostResponse)},
		{"Google delete", googleMutationError("delete", lostResponse)},
	}
	for _, test := range tests {
		if !errors.Is(test.err, vfs.ErrOperationStateUnknown) || !errors.Is(test.err, lostResponse) {
			t.Errorf("%s error = %v, want unknown state wrapping transport failure", test.name, test.err)
		}
	}
}
