package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

type fileOpSafetyProbeVFS struct {
	vfs.VFS
	statPath    string
	statErr     error
	renameErr   error
	createCalls int
	mkdirCalls  int
	renameCalls int
	removeCalls int
	createFn    func(context.Context, string) (io.WriteCloser, error)
	overwrite   bool
	intentKnown bool
	managed     bool
	remote      bool
}

func (p *fileOpSafetyProbeVFS) ManagedTransferWrites() bool { return p.managed }
func (p *fileOpSafetyProbeVFS) RemoteTransfer() bool        { return p.remote }

func (p *fileOpSafetyProbeVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	if path == p.statPath && p.statErr != nil {
		return vfs.VFSItem{}, p.statErr
	}
	return p.VFS.Stat(ctx, path)
}

func (p *fileOpSafetyProbeVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	p.createCalls++
	p.overwrite, p.intentKnown = vfs.DestinationOverwrite(ctx)
	if p.createFn != nil {
		return p.createFn(ctx, path)
	}
	return p.VFS.Create(ctx, path)
}

func (p *fileOpSafetyProbeVFS) Remove(ctx context.Context, path string) error {
	p.removeCalls++
	return p.VFS.Remove(ctx, path)
}

func (p *fileOpSafetyProbeVFS) MkDir(ctx context.Context, path string) error {
	p.mkdirCalls++
	return p.VFS.MkDir(ctx, path)
}

func (p *fileOpSafetyProbeVFS) Rename(ctx context.Context, oldPath, newPath string) error {
	p.renameCalls++
	if p.renameErr != nil {
		return p.renameErr
	}
	return p.VFS.Rename(ctx, oldPath, newPath)
}

func TestRecursiveCopyAbortsWhenDestinationStatIsInconclusive(t *testing.T) {
	probeErr := errors.New("destination stat denied")
	for _, directory := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "directory"}[directory], func(t *testing.T) {
			sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
			sourcePath := filepath.Join(sourceRoot, "source")
			if directory {
				if err := os.Mkdir(sourcePath, 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(sourcePath, []byte("content"), 0o644); err != nil {
				t.Fatal(err)
			}
			destinationPath := filepath.Join(destinationRoot, "target")
			probe := &fileOpSafetyProbeVFS{
				VFS:      vfs.NewOSVFS(destinationRoot),
				statPath: destinationPath,
				statErr:  probeErr,
			}
			err := recursiveCopy(context.Background(), vfs.NewOSVFS(sourceRoot), sourcePath, probe, destinationPath, &FileOpState{}, 0)
			if !errors.Is(err, probeErr) {
				t.Fatalf("recursiveCopy error = %v, want destination Stat error", err)
			}
			if probe.createCalls != 0 || probe.mkdirCalls != 0 {
				t.Fatalf("destination mutated after inconclusive Stat: Create=%d MkDir=%d", probe.createCalls, probe.mkdirCalls)
			}
		})
	}
}

func TestOptimizedRenameRequiresProvenMissingDestination(t *testing.T) {
	root := t.TempDir()
	probeErr := errors.New("destination stat timed out")
	destination := &fileOpSafetyProbeVFS{
		VFS:      vfs.NewOSVFS(root),
		statPath: filepath.Join(root, "target"),
		statErr:  probeErr,
	}
	source := &fileOpSafetyProbeVFS{VFS: vfs.NewOSVFS(root)}
	renamed, err := tryOptimizedRename(context.Background(), source, destination, filepath.Join(root, "source"), destination.statPath)
	if renamed || !errors.Is(err, probeErr) {
		t.Fatalf("tryOptimizedRename = (%v, %v), want (false, Stat error)", renamed, err)
	}
	if source.renameCalls != 0 {
		t.Fatalf("Rename called %d times after inconclusive destination Stat", source.renameCalls)
	}
}

func TestOptimizedRenamePreservesUnknownCanceledMutation(t *testing.T) {
	root := t.TempDir()
	destinationPath := filepath.Join(root, "target")
	destination := &fileOpSafetyProbeVFS{
		VFS:      vfs.NewOSVFS(root),
		statPath: destinationPath,
		statErr:  os.ErrNotExist,
	}
	unknown := &vfs.UnknownOperationStateError{Operation: "rename", Err: context.Canceled}
	source := &fileOpSafetyProbeVFS{VFS: vfs.NewOSVFS(root), renameErr: unknown}
	renamed, err := tryOptimizedRename(context.Background(), source, destination, filepath.Join(root, "source"), destinationPath)
	if renamed || !errors.Is(err, vfs.ErrOperationStateUnknown) {
		t.Fatalf("tryOptimizedRename = (%v, %v), want unknown-state error", renamed, err)
	}
	if source.renameCalls != 1 {
		t.Fatalf("Rename called %d times, want exactly one attempt", source.renameCalls)
	}
}

