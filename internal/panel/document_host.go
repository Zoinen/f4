package panel

import (
	"github.com/unxed/f4/internal/config"
)

// Document opening is application orchestration. A panel reports cancellation
// through these seams instead of sharing the application's pending-open map.
var HasPendingDocumentOpen = func(*PanelsFrame) bool { return false }
var CancelDocumentOpen = func(*PanelsFrame) bool { return false }
var ToggleGuiPresentation = func() config.GuiPresentationMode { return config.App.GuiPresentation }
