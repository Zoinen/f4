package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/unxed/f4/vfs"
)

const (
	applyCommandListRetention       = 5 * time.Minute
	applyCommandShutdownCleanupWait = 5 * time.Second
)

type applyResourceGroup struct {
	once    sync.Once
	mu      sync.Mutex
	timer   *time.Timer
	cleaned bool
	cleanup []func()
	drains  []<-chan struct{}
	done    chan struct{}
}

var applyResourceRegistry = struct {
	sync.Mutex
	groups map[*applyResourceGroup]struct{}
}{groups: make(map[*applyResourceGroup]struct{})}

func (g *applyResourceGroup) release(completed bool) {
	if g == nil {
		return
	}
	if completed {
		applyResourceRegistry.Lock()
		if _, active := applyResourceRegistry.groups[g]; !active {
			applyResourceRegistry.Unlock()
			return
		}
		g.mu.Lock()
		if g.cleaned {
			g.mu.Unlock()
			applyResourceRegistry.Unlock()
			return
		}
		g.timer = time.AfterFunc(applyCommandListRetention, g.cleanupNow)
		g.mu.Unlock()
		applyResourceRegistry.Unlock()
		return
	}
	g.cleanupNow()
}

func (g *applyResourceGroup) addCleanup(cleanup func()) bool {
	return g.addCleanupWithDrain(cleanup, nil)
}

func (g *applyResourceGroup) addCleanupWithDrain(cleanup func(), drain <-chan struct{}) bool {
	if g == nil || cleanup == nil {
		return false
	}
	g.mu.Lock()
	if g.cleaned {
		g.mu.Unlock()
		cleanup()
		return false
	}
	g.cleanup = append(g.cleanup, cleanup)
	if drain != nil {
		g.drains = append(g.drains, drain)
	}
	g.mu.Unlock()
	return true
}

func (g *applyResourceGroup) cleanupNow() {
	if g == nil {
		return
	}
	g.once.Do(func() {
		g.mu.Lock()
		g.cleaned = true
		timer := g.timer
		cleanups := append([]func(){}, g.cleanup...)
		drains := append([]<-chan struct{}{}, g.drains...)
		g.cleanup = nil
		g.drains = nil
		g.mu.Unlock()
		if timer != nil {
			timer.Stop()
		}
		// Resource callbacks may wait for an uncooperative remote Remove call.
		// Start them together so one item's cancellation is bounded by a single
		// cleanup window rather than five seconds per list file. Remote resource
		// closures reserve their target lease before registration, so the clone
		// cleanup callback cannot close the VFS before these calls have started.
		var cleanupWait sync.WaitGroup
		cleanupWait.Add(len(cleanups))
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanup := cleanups[i]
			go func() {
				defer cleanupWait.Done()
				cleanup()
			}()
		}
		cleanupWait.Wait()
		finish := func() {
			applyResourceRegistry.Lock()
			delete(applyResourceRegistry.groups, g)
			applyResourceRegistry.Unlock()
			close(g.done)
		}
		if len(drains) == 0 {
			finish()
			return
		}
		go func() {
			for _, drain := range drains {
				<-drain
			}
			finish()
		}()
	})
}

func cleanupAllApplyCommandResources() {
	cleanupAllApplyCommandResourcesWithin(applyCommandShutdownCleanupWait)
}

func cleanupAllApplyCommandResourcesWithin(timeout time.Duration) bool {
	applyResourceRegistry.Lock()
	groups := make([]*applyResourceGroup, 0, len(applyResourceRegistry.groups))
	for group := range applyResourceRegistry.groups {
		groups = append(groups, group)
	}
	applyResourceRegistry.Unlock()
	if len(groups) == 0 {
		return true
	}
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(len(groups))
	for _, group := range groups {
		go func() {
			defer wg.Done()
			group.cleanupNow()
			<-group.done
		}()
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		// Some third-party VFS operations cannot honor context cancellation.
		// Shutdown must remain bounded; those cleanups continue best-effort
		// until the process exits instead of blocking the UI indefinitely.
		return false
	}
}

