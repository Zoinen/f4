package macro

import (
	"context"
	"errors"
	"fmt"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
	"strings"
	"sync"
)

var errMacroCallProviderNotFound = errors.New("macro plugin-call provider not found")

type RegisteredMacroCallProvider struct {
	provider vfs.MacroCallProvider
	ids      []string
	token    *struct{}
}

var MacroCallRegistry = struct {
	sync.RWMutex
	byID map[string]*RegisteredMacroCallProvider
}{byID: make(map[string]*RegisteredMacroCallProvider)}

func NormalizeMacroCallID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) >= 2 && id[0] == '{' && id[len(id)-1] == '}' {
		id = strings.TrimSpace(id[1 : len(id)-1])
	}
	return strings.ToLower(id)
}

func RegisterMacroCallProvider(provider vfs.MacroCallProvider) (vfs.Registration, error) {
	if provider.Call == nil {
		return nil, errors.New("macro plugin-call provider has no handler")
	}
	ids := make([]string, 0, len(provider.IDs))
	seen := make(map[string]bool, len(provider.IDs))
	for _, rawID := range provider.IDs {
		id := NormalizeMacroCallID(rawID)
		if id == "" {
			return nil, errors.New("macro plugin-call provider has an empty ID")
		}
		if seen[id] {
			return nil, fmt.Errorf("macro plugin-call provider repeats ID %q", rawID)
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, errors.New("macro plugin-call provider has no IDs")
	}

	token := &struct{}{}
	registration := &RegisteredMacroCallProvider{provider: provider, ids: ids, token: token}
	MacroCallRegistry.Lock()
	defer MacroCallRegistry.Unlock()
	for _, id := range ids {
		if _, exists := MacroCallRegistry.byID[id]; exists {
			return nil, fmt.Errorf("macro plugin-call ID %q is already registered", id)
		}
	}
	for _, id := range ids {
		MacroCallRegistry.byID[id] = registration
	}

	return plughost.NewUnregisterFunc(func() {
		MacroCallRegistry.Lock()
		for _, id := range ids {
			if current := MacroCallRegistry.byID[id]; current != nil && current.token == token {
				delete(MacroCallRegistry.byID, id)
			}
		}
		MacroCallRegistry.Unlock()
	}), nil
}

func DispatchMacroPluginCall(ctx context.Context, id string, callContext vfs.MacroCallContext, args []any) ([]any, error) {
	normalized := NormalizeMacroCallID(id)
	MacroCallRegistry.RLock()
	registration := MacroCallRegistry.byID[normalized]
	MacroCallRegistry.RUnlock()
	if registration == nil {
		return nil, fmt.Errorf("%w: %s", errMacroCallProviderNotFound, id)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return registration.provider.Call(ctx, callContext, args)
}
