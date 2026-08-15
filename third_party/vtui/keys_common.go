package vtui

import "github.com/unxed/vtinput"

// defaultRuneForVK returns the character a virtual key stands for when nothing
// better is known.
//
// Backends learn what a key really produces by watching the text the platform
// emits for it, but that only works once the key has been pressed unmodified
// at least once. An accelerator such as Alt+1 or Alt+F is usually the first
// time a key is touched in a session, and arriving with no character leaves
// the application nothing to act on. The letter and digit rows name their own
// character, so this covers exactly the keys accelerators use and returns
// zero for everything else rather than inventing one.
//
// Letters come back lowercase; case is not meaningful for an accelerator and
// the callers that care read the Shift state instead.
func defaultRuneForVK(vk uint16) rune {
	switch {
	case vk >= vtinput.VK_0 && vk <= vtinput.VK_9:
		return rune('0' + (vk - vtinput.VK_0))
	case vk >= vtinput.VK_A && vk <= vtinput.VK_Z:
		return rune('a' + (vk - vtinput.VK_A))
	}
	return 0
}
