package vfs

import (
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// TestDisplayPathErrorKeepsTheErrorIntact pins the contract every caller
// depends on: displayPathError rewrites the path and nothing else. Op, the
// wrapped Errno and the sentinel the message unwraps to all survive, so a
// caller that already branched on os.ErrPermission keeps getting the same
// answer, and errors.As still reaches the *fs.PathError underneath.
func TestDisplayPathErrorKeepsTheErrorIntact(t *testing.T) {
	in := &fs.PathError{Op: "open", Path: os.TempDir(), Err: os.ErrPermission}

	out := displayPathError(in)

	pe, ok := out.(*fs.PathError)
	if !ok {
		t.Fatalf("displayPathError(%T) = %T, want *fs.PathError", in, out)
	}
	if pe.Op != in.Op {
		t.Errorf("Op = %q, want %q", pe.Op, in.Op)
	}
	if pe.Path != in.Path {
		t.Errorf("Path = %q, want %q (unchanged off Windows)", pe.Path, in.Path)
	}
	if !errors.Is(out, os.ErrPermission) {
		t.Errorf("errors.Is(%v, os.ErrPermission) = false, want true", out)
	}
	var reached *fs.PathError
	if !errors.As(out, &reached) {
		t.Errorf("errors.As(%v, **fs.PathError) = false, want true", out)
	}
	// The input is never mutated: a caller holding its own reference to the
	// error it passed in must not see the path change underneath it.
	if in.Path != os.TempDir() {
		t.Errorf("input error was mutated: Path = %q, want %q", in.Path, os.TempDir())
	}
}

// TestDisplayPathErrorPassesOtherErrorsThrough covers what displayPathError
// deliberately does not touch: a plain error, and nil. Wrapping either in a
// PathError would invent a path no syscall ever reported.
func TestDisplayPathErrorPassesOtherErrorsThrough(t *testing.T) {
	plain := errors.New("something went wrong")
	if got := displayPathError(plain); !errors.Is(got, plain) {
		t.Errorf("displayPathError(%v) = %v, want the same error", plain, got)
	}
	if got := displayPathError(nil); got != nil {
		t.Errorf("displayPathError(nil) = %v, want nil", got)
	}
}

// TestDisplayPathErrorStripsExtendedPrefix is the unit-level half of the fix:
// the prefix prepareOSPath adds for the syscall must not reach a message a
// human reads. The prefix only ever exists on Windows, so this is skipped
// elsewhere -- stripExtendedPrefix is a documented no-op off Windows.
func TestDisplayPathErrorStripsExtendedPrefix(t *testing.T) {
	if strings.Contains(prepareOSPath(os.TempDir()), `\\?\`) {
		runExtendedPrefixCases(t)
		return
	}
	t.Skip("the \\?\\ prefix only exists on Windows")
}

func runExtendedPrefixCases(t *testing.T) {
	t.Helper()
	for _, tc := range []struct {
		name string
		path string
		want string
	}{
		{"drive path", `\\?\C:\Users\me\locked`, `C:\Users\me\locked`},
		{"UNC path", `\\?\UNC\server\share\locked`, `\\server\share\locked`},
		{"already plain", `C:\Users\me\locked`, `C:\Users\me\locked`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := &fs.PathError{Op: "open", Path: tc.path, Err: os.ErrPermission}
			out := displayPathError(in)
			if got := out.Error(); strings.Contains(got, `\\?\`) {
				t.Errorf("message still carries the extended-length prefix: %q", got)
			}
			pe, ok := out.(*fs.PathError)
			if !ok {
				t.Fatalf("displayPathError = %T, want *fs.PathError", out)
			}
			if pe.Path != tc.want {
				t.Errorf("Path = %q, want %q", pe.Path, tc.want)
			}
		})
	}
}

// TestDisplayPathErrorCleansNotListableError covers the wrapper SetPath
// reports through. NotListableError.Error() delegates to the wrapped host
// error, so cleaning only the outer Path would leave the prefix in the very
// dialog that started this -- "Cannot access folder: open \\?\...: Access is
// denied" (#814 refused the directory, and the message named the syscalls'
// spelling of the path).
func TestDisplayPathErrorCleansNotListableError(t *testing.T) {
	host := &fs.PathError{Op: "open", Path: `\\?\C:\locked`, Err: os.ErrPermission}
	in := &NotListableError{Path: `C:\locked`, Err: host}

	out := displayPathError(in)

	var nl *NotListableError
	if !errors.As(out, &nl) {
		t.Fatalf("displayPathError(%T) = %T, want *NotListableError", in, out)
	}
	if nl.Path != `C:\locked` {
		t.Errorf("Path = %q, want %q", nl.Path, `C:\locked`)
	}
	if !errors.Is(out, os.ErrPermission) {
		t.Errorf("errors.Is(%v, os.ErrPermission) = false, want true", out)
	}
	if !errors.As(out, new(*fs.PathError)) {
		t.Errorf("errors.As(%v, **fs.PathError) = false, want true", out)
	}
}

// TestDisplayPathErrorCleansJoinedErrors covers the shape PatchInPlace
// returns: a deferred Close joined onto the error of the write itself. The
// join is rebuilt rather than rebuilt-around, so errors.Is still walks every
// part -- and a nil part stays nil, which is what makes a successful Close
// cost nothing.
//
// The prefix only exists on Windows, where stripExtendedPrefix is real; the
// platform-independent assertions below are the ones that must hold either
// way.
func TestDisplayPathErrorCleansJoinedErrors(t *testing.T) {
	writeErr := &fs.PathError{Op: "write", Path: `\\?\C:\data\file`, Err: os.ErrPermission}
	in := errors.Join(writeErr, nil)

	out := displayPathError(in)

	if !errors.Is(out, os.ErrPermission) {
		t.Errorf("errors.Is(%v, os.ErrPermission) = false, want true", out)
	}
	var reached *fs.PathError
	if !errors.As(out, &reached) {
		t.Fatalf("errors.As(%v, **fs.PathError) = false, want true", out)
	}
	if reached.Op != "write" {
		t.Errorf("Op = %q, want %q", reached.Op, "write")
	}
	if !strings.Contains(prepareOSPath(os.TempDir()), `\\?\`) {
		// stripExtendedPrefix is a documented no-op off Windows: the path
		// arrives unchanged, which is exactly what the contract asks for.
		if reached.Path != `\\?\C:\data\file` {
			t.Errorf("Path = %q, want the input unchanged off Windows", reached.Path)
		}
	} else if msg := out.Error(); strings.Contains(msg, `\\?\`) {
		t.Errorf("joined message still carries the extended-length prefix: %q", msg)
	} else if reached.Path != `C:\data\file` {
		t.Errorf("Path = %q, want %q", reached.Path, `C:\data\file`)
	}

	// A join of nothing at all is nil, before and after: a Close that
	// succeeded must not turn into an error.
	if got := displayPathError(errors.Join(nil, nil)); got != nil {
		t.Errorf("displayPathError(errors.Join(nil, nil)) = %v, want nil", got)
	}
}
