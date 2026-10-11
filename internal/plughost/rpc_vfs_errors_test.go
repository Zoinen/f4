package plughost

import (
	"context"
	"errors"
	"io/fs"
	"testing"
)

// A plugin's "does not exist" has to stay a not-exist on this side of the
// RPC boundary: file operations check the destination with errors.Is before
// they write, and uploading a new file to the Android plugin failed on it
// (f4#1761).
func TestRPCVFSKeepsTheKindOfPluginErrors(t *testing.T) {
	answer := errors.New("rpc error: android: \"/sdcard/new.txt\": file does not exist")
	transport := &rpcVFSTestTransport{handler: func(method string, params, result any) error {
		return answer
	}}
	v := NewRPCVFS(transport, "Android")

	_, err := v.Stat(context.Background(), "/sdcard/new.txt")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Stat error %v is not fs.ErrNotExist", err)
	}
	if err.Error() != answer.Error() {
		t.Fatalf("the text of the error changed: %q", err.Error())
	}
	if _, err := v.Open(context.Background(), "/sdcard/new.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Open error %v is not fs.ErrNotExist", err)
	}
	if err := v.Remove(context.Background(), "/sdcard/new.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Remove error %v is not fs.ErrNotExist", err)
	}
}

func TestRPCVFSErrorKinds(t *testing.T) {
	for _, tc := range []struct {
		msg  string
		kind error
	}{
		{"rpc error: open /x: no such file or directory", fs.ErrNotExist},
		{"rpc error: mkdir /x: file exists", fs.ErrExist},
		{"rpc error: file already exists", fs.ErrExist},
		{"rpc error: open /x: permission denied", fs.ErrPermission},
	} {
		if err := rpcVFSError(errors.New(tc.msg)); !errors.Is(err, tc.kind) || err.Error() != tc.msg {
			t.Errorf("%q -> %v, want %v with the same text", tc.msg, err, tc.kind)
		}
	}
	other := errors.New("rpc error: device offline")
	if got := rpcVFSError(other); got != other {
		t.Errorf("an unrelated error was rewritten: %v", got)
	}
	if rpcVFSError(nil) != nil {
		t.Error("nil became an error")
	}
}