func materializeApplyCommandResources(ctx context.Context, target vfs.VFS, dir string, dialect vfs.CommandDialect, requests []ApplyCommandResourceRequest) (map[int]string, func(bool), error) {
	if len(requests) == 0 {
		return nil, nil, nil
	}
	if dialect == vfs.CommandDialectUnknown {
		return nil, nil, fmt.Errorf("%s", Msg("ApplyCommand.UnknownDialect"))
	}
	if target == nil {
		return nil, nil, fmt.Errorf(Msg("ApplyCommand.ResourceErrorFmt"), fmt.Errorf("nil command host"))
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	group := &applyResourceGroup{done: make(chan struct{})}
	applyResourceRegistry.Lock()
	applyResourceRegistry.groups[group] = struct{}{}
	applyResourceRegistry.Unlock()
	_, localTarget := target.(*vfs.OSVFS)
	var resourceTargetWork *sync.WaitGroup
	if !localTarget {
		resourceTarget := target.Clone()
		if resourceTarget == nil {
			group.cleanupNow()
			return nil, nil, fmt.Errorf(Msg("ApplyCommand.ResourceErrorFmt"), fmt.Errorf("command host could not be captured"))
		}
		resourceTargetWork = &sync.WaitGroup{}
		// Keep the captured target usable through materialization itself as
		// well as any bounded Remove call that must continue after cancellation.
		// Same-instance clones are permitted only when their lifetime is shared
		// independently or Close is a no-op, so they use the same work shield.
		resourceTargetWork.Add(1)
		defer resourceTargetWork.Done()
		ownedClone := !sameVFSInstance(resourceTarget, target)
		drained := make(chan struct{})
		if !group.addCleanupWithDrain(func() {
			go func() {
				resourceTargetWork.Wait()
				if ownedClone {
					_ = resourceTarget.Close()
				}
				close(drained)
			}()
		}, drained) {
			return nil, nil, context.Canceled
		}
		target = resourceTarget
	}
	paths := make(map[int]string, len(requests))
	fail := func(err error) (map[int]string, func(bool), error) {
		group.cleanupNow()
		return nil, nil, err
	}
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return fail(err)
		}
		if request.Kind != ApplyCommandListFileResource {
			return fail(fmt.Errorf("apply command: unsupported resource kind %d", request.Kind))
		}
		data, err := encodeApplyCommandListForTarget(request.ListFile, dialect, target)
		if err != nil {
			return fail(fmt.Errorf(Msg("ApplyCommand.ResourceErrorFmt"), err))
		}
		var (
			resourcePath string
			remove       func()
			createErr    error
		)
		if localTarget {
			resourcePath, remove, createErr = createLocalApplyCommandList(data)
		} else {
			resourcePath, remove, createErr = createRemoteApplyCommandList(ctx, target, dir, data, resourceTargetWork)
		}
		if createErr != nil {
			if errors.Is(createErr, context.Canceled) || errors.Is(createErr, context.DeadlineExceeded) {
				return fail(createErr)
			}
			return fail(fmt.Errorf(Msg("ApplyCommand.ResourceErrorFmt"), createErr))
		}
		if !group.addCleanup(remove) {
			return fail(context.Canceled)
		}
		paths[request.ID] = resourcePath
	}
	if err := ctx.Err(); err != nil {
		return fail(err)
	}
	return paths, group.release, nil
}

func encodeApplyCommandListForTarget(spec ApplyCommandListFileSpec, dialect vfs.CommandDialect, target vfs.VFS) ([]byte, error) {
	for i, entry := range spec.Entries {
		if strings.ContainsAny(entry, "\r\n") {
			return nil, fmt.Errorf("list entry %d contains a line break", i+1)
		}
		if !utf8.ValidString(entry) {
			return nil, fmt.Errorf("list entry %d is not valid UTF-8", i+1)
		}
	}
	data := encodeApplyCommandList(spec, dialect)
	if spec.Encoding != ApplyCommandListANSI {
		return data, nil
	}
	if encoder, ok := target.(vfs.CommandListANSIEncoder); ok {
		return encoder.EncodeCommandListANSI(data)
	}
	if _, local := target.(*vfs.OSVFS); local && runtime.GOOS == "windows" {
		if encoding := vfs.GetSystemANSIEncoding(); encoding != nil {
			return encoding.NewEncoder().Bytes(data)
		}
	}
	return data, nil
}

func encodeApplyCommandList(spec ApplyCommandListFileSpec, dialect vfs.CommandDialect) []byte {
	newline := "\n"
	if dialect == vfs.CommandDialectCmd || dialect == vfs.CommandDialectPowerShell {
		newline = "\r\n"
	}
	text := strings.Join(spec.Lines(), newline)
	if len(spec.Entries) > 0 {
		text += newline
	}
	switch spec.Encoding {
	case ApplyCommandListUTF8BOM:
		return append([]byte{0xef, 0xbb, 0xbf}, []byte(text)...)
	case ApplyCommandListUTF16LE:
		words := utf16.Encode([]rune(text))
		data := make([]byte, 2+len(words)*2)
		data[0], data[1] = 0xff, 0xfe
		for i, word := range words {
			binary.LittleEndian.PutUint16(data[2+i*2:], word)
		}
		return data
	default:
		// ANSI is converted by encodeApplyCommandListForTarget when the command
		// host exposes a panel codepage; this pure formatter keeps UTF-8 as the
		// portable fallback.
		return []byte(text)
	}
}

