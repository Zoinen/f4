package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// Accelerators such as Alt+1 and Alt+F need a character to act on, and are
// usually the first time a key is touched in a session, before any backend
// has had a chance to learn what it produces.
func TestDefaultRuneForVK_CoversAcceleratorKeys(t *testing.T) {
	for i := 0; i <= 9; i++ {
		vk := uint16(vtinput.VK_0 + uint16(i))
		if got, want := defaultRuneForVK(vk), rune('0'+i); got != want {
			t.Errorf("defaultRuneForVK(VK_%d) = %q, want %q", i, got, want)
		}
	}
	for i := 0; i < 26; i++ {
		vk := uint16(vtinput.VK_A + uint16(i))
		if got, want := defaultRuneForVK(vk), rune('a'+i); got != want {
			t.Errorf("defaultRuneForVK(VK_%c) = %q, want %q", 'A'+i, got, want)
		}
	}
}

// Everything else must return nothing rather than invent a character.
func TestDefaultRuneForVK_LeavesNonTextKeysAlone(t *testing.T) {
	for _, vk := range []uint16{
		vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_UP, vtinput.VK_DOWN,
		vtinput.VK_F1, vtinput.VK_F12, vtinput.VK_RETURN, vtinput.VK_ESCAPE,
		vtinput.VK_BACK, vtinput.VK_TAB, vtinput.VK_DELETE, vtinput.VK_HOME,
		vtinput.VK_LMENU, vtinput.VK_LCONTROL, vtinput.VK_LSHIFT, 0,
	} {
		if got := defaultRuneForVK(vk); got != 0 {
			t.Errorf("defaultRuneForVK(%d) = %q, want no character", vk, got)
		}
	}
}