func TestUnknownCanceledOperationIsStillDisplayed(t *testing.T) {
	unknown := &vfs.UnknownOperationStateError{Operation: "delete", Err: context.Canceled}
	if !shouldDisplayFileOpError(unknown) {
		t.Fatal("unknown destructive result wrapping cancellation was hidden")
	}
	if shouldDisplayFileOpError(context.Canceled) {
		t.Fatal("plain user cancellation should remain silent")
	}
}

type closeErrorWriter struct {
	err error
}

func (*closeErrorWriter) Write(p []byte) (int, error) { return len(p), nil }
func (w *closeErrorWriter) Close() error              { return w.err }

type bytesThenErrorReader struct {
	payload []byte
	err     error
	sent    bool
}

func (r *bytesThenErrorReader) Read(_ context.Context, p []byte) (int, error) {
	if r.sent {
		return 0, r.err
	}
	r.sent = true
	return copy(p, r.payload), r.err
}

func (r *bytesThenErrorReader) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	if off < 0 || off >= int64(len(r.payload)) {
		return 0, r.err
	}
	n := copy(p, r.payload[off:])
	return n, r.err
}

func (r *bytesThenErrorReader) Size() int64 { return int64(len(r.payload)) }
func (*bytesThenErrorReader) Close() error  { return nil }

type failingReadVFS struct {
	vfs.VFS
	path    string
	payload []byte
	err     error
}

func (f *failingReadVFS) Open(_ context.Context, path string) (vfs.ReadAtCloser, error) {
	if path == f.path {
		return &bytesThenErrorReader{payload: append([]byte(nil), f.payload...), err: f.err}, nil
	}
	return f.VFS.Open(context.Background(), path)
}

type stagedAbortWriter struct {
	ctx        context.Context
	path       string
	staged     []byte
	closeCalls int
	abortCalls int
	abortErr   error
}

func (w *stagedAbortWriter) Write(p []byte) (int, error) {
	w.staged = append(w.staged, p...)
	return len(p), nil
}

func (w *stagedAbortWriter) Close() error {
	w.closeCalls++
	return os.WriteFile(w.path, w.staged, 0o644)
}

func (w *stagedAbortWriter) Abort() error {
	w.abortCalls++
	w.staged = nil
	if !errors.Is(w.ctx.Err(), context.Canceled) {
		return errors.New("writer context was not canceled before Abort")
	}
	return w.abortErr
}

func TestRecursiveCopyAbortsStagedReplacementAfterSourceReadError(t *testing.T) {
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "source.bin")
	destinationPath := filepath.Join(destinationRoot, "existing.bin")
	if err := os.WriteFile(sourcePath, []byte("declared source"), 0o644); err != nil {
		t.Fatal(err)
	}
	const original = "existing destination must survive"
	if err := os.WriteFile(destinationPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	readErr := errors.New("source failed after returning a prefix")
	source := &failingReadVFS{
		VFS: vfs.NewOSVFS(sourceRoot), path: sourcePath,
		payload: []byte("partial replacement"), err: readErr,
	}
	var writer *stagedAbortWriter
	destination := &fileOpSafetyProbeVFS{
		VFS: vfs.NewOSVFS(destinationRoot),
		createFn: func(ctx context.Context, path string) (io.WriteCloser, error) {
			writer = &stagedAbortWriter{ctx: ctx, path: path}
			return writer, nil
		},
	}
	err := recursiveCopy(context.Background(), source, sourcePath, destination, destinationPath, &FileOpState{OverwriteAll: true}, 0)
	if !errors.Is(err, readErr) {
		t.Fatalf("recursiveCopy error = %v, want source read error", err)
	}
	if writer == nil || writer.abortCalls != 1 || writer.closeCalls != 0 {
		t.Fatalf("writer lifecycle = %#v, want one Abort and no commit Close", writer)
	}
	content, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != original {
		t.Fatalf("existing destination changed to %q", content)
	}
	if destination.removeCalls != 0 {
		t.Fatalf("aborted replacement triggered %d Remove call(s)", destination.removeCalls)
	}
}