func createLocalApplyCommandList(data []byte) (string, func(), error) {
	file, err := os.CreateTemp("", "f4-apply-*.lst")
	if err != nil {
		return "", nil, err
	}
	name := file.Name()
	remove := func() { _ = os.Remove(name) }
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		remove()
		return "", nil, err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		remove()
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		remove()
		return "", nil, err
	}
	return name, remove, nil
}

func createRemoteApplyCommandList(ctx context.Context, target vfs.VFS, dir string, data []byte, targetWork *sync.WaitGroup) (string, func(), error) {
	// A few remote libraries expose no cancellable Create/Write/Close calls.
	// Run the complete materialization behind the caller's context when we own
	// a retained target clone. The lease keeps that clone alive if an
	// uncooperative transport continues unwinding after the batch is cancelled.
	if targetWork == nil {
		return createRemoteApplyCommandListBlocking(ctx, target, dir, data, nil)
	}
	type result struct {
		path    string
		remove  func()
		err     error
		abandon chan bool
	}
	results := make(chan result)
	targetWork.Add(1)
	go func() {
		path, remove, err := createRemoteApplyCommandListBlocking(ctx, target, dir, data, targetWork)
		response := result{path: path, remove: remove, err: err, abandon: make(chan bool, 1)}
		results <- response
		if <-response.abandon && response.remove != nil {
			response.remove()
		}
		targetWork.Done()
	}()
	select {
	case response := <-results:
		response.abandon <- false
		return response.path, response.remove, response.err
	case <-ctx.Done():
		go func() {
			response := <-results
			response.abandon <- true
		}()
		return "", nil, ctx.Err()
	}
}

func createRemoteApplyCommandListBlocking(ctx context.Context, target vfs.VFS, dir string, data []byte, targetWork *sync.WaitGroup) (string, func(), error) {
	if target == nil {
		return "", nil, fmt.Errorf("nil target VFS")
	}
	name := ".f4-apply-" + applyResourceNonce() + ".lst"
	resourcePath := target.Join(dir, name)
	privateAtCreate := false
	var (
		file io.WriteCloser
		err  error
	)
	if creator, ok := target.(vfs.PrivateCommandFileCreator); ok {
		file, err = creator.CreatePrivateCommandFile(ctx, resourcePath)
		privateAtCreate = true
	} else {
		file, err = target.Create(ctx, resourcePath)
	}
	if err != nil {
		return "", nil, err
	}
	if !privateAtCreate && target.GetCapabilities().HasUnixPermissions {
		if err := target.SetAttributes(ctx, resourcePath, vfs.VFSItem{UnixMode: 0o600, Uid: -1, Gid: -1}); err != nil {
			_ = file.Close()
			removeRemoteApplyCommandList(ctx, target, resourcePath, targetWork)
			return "", nil, err
		}
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		removeRemoteApplyCommandList(ctx, target, resourcePath, targetWork)
		return "", nil, err
	}
	if err := file.Close(); err != nil {
		removeRemoteApplyCommandList(ctx, target, resourcePath, targetWork)
		return "", nil, err
	}
	leaseReserved := targetWork != nil
	if leaseReserved {
		// Reserve before publishing the cleanup callback. cleanupNow may start
		// the clone-close waiter concurrently with resource callbacks.
		targetWork.Add(1)
	}
	var removeOnce sync.Once
	remove := func() {
		removeOnce.Do(func() {
			removeRemoteApplyCommandListWithLease(context.Background(), target, resourcePath, targetWork, leaseReserved)
		})
	}
	return resourcePath, remove, nil
}

func removeRemoteApplyCommandList(waitCtx context.Context, target vfs.VFS, resourcePath string, targetWork *sync.WaitGroup) {
	removeRemoteApplyCommandListWithLease(waitCtx, target, resourcePath, targetWork, false)
}

func removeRemoteApplyCommandListWithLease(waitCtx context.Context, target vfs.VFS, resourcePath string, targetWork *sync.WaitGroup, leaseReserved bool) {
	removeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	done := make(chan struct{})
	if targetWork != nil && !leaseReserved {
		targetWork.Add(1)
	}
	go func() {
		defer cancel()
		if targetWork != nil {
			defer targetWork.Done()
		}
		_ = target.Remove(removeCtx, resourcePath)
		close(done)
	}()
	select {
	case <-done:
	case <-waitCtx.Done():
	case <-removeCtx.Done():
	}
}

func applyResourceNonce() string {
	var value [12]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("%x", time.Now().UnixNano())
}
