package panel

import "github.com/unxed/f4/internal/plughost"

// panelMediaRegistry owns the media handles referenced by a published catalog.
type panelMediaRegistry interface {
	Register(plughost.MediaSourceRegistration) plughost.ImageSourceDescriptor
	CommitPanel(string, int64, []string)
}

var currentPanelMediaRegistry = func() panelMediaRegistry {
	if broker := plughost.CurrentExtUiMediaBroker(); broker != nil {
		return broker
	}
	return nil
}
