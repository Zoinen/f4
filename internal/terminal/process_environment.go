package terminal

import (
	"errors"
	"fmt"
	"github.com/unxed/f4/vfs"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
)

var portableEnvironmentName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// cmd.exe's expanded batch line is limited to 8191 UTF-16 code units. The
// transfer command also carries FOR syntax plus randomized internal names;
// 7600 leaves more than 500 units for that fixed transport overhead.
const WindowsCmdEnvironmentAssignmentLimit = 7600

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

type ProcessEnvironmentEntry struct {
	Name  string
	Value string
}

type ProcessEnvironmentState map[string]ProcessEnvironmentEntry

type ProcessEnvironmentGeneration struct {
	Generation uint64
	Changes    []vfs.ProcessEnvironmentChange
}

type processEnvironmentManager struct {
	mu          sync.Mutex
	backend     processEnvironmentBackend
	initialized bool
	Generation  uint64
	state       ProcessEnvironmentState
	// history contains only changes made through Apply. Merely observing an
	// os.Setenv performed elsewhere advances Generation for conflict detection
	// but must never inject that value into already-running shells.
	history      []ProcessEnvironmentGeneration
	historyFloor uint64
	managed      map[string]vfs.ProcessEnvironmentChange
	managedOrder []string
}

func NewProcessEnvironmentManager(backend processEnvironmentBackend) *processEnvironmentManager {
	return &processEnvironmentManager{backend: backend}
}

var GlobalProcessEnvironment = NewProcessEnvironmentManager(osProcessEnvironmentBackend{})

func ApplyProcessEnvironmentWithRuntime(manager *processEnvironmentManager, initializeRuntime func() error, changes []vfs.ProcessEnvironmentChange) (vfs.ProcessEnvironmentSnapshot, []ProcessEnvironmentGeneration, error) {
	if err := initializeRuntime(); err != nil {
		return manager.snapshotWithoutObservation(), nil, fmt.Errorf("initialize private environment runtime: %w", err)
	}
	return manager.Apply(changes)
}

func validateProcessEnvironmentChanges(changes []vfs.ProcessEnvironmentChange) error {
	for _, change := range changes {
		if !portableEnvironmentName.MatchString(change.Name) {
			return fmt.Errorf("invalid environment variable name %q", change.Name)
		}
		if !change.Unset && strings.ContainsAny(change.Value, "\x00\r\n") {
			return fmt.Errorf("environment variable %q contains a forbidden NUL or line break", change.Name)
		}
		if runtime.GOOS == "windows" && !change.Unset && len(change.Name)+WindowsEnvironmentUTF16Length(change.Value) > WindowsCmdEnvironmentAssignmentLimit {
			return fmt.Errorf("environment variable %q exceeds the cmd.exe live-update limit", change.Name)
		}
	}
	return nil
}

func WindowsEnvironmentUTF16Length(value string) int {
	length := 0
	for _, char := range value {
		length++
		if char > 0xFFFF {
			length++
		}
	}
	return length
}

func ProcessEnvironmentKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}

func SplitProcessEnvironmentEntry(raw string) (string, string, bool) {
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

func (m *processEnvironmentManager) readStateLocked() ProcessEnvironmentState {
	state := make(ProcessEnvironmentState)
	for _, raw := range m.backend.Environ() {
		name, value, ok := SplitProcessEnvironmentEntry(raw)
		if !ok {
			continue
		}
		state[ProcessEnvironmentKey(name)] = ProcessEnvironmentEntry{Name: name, Value: value}
	}
	return state
}

func CloneProcessEnvironmentChanges(changes []vfs.ProcessEnvironmentChange) []vfs.ProcessEnvironmentChange {
	return append([]vfs.ProcessEnvironmentChange(nil), changes...)
}

func processEnvironmentEntryEqual(a ProcessEnvironmentEntry, aOK bool, b ProcessEnvironmentEntry, bOK bool) bool {
	return aOK == bOK && (!aOK || (a.Name == b.Name && a.Value == b.Value))
}

func processEnvironmentStateEqual(a, b ProcessEnvironmentState) bool {
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

func processEnvironmentStateChanges(before, after ProcessEnvironmentState) []vfs.ProcessEnvironmentChange {
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
			changes = append(changes, vfs.ProcessEnvironmentChange{Name: newEntry.Name, Value: newEntry.Value})
		} else {
			changes = append(changes, vfs.ProcessEnvironmentChange{Name: oldEntry.Name, Unset: true})
		}
	}
	return changes
}

