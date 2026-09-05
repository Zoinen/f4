package main

import "github.com/unxed/f4/vfs"

// rememberedCodepage returns only valid explicit overrides. Older or manually
// edited file_states.json files must not be able to make a view use an
// unsupported decoder forever.
func rememberedCodepage(owner vfs.VFS, path string) (int, bool) {
	if GlobalFileState == nil || path == "" {
		return 0, false
	}
	state := GlobalFileState.GetState(FileStateKey(owner, path))
	if state == nil || state.Codepage <= 0 {
		return 0, false
	}
	if _, ok := vfs.FindCodepage(state.Codepage); !ok {
		return 0, false
	}
	return state.Codepage, true
}

func saveCodepageOverride(owner vfs.VFS, path string, cp int) {
	if GlobalFileState == nil || path == "" {
		return
	}
	if cp > 0 {
		if _, ok := vfs.FindCodepage(cp); !ok {
			return
		}
	}
	GlobalFileState.SaveCodepageAsync(FileStateKey(owner, path), cp)
}
