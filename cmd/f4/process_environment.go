package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

var portableEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// cmd.exe's expanded batch line is limited to 8191 UTF-16 code units. The
// transfer command also carries FOR syntax plus randomized internal names;
// 7600 leaves more than 500 units for that fixed transport overhead.
const windowsCmdEnvironmentAssignmentLimit = 7600

// processEnvironmentBackend is deliberately small so the all-or-nothing
// update contract can be tested without changing the test process itself.
type processEnvironmentBackend interface {
	Environ() []string
	Setenv(string, string) error
	Unsetenv(string) error
}

type osProcessEnvironmentBackend struct{}

func (osProcessEnvironmentBackend) Environ() []string               { return os.Environ() }
func (osProcessEnvironmentBackend) Setenv(name, value string) error { return os.Setenv(name, value) }
func (osProcessEnvironmentBackend) Unsetenv(name string) error      { return os.Unsetenv(name) }

type processEnvironmentEntry struct {
	name  string
	value string
}

type processEnvironmentState map[string]processEnvironmentEntry

type processEnvironmentGeneration struct {
	generation uint64
	changes    []vfs.ProcessEnvironmentChange
}

type processEnvironmentManager struct {
	mu          sync.Mutex
	backend     processEnvironmentBackend
	initialized bool
	generation  uint64
	state       processEnvironmentState
	// history contains only changes made through Apply. Merely observing an
	// os.Setenv performed elsewhere advances generation for conflict detection
	// but must never inject that value into already-running shells.
	history      []processEnvironmentGeneration
	historyFloor uint64
	managed      map[string]vfs.ProcessEnvironmentChange
	managedOrder []string
}

func newProcessEnvironmentManager(backend processEnvironmentBackend) *processEnvironmentManager {
	return &processEnvironmentManager{backend: backend}
}

var globalProcessEnvironment = newProcessEnvironmentManager(osProcessEnvironmentBackend{})

var _ vfs.ProcessEnvironmentHost = (*coreAPI)(nil)

func (c *coreAPI) SnapshotProcessEnvironment() vfs.ProcessEnvironmentSnapshot {
	// EnvMan calls Snapshot during plugin initialization, making this the
	// earliest reliable point to establish this process's isolated runtime.
	_ = initializeProcessEnvironmentRuntime()
	snapshot, _ := globalProcessEnvironment.snapshot()
	return snapshot
}

func (c *coreAPI) ApplyProcessEnvironment(changes []vfs.ProcessEnvironmentChange) (vfs.ProcessEnvironmentSnapshot, error) {
	snapshot, generations, err := applyProcessEnvironmentWithRuntime(globalProcessEnvironment, initializeProcessEnvironmentRuntime, changes)
	broadcastProcessEnvironmentGenerations(generations)
	return snapshot, err
}

func applyProcessEnvironmentWithRuntime(manager *processEnvironmentManager, initializeRuntime func() error, changes []vfs.ProcessEnvironmentChange) (vfs.ProcessEnvironmentSnapshot, []processEnvironmentGeneration, error) {
	if err := initializeRuntime(); err != nil {
		return manager.snapshotWithoutObservation(), nil, fmt.Errorf("initialize private environment runtime: %w", err)
	}
	return manager.apply(changes)
}

func validateProcessEnvironmentChanges(changes []vfs.ProcessEnvironmentChange) error {
	for _, change := range changes {
		if !portableEnvironmentName.MatchString(change.Name) {
			return fmt.Errorf("invalid environment variable name %q", change.Name)
		}
		if !change.Unset && strings.ContainsAny(change.Value, "\x00\r\n") {
			return fmt.Errorf("environment variable %q contains a forbidden NUL or line break", change.Name)
		}
		if runtime.GOOS == "windows" && !change.Unset && len(change.Name)+windowsEnvironmentUTF16Length(change.Value) > windowsCmdEnvironmentAssignmentLimit {
			return fmt.Errorf("environment variable %q exceeds the cmd.exe live-update limit", change.Name)
		}
	}
	return nil
}

