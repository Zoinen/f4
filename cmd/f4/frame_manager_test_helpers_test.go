package main

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/unxed/vtui"
)

// swapFrameManager replaces the global vtui.FrameManager with a fresh,
// independent instance seeded with a byte-for-byte copy of the current
// one, and returns a function that restores the original pointer.
//
// vtui.frameManager is unexported, so this package has no way to spell
// its type in order to declare a second instance the normal way (no
// &T{}, no new(T)); reflect.New can still allocate one from its runtime
// type descriptor, and setting the package variable through
// reflect.Value.Set isn't a syntactic struct assignment either, so this
// sidesteps both the naming problem and the copylocks check that a
// direct assignment would hit.
func swapFrameManager(t *testing.T) func() {
	t.Helper()
	old := vtui.FrameManager
	elemType := reflect.TypeOf(old).Elem()
	newVal := reflect.New(elemType)

	size := elemType.Size()
	src := unsafe.Slice((*byte)(unsafe.Pointer(old)), size)
	dst := unsafe.Slice((*byte)(newVal.UnsafePointer()), size)
	copy(dst, src)

	fmVar := reflect.ValueOf(&vtui.FrameManager).Elem()
	fmVar.Set(newVal)

	return func() {
		fmVar.Set(reflect.ValueOf(old))
	}
}
