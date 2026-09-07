//go:build windows

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	versionDLL              = syscall.NewLazyDLL("version.dll")
	getFileVersionInfoSizeW = versionDLL.NewProc("GetFileVersionInfoSizeW")
	getFileVersionInfoW     = versionDLL.NewProc("GetFileVersionInfoW")
	verQueryValueW          = versionDLL.NewProc("VerQueryValueW")
	wcsicmp                 = syscall.NewLazyDLL("msvcrt.dll").NewProc("_wcsicmp")
)

func readHostProductVersion(path string) (string, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	size, _, callErr := getFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(name)), 0)
	if size == 0 {
		return "", fmt.Errorf("GetFileVersionInfoSizeW(%q): %v", path, callErr)
	}
	data := make([]byte, size)
	if ok, _, callErr := getFileVersionInfoW.Call(uintptr(unsafe.Pointer(name)), 0, size, uintptr(unsafe.Pointer(&data[0]))); ok == 0 {
		return "", fmt.Errorf("GetFileVersionInfoW(%q): %v", path, callErr)
	}
	for _, key := range []string{`\StringFileInfo\040904b0\ProductVersion`, `\StringFileInfo\000004b0\ProductVersion`} {
		keyPtr, err := syscall.UTF16PtrFromString(key)
		if err != nil {
			return "", err
		}
		var value uintptr
		var valueLen uint32
		if ok, _, _ := verQueryValueW.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(keyPtr)), uintptr(unsafe.Pointer(&value)), uintptr(unsafe.Pointer(&valueLen))); ok != 0 && valueLen > 0 {
			return syscall.UTF16ToString((*[1 << 20]uint16)(unsafe.Pointer(value))[:valueLen:valueLen]), nil
		}
	}
	return "", fmt.Errorf("ProductVersion resource is unavailable in %q", path)
}

// The following is the Windows-only transcription of the pinned
// src/winconpty/winconpty.cpp path.  It deliberately does not call the public
// CreatePseudoConsole export: pinned OpenConsole's outside-recipient path
// creates the ConDrv server and reference handles itself, then starts the
// adjacent host with the exact --headless arguments.
// Source symbols: _ConsoleHostPath, _CreatePseudoConsole,
// _ResizePseudoConsole, _ClosePseudoConsoleMembers.
const (
	ptySignalResizeWindow       = 8
	ptySignalClearWindow        = 2
	pseudoconsoleResizeQuirk    = 0x2
	pseudoconsoleWin32InputMode = 0x4
	fileSynchronousIO           = 0x00000020
	objInherit                  = 0x00000002
	objCaseInsensitive          = 0x00000040
	ntStatusSuccess             = 0
	ntGenericRead               = 0x80000000
	ntGenericWrite              = 0x40000000
	ntGenericAll                = 0x10000000
	ntSynchronize               = 0x00100000
	maxWaitMilliseconds         = 120000
)

type unicodeString struct {
	length        uint16
	maximumLength uint16
	buffer        *uint16
}

type objectAttributes struct {
	length                   uint32
	rootDirectory            windows.Handle
	objectName               *unicodeString
	attributes               uint32
	securityDescriptor       uintptr
	securityQualityOfService uintptr
}

type ioStatusBlock struct {
	status      int32
	_pad        uint32
	information uintptr
}

var ntOpenFile = syscall.NewLazyDLL("ntdll.dll").NewProc("NtOpenFile")

// createDeviceHandle is DeviceHandle::_CreateHandle with the same NT object
// namespace, access masks, inherit flag, share mask, and synchronous option.
func createDeviceHandle(name string, access uint32, parent windows.Handle, inheritable bool, options uint32) (windows.Handle, error) {
	name16, err := windows.UTF16FromString(name)
	if err != nil {
		return 0, err
	}
	nameValue := unicodeString{
		length:        uint16((len(name16) - 1) * 2),
		maximumLength: uint16(len(name16) * 2),
		buffer:        &name16[0],
	}
	attrs := objectAttributes{
		length:        uint32(unsafe.Sizeof(objectAttributes{})),
		rootDirectory: parent,
		objectName:    &nameValue,
		attributes:    objCaseInsensitive,
	}
	if inheritable {
		attrs.attributes |= objInherit
	}
	var handle windows.Handle
	var status ioStatusBlock
	r, _, callErr := ntOpenFile.Call(
		uintptr(unsafe.Pointer(&handle)),
		uintptr(access),
		uintptr(unsafe.Pointer(&attrs)),
		uintptr(unsafe.Pointer(&status)),
		uintptr(windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE),
		uintptr(options),
	)
	if int32(r) < ntStatusSuccess {
		return 0, fmt.Errorf("NtOpenFile(%q) returned NTSTATUS 0x%08x: %v", name, uint32(r), callErr)
	}
	return handle, nil
}

