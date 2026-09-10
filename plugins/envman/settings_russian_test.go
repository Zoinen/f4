package envman

import (
	"github.com/unxed/f4/internal/settingstest"
	"github.com/unxed/f4/sdk/f4settings"
	"testing"
)

func TestSettingsRussianCoverage(t *testing.T) {
	for _, c := range []f4settings.Catalog{(&settingsProvider{}).Catalog()} {
		settingstest.RussianCatalog(t, c)
	}
}
