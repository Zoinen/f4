package panel

import (
	"sync/atomic"
	"testing"

	"github.com/unxed/f4/internal/cmdline"
)

func TestEffectiveApplyCommandWorkersSequentialCoverageBatch29(t *testing.T) {
	if got := EffectiveApplyCommandWorkers(cmdline.ApplyCommandSequential, 8, 20, 4); got != 1 {
		t.Fatalf("sequential workers = %d, want 1", got)
	}
}

func TestEffectiveApplyCommandWorkersConfiguredCoverageBatch29(t *testing.T) {
	if got := EffectiveApplyCommandWorkers(cmdline.ApplyCommandParallel, 3, 10, 0); got != 3 {
		t.Fatalf("configured workers = %d, want 3", got)
	}
}

func TestEffectiveApplyCommandWorkersUnlimitedUsesCountCoverageBatch29(t *testing.T) {
	if got := EffectiveApplyCommandWorkers(cmdline.ApplyCommandParallel, 0, 7, 0); got != 7 {
		t.Fatalf("unlimited workers = %d, want 7", got)
	}
}

func TestEffectiveApplyCommandWorkersClampsToCountCoverageBatch29(t *testing.T) {
	if got := EffectiveApplyCommandWorkers(cmdline.ApplyCommandParallel, 20, 4, 0); got != 4 {
		t.Fatalf("count-clamped workers = %d, want 4", got)
	}
}

func TestEffectiveApplyCommandWorkersClampsToProviderCoverageBatch29(t *testing.T) {
	if got := EffectiveApplyCommandWorkers(cmdline.ApplyCommandParallel, 8, 20, 3); got != 3 {
		t.Fatalf("provider-clamped workers = %d, want 3", got)
	}
}

func TestEffectiveApplyCommandWorkersNeverReturnsZeroCoverageBatch29(t *testing.T) {
	if got := EffectiveApplyCommandWorkers(cmdline.ApplyCommandParallel, 0, 0, 0); got != 1 {
		t.Fatalf("empty batch workers = %d, want 1", got)
	}
}

func TestApplyCommandFileForTargetUsesSelectedMetadataCoverageBatch29(t *testing.T) {
	capture := ApplyPanelCapture{}
	capture.Snapshot.Selected = []cmdline.ApplyCommandFile{{Name: "a.txt", ShortName: "A.TXT"}}
	if got := applyCommandFileForTarget(capture, "a.txt"); got.ShortName != "A.TXT" {
		t.Fatalf("selected metadata = %+v", got)
	}
}

func TestApplyCommandFileForTargetFallbackCoverageBatch29(t *testing.T) {
	if got := applyCommandFileForTarget(ApplyPanelCapture{}, "missing.txt"); got.Name != "missing.txt" || got.ShortName != "missing.txt" {
		t.Fatalf("fallback metadata = %+v", got)
	}
}

func TestRegisterForegroundApplyCommandIsIdempotentCoverageBatch29(t *testing.T) {
	before := activeForegroundApplyCommandCount()
	finish := RegisterForegroundApplyCommand(func() {})
	if got := activeForegroundApplyCommandCount(); got != before+1 {
		t.Fatalf("registered count = %d, want %d", got, before+1)
	}
	finish()
	finish()
	if got := activeForegroundApplyCommandCount(); got != before {
		t.Fatalf("cleaned count = %d, want %d", got, before)
	}
}

func TestCancelAllForegroundApplyCommandsCoverageBatch29(t *testing.T) {
	var canceled atomic.Int32
	finish := RegisterForegroundApplyCommand(func() { canceled.Add(1) })
	CancelAllForegroundApplyCommands()
	finish()
	if canceled.Load() != 1 {
		t.Fatalf("cancel calls = %d, want 1", canceled.Load())
	}
}
