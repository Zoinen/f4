package vfs

import "github.com/unxed/f4/sdk/f4settings"

// SettingsContributionHost is optional so existing in-process and RPC plugins
// keep their original host contract. Descriptors contain no frontend widgets.
type SettingsContributionHost interface {
	RegisterSettingsProvider(f4settings.Provider) (Registration, error)
}

// SettingsNavigationHost is optional for contextual configuration shortcuts.
// Record identifies a stable provider record ID or display name; an empty
// collection opens its category. No preference is written by navigation.
type SettingsNavigationHost interface {
	OpenSettings(category, collection, record string, create bool) bool
}