func windowsEnvironmentUTF16Length(value string) int {
	length := 0
	for _, char := range value {
		length++
		if char > 0xFFFF {
			length++
		}
	}
	return length
}

func processEnvironmentKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}

func splitProcessEnvironmentEntry(raw string) (string, string, bool) {
	separator := strings.IndexByte(raw, '=')
	// Windows may expose pseudo-variables such as =C:=C:\dir. Preserve them
	// in snapshots even though callers cannot edit them through this API.
	if separator == 0 {
		if next := strings.IndexByte(raw[1:], '='); next >= 0 {
			separator = next + 1
		}
	}
	if separator <= 0 {
		return "", "", false
	}
	return raw[:separator], raw[separator+1:], true
}

func (m *processEnvironmentManager) readStateLocked() processEnvironmentState {
	state := make(processEnvironmentState)
	for _, raw := range m.backend.Environ() {
		name, value, ok := splitProcessEnvironmentEntry(raw)
		if !ok {
			continue
		}
		state[processEnvironmentKey(name)] = processEnvironmentEntry{name: name, value: value}
	}
	return state
}

func cloneProcessEnvironmentChanges(changes []vfs.ProcessEnvironmentChange) []vfs.ProcessEnvironmentChange {
	return append([]vfs.ProcessEnvironmentChange(nil), changes...)
}

func processEnvironmentEntryEqual(a processEnvironmentEntry, aOK bool, b processEnvironmentEntry, bOK bool) bool {
	return aOK == bOK && (!aOK || (a.name == b.name && a.value == b.value))
}

func processEnvironmentStateEqual(a, b processEnvironmentState) bool {
	if len(a) != len(b) {
		return false
	}
	for key, aEntry := range a {
		bEntry, ok := b[key]
		if !processEnvironmentEntryEqual(aEntry, true, bEntry, ok) {
			return false
		}
	}
	return true
}

