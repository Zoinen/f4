package semantic

import (
	"sync/atomic"
)

var extUiPanelCatalogMetadataEnabled atomic.Bool

var extUiPanelCatalogRowsEnabled atomic.Bool

var extUiPanelGroupingEnabled atomic.Bool

var PanelCatalogDeltaEnabled atomic.Bool
var DirectoryPreviewsEnabled atomic.Bool

// The negotiated extension set belongs to the current Qt host. A nil pointer
// denotes an older host or a terminal session.
var nativeImageExtensions atomic.Pointer[map[string]struct{}]

func BeginNativeImageExtensions(extensions []string) func() {
	var next *map[string]struct{}
	if extensions != nil {
		formats := make(map[string]struct{}, len(extensions))
		for _, extension := range extensions {
			if extension != "" {
				formats[extension] = struct{}{}
			}
		}
		next = &formats
	}
	previous := nativeImageExtensions.Swap(next)
	return func() { nativeImageExtensions.Store(previous) }
}

func NativeImageExtensions() (map[string]struct{}, bool) {
	if formats := nativeImageExtensions.Load(); formats != nil {
		return *formats, true
	}
	return nil, false
}

func SetPanelCatalogMetadataEnabled(enabled bool) bool {
	return extUiPanelCatalogMetadataEnabled.Swap(enabled)
}

func SetPanelCatalogRowsEnabled(enabled bool) bool {
	return extUiPanelCatalogRowsEnabled.Swap(enabled)
}
func PanelCatalogMetadataIsEnabled() bool {
	return extUiPanelCatalogMetadataEnabled.Load()
}

func PanelCatalogRowsIsEnabled() bool {
	return extUiPanelCatalogRowsEnabled.Load()
}

func SetPanelGroupingEnabled(enabled bool) bool {
	return extUiPanelGroupingEnabled.Swap(enabled)
}

func PanelGroupingIsEnabled() bool {
	return extUiPanelGroupingEnabled.Load()
}
