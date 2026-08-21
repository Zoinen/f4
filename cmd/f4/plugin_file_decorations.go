package main

import (
	"errors"
	"strconv"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const fileDecorationColorAttribute = "f4.presentation.color"

var errNilFileDecorationProvider = errors.New("file decoration provider is nil")

// fileDecorationRegistrationToken deliberately has a non-zero size. See the
// corresponding prompt token: distinct zero-sized allocations may compare
// equal, so they cannot safely identify independently unregisterable plugins.
type fileDecorationRegistrationToken struct{ marker byte }

var fileDecorationRegistry = struct {
	sync.RWMutex
	providers map[*fileDecorationRegistrationToken]vfs.FileDecorationProvider
}{providers: make(map[*fileDecorationRegistrationToken]vfs.FileDecorationProvider)}

// RegisterFileDecorationProvider implements the optional cache-backed file
// decoration contribution point.  Providers are invoked by the panel worker,
// never by the panel input/render goroutine, and must return immediately from
// a local cache.
func (c *coreAPI) RegisterFileDecorationProvider(provider vfs.FileDecorationProvider) (vfs.Registration, error) {
	if provider == nil {
		return nil, errNilFileDecorationProvider
	}
	token := &fileDecorationRegistrationToken{}
	fileDecorationRegistry.Lock()
	fileDecorationRegistry.providers[token] = provider
	fileDecorationRegistry.Unlock()
	return &unregisterFunc{fn: func() {
		fileDecorationRegistry.Lock()
		delete(fileDecorationRegistry.providers, token)
		fileDecorationRegistry.Unlock()
	}}, nil
}

func fileDecorationProvidersSnapshot() []vfs.FileDecorationProvider {
	fileDecorationRegistry.RLock()
	providers := make([]vfs.FileDecorationProvider, 0, len(fileDecorationRegistry.providers))
	for _, provider := range fileDecorationRegistry.providers {
		providers = append(providers, provider)
	}
	fileDecorationRegistry.RUnlock()
	return providers
}

func cloneExtendedAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return make(map[string]string)
	}
	cloned := make(map[string]string, len(attributes)+1)
	for key, value := range attributes {
		cloned[key] = value
	}
	return cloned
}

// decorateVFSItem applies only presentation data to a copy.  In particular,
// item.Name stays suitable for Stat/Open/Rename and selection maps even when a
// plugin prepends a status marker to DisplayName.
func decorateVFSItem(filesystem vfs.VFS, directory string, item vfs.VFSItem) vfs.VFSItem {
	if item.Name == ".." || item.Kind != vfs.VFSItemRegular {
		return item
	}
	for _, provider := range fileDecorationProvidersSnapshot() {
		decoration, ok := provider.DecorateFile(filesystem, directory, item)
		if !ok {
			continue
		}
		if decoration.Prefix != "" {
			item.DisplayName = decoration.Prefix + " " + item.PresentationName()
			// A provider label is not a filename, so extension alignment must
			// not split a displayed status prefix from its canonical name.
			item.NoExtension = true
		}
		if len(decoration.Attributes) != 0 || decoration.Color != vfs.FileDecorationDefault {
			item.ExtendedAttributes = cloneExtendedAttributes(item.ExtendedAttributes)
			for key, value := range decoration.Attributes {
				item.ExtendedAttributes[key] = value
			}
			if decoration.Color != vfs.FileDecorationDefault {
				item.ExtendedAttributes[fileDecorationColorAttribute] = strconv.FormatUint(uint64(decoration.Color), 10)
			}
		}
	}
	return item
}

func decorateVFSItems(filesystem vfs.VFS, directory string, items []vfs.VFSItem) []vfs.VFSItem {
	if len(items) == 0 {
		return items
	}
	decorated := make([]vfs.VFSItem, len(items))
	for index, item := range items {
		decorated[index] = decorateVFSItem(filesystem, directory, item)
	}
	return decorated
}

func fileDecorationForeground(base uint64, color vfs.FileDecorationColor) uint64 {
	var role uint64
	switch color {
	case vfs.FileDecorationGitStaged:
		role = vtui.Palette[ColGitStaged]
	case vfs.FileDecorationGitUnstaged:
		role = vtui.Palette[ColGitUnstaged]
	case vfs.FileDecorationGitBoth:
		role = vtui.Palette[ColGitBoth]
	case vfs.FileDecorationGitUntracked:
		role = vtui.Palette[ColGitUntracked]
	case vfs.FileDecorationGitConflict:
		role = vtui.Palette[ColGitConflict]
	case vfs.FileDecorationGitIgnored:
		role = vtui.Palette[ColGitIgnored]
	default:
		return base
	}
	if role&vtui.IsFgRGB != 0 {
		return vtui.SetRGBFore(base, vtui.GetRGBFore(role))
	}
	return vtui.SetIndexFore(base, vtui.GetIndexFore(role))
}

func fileDecorationAttr(item *vfs.VFSItem, base uint64) uint64 {
	if item == nil || len(item.ExtendedAttributes) == 0 {
		return base
	}
	value, ok := item.ExtendedAttributes[fileDecorationColorAttribute]
	if !ok {
		return base
	}
	parsed, err := strconv.ParseUint(value, 10, 8)
	if err != nil {
		return base
	}
	return fileDecorationForeground(base, vfs.FileDecorationColor(parsed))
}

var _ vfs.FileDecorationHost = (*coreAPI)(nil)
