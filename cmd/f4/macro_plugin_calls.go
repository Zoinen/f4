package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

var errMacroCallProviderNotFound = errors.New("macro plugin-call provider not found")

type registeredMacroCallProvider struct {
	provider vfs.MacroCallProvider
	ids      []string
	token    *struct{}
}

var macroCallRegistry = struct {
	sync.RWMutex
	byID map[string]*registeredMacroCallProvider
}{byID: make(map[string]*registeredMacroCallProvider)}

func normalizeMacroCallID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) >= 2 && id[0] == '{' && id[len(id)-1] == '}' {
		id = strings.TrimSpace(id[1 : len(id)-1])
	}
	return strings.ToLower(id)
}

func (c *coreAPI) RegisterMacroCallProvider(provider vfs.MacroCallProvider) (vfs.Registration, error) {
	if provider.Call == nil {
		return nil, errors.New("macro plugin-call provider has no handler")
	}
	ids := make([]string, 0, len(provider.IDs))
	seen := make(map[string]bool, len(provider.IDs))
	for _, rawID := range provider.IDs {
		id := normalizeMacroCallID(rawID)
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
	registration := &registeredMacroCallProvider{provider: provider, ids: ids, token: token}
	macroCallRegistry.Lock()
	defer macroCallRegistry.Unlock()
	for _, id := range ids {
		if _, exists := macroCallRegistry.byID[id]; exists {
			return nil, fmt.Errorf("macro plugin-call ID %q is already registered", id)
		}
	}
	for _, id := range ids {
		macroCallRegistry.byID[id] = registration
	}

	return &unregisterFunc{fn: func() {
		macroCallRegistry.Lock()
		for _, id := range ids {
			if current := macroCallRegistry.byID[id]; current != nil && current.token == token {
				delete(macroCallRegistry.byID, id)
			}
		}
		macroCallRegistry.Unlock()
	}}, nil
}

func dispatchMacroPluginCall(ctx context.Context, id string, callContext vfs.MacroCallContext, args []any) ([]any, error) {
	normalized := normalizeMacroCallID(id)
	macroCallRegistry.RLock()
	registration := macroCallRegistry.byID[normalized]
	macroCallRegistry.RUnlock()
	if registration == nil {
		return nil, fmt.Errorf("%w: %s", errMacroCallProviderNotFound, id)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return registration.provider.Call(ctx, callContext, args)
}