// createServerHandle and createClientHandle mirror the two public
// DeviceHandle.cpp wrappers; there is no fallback to a different device or
// host implementation.
// Source symbols: DeviceHandle::_CreateHandle, DeviceHandle::CreateServerHandle,
// DeviceHandle::CreateClientHandle, WinNTControl::NtOpenFile, class WinNTControl.
func createServerHandle() (windows.Handle, error) {
	return createDeviceHandle(`\Device\ConDrv\Server`, ntGenericAll, 0, true, 0)
}

func createClientHandle(server windows.Handle, name string, inheritable bool) (windows.Handle, error) {
	return createDeviceHandle(name, ntGenericRead|ntGenericWrite|ntSynchronize, server, inheritable, fileSynchronousIO)
}

type pinnedConPTY struct {
	// The first three fields are the exact layout of the pinned
	// `PseudoConsole` struct. The packed pointer is passed to
	// PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE as an HPCON.
	signal          windows.Handle
	ptyReference    windows.Handle
	hostProcess     windows.Handle
	childProcess    windows.Handle
	hostPID         uint32
	childPID        uint32
	input           windows.Handle
	output          windows.Handle
	hostCommandLine string
	// ConptyConnection::_guid is created once for the connection and reused
	// when _LaunchAttachedClient constructs WT_SESSION.
	session string
}

func duplicateInheritable(handle windows.Handle) (windows.Handle, error) {
	var duplicate windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), handle, windows.CurrentProcess(), &duplicate, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return 0, err
	}
	return duplicate, nil
}

func (p *pinnedConPTY) close() {
	if p == nil {
		return
	}
	// This order is the pinned _ClosePseudoConsoleMembers order: break the
	// signal pipe, wait for host output flushing, then close reference.
	if p.signal != 0 {
		_ = windows.CloseHandle(p.signal)
		p.signal = 0
	}
	if p.hostProcess != 0 {
		var exitCode uint32
		if windows.GetExitCodeProcess(p.hostProcess, &exitCode) == nil && exitCode == 259 { // STILL_ACTIVE
			// A healthy host exits after the signal handle is closed.  Keep that
			// flush opportunity bounded: this cleanup also runs on timeout and
			// must never turn a failed probe into an unkillable test process.
			const hostFlushTimeout = 10 * 1000
			if event, _ := windows.WaitForSingleObject(p.hostProcess, hostFlushTimeout); event != windows.WAIT_OBJECT_0 {
				_ = windows.TerminateProcess(p.hostProcess, 1)
				_, _ = windows.WaitForSingleObject(p.hostProcess, 5000)
			}
		}
		_ = windows.TerminateProcess(p.hostProcess, 0)
		_ = windows.CloseHandle(p.hostProcess)
		p.hostProcess = 0
	}
	if p.ptyReference != 0 {
		_ = windows.CloseHandle(p.ptyReference)
		p.ptyReference = 0
	}
}

// processExited checks the terminal state after the native close sequence.
// Querying by PID is intentionally performed only for the process created by
// this session; it is not a substitute host and does not inspect unrelated
// system processes.
func processExited(pid uint32) (bool, error) {
	if pid == 0 {
		return false, fmt.Errorf("process id is zero")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		if err == windows.ERROR_INVALID_PARAMETER {
			return true, nil
		}
		return false, err
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false, err
	}
	return code != 259, nil // STILL_ACTIVE
}

func (p *pinnedConPTY) closePipes() {
	if p == nil {
		return
	}
	if p.input != 0 {
		_ = windows.CloseHandle(p.input)
		p.input = 0
	}
	if p.output != 0 {
		_ = windows.CloseHandle(p.output)
		p.output = 0
	}
}

