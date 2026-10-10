//go:build linux && !android

package vtui

import (
	"math"
	"unsafe"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/shm"
	"github.com/jezek/xgb/xproto"
	"golang.org/x/sys/unix"
)

var (
	shmId   int
	shmAddr uintptr
	shmData []byte
	// shmReady says the segment was created and mapped. shmId cannot say it:
	// 0 is a valid System V shm id (the first segment in a fresh IPC
	// namespace, as on CI runners), and testing shmId > 0 silently disabled
	// MIT-SHM there.
	shmReady bool
)

var shmSetupDone bool

// bytesAtAddress turns the address returned by shmat(2) into a Go slice. The
// address is outside the Go heap, so it cannot be expressed as a Go pointer at
// the syscall boundary; materialize it in a pointer-sized slot here and keep
// the conversion isolated from the rest of the renderer.
//
//go:nocheckptr
func bytesAtAddress(addr uintptr, size int) []byte {
	var p unsafe.Pointer
	*(*uintptr)(unsafe.Pointer(&p)) = addr
	return unsafe.Slice((*byte)(p), size)
}

func setupX11SHM() {
	if shmSetupDone {
		return
	}
	shmSetupDone = true

	// Allocate 32MB segment (sufficient for a 4K display)
	size := 3840 * 2160 * 4

	r1, _, err := unix.Syscall(unix.SYS_SHMGET, uintptr(unix.IPC_PRIVATE), uintptr(size), uintptr(unix.IPC_CREAT|0600))
	if err != 0 {
		DebugLog("X11: shmget failed: %v", err)
		return
	}
	id := int(r1)

	r1, _, err = unix.Syscall(unix.SYS_SHMAT, uintptr(id), 0, 0)
	if err != 0 {
		unix.Syscall(unix.SYS_SHMCTL, uintptr(id), uintptr(unix.IPC_RMID), 0)
		DebugLog("X11: shmat failed: %v", err)
		return
	}
	addr := r1

	shmId = id
	shmAddr = addr
	shmData = bytesAtAddress(shmAddr, size)
	shmReady = true
	DebugLog("X11: Allocated shared memory segment (ID: %d)", shmId)
}

// x11shmInit attaches the segment on the server side and returns its id, or 0
// when MIT-SHM is unavailable. The attach is checked: a server that cannot
// map the segment (another IPC namespace, a sandboxed Xwayland) reports that
// here, and the host then draws through core PutImage from the start instead
// of discovering it on the first frame (f4 #1626).
func x11shmInit(conn *xgb.Conn, id int) uint32 {
	if err := shm.Init(conn); err != nil {
		DebugLog("X11: MIT-SHM extension unavailable: %v", err)
		return 0
	}
	// A SysV shm id is a non-negative C int; anything else is not a segment
	// the server could attach.
	if id < 0 || int64(id) > math.MaxUint32 {
		DebugLog("X11: MIT-SHM segment id %d out of range", id)
		return 0
	}
	seg, err := shm.NewSegId(conn)
	if err != nil {
		DebugLog("X11: MIT-SHM segment id allocation failed: %v", err)
		return 0
	}
	if err := shm.AttachChecked(conn, seg, uint32(id), false).Check(); err != nil {
		DebugLog("X11: MIT-SHM attach failed: %v", err)
		return 0
	}
	return uint32(seg)
}

func x11shmDetach(conn *xgb.Conn, seg uint32) {
	shm.Detach(conn, shm.Seg(seg))
}

// x11shmMajorOpcode is the major opcode the server assigned to MIT-SHM, so
// that an asynchronous X error can be traced back to a ShmPutImage; 0 when
// the extension was not initialized.
func x11shmMajorOpcode(conn *xgb.Conn) byte {
	if conn == nil {
		return 0
	}
	conn.ExtLock.RLock()
	defer conn.ExtLock.RUnlock()
	return conn.Extensions["MIT-SHM"]
}

// x11shmPutImage sends rows minY..maxY of the w x h2 image in the segment.
// With checked set it waits for the server's verdict and returns the X error,
// if any; otherwise the request is fire-and-forget and an error arrives later
// through WaitForEvent.
func x11shmPutImage(conn *xgb.Conn, wid xproto.Window, gc xproto.Gcontext, w, h2 uint16, minY, maxY int, depth byte, seg uint32, checked bool) error {
	put := shm.PutImage
	if checked {
		put = shm.PutImageChecked
	}
	cookie := put(conn, xproto.Drawable(wid), gc,
		w, h2,
		0, uint16(minY),
		w, uint16(maxY-minY+1),
		0, int16(minY),
		depth, xproto.ImageFormatZPixmap, 0,
		shm.Seg(seg), 0)
	if checked {
		return cookie.Check()
	}
	return nil
}
