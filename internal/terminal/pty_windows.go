//go:build windows

package terminal

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/internal/update"
	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
)

// conPTYAPI is the three ConPTY entry points of one implementation: the
// redistributable f4 downloads into the profile, or the in-box kernel32.dll.
// The struct lets resize and close use whichever API created the console.
type conPTYAPI struct {
	create procer
	resize procer
	close  procer
	path   string // "package:<dll path>" or "system:kernel32.dll"
	// logicalLines records that this ConPTY hands a VT-mode write to the
	// terminal unwrapped, so that the view may reflow what it wraps itself.
	logicalLines bool
}

// procer is the subset of *windows.Proc and *windows.LazyProc that
// conPTYAPI needs: just Call.
type procer interface {
	Call(args ...uintptr) (r1, r2 uintptr, err error)
}

// conPTYHostEnv selects the ConPTY for diagnostics: "system" runs the shell
// in the in-box ConPTY without reflow, "package" makes a test binary use the
// downloaded one as the real program does. Anything else, or nothing, is the
// normal choice.
const conPTYHostEnv = "F4_CONPTY_HOST"

// conPTYFetchWait is how long the first shell waits for the download before
// it starts in the in-box ConPTY instead. The download goes on, and the next
// shell finds the package in place.
const conPTYFetchWait = 5 * time.Second

var (
	// These compatibility hooks retain the pre-package ConPTY loading seam.
	// The production selector below uses the verified profile package, while
	// older Windows tests still place a bundle beside the test working tree.
	conPTYOnce sync.Once
	conPTY     *conPTYAPI
	conPTYErr  error

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

func bundledConPTY() (*conPTYAPI, error) {
	conPTYOnce.Do(func() {
		dir, err := conPTYBundleDirectory()
		if err != nil {
			conPTYErr = err
			return
		}
		api, loadErr := loadConPTYPackage(dir)
		if loadErr != nil {
			conPTYErr = loadErr
			return
		}
		api.path = strings.Replace(api.path, "package:", "bundled:", 1)
		conPTY = api
	})
	return conPTY, conPTYErr
}

// loadConPTYPackage loads conpty.dll from an installed package directory.
// conpty.dll starts OpenConsole.exe from its own directory, so both must be
// there.
func loadConPTYPackage(dir string) (*conPTYAPI, error) {
	dllPath := filepath.Join(dir, "conpty.dll")
	openConsolePath := filepath.Join(dir, "OpenConsole.exe")
	for _, path := range []string{dllPath, openConsolePath} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return nil, fmt.Errorf("ConPTY package is incomplete: %s: %w", path, statErr)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("ConPTY package member is not a regular file: %s", path)
		}
	}

	dll, err := windows.LoadDLL(dllPath)
	if err != nil {
		return nil, fmt.Errorf("load ConPTY package %s: %w", dllPath, err)
	}
	find := func(names ...string) (*windows.Proc, error) {
		for _, name := range names {
			if proc, findErr := dll.FindProc(name); findErr == nil {
				return proc, nil
			}
		}
		return nil, fmt.Errorf("ConPTY package %s exports none of %q", dllPath, names)
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
	return &conPTYAPI{create: create, resize: resize, close: close, path: "package:" + dllPath, logicalLines: true}, nil
}

// inboxConhostPreservingLines lists in-box conhost.exe builds, by the SHA-256
// of %SystemRoot%\\System32\\conhost.exe, known to deliver long lines whole:
// no CRLF at the wrap point, in the live stream and in the repaint after a
// resize. On such a system the shell runs in the in-box ConPTY, with reflow,
// and nothing is downloaded.
//
// 10.0.22000.2538 (x64, KB5031358) was measured on a real machine in issue
// #425: a 65-character echo in a 40-column console arrived as one run
// followed by a CUP, and the repaint after narrowing wrote the line whole
// too. Later builds are not listed because they have not been measured; a
// build that is not listed gets the package. The hash is the file's SHA-256
// as Winbindex records it, so the same build on another architecture is a
// different entry.
var inboxConhostPreservingLines = map[string]string{
	"28daaac4be1f111892aa55db7f148817b497f4a1a65640d77a05a69050eb4910": "10.0.22000.2538 x64",
}

