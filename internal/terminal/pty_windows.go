//go:build windows

package terminal

import (
	"fmt"
	"github.com/unxed/f4/internal/update"
	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"
)

// conPTYAPI wraps either the bundled ConPTY redistributable shipped next to
// f4.exe or the in-box kernel32.dll API. Both expose the same three procs;
// the struct lets PTY resize/close use whichever API created it.
type conPTYAPI struct {
	create procer
	resize procer
	close  procer
	path   string // "bundled:<dll path>" or "system:kernel32.dll"
}

// procer is the subset of *windows.Proc and *windows.LazyProc that
// conPTYAPI needs: just Call.
type procer interface {
	Call(args ...uintptr) (r1, r2 uintptr, err error)
}

var (
	conPTYOnce sync.Once
	conPTY     *conPTYAPI
	conPTYErr  error

	// Tests run from the package source directory while the test executable
	// itself lives in Go's temporary build directory. The test-only init in
	// pty_windows_test.go points this hook at that source directory; normal
	// binaries always resolve the bundle next to their own executable.
	conPTYBundleDirectoryOverride func() (string, error)
)

func conPTYBundleDirectory() (string, error) {
	if conPTYBundleDirectoryOverride != nil {
		return conPTYBundleDirectoryOverride()
	}
	exe, err := update.Executable()
	if err != nil {
		return "", fmt.Errorf("find f4 executable: %w", err)
	}
	return filepath.Dir(exe), nil
}

func loadConPTY() (*conPTYAPI, error) {
	dir, err := conPTYBundleDirectory()
	if err != nil {
		return nil, err
	}
	dllPath := filepath.Join(dir, "conpty.dll")
	openConsolePath := filepath.Join(dir, "OpenConsole.exe")
	for _, path := range []string{dllPath, openConsolePath} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, fmt.Errorf("ConPTY bundle is incomplete: %s: %w", path, statErr)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("ConPTY bundle member is not a regular file: %s", path)
		}
	}

	dll, err := windows.LoadDLL(dllPath)
	if err != nil {
		return nil, fmt.Errorf("load bundled ConPTY %s: %w", dllPath, err)
	}
	find := func(names ...string) (*windows.Proc, error) {
		for _, name := range names {
			if proc, findErr := dll.FindProc(name); findErr == nil {
				return proc, nil
			}
		}
		return nil, fmt.Errorf("bundled ConPTY %s exports none of %q", dllPath, names)
	}
	create, err := find("ConptyCreatePseudoConsole", "CreatePseudoConsole")
	if err != nil {
		return nil, err
	}
	resize, err := find("ConptyResizePseudoConsole", "ResizePseudoConsole")
	if err != nil {
		return nil, err
	}
	close, err := find("ConptyClosePseudoConsole", "ClosePseudoConsole")
	if err != nil {
		return nil, err
	}
	return &conPTYAPI{create: create, resize: resize, close: close, path: "bundled:" + dllPath}, nil
}

// loadSystemConPTY loads the ConPTY API from kernel32.dll. This is the
// in-box API available on Windows 10 build 1809+. It works but may shred
// long logical lines; the bundled conpty.dll is preferred when present.
func loadSystemConPTY() (*conPTYAPI, error) {
	mod := windows.NewLazySystemDLL("kernel32.dll")
	create := mod.NewProc("CreatePseudoConsole")
	resize := mod.NewProc("ResizePseudoConsole")
	close := mod.NewProc("ClosePseudoConsole")
	for _, proc := range []*windows.LazyProc{create, resize, close} {
		if proc.Find() != nil {
			return nil, fmt.Errorf("kernel32.dll ConPTY procs not all available")
		}
	}
	return &conPTYAPI{create: create, resize: resize, close: close, path: "system:kernel32.dll"}, nil
}

func bundledConPTY() (*conPTYAPI, error) {
	conPTYOnce.Do(func() {
		conPTY, conPTYErr = loadConPTY()
	})
	return conPTY, conPTYErr
}

var (
	systemPTYOnce sync.Once
	systemPTY     *conPTYAPI
	systemPTYErr  error
)

func systemConPTY() (*conPTYAPI, error) {
	systemPTYOnce.Do(func() {
		systemPTY, systemPTYErr = loadSystemConPTY()
	})
	return systemPTY, systemPTYErr
}

func packedConPTYCoord(size windows.Coord) uintptr {
	return uintptr(*(*uint32)(unsafe.Pointer(&size)))
}