func processEnvironmentStateChanges(before, after processEnvironmentState) []vfs.ProcessEnvironmentChange {
	keys := make([]string, 0, len(before)+len(after))
	seen := make(map[string]bool, len(before)+len(after))
	for key := range before {
		seen[key] = true
		keys = append(keys, key)
	}
	for key := range after {
		if !seen[key] {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	changes := make([]vfs.ProcessEnvironmentChange, 0, len(keys))
	for _, key := range keys {
		oldEntry, oldOK := before[key]
		newEntry, newOK := after[key]
		if processEnvironmentEntryEqual(oldEntry, oldOK, newEntry, newOK) {
			continue
		}
		if newOK {
			changes = append(changes, vfs.ProcessEnvironmentChange{Name: newEntry.name, Value: newEntry.value})
		} else {
			changes = append(changes, vfs.ProcessEnvironmentChange{Name: oldEntry.name, Unset: true})
		}
	}
	return changes
}

func processEnvironmentSnapshot(generation uint64, state processEnvironmentState) vfs.ProcessEnvironmentSnapshot {
	variables := make([]vfs.ProcessEnvironmentVariable, 0, len(state))
	for _, entry := range state {
		variables = append(variables, vfs.ProcessEnvironmentVariable{Name: entry.name, Value: entry.value})
	}
	sort.Slice(variables, func(i, j int) bool {
		iKey := processEnvironmentKey(variables[i].Name)
		jKey := processEnvironmentKey(variables[j].Name)
		if iKey == jKey {
			return variables[i].Name < variables[j].Name
		}
		return iKey < jKey
	})
	return vfs.ProcessEnvironmentSnapshot{Generation: generation, Variables: variables}
}

func (m *processEnvironmentManager) observeLocked() *processEnvironmentGeneration {
	observed := m.readStateLocked()
	if !m.initialized {
		m.initialized = true
		m.state = observed
		return nil
	}
	if processEnvironmentStateEqual(m.state, observed) {
		return nil
	}
	changes := processEnvironmentStateChanges(m.state, observed)
	m.state = observed
	m.generation++
	record := processEnvironmentGeneration{generation: m.generation, changes: changes}
	return &record
}

func (m *processEnvironmentManager) snapshot() (vfs.ProcessEnvironmentSnapshot, []processEnvironmentGeneration) {
	m.mu.Lock()
	record := m.observeLocked()
	snapshot := processEnvironmentSnapshot(m.generation, m.state)
	m.mu.Unlock()
	if record == nil {
		return snapshot, nil
	}
	return snapshot, []processEnvironmentGeneration{*record}
}

func (m *processEnvironmentManager) snapshotWithoutObservation() vfs.ProcessEnvironmentSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized {
		m.initialized = true
		m.state = m.readStateLocked()
	}
	return processEnvironmentSnapshot(m.generation, m.state)
}

func requestedProcessEnvironmentChanges(requested []vfs.ProcessEnvironmentChange, before, after processEnvironmentState) []vfs.ProcessEnvironmentChange {
	// Keep the order of the last occurrence of each requested name. This
	// preserves caller ordering while avoiding redundant shell assignments.
	reversed := make([]vfs.ProcessEnvironmentChange, 0, len(requested))
	seen := make(map[string]bool, len(requested))
	for i := len(requested) - 1; i >= 0; i-- {
		key := processEnvironmentKey(requested[i].Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		oldEntry, oldOK := before[key]
		newEntry, newOK := after[key]
		if processEnvironmentEntryEqual(oldEntry, oldOK, newEntry, newOK) {
			continue
		}
		if newOK {
			reversed = append(reversed, vfs.ProcessEnvironmentChange{Name: newEntry.name, Value: newEntry.value})
		} else {
			name := requested[i].Name
			if oldOK {
				name = oldEntry.name
			}
			reversed = append(reversed, vfs.ProcessEnvironmentChange{Name: name, Unset: true})
		}
	}
	for left, right := 0, len(reversed)-1; left < right; left, right = left+1, right-1 {
		reversed[left], reversed[right] = reversed[right], reversed[left]
	}
	return reversed
}

const processEnvironmentHistoryLimit = 128

func (m *processEnvironmentManager) rememberAppliedLocked(record processEnvironmentGeneration) {
	if len(record.changes) == 0 {
		return
	}
	m.history = append(m.history, processEnvironmentGeneration{
		generation: record.generation,
		changes:    cloneProcessEnvironmentChanges(record.changes),
	})
	if len(m.history) > processEnvironmentHistoryLimit {
		removed := len(m.history) - processEnvironmentHistoryLimit
		m.historyFloor = m.history[removed-1].generation
		m.history = append([]processEnvironmentGeneration(nil), m.history[removed:]...)
	}
	if m.managed == nil {
		m.managed = make(map[string]vfs.ProcessEnvironmentChange)
	}
	for _, change := range record.changes {
		key := processEnvironmentKey(change.Name)
		for i, existing := range m.managedOrder {
			if existing == key {
				m.managedOrder = append(m.managedOrder[:i], m.managedOrder[i+1:]...)
				break
			}
		}
		m.managedOrder = append(m.managedOrder, key)
		m.managed[key] = change
	}
}

func (m *processEnvironmentManager) recordAppliedStateLocked(before processEnvironmentState, observed processEnvironmentState, requested []vfs.ProcessEnvironmentChange) *processEnvironmentGeneration {
	if processEnvironmentStateEqual(m.state, observed) {
		return nil
	}
	changes := requestedProcessEnvironmentChanges(requested, before, observed)
	m.state = observed
	m.generation++
	record := processEnvironmentGeneration{generation: m.generation, changes: changes}
	m.rememberAppliedLocked(record)
	return &record
}

func (m *processEnvironmentManager) apply(changes []vfs.ProcessEnvironmentChange) (vfs.ProcessEnvironmentSnapshot, []processEnvironmentGeneration, error) {
	if err := validateProcessEnvironmentChanges(changes); err != nil {
		snapshot, _ := m.snapshot()
		return snapshot, nil, err
	}

	m.mu.Lock()
	var generations []processEnvironmentGeneration
	// Observation is generation-only. In particular, PROMPT is set directly
	// by PanelsFrame while starting cmd.exe and must not be copied to shells.
	m.observeLocked()
	before := m.state
	originals := make(map[string]processEnvironmentEntry, len(changes))
	originalPresent := make(map[string]bool, len(changes))
	successful := make([]string, 0, len(changes))

	var applyErr error
	for _, change := range changes {
		key := processEnvironmentKey(change.Name)
		if _, seen := originalPresent[key]; !seen {
			originals[key], originalPresent[key] = before[key]
		}
		if change.Unset {
			applyErr = m.backend.Unsetenv(change.Name)
		} else {
			applyErr = m.backend.Setenv(change.Name, change.Value)
		}
		if applyErr != nil {
			applyErr = fmt.Errorf("apply environment variable %q: %w", change.Name, applyErr)
			break
		}
		successful = append(successful, key)
	}

	if applyErr != nil {
		var rollbackErrs []error
		restored := make(map[string]bool, len(successful))
		for i := len(successful) - 1; i >= 0; i-- {
			key := successful[i]
			if restored[key] {
				continue
			}
			restored[key] = true
			entry, present := originals[key]
			var err error
			if present {
				err = m.backend.Setenv(entry.name, entry.value)
			} else {
				// The caller's spelling is portable and key is identical to it on
				// case-sensitive systems. On Windows Unsetenv is case-insensitive.
				err = m.backend.Unsetenv(key)
			}
			if err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("roll back environment variable %q: %w", entry.name, err))
			}
		}

		observed := m.readStateLocked()
		if record := m.recordAppliedStateLocked(before, observed, changes); record != nil && len(record.changes) > 0 {
			generations = append(generations, *record)
		}
		snapshot := processEnvironmentSnapshot(m.generation, m.state)
		m.mu.Unlock()
		if len(rollbackErrs) > 0 {
			applyErr = errors.Join(append([]error{applyErr}, rollbackErrs...)...)
		}
		return snapshot, generations, applyErr
	}

	observed := m.readStateLocked()
	if record := m.recordAppliedStateLocked(before, observed, changes); record != nil && len(record.changes) > 0 {
		generations = append(generations, *record)
	}
	snapshot := processEnvironmentSnapshot(m.generation, m.state)
	m.mu.Unlock()
	return snapshot, generations, nil
}