func waitPinnedClient(pty *pinnedConPTY) error {
	if pty.childProcess == 0 {
		return fmt.Errorf("attached client process was not created")
	}
	event, err := windows.WaitForSingleObject(pty.childProcess, maxWaitMilliseconds)
	if err != nil {
		return fmt.Errorf("WaitForSingleObject(attached client): %w", err)
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("attached client did not finish within %d ms", maxWaitMilliseconds)
	}
	_ = windows.CloseHandle(pty.childProcess)
	pty.childProcess = 0
	return nil
}

func createPinnedPseudoConsole(hostPath string, width, height int) (*pinnedConPTY, error) {
	return createPinnedPseudoConsoleWithQuirk(hostPath, width, height, true)
}

func createPinnedPseudoConsoleWithQuirk(hostPath string, width, height int, resizeQuirk bool) (*pinnedConPTY, error) {
	// _CreatePseudoConsole rejects a null PseudoConsole and zero dimensions
	// before opening any ConDrv or pipe handles.
	if width == 0 || height == 0 {
		return nil, fmt.Errorf("_CreatePseudoConsole: invalid dimensions %dx%d", width, height)
	}
	guid, err := windows.GenerateGUID()
	if err != nil {
		return nil, fmt.Errorf("CoCreateGuid: %w", err)
	}
	// Utils::GuidToString formats the GUID with braces; _LaunchAttachedClient
	// removes those braces before assigning WT_SESSION.
	session := strings.Trim(guid.String(), "{}")
	server, err := createServerHandle()
	if err != nil {
		return nil, fmt.Errorf("DeviceHandle::CreateServerHandle: %w", err)
	}
	closeServer := true
	defer func() {
		if closeServer {
			_ = windows.CloseHandle(server)
		}
	}()

	var inPseudo, inOur windows.Handle
	var outOur, outPseudo windows.Handle
	if err := windows.CreatePipe(&inPseudo, &inOur, nil, 0); err != nil {
		return nil, fmt.Errorf("CreatePipe(input): %w", err)
	}
	if err := windows.CreatePipe(&outOur, &outPseudo, nil, 0); err != nil {
		_ = windows.CloseHandle(inPseudo)
		_ = windows.CloseHandle(inOur)
		return nil, fmt.Errorf("CreatePipe(output): %w", err)
	}
	closePipeHandles := true
	defer func() {
		if closePipeHandles {
			for _, handle := range []windows.Handle{inPseudo, inOur, outOur, outPseudo} {
				if handle != 0 {
					_ = windows.CloseHandle(handle)
				}
			}
		}
	}()
	var signalHost, signalOur windows.Handle
	if err := windows.CreatePipe(&signalHost, &signalOur, nil, 0); err != nil {
		return nil, fmt.Errorf("CreatePipe(signal): %w", err)
	}
	if err := windows.SetHandleInformation(signalHost, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
		_ = windows.CloseHandle(signalHost)
		_ = windows.CloseHandle(signalOur)
		return nil, fmt.Errorf("SetHandleInformation(signal): %w", err)
	}
	// The pinned implementation passes the pipe ends in the explicit handle
	// list; CreateProcess requires every listed handle to be inheritable.
	for name, handle := range map[string]windows.Handle{"input": inPseudo, "output": outPseudo} {
		if err := windows.SetHandleInformation(handle, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return nil, fmt.Errorf("SetHandleInformation(%s): %w", name, err)
		}
	}

	flags := uint32(pseudoconsoleWin32InputMode)
	if resizeQuirk {
		flags |= pseudoconsoleResizeQuirk
	}
	cmd := fmt.Sprintf(`"%s" --headless %s%s--width %d --height %d --signal 0x%x --server 0x%x`, hostPath,
		func() string {
			if flags&pseudoconsoleWin32InputMode != 0 {
				return "--win32input "
			}
			return ""
		}(),
		func() string {
			if flags&pseudoconsoleResizeQuirk != 0 {
				return "--resizeQuirk "
			}
			return ""
		}(), width, height, uintptr(signalHost), uintptr(server))
	cmd16, err := windows.UTF16FromString(cmd)
	if err != nil {
		return nil, err
	}
	host16, err := windows.UTF16FromString(filepath.Clean(hostPath))
	if err != nil {
		return nil, err
	}
	inherited := []windows.Handle{server, inPseudo, outPseudo, signalHost}
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, fmt.Errorf("InitializeProcThreadAttributeList(host): %w", err)
	}
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&inherited[0]), uintptr(len(inherited))*unsafe.Sizeof(inherited[0])); err != nil {
		attrs.Delete()
		return nil, fmt.Errorf("UpdateProcThreadAttribute(host handles): %w", err)
	}
	defer attrs.Delete()

	startup := windows.StartupInfoEx{}
	startup.StartupInfo.Cb = uint32(unsafe.Sizeof(startup))
	startup.StartupInfo.Flags = windows.STARTF_USESTDHANDLES
	startup.StartupInfo.StdInput = inPseudo
	startup.StartupInfo.StdOutput = outPseudo
	startup.StartupInfo.StdErr = outPseudo
	startup.ProcThreadAttributeList = attrs.List()
	var hostInfo windows.ProcessInformation
	if err := windows.CreateProcess(&host16[0], &cmd16[0], nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT, nil, nil, &startup.StartupInfo, &hostInfo); err != nil {
		_ = windows.CloseHandle(signalHost)
		_ = windows.CloseHandle(signalOur)
		return nil, fmt.Errorf("CreateProcess(OpenConsole --headless): %w", err)
	}
	_ = windows.CloseHandle(hostInfo.Thread)
	_ = windows.CloseHandle(signalHost)
	closePipeHandles = false
	_ = windows.CloseHandle(inPseudo)
	_ = windows.CloseHandle(outPseudo)

	ptyReference, err := createClientHandle(server, `\Reference`, false)
	if err != nil {
		_ = windows.CloseHandle(signalOur)
		_ = windows.CloseHandle(inOur)
		_ = windows.CloseHandle(outOur)
		_ = windows.TerminateProcess(hostInfo.Process, 0)
		_ = windows.CloseHandle(hostInfo.Process)
		return nil, fmt.Errorf("DeviceHandle::CreateClientHandle: %w", err)
	}
	_ = windows.CloseHandle(server)
	closeServer = false
	return &pinnedConPTY{signal: signalOur, ptyReference: ptyReference, hostProcess: hostInfo.Process, input: inOur, output: outOur, session: session, hostCommandLine: cmd}, nil
}