func TestRecursiveCopySurfacesSourceAndAbortFailures(t *testing.T) {
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "source.bin")
	destinationPath := filepath.Join(destinationRoot, "new.bin")
	if err := os.WriteFile(sourcePath, []byte("declared source"), 0o644); err != nil {
		t.Fatal(err)
	}
	readErr := errors.New("source failed after returning a prefix")
	abortErr := errors.New("remote multipart cleanup failed")
	source := &failingReadVFS{
		VFS: vfs.NewOSVFS(sourceRoot), path: sourcePath,
		payload: []byte("partial replacement"), err: readErr,
	}
	var writer *stagedAbortWriter
	destination := &fileOpSafetyProbeVFS{
		VFS: vfs.NewOSVFS(destinationRoot),
		createFn: func(ctx context.Context, path string) (io.WriteCloser, error) {
			writer = &stagedAbortWriter{ctx: ctx, path: path, abortErr: abortErr}
			return writer, nil
		},
	}

	err := recursiveCopy(context.Background(), source, sourcePath, destination, destinationPath, &FileOpState{}, 0)
	if !errors.Is(err, readErr) || !errors.Is(err, abortErr) {
		t.Fatalf("recursiveCopy error = %v, want joined source %v and abort %v", err, readErr, abortErr)
	}
	if writer == nil || writer.abortCalls != 1 || writer.closeCalls != 0 {
		t.Fatalf("writer lifecycle = %#v, want one Abort and no commit Close", writer)
	}
	if destination.removeCalls != 0 {
		t.Fatalf("failed abort triggered %d Remove call(s)", destination.removeCalls)
	}
}

type reportingOpenVFS struct {
	vfs.VFS
	reportPath  string
	unknownSize bool
}

func (*reportingOpenVFS) RemoteTransfer() bool { return true }

func (v *reportingOpenVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	item, err := v.VFS.Stat(ctx, path)
	if err == nil && v.unknownSize && path == v.reportPath {
		item.Size = 0
		item.SizeKnown = false
	}
	return item, err
}

func (v *reportingOpenVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	if path == v.reportPath {
		if reporter, ok := ctx.Value(vfs.ReporterKey).(vfs.TaskReporter); ok {
			reporter.UpdateTransfer("Downloading", filepath.Base(path), 100, "", 100, "")
		}
	}
	return v.VFS.Open(ctx, path)
}

type safetyTaskReporter struct {
	events []safetyProgressEvent
}

type safetyProgressEvent struct {
	action, name   string
	current, total int
}

func (*safetyTaskReporter) UpdateScan(string, int64, int64) {}
func (*safetyTaskReporter) IsCancelled() bool               { return false }
func (r *safetyTaskReporter) UpdateTransfer(action, name string, current int, _ string, total int, _ string) {
	r.events = append(r.events, safetyProgressEvent{action: action, name: name, current: current, total: total})
}

func TestRecursiveCopyResetsProviderProgressThroughProductionStartPath(t *testing.T) {
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	first := filepath.Join(sourceRoot, "first.bin")
	second := filepath.Join(sourceRoot, "second.bin")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte{'x'}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	source := &reportingOpenVFS{VFS: vfs.NewOSVFS(sourceRoot), reportPath: first}
	destination := vfs.NewOSVFS(destinationRoot)
	tracker := NewFileOpTracker(vfs.OpStats{Files: 2, Bytes: 2})
	original := &safetyTaskReporter{}
	accounted := 0
	reporter := &globalAwareReporter{
		original: original, tracker: tracker,
		getGlobal: func(string) (string, int, string) { return "", 0, "" },
		onBytes:   func(n int) { accounted += n },
	}
	state := &FileOpState{
		Tracker: tracker, StartFile: reporter.StartFileKnown, OnBytes: reporter.UpdateBytes,
	}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, reporter)
	if err := recursiveCopy(ctx, source, first, destination, filepath.Join(destinationRoot, "first.bin"), state, 0); err != nil {
		t.Fatal(err)
	}
	if err := recursiveCopy(ctx, source, second, destination, filepath.Join(destinationRoot, "second.bin"), state, 0); err != nil {
		t.Fatal(err)
	}
	if accounted != 2 {
		t.Fatalf("accounted source bytes = %d, want both files; provider phase leaked across file boundary", accounted)
	}
}

type phaseUploadWriter struct {
	ctx     context.Context
	path    string
	staged  []byte
	aborted bool
}

func (w *phaseUploadWriter) Write(p []byte) (int, error) {
	w.staged = append(w.staged, p...)
	return len(p), nil
}