// changesSince returns a coalesced view of every change newer than generation.
// It is used when a local shell was being created while the process changed.
func (m *processEnvironmentManager) changesSince(generation uint64) (uint64, []vfs.ProcessEnvironmentChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeLocked()
	if generation >= m.generation {
		return m.generation, nil
	}
	if generation < m.historyFloor {
		changes := make([]vfs.ProcessEnvironmentChange, 0, len(m.managedOrder))
		for _, key := range m.managedOrder {
			changes = append(changes, m.managed[key])
		}
		return m.generation, cloneProcessEnvironmentChanges(changes)
	}
	var changes []vfs.ProcessEnvironmentChange
	for _, record := range m.history {
		if record.generation > generation {
			changes = append(changes, record.changes...)
		}
	}
	return m.generation, coalesceProcessEnvironmentChanges(changes)
}

func (m *processEnvironmentManager) currentGeneration() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeLocked()
	return m.generation
}

func coalesceProcessEnvironmentChanges(changes []vfs.ProcessEnvironmentChange) []vfs.ProcessEnvironmentChange {
	result := make([]vfs.ProcessEnvironmentChange, 0, len(changes))
	positions := make(map[string]int, len(changes))
	for _, change := range changes {
		key := processEnvironmentKey(change.Name)
		if old, ok := positions[key]; ok {
			result = append(result[:old], result[old+1:]...)
			delete(positions, key)
			for existingKey, position := range positions {
				if position > old {
					positions[existingKey] = position - 1
				}
			}
		}
		positions[key] = len(result)
		result = append(result, change)
	}
	return result
}
