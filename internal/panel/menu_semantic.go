package panel

import (
	"github.com/unxed/vtui"
)

func (f *BookmarksFrame) SemanticMenuControl() (*vtui.VMenu, string) { return f.VMenu, f.bottomHint }
func (f *UserMenuFrame) SemanticMenuControl() (*vtui.VMenu, string)  { return f.VMenu, f.BottomHint }