// processImagePath resolves the executable behind a live process handle. This
// is deliberately checked in addition to the requested CreateProcess path:
// the native probe must fail closed if a launcher, package registration, or
// filesystem redirection substituted the system OpenConsole.
func processImagePath(process windows.Handle) (string, error) {
	buffer := make([]uint16, 512)
	for {
		size := uint32(len(buffer))
		err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size)
		if err == nil {
			return windows.UTF16ToString(buffer[:size]), nil
		}
		if err != windows.ERROR_INSUFFICIENT_BUFFER || len(buffer) >= 32768 {
			return "", fmt.Errorf("QueryFullProcessImageName: %w", err)
		}
		buffer = make([]uint16, len(buffer)*2)
	}
}

func verifyPinnedHostProcess(process windows.Handle, expectedPath string) (uint32, pinnedHostIdentity, error) {
	if process == 0 {
		return 0, pinnedHostIdentity{}, fmt.Errorf("pinned host process handle is invalid")
	}
	pid, err := windows.GetProcessId(process)
	if err != nil {
		return 0, pinnedHostIdentity{}, fmt.Errorf("GetProcessId(pinned OpenConsole): %w", err)
	}
	actualPath, err := processImagePath(process)
	if err != nil {
		return pid, pinnedHostIdentity{}, err
	}
	expectedAbs, err := filepath.Abs(expectedPath)
	if err != nil {
		return pid, pinnedHostIdentity{}, fmt.Errorf("resolve pinned host path: %w", err)
	}
	actualAbs, err := filepath.Abs(actualPath)
	if err != nil {
		return pid, pinnedHostIdentity{}, fmt.Errorf("resolve launched host path: %w", err)
	}
	if !strings.EqualFold(filepath.Clean(actualAbs), filepath.Clean(expectedAbs)) {
		return pid, pinnedHostIdentity{Path: actualPath}, fmt.Errorf("pinned host process path mismatch: got %q want %q", actualPath, expectedPath)
	}
	identity, err := verifyPinnedHost(actualPath)
	if err != nil {
		return pid, identity, fmt.Errorf("launched host failed pinned identity check: %w", err)
	}
	return pid, identity, nil
}

