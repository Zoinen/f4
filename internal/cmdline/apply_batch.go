package cmdline

import (
	"context"
	"errors"
	"fmt"
	"github.com/unxed/f4/vfs"
	"sync"
)

// ApplyCommandMode selects how an Apply command batch is dispatched.  The
// queued mode deliberately remains sequential: choosing parallel is an
// explicit foreground decision.
type ApplyCommandMode uint8

const (
	ApplyCommandSequential ApplyCommandMode = iota
	ApplyCommandParallel
	ApplyCommandQueued
)

type applyItemState uint8

const (
	applyItemPending applyItemState = iota
	applyItemRunning
	applyItemSucceeded
	applyItemFailed
	applyItemCancelled
)

type ApplyExpandedCommand struct {
	Command string
	Silent  bool
	// Cleanup releases resources created while expanding this item.  completed
	// is true only when a command was successfully handed to the runner and
	// returned; resource stores may use that distinction for delayed cleanup.
	Cleanup func(completed bool)
}

type ApplyBatchItem struct {
	Name          string
	AffectedNames []string
}

type ApplyBatchItemResult struct {
	Index         int
	Name          string
	AffectedNames []string
	Command       string
	ExitCode      int
	State         applyItemState
	Err           error
}

type ApplyBatchResult struct {
	Items                          []ApplyBatchItemResult
	Succeeded, Failed, Cancelled   int
	Started, Completed, NotStarted int
}

type applyBatchEventKind uint8

const (
	applyBatchItemStarted applyBatchEventKind = iota
	applyBatchCommandReady
	applyBatchOutput
	ApplyBatchItemFinished
)

type ApplyBatchEvent struct {
	Kind   applyBatchEventKind
	Index  int
	Total  int
	Name   string
	Line   string
	Silent bool
	Result ApplyBatchItemResult
}

type ApplyBatchRequest struct {
	Dir         string
	Items       []ApplyBatchItem
	Runner      vfs.CommandRunner
	Parallelism int
	Expand      func(ctx context.Context, index int, item ApplyBatchItem) (ApplyExpandedCommand, error)
	Observe     func(ApplyBatchEvent)
}

// runApplyCommandBatch contains no UI or panel access.  Callers may run it in
// vtui.RunAsync, the operation queue, or directly from tests.
func RunApplyCommandBatch(ctx context.Context, req ApplyBatchRequest) ApplyBatchResult {
	result := ApplyBatchResult{Items: make([]ApplyBatchItemResult, len(req.Items))}
	for i, item := range req.Items {
		result.Items[i] = ApplyBatchItemResult{
			Index:         i,
			Name:          item.Name,
			AffectedNames: append([]string(nil), item.AffectedNames...),
			State:         applyItemPending,
			ExitCode:      -1,
		}
	}
	if len(req.Items) == 0 {
		return result
	}
	if req.Runner == nil {
		for i := range result.Items {
			result.Items[i].State = applyItemFailed
			result.Items[i].Err = errors.New("apply command: no command runner")
		}
		result.Failed = len(result.Items)
		result.Completed = len(result.Items)
		return result
	}

	workers := req.Parallelism
	if workers <= 0 || workers > len(req.Items) {
		workers = len(req.Items)
	}
	if workers < 1 {
		workers = 1
	}

	var (
		mu   sync.Mutex
		next int
		wg   sync.WaitGroup
	)
	emit := func(ev ApplyBatchEvent) {
		if req.Observe != nil {
			req.Observe(ev)
		}
	}

	worker := func() {
		defer wg.Done()
		for {
			mu.Lock()
			if ctx.Err() != nil || next >= len(req.Items) {
				mu.Unlock()
				return
			}
			idx := next
			next++
			item := req.Items[idx]
			result.Items[idx].State = applyItemRunning
			result.Started++
			mu.Unlock()

			emit(ApplyBatchEvent{Kind: applyBatchItemStarted, Index: idx, Total: len(req.Items), Name: item.Name})
			expanded, expandErr := req.Expand(ctx, idx, item)
			itemResult := ApplyBatchItemResult{
				Index:         idx,
				Name:          item.Name,
				AffectedNames: append([]string(nil), item.AffectedNames...),
				ExitCode:      -1,
			}
			completedCommand := false
			if expandErr != nil {
				itemResult.Err = expandErr
				if errors.Is(expandErr, context.Canceled) || ctx.Err() != nil {
					itemResult.State = applyItemCancelled
				} else {
					itemResult.State = applyItemFailed
				}
			} else if expanded.Command == "" {
				itemResult.State = applyItemFailed
				itemResult.Err = errors.New("apply command: expansion produced an empty command")
			} else {
				itemResult.Command = expanded.Command
				emit(ApplyBatchEvent{
					Kind: applyBatchCommandReady, Index: idx, Total: len(req.Items), Name: item.Name,
					Line: expanded.Command, Silent: expanded.Silent,
				})
				code, runErr := req.Runner.RunCommand(ctx, req.Dir, expanded.Command, func(line string) {
					emit(ApplyBatchEvent{Kind: applyBatchOutput, Index: idx, Total: len(req.Items), Name: item.Name, Line: line})
				})
				itemResult.ExitCode = code
				cancelled := errors.Is(runErr, context.Canceled) || ctx.Err() != nil
				// A nil runner error means the process reached completion even if
				// the shared context flipped immediately afterward. Retain list
				// resources for that completed consumer (and possible detached
				// children); runners report context.Canceled when cancellation won.
				completedCommand = runErr == nil
				switch {
				case cancelled:
					itemResult.State = applyItemCancelled
					if runErr != nil {
						itemResult.Err = runErr
					} else {
						itemResult.Err = ctx.Err()
					}
				case runErr != nil:
					itemResult.State = applyItemFailed
					itemResult.Err = runErr
				case code != 0:
					itemResult.State = applyItemFailed
					itemResult.Err = fmt.Errorf("exit status %d", code)
				default:
					itemResult.State = applyItemSucceeded
				}
			}
			if expanded.Cleanup != nil {
				expanded.Cleanup(completedCommand)
			}

			mu.Lock()
			result.Items[idx] = itemResult
			result.Completed++
			switch itemResult.State {
			case applyItemSucceeded:
				result.Succeeded++
			case applyItemCancelled:
				result.Cancelled++
			default:
				result.Failed++
			}
			mu.Unlock()
			emit(ApplyBatchEvent{Kind: ApplyBatchItemFinished, Index: idx, Total: len(req.Items), Name: item.Name, Result: itemResult})
		}
	}

	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go worker()
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	result.NotStarted = len(req.Items) - result.Started
	return result
}

// AddTranscript appends one line to the run's transcript. The transcript
// itself stays private: it is written from the worker goroutine as well, and a
// caller holding it could write without the model's ordering.
func (m *ApplyBatchViewModel) AddTranscript(line string) {
	m.Transcript.Add(line)
}
