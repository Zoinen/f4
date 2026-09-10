package app

import (
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/nativeui"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
)

func init() {
	plughost.Presentation = nativeui.Adapter{}
	history.PathIdentity = fileops.FolderHistoryPathIdentity
	panel.HasPendingDocumentOpen = func(pf *panel.PanelsFrame) bool { return PendingDocumentOpens[pf] != nil }
	panel.CancelDocumentOpen = CancelPendingDocumentOpen
	panel.ToggleGuiPresentation = ToggleGuiPresentation
}
