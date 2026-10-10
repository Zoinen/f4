package cloudfox

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/f4/vfs"
)

type rpcPreviewReader struct {
	vfs.ReadAtCloser
	data *bytes.Reader
}

func (r *rpcPreviewReader) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	return r.data.ReadAt(p, off)
}
func (r *rpcPreviewReader) Close() error { return nil }

func TestRPCPluginPreviewShortReadPreservesBytes(t *testing.T) {
	plugin := &RPCPlugin{readers: map[uint32]vfs.ReadAtCloser{
		1: &rpcPreviewReader{data: bytes.NewReader([]byte("image bytes"))},
	}}
	client, server := net.Pipe()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	_ = server.SetDeadline(time.Now().Add(5 * time.Second))
	host := f4rpc.NewSession(client, client)
	doneHost, donePlugin := make(chan error, 1), make(chan error, 1)
	go func() { doneHost <- host.Serve() }()
	go func() { donePlugin <- f4plugin.Serve(plugin, server, server) }()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
		<-doneHost
		<-donePlugin
	})

	for _, tc := range []struct {
		name   string
		length int
		offset int64
		want   string
	}{
		{name: "partial EOF", length: 64, offset: 6, want: "bytes"},
		{name: "at EOF", length: 64, offset: 11, want: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got []byte
			err := host.Call("VFS.ReadAt", f4plugin.ReadAtReq{ID: 1, Len: tc.length, Off: tc.offset}, &got)
			if err != nil || string(got) != tc.want {
				t.Fatalf("RPC preview read = %q, %v; want %q without wire error", got, err, tc.want)
			}
		})
	}
}
