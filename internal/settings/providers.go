package settings

import (
	"context"
	"sync"

	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/f4/vfs"
)

var settingsProviders = struct {
	sync.RWMutex
	providers []f4settings.Provider
}{}

// Each registration has a distinct comparable identity, even for value providers.
type registeredSettingsProvider struct {
	provider f4settings.Provider
	catalog  f4settings.Catalog
}

func (p *registeredSettingsProvider) Catalog() f4settings.Catalog { return p.catalog }
func (p *registeredSettingsProvider) Begin(ctx context.Context) (*f4settings.Draft, error) {
	return p.provider.Begin(ctx)
}

func RegisterProvider(p f4settings.Provider) (vfs.Registration, error) {
	if p == nil {
		return nil, settingsError("nil settings provider")
	}
	c := settingsCatalogChoiceHelp(p.Catalog())
	if err := f4settings.ValidateCatalog(c); err != nil {
		return nil, err
	}
	p = &registeredSettingsProvider{p, c}
	settingsProviders.Lock()
	for _, existing := range settingsProviders.providers {
		if existing.Catalog().ID == c.ID {
			settingsProviders.Unlock()
			return nil, settingsError("duplicate settings provider %s", c.ID)
		}
	}
	settingsProviders.providers = append(settingsProviders.providers, p)
	settingsProviders.Unlock()
	return plughost.NewUnregisterFunc(func() {
		settingsProviders.Lock()
		defer settingsProviders.Unlock()
		for i, current := range settingsProviders.providers {
			if current == p {
				settingsProviders.providers = append(settingsProviders.providers[:i], settingsProviders.providers[i+1:]...)
				break
			}
		}
	}), nil
}

func settingsProviderAlive(p f4settings.Provider) bool {
	settingsProviders.RLock()
	defer settingsProviders.RUnlock()
	for _, current := range settingsProviders.providers {
		if current == p {
			return true
		}
	}
	return false
}

type settingsSession struct {
	provider    f4settings.Provider
	catalog     f4settings.Catalog
	draft       *f4settings.Draft
	contributed bool
}

func beginSettingsSessions(ctx context.Context) ([]*settingsSession, error) {
	coreCatalog := (coreSettingsProvider{}).Catalog()
	providers := []f4settings.Provider{coreSettingsProvider{catalog: &coreCatalog}, newCoreRecordSettingsProvider(), aiSettingsProvider{}, hotkeySettingsProvider{}, settingsOperationsProvider{}, pluginSettingsProvider{}, &catalogSettingsProvider{}}
	settingsProviders.RLock()
	providers = append(providers, settingsProviders.providers...)
	settingsProviders.RUnlock()
	var sessions []*settingsSession
	for i, p := range providers {
		d, err := p.Begin(ctx)
		if err != nil {
			for _, s := range sessions {
				s.draft.Close()
			}
			return nil, err
		}
		sessions = append(sessions, &settingsSession{p, settingsCatalogChoiceHelp(p.Catalog()), d, i > 6})
	}
	return sessions, nil
}
