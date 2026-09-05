package main

import (
	"errors"
	"runtime/debug"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// MappedFile is a read-only memory map of a local file. The editor uses it as
// the piece table's original buffer: a mapping is one contiguous []byte, so
// PieceTable.View can hand out a window onto any part of it and a search runs
// over the file itself instead of over a copy assembled for the occasion.
//
// Nothing is read at map time. The kernel faults pages in as they are touched
// and can drop them again under memory pressure without swapping, because they
// are clean and file-backed — which is what lets the editor hold a file larger
// than the memory it is allowed to keep.
type MappedFile struct {
	data   []byte
	mapped []byte
	// offset is how many bytes of the file data starts at. It is what a
	// reader has to add to a position in this buffer to get the position of
	// the same byte in the file.
	offset int64
}

// FileOffset reports where in the file the mapped contents begin: zero, or
// the length of a UTF-8 byte-order mark that is not part of the text.
func (m *MappedFile) FileOffset() int64 {
	if m == nil {
		return 0
	}
	return m.offset
}

// Bytes returns the mapped contents. The result aliases the mapping and must
// not be modified: it is mapped read-only, so a write would fault.
func (m *MappedFile) Bytes() []byte {
	if m == nil {
		return nil
	}
	return m.data
}

// Size reports the mapped length.
func (m *MappedFile) Size() int {
	if m == nil {
		return 0
	}
	return len(m.data)
}

// Close releases the mapping. Every window handed out by Bytes becomes invalid,
// so callers must be done reading before this runs.
func (m *MappedFile) Close() error {
	if m == nil || (m.data == nil && m.mapped == nil) {
		return nil
	}
	data := m.mapped
	if data == nil {
		data = m.data
	}
	m.data = nil
	m.mapped = nil
	return unmapFileRegion(data)
}

// errNotMappable is the ordinary answer for a file the editor should open the
// usual way, not a failure worth reporting.
var errNotMappable = errors.New("file cannot be memory mapped")

// fileDescriptor is what a local vfs.ReadAtCloser exposes through the *os.File
// it embeds. A remote one does not implement it, which is exactly the test the
// editor needs: no descriptor, no mapping.
type fileDescriptor interface {
	Fd() uintptr
}

// MapEditorFile maps an already opened local file, or answers errNotMappable
// when it should not be mapped at all: an empty file has nothing to map, a
// non-local backing has no descriptor to map, and a file too large for the
// address space cannot be described by one slice.
func MapEditorFile(v vfs.VFS, f vfs.ReadAtCloser) (*MappedFile, error) {
	return MapEditorFileWithOffset(v, f, 0)
}

// MapEditorFileWithOffset maps an already opened local file and exposes it
// from fileOffset onward. The complete mapping is retained separately so a
// logical slice that skips a UTF-8 BOM can still be unmapped from its true
// platform-specific base address.
func MapEditorFileWithOffset(v vfs.VFS, f vfs.ReadAtCloser, fileOffset int64) (*MappedFile, error) {
	if f == nil || !isLocalOSVFS(v) {
		return nil, errNotMappable
	}
	size := f.Size()
	if size <= 0 || fileOffset < 0 || fileOffset >= size {
		return nil, errNotMappable
	}
	if int64(int(size)) != size {
		return nil, errNotMappable
	}
	fd, ok := f.(fileDescriptor)
	if !ok || fd.Fd() == 0 || fd.Fd() == ^uintptr(0) {
		return nil, errNotMappable
	}

	data, err := mapFileRegion(fd.Fd(), size)
	if err != nil {
		return nil, err
	}
	return &MappedFile{data: data[fileOffset:], mapped: data, offset: fileOffset}, nil
}

// guardMappedFaults makes a fault on mapped memory recoverable for the calling
// goroutine, and must therefore be called by every goroutine that reads through
// a mapping. A file truncated after it was mapped leaves the pages past the new
// end with nothing behind them, and touching one raises SIGBUS — which Go turns
// into a process-wide crash unless the faulting goroutine has asked for a panic
// instead. The editor would rather lose the buffer than the session.
//
// The flag is per goroutine and is not inherited, so the returned function must
// be deferred by the same goroutine that set it.
func guardMappedFaults(what string, onFault func()) func() {
	previous := debug.SetPanicOnFault(true)
	return func() {
		debug.SetPanicOnFault(previous)
		r := recover()
		if r == nil {
			return
		}
		// A fault is the only panic worth swallowing here; anything else is a
		// bug that should keep travelling.
		if _, ok := r.(runtime_Error); !ok {
			panic(r)
		}
		vtui.DebugLog("EDITOR_MMAP: fault while %s: %v", what, r)
		if onFault != nil {
			onFault()
		}
	}
}

// guardMapping arms fault recovery for the calling goroutine, but only for an
// editor that actually reads through a mapping: everywhere else the zero value
// is a no-op, so an ordinary buffer keeps crashing honestly on a wild address
// instead of quietly swallowing it.
//
// Use it as `defer ev.guardMapping("...")()`, which defers the returned
// function — the one that calls recover.
func (ev *EditorView) guardMapping(what string) func() {
	if ev == nil || ev.mapped == nil {
		return func() {}
	}
	return guardMappedFaults(what, ev.noteMappingFault)
}

// noteMappingFault tells the user their file went out from under the editor.
// A mapping faults when the file it describes is truncated, so the buffer on
// screen no longer matches anything on disk and the only honest move is to say
// so; recovering the text would mean reading through the same broken mapping.
func (ev *EditorView) noteMappingFault() {
	vtui.FrameManager.PostTask(func() {
		if ev.mapFaulted {
			return
		}
		ev.mapFaulted = true
		vtui.ShowMessage(" Error ",
			"The file was truncated on disk while it was open here.\n"+
				"Close the editor and open it again to see its current contents.",
			[]string{"&Ok"})
	})
}

// runtime_Error matches runtime.Error without importing it for its own sake:
// a fault arrives as *runtime.Error (runtime.errorAddressString), while an
// ordinary panic does not implement RuntimeError.
type runtime_Error interface {
	error
	RuntimeError()
}
