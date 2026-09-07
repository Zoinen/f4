package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

var (
	errCommandPrefixUnregistered = errors.New("command prefix registration is no longer active")
	commandPrefixRegistry        = struct {
		sync.RWMutex
		byID     map[string]*commandPrefixRegistration
		byPrefix map[string]*commandPrefixRegistration
	}{
		byID:     make(map[string]*commandPrefixRegistration),
		byPrefix: make(map[string]*commandPrefixRegistration),
	}
)

type commandPrefixRegistration struct {
	id      string
	prefix  string
	handler func(vfs.App, string)
	active  bool
	once    sync.Once
}

type commandPrefixSnapshotEntry struct {
	id     string
	prefix string
}

func commandPrefixSnapshot() []commandPrefixSnapshotEntry {
	commandPrefixRegistry.RLock()
	defer commandPrefixRegistry.RUnlock()
	result := make([]commandPrefixSnapshotEntry, 0, len(commandPrefixRegistry.byID))
	for id, registration := range commandPrefixRegistry.byID {
		if registration == nil || !registration.active || registration.prefix == "" {
			continue
		}
		result = append(result, commandPrefixSnapshotEntry{id: id, prefix: registration.prefix})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].id < result[j].id })
	return result
}

func normalizeCommandPrefix(prefix string) (string, error) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", nil
	}
	for index, r := range prefix {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if index > 0 {
			valid = valid || r >= '0' && r <= '9' || r == '_' || r == '-'
		}
		if !valid {
			return "", fmt.Errorf("invalid command prefix %q", prefix)
		}
	}
	return strings.ToLower(prefix), nil
}

func (c *coreAPI) RegisterCommandPrefix(id, prefix string, handler func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("command prefix registration ID is empty")
	}
	if handler == nil {
		return nil, fmt.Errorf("command prefix %q has no handler", id)
	}
	normalized, err := normalizeCommandPrefix(prefix)
	if err != nil {
		return nil, err
	}

	registration := &commandPrefixRegistration{id: id, prefix: normalized, handler: handler, active: true}
	commandPrefixRegistry.Lock()
	defer commandPrefixRegistry.Unlock()
	if _, exists := commandPrefixRegistry.byID[id]; exists {
		return nil, fmt.Errorf("command prefix registration %q already exists", id)
	}
	if owner, exists := commandPrefixRegistry.byPrefix[normalized]; normalized != "" && exists {
		return nil, fmt.Errorf("command prefix %q is already registered by %q", prefix, owner.id)
	}
	commandPrefixRegistry.byID[id] = registration
	if normalized != "" {
		commandPrefixRegistry.byPrefix[normalized] = registration
	}
	return registration, nil
}

func (r *commandPrefixRegistration) SetPrefix(prefix string) error {
	normalized, err := normalizeCommandPrefix(prefix)
	if err != nil {
		return err
	}
	commandPrefixRegistry.Lock()
	defer commandPrefixRegistry.Unlock()
	if r == nil || !r.active || commandPrefixRegistry.byID[r.id] != r {
		return errCommandPrefixUnregistered
	}
	if normalized == r.prefix {
		return nil
	}
	if owner, exists := commandPrefixRegistry.byPrefix[normalized]; normalized != "" && exists && owner != r {
		return fmt.Errorf("command prefix %q is already registered by %q", prefix, owner.id)
	}
	if r.prefix != "" {
		delete(commandPrefixRegistry.byPrefix, r.prefix)
	}
	r.prefix = normalized
	if normalized != "" {
		commandPrefixRegistry.byPrefix[normalized] = r
	}
	return nil
}

func (r *commandPrefixRegistration) Unregister() {
	if r == nil {
		return
	}
	r.once.Do(func() {
		commandPrefixRegistry.Lock()
		if commandPrefixRegistry.byID[r.id] == r {
			delete(commandPrefixRegistry.byID, r.id)
		}
		if r.prefix != "" && commandPrefixRegistry.byPrefix[r.prefix] == r {
			delete(commandPrefixRegistry.byPrefix, r.prefix)
		}
		r.active = false
		commandPrefixRegistry.Unlock()
	})
}

// dispatchCommandPrefix consumes input only when the text before its first
// colon names a registered prefix. The argument is deliberately left raw so
// each plugin can apply its own quoting rules.
func dispatchCommandPrefix(app vfs.App, input string) bool {
	colon := strings.IndexByte(input, ':')
	if colon <= 0 {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(input[:colon]))
	commandPrefixRegistry.RLock()
	registration := commandPrefixRegistry.byPrefix[normalized]
	if registration != nil && registration.active {
		handler := registration.handler
		commandPrefixRegistry.RUnlock()
		handler(app, input[colon+1:])
		return true
	}
	commandPrefixRegistry.RUnlock()
	return false
}