func (api *conPTYAPI) createPseudoConsole(size windows.Coord, in, out windows.Handle, flags uint32, console *windows.Handle) error {
	hr, _, callErr := api.create.Call(
		packedConPTYCoord(size), uintptr(in), uintptr(out), uintptr(flags), uintptr(unsafe.Pointer(console)),
	)
	if hr != 0 {
		return fmt.Errorf("%s!CreatePseudoConsole failed with HRESULT 0x%08x: %w", api.path, uint32(hr), callErr)
	}
	return nil
}

func (api *conPTYAPI) resizePseudoConsole(console windows.Handle, size windows.Coord) error {
	hr, _, callErr := api.resize.Call(uintptr(console), packedConPTYCoord(size))
	if hr != 0 {
		return fmt.Errorf("%s!ResizePseudoConsole failed with HRESULT 0x%08x: %w", api.path, uint32(hr), callErr)
	}
	return nil
}

func (api *conPTYAPI) closePseudoConsole(console windows.Handle) {
	_, _, _ = api.close.Call(uintptr(console))
}

// ConPTYAvailable checks whether any ConPTY API is reachable: the bundled
// redistributable is preferred (correct long-line handling), the in-box
// kernel32.dll API is the fallback. Older Windows versions remain usable
// through the other console backends.
func ConPTYAvailable() bool {
	if vtui.IsWine() {
		return false
	}
	_, err := bundledConPTY()
	if err == nil {
		return true
	}
	vtui.DebugLog("PTY_WIN: bundled ConPTY unavailable: %v — trying kernel32.dll fallback", err)
	if _, err := systemConPTY(); err != nil {
		vtui.DebugLog("PTY_WIN: kernel32.dll ConPTY also unavailable: %v", err)
		return false
	}
	return true
}
func isPlatformPTYUsable() bool {
	return ConPTYAvailable()
}

// PTY для Windows реализован через ConPTY API (доступно в Windows 10+).
type PTY struct {
	mu        sync.Mutex
	api       *conPTYAPI // which API created this console (bundled or system)
	console   windows.Handle
	inPipe    windows.Handle
	outPipe   windows.Handle
	process   *windows.ProcessInformation
	inWriter  *os.File
	outReader *os.File

	lastBusyCheck    time.Time
	lastBusyState    bool
	busyCheckPending bool

	// consoleClosed records that ClosePseudoConsole has run, whether from
	// Close or from the exit watcher, so the two never close it twice.
	consoleClosed bool

	Cmd *exec.Cmd //for interface compatability with Pty_unix, DO NOT USE
}

func NewPTY() (*PTY, error) {
	if vtui.IsWine() {
		return nil, fmt.Errorf("ConPTY is unavailable under Wine")
	}
	// Prefer the bundled ConPTY (correct long-line handling); fall back to
	// the kernel32.dll in-box API so that users without the bundle still get
	// a real PTY instead of ShellModeSimpleInline.
	api, err := bundledConPTY()
	if err != nil {
		vtui.DebugLog("PTY_WIN: bundled ConPTY unavailable: %v — falling back to kernel32.dll", err)
		api, err = systemConPTY()
		if err != nil {
			return nil, fmt.Errorf("no ConPTY API available (bundled or system): %w", err)
		}
	}

	var inPipeOur, inPipePty windows.Handle
	var outPipeOur, outPipePty windows.Handle

	// Создаем пайпы для ввода-вывода (CreatePipe: readHandle, writeHandle)
	// inPipe: PTY читает, мы пишем
	if err := windows.CreatePipe(&inPipePty, &inPipeOur, nil, 0); err != nil {
		return nil, err
	}
	// outPipe: мы читаем, PTY пишет
	if err := windows.CreatePipe(&outPipeOur, &outPipePty, nil, 0); err != nil {
		windows.CloseHandle(inPipePty)
		windows.CloseHandle(inPipeOur)
		return nil, err
	}

	// Создаем псевдоконсоль
	var console windows.Handle
	size := windows.Coord{X: 80, Y: 24}
	err = api.createPseudoConsole(size, inPipePty, outPipePty, 0, &console)
	if err != nil {
		windows.CloseHandle(inPipePty)
		windows.CloseHandle(inPipeOur)
		windows.CloseHandle(outPipePty)
		windows.CloseHandle(outPipeOur)
		return nil, fmt.Errorf("failed to create pseudo console: %w (requires Windows 10+)", err)
	}

	// Закрываем наши копии хэндлов PTY, чтобы EOF корректно передавался при закрытии дочернего процесса
	windows.CloseHandle(inPipePty)
	windows.CloseHandle(outPipePty)

	return &PTY{
		api:       api,
		console:   console,
		inPipe:    inPipeOur,
		outPipe:   outPipeOur,
		inWriter:  os.NewFile(uintptr(inPipeOur), "|in"),
		outReader: os.NewFile(uintptr(outPipeOur), "|out"),
	}, nil
}

