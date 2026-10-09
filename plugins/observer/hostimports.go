package observer

import (
	"bytes"
	"context"
	"os"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ProgressFunc is the host side of Observer's ExtractProgressFunc(HANDLE,
// __int64), exposed to a module as the "observer.progress" wazero host
// import. signalContext is the HANDLE value the module was itself given in
// ExtractProcessCallbacks.signalContext; bytesDone is how many bytes have
// been produced so far. Returning 0 aborts the operation, matching the real
// ABI's FALSE-cancels convention; a nil ProgressFunc always continues.
//
// This part does not yet drive ExtractItem (reserved for a later part of
// f4#1563), but wires up and tests the import itself, per item 2 of the
// part-1 plan: testdata/stub calls it once from f4observer_open_storage so
// the round trip is exercised end-to-end.
type ProgressFunc func(signalContext uint32, bytesDone int64) int32

// hostState is what the "observer" host module's imports share with a
// Module.
type hostState struct {
	progress ProgressFunc

	// lastStderr is the last line the guest wrote to stderr before a call
	// returned or trapped, the way colorer4go's streamWriter feeds
	// FatalError.Reason: a wasi-sdk libc abort handler (assert(), an
	// unhandled exception's default terminate, ...) writes its message
	// there right before the call that triggered it fails to return.
	lastStderr string
}

// beginCall resets lastStderr so FatalError.Reason reflects only what the
// guest wrote during the call that is about to run.
func (h *hostState) beginCall() {
	h.lastStderr = ""
}

// stderrWriter captures the module's stderr into hostState.lastStderr, and
// -- like colorer4go's streamWriter does absent a diagnostics handler --
// also forwards it to the host process's own stderr, so a module's own
// debug output (or an abort message on a live run) is not silently dropped.
type stderrWriter struct {
	host *hostState
	buf  []byte
}

func (w *stderrWriter) Write(p []byte) (int, error) {
	_, _ = os.Stderr.Write(p)
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		if line := string(bytes.TrimRight(w.buf[:i], "\r")); line != "" {
			w.host.lastStderr = line
		}
		w.buf = w.buf[i+1:]
	}
	if len(w.buf) > 0 {
		// An abort message need not end in a newline; keep what there is.
		w.host.lastStderr = string(w.buf)
	}
	return len(p), nil
}

// newObserverHostModule builds and instantiates the "observer" host module
// with the progress import. h must outlive every call the guest makes.
func newObserverHostModule(ctx context.Context, r wazero.Runtime, h *hostState) error {
	i32, i64 := api.ValueTypeI32, api.ValueTypeI64
	_, err := r.NewHostModuleBuilder("observer").
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(func(_ context.Context, _ api.Module, stack []uint64) {
			// #nosec G115 -- stack[0] is the i32 param wazero itself declared
			// for this function below, so it is already zero-extended from
			// 32 bits.
			signalContext := uint32(stack[0])
			// #nosec G115 -- stack[1] is the i64 param wazero declared below;
			// this reinterprets its bits as the signed __int64 bytesDone the
			// ABI specifies, not a narrowing conversion.
			bytesDone := int64(stack[1])
			result := int32(1)
			if h.progress != nil {
				result = h.progress(signalContext, bytesDone)
			}
			stack[0] = uint64(uint32(result))
		}), []api.ValueType{i32, i64}, []api.ValueType{i32}).
		Export("progress").
		Instantiate(ctx)
	return err
}
