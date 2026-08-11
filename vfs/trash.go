package vfs

import (
	"context"
	"errors"
	"fmt"
)

// TrashVFS is an optional capability for moving an item to a recoverable
// trash location. Remove remains the permanent-delete primitive used by moves
// and other cleanup paths.
type TrashVFS interface {
	MoveToTrash(ctx context.Context, path string) error
}

// DeleteDisposition is captured when a delete action is scheduled. Keeping it
// in the task prevents a later configuration change from turning a queued
// trash operation into permanent deletion (or the reverse).
type DeleteDisposition uint8

const (
	DeletePermanently DeleteDisposition = iota
	DeleteToTrash
)

var (
	// ErrTrashUnsupported is returned when the selected VFS has no recoverable
	// trash implementation. Callers must not silently fall back to Remove.
	ErrTrashUnsupported = errors.New("trash is not supported by this file system")

	// ErrOperationPartial identifies operations which changed only part of
	// their target set. PartialOperationError carries the concrete paths.
	ErrOperationPartial = errors.New("operation completed only partially")

	// ErrOperationStateUnknown identifies an interrupted remote/native
	// operation whose final state cannot be established safely.
	ErrOperationStateUnknown = errors.New("operation final state is unknown")
)

// PartialOperationError reports a non-atomic operation that completed for
// some paths and failed for others.
type PartialOperationError struct {
	Operation string
	Completed []string
	Failed    []string
	Err       error
}

func (e *PartialOperationError) Error() string {
	op := e.Operation
	if op == "" {
		op = "operation"
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v (%d completed, %d failed)", op, e.Err, len(e.Completed), len(e.Failed))
	}
	return fmt.Sprintf("%s: %d completed, %d failed", op, len(e.Completed), len(e.Failed))
}

func (e *PartialOperationError) Unwrap() error { return e.Err }

func (e *PartialOperationError) Is(target error) bool { return target == ErrOperationPartial }

// UnknownOperationStateError wraps the provider/native error that made the
// final operation state impossible to determine.
type UnknownOperationStateError struct {
	Operation string
	Err       error
}

func (e *UnknownOperationStateError) Error() string {
	if e.Operation == "" {
		if e.Err == nil {
			return ErrOperationStateUnknown.Error()
		}
		return fmt.Sprintf("%v: %v", ErrOperationStateUnknown, e.Err)
	}
	if e.Err == nil {
		return fmt.Sprintf("%s: %v", e.Operation, ErrOperationStateUnknown)
	}
	return fmt.Sprintf("%s: %v: %v", e.Operation, ErrOperationStateUnknown, e.Err)
}

func (e *UnknownOperationStateError) Unwrap() error { return e.Err }

func (e *UnknownOperationStateError) Is(target error) bool { return target == ErrOperationStateUnknown }

// MoveToTrash invokes the optional trash capability without a destructive
// fallback.
func MoveToTrash(ctx context.Context, filesystem VFS, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	trash, ok := filesystem.(TrashVFS)
	if !ok {
		return fmt.Errorf("%w: %T", ErrTrashUnsupported, filesystem)
	}
	return trash.MoveToTrash(ctx, path)
}