func processEnvironmentSnapshot(Generation uint64, state ProcessEnvironmentState) vfs.ProcessEnvironmentSnapshot {
	variables := make([]vfs.ProcessEnvironmentVariable, 0, len(state))
	for _, entry := range state {
		variables = append(variables, vfs.ProcessEnvironmentVariable{Name: entry.Name, Value: entry.Value})
	}
	sort.Slice(variables, func(i, j int) bool {
		iKey := ProcessEnvironmentKey(variables[i].Name)
		jKey := ProcessEnvironmentKey(variables[j].Name)
		if iKey == jKey {
			return variables[i].Name < variables[j].Name
		}
		return iKey < jKey
	})
	return vfs.ProcessEnvironmentSnapshot{Generation: Generation, Variables: variables}
}

func (m *processEnvironmentManager) observeLocked() *ProcessEnvironmentGeneration {
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
	m.Generation++
	record := ProcessEnvironmentGeneration{Generation: m.Generation, Changes: changes}
	return &record
}

func (m *processEnvironmentManager) Snapshot() (vfs.ProcessEnvironmentSnapshot, []ProcessEnvironmentGeneration) {
	m.mu.Lock()
	record := m.observeLocked()
	Snapshot := processEnvironmentSnapshot(m.Generation, m.state)
	m.mu.Unlock()
	if record == nil {
		return Snapshot, nil
	}
	return Snapshot, []ProcessEnvironmentGeneration{*record}
}

func (m *processEnvironmentManager) snapshotWithoutObservation() vfs.ProcessEnvironmentSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized {
		m.initialized = true
		m.state = m.readStateLocked()
	}
	return processEnvironmentSnapshot(m.Generation, m.state)
}

