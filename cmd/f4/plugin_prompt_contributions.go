package main

import (
	"errors"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var errNilPromptSegmentProvider = errors.New("prompt segment provider is nil")

// promptSegmentRegistrationToken deliberately has a non-zero size. Pointers
// to distinct zero-sized values are permitted to compare equal in Go, which
// would let one plugin registration replace another in the registry below.
type promptSegmentRegistrationToken struct{ marker byte }

var promptSegmentRegistry = struct {
	sync.RWMutex
	providers map[*promptSegmentRegistrationToken]vfs.PromptSegmentProvider
}{providers: make(map[*promptSegmentRegistrationToken]vfs.PromptSegmentProvider)}

// RegisterPromptSegmentProvider implements the optional vfs prompt extension.
// Providers are called on the UI render path, so their contract is deliberately
// cache-only; slow discovery belongs in the plugin's own worker.
func (c *coreAPI) RegisterPromptSegmentProvider(provider vfs.PromptSegmentProvider) (vfs.Registration, error) {
	if provider == nil {
		return nil, errNilPromptSegmentProvider
	}
	token := &promptSegmentRegistrationToken{}
	promptSegmentRegistry.Lock()
	promptSegmentRegistry.providers[token] = provider
	promptSegmentRegistry.Unlock()
	return &unregisterFunc{fn: func() {
		promptSegmentRegistry.Lock()
		delete(promptSegmentRegistry.providers, token)
		promptSegmentRegistry.Unlock()
	}}, nil
}

// promptSegmentsSnapshot holds the lock only while copying references.  The
// provider call itself may take its own cache lock but must not perform I/O.
func promptSegmentsSnapshot(filesystem vfs.VFS, path string) []vfs.PromptSegment {
	promptSegmentRegistry.RLock()
	providers := make([]vfs.PromptSegmentProvider, 0, len(promptSegmentRegistry.providers))
	for _, provider := range promptSegmentRegistry.providers {
		providers = append(providers, provider)
	}
	promptSegmentRegistry.RUnlock()

	segments := make([]vfs.PromptSegment, 0, len(providers))
	for _, provider := range providers {
		if segment, ok := provider.PromptSegment(filesystem, path); ok && segment.Text != "" {
			segments = append(segments, segment)
		}
	}
	return segments
}

func promptSegmentAttr(color vfs.PromptSegmentColor) uint64 {
	switch color {
	case vfs.PromptSegmentGitBranch:
		return vtui.Palette[ColGitPromptBranch]
	case vfs.PromptSegmentGitDetached:
		return vtui.Palette[ColGitPromptDetached]
	case vfs.PromptSegmentGitUnborn:
		return vtui.Palette[ColGitPromptUnborn]
	default:
		return vtui.Palette[ColCommandLinePrompt]
	}
}

var _ vfs.PromptSegmentHost = (*coreAPI)(nil)
