package plughost

import (
	"errors"
	"io/fs"
	"strings"
)

// rpcVFSError gives an error that crossed the plugin boundary its meaning back.
//
// Only the text of an error travels over RPC, so a plugin's "file does not
// exist" arrived here as "rpc error: ... file does not exist", which
// errors.Is(err, fs.ErrNotExist) does not recognise. File operations ask
// exactly that of the destination before they copy: a missing destination is
// the normal case, a failed Stat is an error. Uploading to an RPC plugin (the
// Android one, f4#1761) therefore failed on every new file. The text is kept
// as it is; only the kind is put back underneath it.
func rpcVFSError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	for _, known := range []struct {
		kind     error
		suffixes []string
	}{
		{fs.ErrNotExist, []string{fs.ErrNotExist.Error(), "no such file or directory"}},
		{fs.ErrExist, []string{fs.ErrExist.Error(), "file exists"}},
		{fs.ErrPermission, []string{fs.ErrPermission.Error()}},
	} {
		if errors.Is(err, known.kind) {
			return err
		}
		for _, suffix := range known.suffixes {
			if strings.HasSuffix(lower, suffix) {
				return &rpcKindError{msg: msg, kind: known.kind}
			}
		}
	}
	return err
}

type rpcKindError struct {
	msg  string
	kind error
}

func (e *rpcKindError) Error() string { return e.msg }
func (e *rpcKindError) Unwrap() error { return e.kind }