func inboxConhostPreservesLines() bool {
	if len(inboxConhostPreservingLines) == 0 {
		return false
	}
	system, err := windows.GetSystemDirectory()
	if err != nil {
		return false
	}
	sum, err := fileSHA256(filepath.Join(system, "conhost.exe"))
	if err != nil {
		vtui.DebugLog("PTY_WIN: cannot hash the in-box conhost.exe: %v", err)
		return false
	}
	build, ok := inboxConhostPreservingLines[sum]
	if ok {
		vtui.DebugLog("PTY_WIN: in-box conhost.exe %s (%s) keeps long lines whole", sum, build)
	}
	return ok
}

// loadSystemConPTY loads the ConPTY API from kernel32.dll, the in-box API of
// Windows 10 build 1809 and later.
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
	return &conPTYAPI{create: create, resize: resize, close: close, path: "system:kernel32.dll", logicalLines: inboxConhostPreservesLines()}, nil
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

// packageConPTY is the downloaded ConPTY, once it is installed and loaded.
var packageConPTY struct {
	mu      sync.Mutex
	api     *conPTYAPI
	loadErr error
	fetch   chan struct{} // closed when the one download of this process ends
}

// runningUnderGoTest reports whether this is a test binary, which must not
// reach for the network behind the back of the test that started a shell.
func runningUnderGoTest() bool {
	return flag.Lookup("test.v") != nil
}

// conPTYForShell picks the ConPTY a new shell runs in, in this order: the
// in-box one when its conhost.exe is known to deliver long lines whole; the
// downloaded package when it is installed or can be installed within
// conPTYFetchWait; and otherwise the in-box one, without reflow.
func conPTYForShell() (*conPTYAPI, error) {
	system, err := systemConPTY()
	if err == nil && system.logicalLines && os.Getenv(conPTYHostEnv) != "package" {
		return system, nil
	}
	if api := packageConPTYForShell(); api != nil {
		return api, nil
	}
	return system, err
}

func packageConPTYForShell() *conPTYAPI {
	mode := os.Getenv(conPTYHostEnv)
	if mode == "system" {
		vtui.DebugLog("PTY_WIN: %s=system, using the in-box ConPTY", conPTYHostEnv)
		return nil
	}
	if runningUnderGoTest() && mode != "package" {
		return nil
	}
	pkg, ok := conPTYPackageFor(runtime.GOARCH)
	if !ok {
		vtui.DebugLog("PTY_WIN: no ConPTY package for %s", runtime.GOARCH)
		return nil
	}
	dir := conPTYPackageDir(config.GetF4ConfigDir(), pkg)

	if api, done := loadInstalledConPTYPackage(dir, pkg); done {
		return api
	}

	packageConPTY.mu.Lock()
	if packageConPTY.fetch != nil {
		// This process has already tried; a download still running or one
		// that failed is not waited for again.
		packageConPTY.mu.Unlock()
		return nil
	}
	fetch := make(chan struct{})
	packageConPTY.fetch = fetch
	packageConPTY.mu.Unlock()

	go func() {
		defer close(fetch)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		vtui.DebugLog("PTY_WIN: downloading ConPTY %s from %s", pkg.Version, pkg.URL)
		if _, err := installConPTYPackage(ctx, netproxy.HTTPClient(0), config.GetF4ConfigDir(), pkg); err != nil {
			vtui.DebugLog("PTY_WIN: ConPTY download failed, reflow stays off: %v", err)
			return
		}
		vtui.DebugLog("PTY_WIN: ConPTY %s installed in %s", pkg.Version, dir)
	}()

	select {
	case <-fetch:
	case <-time.After(conPTYFetchWait):
		vtui.DebugLog("PTY_WIN: ConPTY download not done within %v; this shell runs in the in-box ConPTY", conPTYFetchWait)
		return nil
	}
	api, _ := loadInstalledConPTYPackage(dir, pkg)
	return api
}