func attachPinnedClient(pty *pinnedConPTY, command string) error {
	expanded, err := expandEnvironmentStrings(command)
	if err != nil {
		return err
	}
	cmd16, err := windows.UTF16FromString(expanded)
	if err != nil {
		return err
	}
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return fmt.Errorf("InitializeProcThreadAttributeList(client): %w", err)
	}
	defer attrs.Delete()
	// ConptyConnection::_LaunchAttachedClient passes the HPCON value, not the
	// reference handle, as PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE. The first
	// three fields of pinnedConPTY are the packed PseudoConsole members.
	// UpdateProcThreadAttribute receives the HPCON value itself (the packed
	// PseudoConsole pointer), matching ConptyConnection::_LaunchAttachedClient
	// and the ordinary public-ConPTY path. Passing the
	// address of a local uintptr would describe a different, invalid handle.
	if err := attrs.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(pty), unsafe.Sizeof(uintptr(0))); err != nil {
		return fmt.Errorf("UpdateProcThreadAttribute(pseudoconsole): %w", err)
	}
	startup := windows.StartupInfoEx{}
	startup.StartupInfo.Cb = uint32(unsafe.Sizeof(startup))
	// Match ConptyConnection::_LaunchAttachedClient.
	startup.StartupInfo.Flags = windows.STARTF_USESTDHANDLES
	startup.ProcThreadAttributeList = attrs.List()
	_, environmentBlock, err := attachedClientEnvironment(pty.session)
	if err != nil {
		return err
	}
	var info windows.ProcessInformation
	if err := windows.CreateProcess(nil, &cmd16[0], nil, nil, false, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, environmentBlock, nil, &startup.StartupInfo, &info); err != nil {
		return fmt.Errorf("CreateProcess(attached client): %w", err)
	}
	_ = windows.CloseHandle(info.Thread)
	pty.childProcess = info.Process
	pty.childPID = info.ProcessId
	return nil
}

func expandEnvironmentStrings(command string) (string, error) {
	source, err := windows.UTF16FromString(command)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, 256)
	for {
		length, callErr := windows.ExpandEnvironmentStrings(&source[0], &buffer[0], uint32(len(buffer)))
		if length == 0 {
			return "", fmt.Errorf("ExpandEnvironmentStringsW: %w", callErr)
		}
		if length <= uint32(len(buffer)) {
			return windows.UTF16ToString(buffer[:length-1]), nil
		}
		buffer = make([]uint16, length)
	}
}

func attachedClientEnvironment(session string) ([]string, *uint16, error) {
	// ConptyConnection::_LaunchAttachedClient starts from the current process
	// environment, then uses Utils::EnvironmentVariableMapW.  The map is
	// case-insensitive (_wcsicmp), try_emplace keeps the first spelling from
	// GetEnvironmentStringsW, and insert_or_assign updates the existing value
	// without changing that spelling.  Its final iteration is therefore sorted
	// by the same comparator, not by the order returned by os.Environ.
	currentEnvironment := windows.Environ()
	environment := make([]environmentVariable, 0, len(currentEnvironment)+1)
	for _, entry := range currentEnvironment {
		equal := strings.IndexByte(entry, '=')
		if equal < 0 {
			return nil, nil, fmt.Errorf("GetEnvironmentStringsW returned entry without '=': %q", entry)
		}
		insertEnvironmentVariable(&environment, entry[:equal], entry[equal+1:], false)
	}
	insertEnvironmentVariable(&environment, "WT_SESSION", session, true)
	wslEnv := environmentValue(environment, "WSLENV")
	insertEnvironmentVariable(&environment, "WSLENV", "WT_SESSION:"+wslEnv, true)
	sort.SliceStable(environment, func(i, j int) bool {
		return compareEnvironmentNames(environment[i].Name, environment[j].Name) < 0
	})
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		filtered = append(filtered, entry.Name+"="+entry.Value)
	}
	block := make([]uint16, 0)
	for _, entry := range filtered {
		units, err := windows.UTF16FromString(entry)
		if err != nil {
			return nil, nil, err
		}
		block = append(block, units...)
	}
	block = append(block, 0)
	return filtered, &block[0], nil
}