func requestedProcessEnvironmentChanges(requested []vfs.ProcessEnvironmentChange, before, after ProcessEnvironmentState) []vfs.ProcessEnvironmentChange {
	// Keep the order of the last occurrence of each requested name. This
	// preserves caller ordering while avoiding redundant shell assignments.
	reversed := make([]vfs.ProcessEnvironmentChange, 0, len(requested))
	seen := make(map[string]bool, len(requested))
	for i := len(requested) - 1; i >= 0; i-- {
		key := ProcessEnvironmentKey(requested[i].Name)
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
			reversed = append(reversed, vfs.ProcessEnvironmentChange{Name: newEntry.Name, Value: newEntry.Value})
		} else {
			name := requested[i].Name
			if oldOK {
				name = oldEntry.Name
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

func (m *processEnvironmentManager) rememberAppliedLocked(record ProcessEnvironmentGeneration) {
	if len(record.Changes) == 0 {
		return
	}
	m.history = append(m.history, ProcessEnvironmentGeneration{
		Generation: record.Generation,
		Changes:    CloneProcessEnvironmentChanges(record.Changes),
	})
	if len(m.history) > processEnvironmentHistoryLimit {
		removed := len(m.history) - processEnvironmentHistoryLimit
		m.historyFloor = m.history[removed-1].Generation
		m.history = append([]ProcessEnvironmentGeneration(nil), m.history[removed:]...)
	}
	if m.managed == nil {
		m.managed = make(map[string]vfs.ProcessEnvironmentChange)
	}
	for _, change := range record.Changes {
		key := ProcessEnvironmentKey(change.Name)
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

func (m *processEnvironmentManager) recordAppliedStateLocked(before ProcessEnvironmentState, observed ProcessEnvironmentState, requested []vfs.ProcessEnvironmentChange) *ProcessEnvironmentGeneration {
	if processEnvironmentStateEqual(m.state, observed) {
		return nil
	}
	changes := requestedProcessEnvironmentChanges(requested, before, observed)
	m.state = observed
	m.Generation++
	record := ProcessEnvironmentGeneration{Generation: m.Generation, Changes: changes}
	m.rememberAppliedLocked(record)
	return &record
}

func (m *processEnvironmentManager) Apply(changes []vfs.ProcessEnvironmentChange) (vfs.ProcessEnvironmentSnapshot, []ProcessEnvironmentGeneration, error) {
	if err := validateProcessEnvironmentChanges(changes); err != nil {
		Snapshot, _ := m.Snapshot()
		return Snapshot, nil, err
	}

	m.mu.Lock()
	var generations []ProcessEnvironmentGeneration
	// Observation is Generation-only. In particular, PROMPT is set directly
	// by PanelsFrame while starting cmd.exe and must not be copied to shells.
	m.observeLocked()
	before := m.state
	originals := make(map[string]ProcessEnvironmentEntry, len(changes))
	originalPresent := make(map[string]bool, len(changes))
	successful := make([]string, 0, len(changes))

	var applyErr error
	for _, change := range changes {
		key := ProcessEnvironmentKey(change.Name)
		if _, seen := originalPresent[key]; !seen {
			originals[key], originalPresent[key] = before[key]
		}
		if change.Unset {
			applyErr = m.backend.Unsetenv(change.Name)
		} else {
			applyErr = m.backend.Setenv(change.Name, change.Value)
		}
		if applyErr != nil {
			applyErr = fmt.Errorf("Apply environment variable %q: %w", change.Name, applyErr)
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
				err = m.backend.Setenv(entry.Name, entry.Value)
			} else {
				// The caller's spelling is portable and key is identical to it on
				// case-sensitive systems. On Windows Unsetenv is case-insensitive.
				err = m.backend.Unsetenv(key)
			}
			if err != nil {
				rollbackErrs = append(rollbackErrs, fmt.Errorf("roll back environment variable %q: %w", entry.Name, err))
			}
		}

		observed := m.readStateLocked()
		if record := m.recordAppliedStateLocked(before, observed, changes); record != nil && len(record.Changes) > 0 {
			generations = append(generations, *record)
		}
		Snapshot := processEnvironmentSnapshot(m.Generation, m.state)
		m.mu.Unlock()
		if len(rollbackErrs) > 0 {
			applyErr = errors.Join(append([]error{applyErr}, rollbackErrs...)...)
		}
		return Snapshot, generations, applyErr
	}

	observed := m.readStateLocked()
	if record := m.recordAppliedStateLocked(before, observed, changes); record != nil && len(record.Changes) > 0 {
		generations = append(generations, *record)
	}
	Snapshot := processEnvironmentSnapshot(m.Generation, m.state)
	m.mu.Unlock()
	return Snapshot, generations, nil
}

// ChangesSince returns a coalesced view of every change newer than Generation.
// It is used when a local shell was being created while the process changed.
func (m *processEnvironmentManager) ChangesSince(Generation uint64) (uint64, []vfs.ProcessEnvironmentChange) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeLocked()
	if Generation >= m.Generation {
		return m.Generation, nil
	}
	if Generation < m.historyFloor {
		changes := make([]vfs.ProcessEnvironmentChange, 0, len(m.managedOrder))
		for _, key := range m.managedOrder {
			changes = append(changes, m.managed[key])
		}
		return m.Generation, CloneProcessEnvironmentChanges(changes)
	}
	var changes []vfs.ProcessEnvironmentChange
	for _, record := range m.history {
		if record.Generation > Generation {
			changes = append(changes, record.Changes...)
		}
	}
	return m.Generation, CoalesceProcessEnvironmentChanges(changes)
}

func (m *processEnvironmentManager) CurrentGeneration() uint64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observeLocked()
	return m.Generation
}

func CoalesceProcessEnvironmentChanges(changes []vfs.ProcessEnvironmentChange) []vfs.ProcessEnvironmentChange {
	result := make([]vfs.ProcessEnvironmentChange, 0, len(changes))
	positions := make(map[string]int, len(changes))
	for _, change := range changes {
		key := ProcessEnvironmentKey(change.Name)
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