func (p *PTY) Write(b []byte) (int, error) {
	vtui.DebugLog("PTY_WIN_TRACE: Writing %d bytes: %q", len(b), string(b))
	return p.inWriter.Write(b)
}

func (p *PTY) Read(b []byte) (int, error) {
	n, err := p.outReader.Read(b)
	if n > 0 {
		vtui.DebugLog("PTY_WIN_TRACE: Read %d bytes: %q", n, string(b[:n]))
	}
	return n, err
}

func (p *PTY) SetSize(cols, rows int) {
	_ = p.SetSizeChecked(cols, rows)
}

func (p *PTY) SetSizeChecked(cols, rows int) error {
	// Ignore minimized/invalid dimensions before touching ConPTY.
	if cols <= 0 || rows <= 0 {
		vtui.DebugLog("PTY_WIN_SIZE: resize to %dx%d ignored (non-positive)", cols, rows)
		return nil
	}
	// COORD carries int16; anything above that cannot be what the host
	// window measures, and silently truncating it would hand the child a
	// different width than the one f4 lays its own screen out for (#907).
	if cols > 0x7FFF || rows > 0x7FFF {
		vtui.DebugLog("PTY_WIN_SIZE: resize to %dx%d ignored (exceeds COORD)", cols, rows)
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.consoleClosed {
		vtui.DebugLog("PTY_WIN_SIZE: resize to %dx%d ignored (console closed)", cols, rows)
		return nil
	}
	// The child wraps its output at the width recorded here, so when its
	// lines break earlier than the window edge (#907) this is the line to
	// compare against REFLOW_PTY and FM_RESIZE. The HRESULT used to be
	// dropped on the floor; a refused resize left the pseudoconsole at its
	// previous size with nothing in the log to say so.
	err := p.api.resizePseudoConsole(p.console, windows.Coord{X: int16(cols), Y: int16(rows)})
	if err != nil {
		vtui.DebugLog("PTY_WIN_SIZE: ResizePseudoConsole(%dx%d) failed: %v", cols, rows, err)
		return err
	}
	vtui.DebugLog("PTY_WIN_SIZE: ResizePseudoConsole(%dx%d) ok", cols, rows)
	return nil
}

func (p *PTY) Run(name string, args ...string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	cmdLine := windows.StringToUTF16Ptr(name)

	var attrList *windows.ProcThreadAttributeListContainer
	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return err
	}

	err = attrList.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(p.console), unsafe.Sizeof(p.console))
	if err != nil {
		return err
	}

	si := &windows.StartupInfoEx{
		StartupInfo: windows.StartupInfo{
			Cb:    uint32(unsafe.Sizeof(windows.StartupInfoEx{})),
			Flags: windows.STARTF_USESTDHANDLES,
		},
		ProcThreadAttributeList: attrList.List(),
	}

	pi := &windows.ProcessInformation{}
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	env := utf16.Encode([]rune(strings.Join(TerminalChildEnv(), "\x00") + "\x00"))
	env = append(env, 0)

	err = windows.CreateProcess(nil, cmdLine, nil, nil, false, flags, &env[0], nil, &si.StartupInfo, pi)
	if err != nil {
		return err
	}

	p.process = pi
	p.lastBusyCheck = time.Time{}
	p.lastBusyState = false
	p.busyCheckPending = false
	p.watchExit(pi.Process)
	return nil
}