func (w *phaseUploadWriter) Close() error {
	if w.aborted {
		return nil
	}
	if reporter, ok := w.ctx.Value(vfs.ReporterKey).(vfs.TaskReporter); ok {
		for _, percent := range []int{0, 50, 100} {
			reporter.UpdateTransfer("Uploading", filepath.Base(w.path), percent, "", percent, "")
		}
	}
	return os.WriteFile(w.path, w.staged, 0o644)
}

func (w *phaseUploadWriter) Abort() error {
	w.aborted = true
	w.staged = nil
	return nil
}

func TestRecursiveCopyWeightsDownloadAndManagedUploadPhases(t *testing.T) {
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "remote-source.bin")
	destinationPath := filepath.Join(destinationRoot, "managed-destination.bin")
	payload := make([]byte, 100)
	for i := range payload {
		payload[i] = byte(i)
	}
	if err := os.WriteFile(sourcePath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	source := &reportingOpenVFS{VFS: vfs.NewOSVFS(sourceRoot), reportPath: sourcePath}
	destination := &fileOpSafetyProbeVFS{
		VFS: vfs.NewOSVFS(destinationRoot), managed: true,
		createFn: func(ctx context.Context, path string) (io.WriteCloser, error) {
			return &phaseUploadWriter{ctx: ctx, path: path}, nil
		},
	}
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: int64(len(payload))})
	capture := &safetyTaskReporter{}
	reporter := &globalAwareReporter{original: capture, tracker: tracker}
	reporter.getGlobal = func(string) (string, int, string) {
		_, total, _ := tracker.GetProgress()
		return "", total, ""
	}
	state := &FileOpState{Tracker: tracker, StartFile: reporter.StartFileKnown, OnBytes: reporter.UpdateBytes}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, reporter)
	if err := recursiveCopy(ctx, source, sourcePath, destination, destinationPath, state, 0); err != nil {
		t.Fatal(err)
	}
	var downloading100, uploading0, uploading50 bool
	for _, event := range capture.events {
		switch {
		case event.action == "Downloading" && event.current == 100:
			downloading100 = event.total == 50
		case event.action == "Uploading" && event.current == 0:
			uploading0 = event.total == 50
		case event.action == "Uploading" && event.current == 50:
			uploading50 = event.total == 75
		}
	}
	if !downloading100 || !uploading0 || !uploading50 {
		t.Fatalf("phase events = %#v, want download 100=>total 50, upload 0=>50 and upload 50=>75", capture.events)
	}
	_, total, _ := tracker.GetProgress()
	if total != 100 {
		t.Fatalf("final total = %d, want 100", total)
	}
}

func TestRecursiveCopyWeightsMaterializedDownloadAndStreamingCloudWrite(t *testing.T) {
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "materialized-source.bin")
	destinationPath := filepath.Join(destinationRoot, "streaming-destination.bin")
	payload := make([]byte, 100)
	if err := os.WriteFile(sourcePath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	source := &reportingOpenVFS{VFS: vfs.NewOSVFS(sourceRoot), reportPath: sourcePath}
	destination := &fileOpSafetyProbeVFS{
		VFS: vfs.NewOSVFS(destinationRoot), remote: true,
		createFn: func(ctx context.Context, path string) (io.WriteCloser, error) {
			return &phaseUploadWriter{ctx: ctx, path: path}, nil
		},
	}
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: int64(len(payload))})
	capture := &safetyTaskReporter{}
	reporter := &globalAwareReporter{original: capture, tracker: tracker}
	reporter.getGlobal = func(string) (string, int, string) {
		_, total, _ := tracker.GetProgress()
		return "", total, ""
	}
	uploadProgress := -1
	state := &FileOpState{
		Tracker: tracker, StartFile: reporter.StartFileKnown,
		OnBytes: func(n int) {
			reporter.UpdateBytes(n)
			_, uploadProgress, _ = tracker.GetProgress()
		},
	}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, reporter)
	if err := recursiveCopy(ctx, source, sourcePath, destination, destinationPath, state, 0); err != nil {
		t.Fatal(err)
	}
	if len(capture.events) == 0 || capture.events[0].action != "Downloading" || capture.events[0].total != 50 {
		t.Fatalf("download phase events = %#v, want total 50 at materialization completion", capture.events)
	}
	if uploadProgress != 99 {
		t.Fatalf("streaming destination progress before commit = %d, want commit-reserved 99", uploadProgress)
	}
}

