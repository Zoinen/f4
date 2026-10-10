//go:build darwin && !ios && !vtui_nococoa

package vtui

import (
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/ebitengine/purego"
)

// cocoaKeyTranslator turns a typing key into text with the current keyboard
// layout, through UCKeyTranslate, and keeps the dead key state between key
// presses so that ´ followed by e types é.
//
// This is the job an AppKit view normally hands to the text input system
// through the NSTextInputClient protocol. That protocol cannot be
// implemented here: several of its methods take or return NSRange and
// NSRect by value, and the FFI layer's callbacks take no structures on
// arm64 and return none anywhere, so the class would not even register.
// Input methods that compose in a window of their own (Chinese, Japanese,
// Korean) need that protocol and do not work; keyboard layouts, dead keys
// included, do.
type cocoaKeyTranslator struct {
	deadKeyState uint32
}

var cocoaKeyLayout struct {
	once sync.Once
	ok   bool

	copyCurrentKeyboardLayoutInputSource func() uintptr
	getInputSourceProperty               func(source, key uintptr) uintptr
	kbdType                              func() uint8
	keyTranslate                         func(layout uintptr, keyCode, action uint16, modifiers, keyboardType, options uint32, deadKeyState *uint32, maxLength uintptr, length *uintptr, text *uint16) int32
	dataGetBytePtr                       func(data uintptr) uintptr
	release                              func(ref uintptr)

	// unicodeKeyLayoutData is the value of kTISPropertyUnicodeKeyLayoutData.
	unicodeKeyLayoutData uintptr
}

// loadCocoaKeyLayout resolves the Text Input Sources and Unicode Utilities
// functions. Without them the backend types what [NSEvent characters]
// holds, which is the same text except that dead keys do not combine.
func loadCocoaKeyLayout() bool {
	l := &cocoaKeyLayout
	l.once.Do(func() {
		carbon, err := purego.Dlopen("/System/Library/Frameworks/Carbon.framework/Carbon", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			DebugLog("COCOA: no Carbon, dead keys will not combine: %v", err)
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY|purego.RTLD_GLOBAL)
		if err != nil {
			return
		}
		for _, fn := range []struct {
			lib  uintptr
			name string
			ptr  any
		}{
			{carbon, "TISCopyCurrentKeyboardLayoutInputSource", &l.copyCurrentKeyboardLayoutInputSource},
			{carbon, "TISGetInputSourceProperty", &l.getInputSourceProperty},
			{carbon, "LMGetKbdType", &l.kbdType},
			{carbon, "UCKeyTranslate", &l.keyTranslate},
			{cf, "CFDataGetBytePtr", &l.dataGetBytePtr},
			{cf, "CFRelease", &l.release},
		} {
			sym, err := purego.Dlsym(fn.lib, fn.name)
			if err != nil {
				DebugLog("COCOA: %s not found, dead keys will not combine: %v", fn.name, err)
				return
			}
			purego.RegisterFunc(fn.ptr, sym)
		}
		// kTISPropertyUnicodeKeyLayoutData is a variable holding a
		// CFStringRef; dlsym gives its address, and memmove reads it.
		keyVar, err := purego.Dlsym(carbon, "kTISPropertyUnicodeKeyLayoutData")
		if err != nil {
			return
		}
		var memmove func(dst unsafe.Pointer, src uintptr, n uintptr) unsafe.Pointer
		sym, err := purego.Dlsym(libSystem, "memmove")
		if err != nil {
			return
		}
		purego.RegisterFunc(&memmove, sym)
		memmove(unsafe.Pointer(&l.unicodeKeyLayoutData), keyVar, unsafe.Sizeof(l.unicodeKeyLayoutData))
		l.ok = l.unicodeKeyLayoutData != 0
	})
	return l.ok
}

// Carbon's modifier bits, shifted down by eight the way UCKeyTranslate
// takes them.
const (
	ucShiftKey     = 1 << (9 - 8)
	ucAlphaLockKey = 1 << (10 - 8)
)

// translate returns the text the key types. ok is false when the layout
// could not be asked, and the caller falls back to the event's own text.
// A dead key types nothing and leaves pending true until the next key.
//
// Only typing keys come here -- Shift and Caps Lock are the only modifiers
// that can be down -- so those are the only ones passed on.
func (t *cocoaKeyTranslator) translate(keyCode uint16, flags uint64) (text string, ok bool) {
	if !loadCocoaKeyLayout() {
		return "", false
	}
	l := &cocoaKeyLayout
	source := l.copyCurrentKeyboardLayoutInputSource()
	if source == 0 {
		return "", false
	}
	defer l.release(source)
	data := l.getInputSourceProperty(source, l.unicodeKeyLayoutData)
	if data == 0 {
		return "", false
	}
	layout := l.dataGetBytePtr(data)
	if layout == 0 {
		return "", false
	}
	var modifiers uint32
	if flags&nsEventModifierFlagShift != 0 {
		modifiers |= ucShiftKey
	}
	if flags&nsEventModifierFlagCapsLock != 0 {
		modifiers |= ucAlphaLockKey
	}
	const kUCKeyActionDown = 0
	var buf [8]uint16
	var n uintptr
	status := l.keyTranslate(layout, keyCode, kUCKeyActionDown, modifiers, uint32(l.kbdType()), 0,
		&t.deadKeyState, uintptr(len(buf)), &n, &buf[0])
	if status != 0 || n > uintptr(len(buf)) {
		t.deadKeyState = 0
		return "", false
	}
	return string(utf16.Decode(buf[:n])), true
}

// pending reports a dead key waiting for the key it combines with.
func (t *cocoaKeyTranslator) pending() bool { return t.deadKeyState != 0 }

// reset forgets a dead key.
func (t *cocoaKeyTranslator) reset() { t.deadKeyState = 0 }
