package main

/*
#include <stddef.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

typedef struct vtui_session vtui_session;
*/
import "C"
import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sync"
	"unsafe"

	"github.com/unxed/vtui"
)

type cSession struct {
	id       uintptr
	fm       *vtui.FrameManagerType
	session  *vtui.ProtocolSession
	inPipeR  *io.PipeReader
	inPipeW  *io.PipeWriter
	outPipeR *io.PipeReader
	outPipeW *io.PipeWriter
	reader   *bufio.Reader
	eventFd  int
	eventW   *os.File
	eventR   *os.File
}

var (
	sessMu   sync.Mutex
	sessions = make(map[uintptr]*cSession)
	errMu    sync.RWMutex
	lastErr  string
)

func setLastError(err string) {
	errMu.Lock()
	lastErr = err
	errMu.Unlock()
}

//export vtui_last_error
func vtui_last_error() *C.char {
	errMu.RLock()
	defer errMu.RUnlock()
	return C.CString(lastErr)
}

//export vtui_open
func vtui_open(configJSON *C.char) *C.vtui_session {
	cfgStr := "{}"
	if configJSON != nil {
		cfgStr = C.GoString(configJSON)
	}

	var cfg struct {
		Backend string `json:"backend"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
	}
	_ = json.Unmarshal([]byte(cfgStr), &cfg)

	w := cfg.Width
	if w <= 0 {
		w = 80
	}
	h := cfg.Height
	if h <= 0 {
		h = 25
	}

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(w, h)
	fm := vtui.NewFrameManager()
	fm.Init(scr)

	inR, inW := io.Pipe()
	outR, outW := io.Pipe()

	eventR, eventW, err := os.Pipe()
	eventFd := -1
	if err == nil {
		eventFd = int(eventR.Fd())
	}

	sess := vtui.NewProtocolSession(inR, outW, fm)
	go func() {
		_ = sess.Serve()
	}()

	// Keep the opaque handle in C-owned memory. Returning a fabricated pointer
	// from an integer handle triggers go vet and is not a valid C pointer.
	token := C.malloc(1)
	if token == nil {
		setLastError("failed to allocate session token")
		return nil
	}
	id := uintptr(token)
	sessMu.Lock()
	cs := &cSession{
		id:       id,
		fm:       fm,
		session:  sess,
		inPipeR:  inR,
		inPipeW:  inW,
		outPipeR: outR,
		outPipeW: outW,
		reader:   bufio.NewReader(outR),
		eventFd:  eventFd,
		eventR:   eventR,
		eventW:   eventW,
	}
	sessions[id] = cs
	sessMu.Unlock()

	return (*C.vtui_session)(token)
}

//export vtui_send
func vtui_send(s *C.vtui_session, line *C.char, length C.size_t) C.int {
	sessMu.Lock()
	cs, ok := sessions[uintptr(unsafe.Pointer(s))]
	sessMu.Unlock()
	if !ok {
		setLastError("invalid session pointer")
		return -1
	}

	goBytes := C.GoBytes(unsafe.Pointer(line), C.int(length))
	if length == 0 || goBytes[length-1] != '\n' {
		goBytes = append(goBytes, '\n')
	}
	_, err := cs.inPipeW.Write(goBytes)
	if err != nil {
		setLastError(err.Error())
		return -1
	}
	return 0
}

//export vtui_event_fd
func vtui_event_fd(s *C.vtui_session) C.int {
	sessMu.Lock()
	cs, ok := sessions[uintptr(unsafe.Pointer(s))]
	sessMu.Unlock()
	if !ok {
		return -1
	}
	return C.int(cs.eventFd)
}

//export vtui_recv
func vtui_recv(s *C.vtui_session, buf *C.char, capacity C.size_t, outLen *C.size_t) C.int {
	sessMu.Lock()
	cs, ok := sessions[uintptr(unsafe.Pointer(s))]
	sessMu.Unlock()
	if !ok {
		setLastError("invalid session pointer")
		return -1
	}

	line, err := cs.reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		setLastError(err.Error())
		return -1
	}

	lineBytes := []byte(line)
	if C.size_t(len(lineBytes)) > capacity {
		lineBytes = lineBytes[:capacity]
	}

	C.memcpy(unsafe.Pointer(buf), unsafe.Pointer(unsafe.SliceData(lineBytes)), C.size_t(len(lineBytes)))
	if outLen != nil {
		*outLen = C.size_t(len(lineBytes))
	}
	return 0
}

//export vtui_close
func vtui_close(s *C.vtui_session) {
	sessMu.Lock()
	id := uintptr(unsafe.Pointer(s))
	cs, ok := sessions[id]
	if ok {
		delete(sessions, id)
	}
	sessMu.Unlock()

	if ok {
		cs.session.Close()
		_ = cs.inPipeW.Close()
		_ = cs.outPipeW.Close()
		if cs.eventR != nil {
			_ = cs.eventR.Close()
		}
		if cs.eventW != nil {
			_ = cs.eventW.Close()
		}
		C.free(unsafe.Pointer(s))
	}
}

func main() {}