func TestRecursiveCopyLearnsUnknownMaterializedSizeForManagedPhases(t *testing.T) {
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "unknown-export.bin")
	destinationPath := filepath.Join(destinationRoot, "managed-upload.bin")
	payload := make([]byte, 100)
	if err := os.WriteFile(sourcePath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	source := &reportingOpenVFS{VFS: vfs.NewOSVFS(sourceRoot), reportPath: sourcePath, unknownSize: true}
	destination := &fileOpSafetyProbeVFS{
		VFS: vfs.NewOSVFS(destinationRoot), managed: true,
		createFn: func(ctx context.Context, path string) (io.WriteCloser, error) {
			return &phaseUploadWriter{ctx: ctx, path: path}, nil
		},
	}
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, UnknownSizeFiles: 1})
	capture := &safetyTaskReporter{}
	reporter := &globalAwareReporter{original: capture, tracker: tracker}
	reporter.getGlobal = func(string) (string, int, string) {
		_, total, _ := tracker.GetProgress()
		return "", total, ""
	}
	state := &FileOpState{
		Tracker: tracker, StartFile: reporter.StartFileKnown, SetFileSize: reporter.SetCurrentSize,
		OnBytes: reporter.UpdateBytes,
	}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, reporter)
	if err := recursiveCopy(ctx, source, sourcePath, destination, destinationPath, state, 0); err != nil {
		t.Fatal(err)
	}
	var uploadHalfAt int = -1
	for _, event := range capture.events {
		if event.action == "Uploading" && event.current == 50 {
			uploadHalfAt = event.total
		}
	}
	if uploadHalfAt != 75 {
		t.Fatalf("unknown-size phase events = %#v, upload 50 should map to total 75", capture.events)
	}
}

func TestRecursiveCopyZeroByteManagedUploadWaitsForCommitPhase(t *testing.T) {
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "empty.bin")
	destinationPath := filepath.Join(destinationRoot, "empty.bin")
	if err := os.WriteFile(sourcePath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	destination := &fileOpSafetyProbeVFS{
		VFS: vfs.NewOSVFS(destinationRoot), managed: true,
		createFn: func(ctx context.Context, path string) (io.WriteCloser, error) {
			return &phaseUploadWriter{ctx: ctx, path: path}, nil
		},
	}
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1})
	capture := &safetyTaskReporter{}
	reporter := &globalAwareReporter{original: capture, tracker: tracker}
	reporter.getGlobal = func(string) (string, int, string) {
		_, total, _ := tracker.GetProgress()
		return "", total, ""
	}
	state := &FileOpState{Tracker: tracker, StartFile: reporter.StartFileKnown, OnBytes: reporter.UpdateBytes}
	ctx := context.WithValue(context.Background(), vfs.ReporterKey, reporter)
	if err := recursiveCopy(ctx, vfs.NewOSVFS(sourceRoot), sourcePath, destination, destinationPath, state, 0); err != nil {
		t.Fatal(err)
	}
	for _, event := range capture.events {
		if event.action == "Uploading" && event.current == 0 && event.total != 50 {
			t.Fatalf("zero-byte upload started at total %d, want source-complete 50", event.total)
		}
	}
}

func TestRecursiveCopyDoesNotDeleteConcurrentDestinationAfterConditionalCloseFailure(t *testing.T) {
	sourceRoot, destinationRoot := t.TempDir(), t.TempDir()
	sourcePath := filepath.Join(sourceRoot, "source.txt")
	destinationPath := filepath.Join(destinationRoot, "target.txt")
	if err := os.WriteFile(sourcePath, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	probe := &fileOpSafetyProbeVFS{
		VFS:      vfs.NewOSVFS(destinationRoot),
		statPath: destinationPath,
		statErr:  os.ErrNotExist,
		createFn: func(context.Context, string) (io.WriteCloser, error) {
			return &closeErrorWriter{err: os.ErrExist}, nil
		},
	}
	err := recursiveCopy(context.Background(), vfs.NewOSVFS(sourceRoot), sourcePath, probe, destinationPath, &FileOpState{}, 0)
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("recursiveCopy=%v, want conditional destination collision", err)
	}
	if !probe.intentKnown || probe.overwrite {
		t.Fatalf("Create overwrite intent=(%v,%v), want explicit no-replace", probe.overwrite, probe.intentKnown)
	}
	if probe.removeCalls != 0 {
		t.Fatalf("conditional destination collision triggered %d destructive Remove call(s)", probe.removeCalls)
	}
}
