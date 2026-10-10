package plughost

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"

	"github.com/unxed/f4/sdk/f4plugin"
)

type uriRecordingTransport struct{ requests []f4plugin.URIRequest }

func (s *uriRecordingTransport) Call(_ string, params any, result any) error {
	req := params.(f4plugin.URIRequest)
	s.requests = append(s.requests, req)
	switch req.Operation {
	case "openURI":
		*result.(*f4plugin.URIMount) = f4plugin.URIMount{ID: 7, Path: req.Path}
	case "join":
		*result.(*string) = "ios://Phone/100%25/a%2Fb"
	case "open":
		*result.(*f4plugin.URIFile) = f4plugin.URIFile{ID: 9, Size: 3}
	case "readAt":
		*result.(*[]byte) = []byte("end")
	}
	return nil
}

func TestDeferredURIWaitsForRegistrationAndPreservesPaths(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := newDeferredURIProvider("ios")
		transport := &uriRecordingTransport{}
		finished := make(chan struct{})
		var opened *rpcURIVFS
		go func() {
			mounted, err := provider.OpenURI(t.Context(), nil, "ios://Phone/100%25/")
			if err != nil {
				t.Error(err)
			} else {
				opened = mounted.(*rpcURIVFS)
			}
			close(finished)
		}()
		synctest.Wait()
		select {
		case <-finished:
			t.Fatal("restore finished before RPC registration")
		default:
		}
		provider.complete(transport, nil)
		<-finished
		if opened == nil || opened.GetPath() != "ios://Phone/100%25/" {
			t.Fatal("lost escaped restore target")
		}
		if got := opened.Join(opened.GetPath(), "a/b"); got != "ios://Phone/100%25/a%2Fb" {
			t.Fatalf("Join = %q", got)
		}
		file, err := opened.Open(t.Context(), "ios://Phone/100%25/file")
		if err != nil {
			t.Fatal(err)
		}
		buffer := make([]byte, 3)
		if n, err := file.ReadAt(t.Context(), buffer, 0); n != 3 || err != nil || string(buffer) != "end" {
			t.Fatalf("preview tail = %q, %d, %v", buffer, n, err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDeferredURICancellationAndFailure(t *testing.T) {
	provider := newDeferredURIProvider("ios")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := provider.OpenURI(ctx, nil, "ios://Phone/"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
	failure := errors.New("permission denied")
	provider.complete(nil, failure)
	if _, err := provider.OpenURI(t.Context(), nil, "ios://Phone/"); !errors.Is(err, failure) {
		t.Fatalf("failed registration = %v", err)
	}
	// Deferring permission must not permanently poison registration: an approved
	// reload can bind the same provider without changing any permission grants.
	provider.complete(&uriRecordingTransport{}, nil)
	provider.complete(nil, failure)
	mounted, err := provider.OpenURI(t.Context(), nil, "ios://Phone/")
	if err != nil || mounted.GetPath() != "ios://Phone/" {
		t.Fatalf("registration after deferred permission = %v, %v", mounted, err)
	}
}