type environmentVariable struct {
	Name  string
	Value string
}

func compareEnvironmentNames(left, right string) int {
	left16, err := windows.UTF16FromString(left)
	if err != nil {
		panic(err)
	}
	right16, err := windows.UTF16FromString(right)
	if err != nil {
		panic(err)
	}
	result, _, _ := wcsicmp.Call(uintptr(unsafe.Pointer(&left16[0])), uintptr(unsafe.Pointer(&right16[0])))
	return int(int32(result))
}

func environmentNameEqual(left, right string) bool {
	return compareEnvironmentNames(left, right) == 0
}

func environmentValue(environment []environmentVariable, name string) string {
	for _, entry := range environment {
		if environmentNameEqual(entry.Name, name) {
			return entry.Value
		}
	}
	return ""
}

func insertEnvironmentVariable(environment *[]environmentVariable, name, value string, assign bool) {
	for index := range *environment {
		if environmentNameEqual((*environment)[index].Name, name) {
			if assign {
				(*environment)[index].Value = value
			}
			return
		}
	}
	*environment = append(*environment, environmentVariable{Name: name, Value: value})
}

func resizePinnedPseudoConsole(pty *pinnedConPTY, width, height uint16) error {
	if pty == nil {
		return fmt.Errorf("_ResizePseudoConsole: nil PseudoConsole")
	}
	packet := []uint16{ptySignalResizeWindow, width, height}
	// _ResizePseudoConsole passes nullptr for lpNumberOfBytesWritten and
	// treats the WriteFile boolean as the complete result.  Keep that API
	// boundary instead of adding a short-write policy to the transcription.
	if err := windows.WriteFile(pty.signal, unsafeBytes(packet), nil, nil); err != nil {
		return fmt.Errorf("WriteFile(PTY_SIGNAL_RESIZE_WINDOW): %w", err)
	}
	return nil
}

func unsafeBytes(values []uint16) []byte {
	if len(values) == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(&values[0])), len(values)*2)
}

func readPinnedOutput(pty *pinnedConPTY) ([]byte, error) {
	return readPinnedOutputRecorded(pty, nil)
}

func readPinnedOutputRecorded(pty *pinnedConPTY, recorder *hostCaptureRecorder) ([]byte, error) {
	var result bytes.Buffer
	buffer := make([]byte, 32*1024)
	for {
		var read uint32
		err := windows.ReadFile(pty.output, buffer, &read, nil)
		if err != nil {
			if err == windows.ERROR_BROKEN_PIPE || err == windows.ERROR_HANDLE_EOF {
				break
			}
			return nil, fmt.Errorf("ReadFile(ConPTY output): %w", err)
		}
		if read == 0 {
			break
		}
		_, _ = result.Write(buffer[:read])
		if recorder != nil {
			recorder.append(streamObservedOutput, buffer[:read], "pinned-host-observed-output")
		}
	}
	return result.Bytes(), nil
}

func terminatePinnedClient(pty *pinnedConPTY) {
	if pty == nil || pty.childProcess == 0 {
		return
	}
	_ = windows.TerminateProcess(pty.childProcess, 1)
	_ = windows.CloseHandle(pty.childProcess)
	pty.childProcess = 0
}

