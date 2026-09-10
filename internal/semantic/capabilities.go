package semantic

import (
	"sync/atomic"
)

var extUiPanelCatalogMetadataEnabled atomic.Bool

var extUiPanelCatalogRowsEnabled atomic.Bool

var PanelCatalogDeltaEnabled atomic.Bool

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
