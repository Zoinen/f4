package observer

import "strings"

// FatalError reports a call into an Observer module that did not return.
// The module was stopped in the middle of whatever it was doing, so its
// state is unknown: the Module refuses every later call, returning this
// same error, and the only thing left to do with it is Close.
//
// This is the same policy colorer4go uses for a C++ module compiled without
// exceptions (see its FatalError): a throw the module cannot catch aborts
// the call, Reason is the last line the module wrote to stderr before that
// (a wasi-sdk libc default abort handler prints the assertion or the
// exception's what() there), and the caller is left with nothing to do but
// tear the instance down. What a trap means for the caller above this
// package -- "not my format" during OpenStorage vs. a real error during
// GetItem/ExtractItem -- is a provider-level policy decision left to a
// later part of f4#1563.
type FatalError struct {
	// Op is the export that failed, e.g. observer.ExportOpenStorage.
	Op string
	// Reason is the last line the module wrote to stderr before it
	// stopped. Empty when it stopped without saying anything.
	Reason string
	// Err is the error the wasm runtime returned for the call.
	Err error
}

func (e *FatalError) Error() string {
	var b strings.Builder
	b.WriteString("observer: ")
	b.WriteString(e.Op)
	b.WriteString(" failed")
	if e.Reason != "" {
		b.WriteString(": ")
		b.WriteString(e.Reason)
	}
	if e.Err != nil {
		// The runtime appends a multi-line wasm stack trace; Unwrap keeps it.
		msg := e.Err.Error()
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		b.WriteString(" (")
		b.WriteString(msg)
		b.WriteString(")")
	}
	return b.String()
}

func (e *FatalError) Unwrap() error { return e.Err }