/*
The former scenario runner was intentionally removed. The standalone gate
must never execute a simulated or grid-reconciliation path.
func runPinnedHost(path, reportPath string) error {
	identity, err := verifyPinnedHost(path)
	if err != nil {
		return err
	}
	if err := pinnedHostCaptureBoundary(); err != nil {
		return err
	}
	if reportPath == "" {
		return fmt.Errorf("host report path is required")
	}
	artifactDirectory := reportPath + ".host-runs"
	if err := os.MkdirAll(artifactDirectory, 0o755); err != nil {
		return fmt.Errorf("create host artifact directory: %w", err)
	}
	if err := runPinnedRecursiveDir(path, identity, artifactDirectory); err != nil {
		return err
	}
	if err := validateRecordedSeeds(); err != nil {
		return err
	}
	for _, width := range edgeScenarioWidths() {
		scenarioCase := edgeScenario(width)
		if err := runPinnedScenario(path, identity, scenarioCase, int64(width), artifactDirectory, true); err != nil {
			return err
		}
	}
	for _, seed := range recordedSeeds {
		scenarioCase := scenarioForSeed(int64(seed))
		if err := runPinnedScenario(path, identity, scenarioCase, int64(seed^11400714819323198485), artifactDirectory, false); err != nil {
			return err
		}
	}
	return writeJSON(reportPath, struct {
		Mode         string             `json:"mode"`
		SourceCommit string             `json:"source_commit"`
		Host         pinnedHostIdentity `json:"host"`
		SeedCount    int                `json:"seed_count"`
		MatrixWidths []int              `json:"matrix_widths"`
		CompletedAt  time.Time          `json:"completed_at"`
	}{
		Mode:         "pinned-host",
		SourceCommit: pinnedOpenConsoleCommit,
		Host:         identity,
		SeedCount:    len(recordedSeeds),
		MatrixWidths: edgeScenarioWidths(),
		CompletedAt:  time.Now().UTC(),
	})
}

// pinnedHostCaptureBoundary records a protocol limitation, not a test
// result. The pinned winconpty.cpp path gives the client one hOutput pipe;
// neither that source nor the stock OpenConsole process attaches provenance
// to bytes on the pipe. Treating ReadFile calls, timestamps, markers, parser
// state, or resize notifications as a live/frame tag would add semantics not
// present in the pinned source.
func pinnedHostCaptureBoundary() error {
	return fmt.Errorf("pinned host gate blocked: stock ConPTY exposes one untagged output stream; live/frame bytes cannot be separated by the pinned API")
}

func runPinnedRecursiveDir(path string, identity pinnedHostIdentity, artifactDirectory string) error {
	pty, err := createPinnedPseudoConsole(path, 80, 25)
	if err != nil {
		return err
	}
	defer pty.close()
	defer pty.closePipes()
	marker := "__CONPTYRECONCILE_PINNED_END__"
	command := fmt.Sprintf(`cmd.exe /d /c "dir /s C:\Windows\System32 && echo %s"`, marker)
	if err := attachPinnedClient(pty, command); err != nil {
		return err
	}
	outputReady := make(chan struct {
		data []byte
		err  error
	}, 1)
	go func() {
		data, readErr := readPinnedOutput(pty)
		outputReady <- struct {
			data []byte
			err  error
		}{data: data, err: readErr}
	}()
	if err := waitPinnedClient(pty); err != nil {
		terminatePinnedClient(pty)
		return err
	}
	if pty.input != 0 {
		_ = windows.CloseHandle(pty.input)
		pty.input = 0
	}
	pty.close()
	readResult := <-outputReady
	if readResult.err != nil {
		return readResult.err
	}
	if !bytes.Contains(readResult.data, []byte(marker)) {
		return fmt.Errorf("pinned command completed without marker %q (host %s)", marker, identity.SHA256)
	}
	return os.WriteFile(filepath.Join(artifactDirectory, "recursive-dir.raw"), readResult.data, 0o644)
}

func runPinnedScenario(path string, identity pinnedHostIdentity, scenarioCase scenario, delaySeed int64, artifactDirectory string, edge bool) (runErr error) {
	recorder := newHostCaptureRecorder(scenarioCase.Seed, scenarioCase.InitialWidth, scenarioCase.InitialHeight)
	recorder.capture.HostPath = identity.Path
	recorder.capture.HostSHA256 = identity.SHA256
	base := filepath.Join(artifactDirectory, "seed-"+fmt.Sprint(uint64(scenarioCase.Seed)))
	defer func() {
		logged := recorder.snapshot()
		if writeErr := os.WriteFile(base+".raw", bytesForKind(logged, streamObservedOutput), 0o644); writeErr != nil && runErr == nil {
			runErr = writeErr
		}
		artifact := struct {
			Scenario scenario           `json:"scenario"`
			Capture  capture            `json:"capture"`
			Host     pinnedHostIdentity `json:"host"`
			Error    string             `json:"error,omitempty"`
		}{Scenario: scenarioCase, Capture: logged, Host: identity}
		if runErr != nil {
			artifact.Error = runErr.Error()
		}
		if writeErr := writeJSON(base+".json", artifact); writeErr != nil && runErr == nil {
			runErr = writeErr
		}
	}()
	pty, err := createPinnedPseudoConsole(path, scenarioCase.InitialWidth, scenarioCase.InitialHeight)
	if err != nil {
		return fmt.Errorf("seed %d create pinned pty: %w", scenarioCase.Seed, err)
	}
	defer pty.close()
	defer pty.closePipes()
	cause := "pinned-host-client-input"
	if edge {
		cause += "-edge"
	}
	// This is the child's input workload, not host output. The ConPTY output
	// pipe is recorded independently by readPinnedOutputRecorded below.
	recorder.append(streamInput, scenarioCase.Input, cause)
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := fmt.Sprintf(`"%s" -emit-seed %d`, executable, uint64(scenarioCase.Seed))
	if edge {
		command = fmt.Sprintf(`"%s" -emit-width %d`, executable, scenarioCase.InitialWidth)
	}
	if err := attachPinnedClient(pty, command); err != nil {
		return fmt.Errorf("seed %d attach client: %w", scenarioCase.Seed, err)
	}
	outputReady := make(chan error, 1)
	go func() {
		_, readErr := readPinnedOutputRecorded(pty, recorder)
		outputReady <- readErr
	}()
	rng := rand.New(rand.NewSource(delaySeed))
	resizeDone := make(chan error, 1)
	go func() {
		for _, resize := range sortedResizeEvents(scenarioCase.Resizes) {
			if delay := rng.Intn(1000); delay != 0 {
				time.Sleep(time.Duration(delay) * time.Microsecond)
			}
			if err := resizePinnedPseudoConsole(pty, uint16(resize.Width), uint16(resize.Height)); err != nil {
				resizeDone <- err
				return
			}
			recorder.resize(resize.Width, resize.Height)
		}
		resizeDone <- nil
	}()
	if err := waitPinnedClient(pty); err != nil {
		terminatePinnedClient(pty)
		return fmt.Errorf("seed %d attached client: %w", scenarioCase.Seed, err)
	}
	if err := <-resizeDone; err != nil {
		return fmt.Errorf("seed %d resize schedule: %w", scenarioCase.Seed, err)
	}
	// The pinned close path breaks the signal pipe and waits for OpenConsole
	// to flush before the reader is allowed to observe EOF.  Without this
	// boundary the host-owned output handle remains open after the attached
	// client exits and ReadFile can wait indefinitely.
	pty.close()
	if err := <-outputReady; err != nil {
		return fmt.Errorf("seed %d host output: %w", scenarioCase.Seed, err)
	}
	if pty.input != 0 {
		_ = windows.CloseHandle(pty.input)
		pty.input = 0
	}
	logged := recorder.snapshot()
	frameBytes := bytesForKind(logged, streamObservedOutput)
	marker := scenarioCase.Marker
	if edge {
		marker = "__EDGE_END_" + fmt.Sprint(scenarioCase.InitialWidth) + "__"
	}
	if !bytes.Contains(frameBytes, []byte(marker)) {
		return fmt.Errorf("seed %d host output lost marker %q", scenarioCase.Seed, marker)
	}
	whole, err := parseCapturedFrameEvents(scenarioCase.InitialWidth, scenarioCase.InitialHeight, logged.Events, false)
	if err != nil {
		return fmt.Errorf("seed %d host whole-output parse: %w", scenarioCase.Seed, err)
	}
	chunked, err := parseCapturedFrameEvents(scenarioCase.InitialWidth, scenarioCase.InitialHeight, logged.Events, true)
	if err != nil {
		return fmt.Errorf("seed %d host byte-output parse: %w", scenarioCase.Seed, err)
	}
	if whole.snapshot() != chunked.snapshot() {
		return fmt.Errorf("seed %d host output chunking changed snapshot", scenarioCase.Seed)
	}
	if chunked.buffer.text() != scenarioCase.ExpectedText {
		return fmt.Errorf("seed %d host output text mismatch: got %q want %q", scenarioCase.Seed, chunked.buffer.text(), scenarioCase.ExpectedText)
	}
	return nil
}

func bytesForKind(c capture, kind streamKind) []byte {
	var result []byte
	for _, event := range c.Events {
		if event.Kind == kind {
			result = append(result, event.Bytes...)
		}
	}
	return result
}
*/