// loadInstalledConPTYPackage returns the loaded package if dir holds it. done
// is true when the answer is final for this process: the package is loaded,
// or it is installed and failed to load, which a download cannot fix.
func loadInstalledConPTYPackage(dir string, pkg conPTYPackage) (*conPTYAPI, bool) {
	packageConPTY.mu.Lock()
	defer packageConPTY.mu.Unlock()
	if packageConPTY.api != nil {
		return packageConPTY.api, true
	}
	if packageConPTY.loadErr != nil {
		return nil, true
	}
	if err := verifyConPTYPackage(dir, pkg); err != nil {
		vtui.DebugLog("PTY_WIN: ConPTY package not installed in %s: %v", dir, err)
		return nil, false
	}
	api, err := loadConPTYPackage(dir)
	if err != nil {
		vtui.DebugLog("PTY_WIN: ConPTY package in %s did not load: %v", dir, err)
		packageConPTY.loadErr = err
		return nil, true
	}
	vtui.DebugLog("PTY_WIN: using ConPTY %s from %s", pkg.Version, dir)
	packageConPTY.api = api
	return api, true
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

// conPTYWorks caches the answer to "can this system actually allocate a
// pseudo console", which is a different question from "does kernel32 export
// the name". The probe runs at most once per process.
var conPTYWorks struct {
	once sync.Once
	ok   bool
}

// probeSystemConPTY allocates a pseudo console and closes it again. A
// resolvable entry point is not a working one: on Windows releases older than
// 10 the three ConPTY names can be present and still refuse the call — that is
// what happens on ReactOS 0.4.16 (NT 5.2), where CreatePseudoConsole answers
// E_NOTIMPL and the shell f4 then starts has nothing behind it. Measured
// there: without this probe ResolveShellMode picked ShellModeOwn and the
// Terminal tab stayed empty, because the mode that needs a PTY had been
// chosen on the strength of a symbol lookup.
func probeSystemConPTY(api *conPTYAPI) bool {
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		vtui.DebugLog("PTY_WIN: ConPTY probe could not make a pipe: %v", err)
		return false
	}
	defer windows.CloseHandle(inRead)
	defer windows.CloseHandle(inWrite)
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		vtui.DebugLog("PTY_WIN: ConPTY probe could not make a pipe: %v", err)
		return false
	}
	defer windows.CloseHandle(outRead)
	defer windows.CloseHandle(outWrite)

	var console windows.Handle
	if err := api.createPseudoConsole(windows.Coord{X: 80, Y: 25}, inRead, outWrite, 0, &console); err != nil {
		vtui.DebugLog("PTY_WIN: ConPTY entry points resolve but do not work: %v", err)
		return false
	}
	api.closePseudoConsole(console)
	return true
}

// ConPTYAvailable checks whether a ConPTY API is reachable at all. The in-box
// API is the test: the package needs the same Windows build, and checking for
// it must not start a download. Older Windows versions remain usable through
// the other console backends.
func ConPTYAvailable() bool {
	if vtui.IsWine() {
		return false
	}
	conPTYWorks.once.Do(func() {
		api, err := systemConPTY()
		if err != nil {
			vtui.DebugLog("PTY_WIN: kernel32.dll ConPTY unavailable: %v", err)
			return
		}
		conPTYWorks.ok = probeSystemConPTY(api)
	})
	return conPTYWorks.ok
}
func isPlatformPTYUsable() bool {
	// ConPTY where it exists; under Wine, a native host pty when libwinescape
	// is allowed and works (pty_wine_windows.go).
	return ConPTYAvailable() || winePTYUsable()
}

// PTY для Windows реализован через ConPTY API (доступно в Windows 10+).
type PTY struct {
	mu        sync.Mutex
	api       *conPTYAPI // which API created this console (package or system)
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

// PreservesLogicalLines reports whether the ConPTY behind this shell passes
// long lines through unwrapped, which is what allows the view to reflow.
func (p *PTY) PreservesLogicalLines() bool {
	return p.api != nil && p.api.logicalLines
}

func NewPTY() (*PTY, error) {
	if vtui.IsWine() {
		return nil, fmt.Errorf("ConPTY is unavailable under Wine")
	}
	api, err := conPTYForShell()
	if err != nil {
		return nil, fmt.Errorf("no ConPTY API available: %w", err)
	}
	return newPTYWithAPI(api)
}

func newPTYWithAPI(api *conPTYAPI) (*PTY, error) {
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
	err := api.createPseudoConsole(size, inPipePty, outPipePty, 0, &console)
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
	if shell, ok := nativeSystemShell(); ok {
		return shell
	}
	shell := os.Getenv("COMSPEC")
	if shell == "" {
		return "cmd.exe"
	}
	return shell
}
