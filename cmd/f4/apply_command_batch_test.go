package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

type applyBatchTestRunner struct {
	run func(context.Context, string, string, func(string)) (int, error)
}

func (r applyBatchTestRunner) RunCommand(ctx context.Context, dir, command string, cb func(string)) (int, error) {
	return r.run(ctx, dir, command, cb)
}

func TestApplyBatchSequentialOrderAndContinues(t *testing.T) {
	var mu sync.Mutex
	var got []string
	runner := applyBatchTestRunner{run: func(_ context.Context, _, command string, _ func(string)) (int, error) {
		mu.Lock()
		got = append(got, command)
		mu.Unlock()
		if command == "b" {
			return 7, nil
		}
		return 0, nil
	}}
	items := []applyBatchItem{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	res := runApplyCommandBatch(context.Background(), applyBatchRequest{
		Items: items, Runner: runner, Parallelism: 1,
		Expand: func(_ context.Context, _ int, item applyBatchItem) (applyExpandedCommand, error) {
			return applyExpandedCommand{Command: item.Name}, nil
		},
	})
	if fmt.Sprint(got) != "[a b c]" {
		t.Fatalf("order = %v", got)
	}
	if res.Succeeded != 2 || res.Failed != 1 || res.Completed != 3 {
		t.Fatalf("result = %+v", res)
	}
}

func TestApplyBatchParallelBoundAndResultOrder(t *testing.T) {
	var active, maximum atomic.Int32
	runner := applyBatchTestRunner{run: func(_ context.Context, _, command string, _ func(string)) (int, error) {
		n := active.Add(1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		time.Sleep(10 * time.Millisecond)
		active.Add(-1)
		return 0, nil
	}}
	items := make([]applyBatchItem, 12)
	for i := range items {
		items[i].Name = fmt.Sprintf("%02d", i)
	}
	res := runApplyCommandBatch(context.Background(), applyBatchRequest{
		Items: items, Runner: runner, Parallelism: 3,
		Expand: func(_ context.Context, _ int, item applyBatchItem) (applyExpandedCommand, error) {
			return applyExpandedCommand{Command: item.Name}, nil
		},
	})
	if maximum.Load() > 3 || maximum.Load() < 2 {
		t.Fatalf("maximum active = %d", maximum.Load())
	}
	for i, item := range res.Items {
		if item.Index != i || item.Name != items[i].Name || item.State != applyItemSucceeded {
			t.Fatalf("item %d = %+v", i, item)
		}
	}
}

func TestApplyBatchCancelStopsNewItems(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	runner := applyBatchTestRunner{run: func(ctx context.Context, _, _ string, _ func(string)) (int, error) {
		close(started)
		<-ctx.Done()
		return -1, ctx.Err()
	}}
	done := make(chan applyBatchResult, 1)
	go func() {
		done <- runApplyCommandBatch(ctx, applyBatchRequest{
			Items: []applyBatchItem{{Name: "a"}, {Name: "b"}}, Runner: runner, Parallelism: 1,
			Expand: func(_ context.Context, _ int, item applyBatchItem) (applyExpandedCommand, error) {
				return applyExpandedCommand{Command: item.Name}, nil
			},
		})
	}()
	<-started
	cancel()
	res := <-done
	if res.Started != 1 || res.NotStarted != 1 || res.Items[0].State != applyItemCancelled || res.Items[1].State != applyItemPending {
		t.Fatalf("result = %+v", res)
	}
	if res.Cancelled != 1 {
		t.Fatalf("cancelled count = %d, want only the one attempted item", res.Cancelled)
	}
}

func TestApplyBatchExpansionFailureIsAttempted(t *testing.T) {
	runner := applyBatchTestRunner{run: func(context.Context, string, string, func(string)) (int, error) {
		t.Fatal("runner must not be called")
		return 0, nil
	}}
	want := errors.New("bad token")
	res := runApplyCommandBatch(context.Background(), applyBatchRequest{
		Items: []applyBatchItem{{Name: "a"}}, Runner: runner, Parallelism: 1,
		Expand: func(context.Context, int, applyBatchItem) (applyExpandedCommand, error) {
			return applyExpandedCommand{}, want
		},
	})
	if res.Started != 1 || res.Failed != 1 || !errors.Is(res.Items[0].Err, want) {
		t.Fatalf("result = %+v", res)
	}
}

func TestApplyBatchRetainsResourcesWhenCancellationArrivesAfterCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	retained := false
	runner := applyBatchTestRunner{run: func(context.Context, string, string, func(string)) (int, error) {
		cancel()
		return 0, nil
	}}
	result := runApplyCommandBatch(ctx, applyBatchRequest{
		Items: []applyBatchItem{{Name: "a"}}, Runner: runner, Parallelism: 1,
		Expand: func(context.Context, int, applyBatchItem) (applyExpandedCommand, error) {
			return applyExpandedCommand{Command: "done", Cleanup: func(completed bool) { retained = completed }}, nil
		},
	})
	if !retained {
		t.Fatal("completed command resources were deleted immediately after a late cancellation")
	}
	if result.Items[0].State != applyItemCancelled {
		t.Fatalf("late-cancel result = %+v", result.Items[0])
	}
}

func TestApplyTranscriptBounded(t *testing.T) {
	tr := newApplyTranscript()
	for i := 0; i < applyTranscriptMaxLines+25; i++ {
		tr.Add(fmt.Sprintf("line-%d", i))
	}
	lines := tr.Snapshot()
	if len(lines) != applyTranscriptMaxLines+1 {
		t.Fatalf("lines = %d", len(lines))
	}
	if lines[0] != Msg("ApplyCommand.OutputOmitted") {
		t.Fatalf("marker = %q", lines[0])
	}
}

func TestApplyTranscriptByteBoundPreservesUTF8(t *testing.T) {
	tr := newApplyTranscript()
	tr.Add(string([]byte{0xff}) + strings.Repeat("я", applyTranscriptMaxBytes))
	lines := tr.Snapshot()
	if len(lines) != 2 || lines[0] != Msg("ApplyCommand.OutputOmitted") || len(lines[1]) > applyTranscriptMaxBytes || !utf8.ValidString(lines[1]) {
		t.Fatalf("bounded transcript: count=%d marker=%q bytes=%d valid=%v", len(lines), lines[0], len(lines[len(lines)-1]), utf8.ValidString(lines[len(lines)-1]))
	}
}