// watchExit closes the pseudoconsole once the shell process is gone, so that
// Read returns EOF the way a Unix Pty master does when its shell exits.
//
// ConPTY does not do this by itself: conhost keeps the output pipe open after
// the client process has exited, until ClosePseudoConsole is called. Without
// the watcher, `exit` inside a batch file (which ends cmd.exe itself, unlike
// `exit /b`) left f4 reading a pipe that would never deliver another byte:
// no prompt could arrive, the panels stayed hidden behind a shell that no
// longer existed, and neither Ctrl+C nor Ctrl+Break had anyone to reach
// (issue #409).
//
// The watcher waits on its own duplicate of the process handle, so Close
// releasing the original cannot pull the handle out from under the wait.
func (p *PTY) watchExit(process windows.Handle) {
	var dup windows.Handle
	self := windows.CurrentProcess()
	if err := windows.DuplicateHandle(self, process, self, &dup, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		vtui.DebugLog("PTY_WIN: cannot watch the shell process for exit: %v", err)
		return
	}
	go func() {
		defer windows.CloseHandle(dup)
		if _, err := windows.WaitForSingleObject(dup, windows.INFINITE); err != nil {
			vtui.DebugLog("PTY_WIN: waiting for the shell process failed: %v", err)
			return
		}
		vtui.DebugLog("PTY_WIN: shell process exited, closing the pseudoconsole")
		p.closeConsole()
	}()
}

// closeConsole runs ClosePseudoConsole once. The read loop must keep
// draining the output pipe meanwhile: ClosePseudoConsole flushes conhost's
// remaining output and does not return until it has been read.
func (p *PTY) closeConsole() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.consoleClosed {
		return
	}
	p.consoleClosed = true
	p.api.closePseudoConsole(p.console)
}

func (p *PTY) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.process != nil {
		windows.TerminateProcess(p.process.Process, 0)
		// TerminateProcess only asks; the process is gone a little later.
		// Wait for it (bounded), so that whatever it held -- its working
		// directory above all -- is released by the time Close returns.
		windows.WaitForSingleObject(p.process.Process, 2000)
		windows.CloseHandle(p.process.Process)
		windows.CloseHandle(p.process.Thread)
		p.process = nil
	}
	if !p.consoleClosed {
		p.consoleClosed = true
		p.api.closePseudoConsole(p.console)
	}
	p.inWriter.Close()
	p.outReader.Close()
	return nil
}

func (p *PTY) Wait() error {
	if p.process == nil {
		return nil
	}
	_, err := windows.WaitForSingleObject(p.process.Process, windows.INFINITE)
	return err
}

func (p *PTY) IsBusy() bool {
	p.mu.Lock()
	if p.process == nil {
		p.mu.Unlock()
		return false
	}

	state := p.lastBusyState
	// Enumerating every Windows process costs several milliseconds on a busy
	// machine. IsBusy runs on the UI thread during layout, hotkey checks and
	// semantic projection, so a synchronous refresh used to stall otherwise
	// tiny cursor/menu/surface updates. Keep the existing one-second cache but
	// refresh it off-thread; callers receive the last known state immediately.
	if time.Since(p.lastBusyCheck) < time.Second || p.busyCheckPending {
		p.mu.Unlock()
		return state
	}
	processHandle := p.process.Process
	processID := p.process.ProcessId
	p.busyCheckPending = true
	probe := windowsPTYBusyProbe
	p.mu.Unlock()

	go p.refreshBusyState(processHandle, processID, probe)
	return state
}

type windowsPTYBusyProbeFunc func(windows.Handle, uint32) bool

var windowsPTYBusyProbe windowsPTYBusyProbeFunc = probeWindowsPTYBusy

func probeWindowsPTYBusy(processHandle windows.Handle, processID uint32) bool {
	var exitCode uint32
	if err := windows.GetExitCodeProcess(processHandle, &exitCode); err != nil || exitCode != 259 { // 259 = STILL_ACTIVE
		return false
	}

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)

	var pe32 windows.ProcessEntry32
	pe32.Size = uint32(unsafe.Sizeof(pe32))

	if err := windows.Process32First(snapshot, &pe32); err != nil {
		return false
	}

	for {
		if pe32.ParentProcessID == processID {
			return true
		}
		if err := windows.Process32Next(snapshot, &pe32); err != nil {
			break
		}
	}

	return false
}

func (p *PTY) refreshBusyState(processHandle windows.Handle, processID uint32, probe windowsPTYBusyProbeFunc) {
	state := probe(processHandle, processID)

	p.mu.Lock()
	p.busyCheckPending = false
	if p.process == nil || p.process.Process != processHandle || p.process.ProcessId != processID {
		p.mu.Unlock()
		return
	}
	changed := p.lastBusyState != state
	p.lastBusyState = state
	p.lastBusyCheck = time.Now()
	p.mu.Unlock()

	if changed && vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

func GetSystemShell() string {
	shell := os.Getenv("COMSPEC")
	if shell == "" {
		return "cmd.exe"
	}
	return shell
}
