package vfs

import (
	"context"
	"errors"
	"testing"
)

type noTrashVFS struct{ VFS }

func TestMoveToTrashNeverFallsBack(t *testing.T) {
	err := MoveToTrash(context.Background(), &noTrashVFS{}, "item")
	if !errors.Is(err, ErrTrashUnsupported) {
		t.Fatalf("MoveToTrash error = %v, want ErrTrashUnsupported", err)
	}
}

func TestOperationErrorsExposeSentinels(t *testing.T) {
	partial := &PartialOperationError{Operation: "move", Completed: []string{"a"}, Failed: []string{"b"}, Err: errors.New("failed")}
	if !errors.Is(partial, ErrOperationPartial) {
		t.Fatal("PartialOperationError must match ErrOperationPartial")
	}
	unknown := &UnknownOperationStateError{Operation: "upload", Err: errors.New("connection lost")}
	if !errors.Is(unknown, ErrOperationStateUnknown) {
		t.Fatal("UnknownOperationStateError must match ErrOperationStateUnknown")
	}
}
