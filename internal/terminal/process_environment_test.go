package terminal

import (
	"errors"
	"github.com/unxed/f4/vfs"
	"runtime"
	"strings"
	"testing"
)

type fakeProcessEnvironmentBackend struct {
	entries map[string]ProcessEnvironmentEntry
	order   []string
	ops     []string
	calls   int
	fail    map[int]error
}

func newFakeProcessEnvironmentBackend(values ...string) *fakeProcessEnvironmentBackend {
	fake := &fakeProcessEnvironmentBackend{entries: make(map[string]ProcessEnvironmentEntry)}
	for _, raw := range values {
		name, value, ok := SplitProcessEnvironmentEntry(raw)
		if !ok {
			continue
		}
		key := ProcessEnvironmentKey(name)
		fake.entries[key] = ProcessEnvironmentEntry{Name: name, Value: value}
		fake.order = append(fake.order, key)
	}
	return fake
}

func (f *fakeProcessEnvironmentBackend) Environ() []string {
	result := make([]string, 0, len(f.entries))
	seen := make(map[string]bool, len(f.entries))
	for _, key := range f.order {
		if entry, ok := f.entries[key]; ok && !seen[key] {
			result = append(result, entry.Name+"="+entry.Value)
			seen[key] = true
		}
	}
	for key, entry := range f.entries {
		if !seen[key] {
			result = append(result, entry.Name+"="+entry.Value)
		}
	}
	return result
}

func (f *fakeProcessEnvironmentBackend) operationError() error {
	f.calls++
	if f.fail != nil {
		return f.fail[f.calls]
	}
	return nil
}

func (f *fakeProcessEnvironmentBackend) Setenv(name, value string) error {
	f.ops = append(f.ops, "set:"+name)
	if err := f.operationError(); err != nil {
		return err
	}
	key := ProcessEnvironmentKey(name)
	if _, exists := f.entries[key]; !exists {
		f.order = append(f.order, key)
	}
	f.entries[key] = ProcessEnvironmentEntry{Name: name, Value: value}
	return nil
}

func (f *fakeProcessEnvironmentBackend) Unsetenv(name string) error {
	f.ops = append(f.ops, "unset:"+name)
	if err := f.operationError(); err != nil {
		return err
	}
	delete(f.entries, ProcessEnvironmentKey(name))
	return nil
}

func snapshotVariable(Snapshot vfs.ProcessEnvironmentSnapshot, name string) (string, bool) {
	key := ProcessEnvironmentKey(name)
	for _, variable := range Snapshot.Variables {
		if ProcessEnvironmentKey(variable.Name) == key {
			return variable.Value, true
		}
	}
	return "", false
}

func TestProcessEnvironmentSnapshotStableAndNoOpGeneration(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("B=2", "A=1")
	manager := NewProcessEnvironmentManager(backend)
	Snapshot, records := manager.Snapshot()
	if Snapshot.Generation != 0 || len(records) != 0 {
		t.Fatalf("initial Snapshot = Generation %d, records %v", Snapshot.Generation, records)
	}
	if got := []string{Snapshot.Variables[0].Name, Snapshot.Variables[1].Name}; strings.Join(got, ",") != "A,B" {
		t.Fatalf("Snapshot order = %v", got)
	}

	Snapshot, records, err := manager.Apply([]vfs.ProcessEnvironmentChange{{Name: "A", Value: "1"}})
	if err != nil || Snapshot.Generation != 0 || len(records) != 0 {
		t.Fatalf("no-op Apply = Generation %d, records %v, error %v", Snapshot.Generation, records, err)
	}
	if strings.Join(backend.ops, ",") != "set:A" {
		t.Fatalf("backend operations = %v", backend.ops)
	}
}

func TestProcessEnvironmentApplyPreservesOrderAndReturnsActualSnapshot(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old", "B=old")
	manager := NewProcessEnvironmentManager(backend)
	manager.Snapshot()
	changes := []vfs.ProcessEnvironmentChange{
		{Name: "B", Unset: true},
		{Name: "A", Value: "new"},
		{Name: "C", Value: "three"},
	}
	Snapshot, records, err := manager.Apply(changes)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(backend.ops, ","); got != "unset:B,set:A,set:C" {
		t.Fatalf("backend operation order = %s", got)
	}
	if Snapshot.Generation != 1 || len(records) != 1 || records[0].Generation != 1 {
		t.Fatalf("Apply generations = Snapshot %d, records %#v", Snapshot.Generation, records)
	}
	if got, ok := snapshotVariable(Snapshot, "A"); !ok || got != "new" {
		t.Fatalf("A in actual Snapshot = %q, %v", got, ok)
	}
	if _, ok := snapshotVariable(Snapshot, "B"); ok {
		t.Fatal("B remained in actual Snapshot")
	}
	if got := []string{records[0].Changes[0].Name, records[0].Changes[1].Name, records[0].Changes[2].Name}; strings.Join(got, ",") != "B,A,C" {
		t.Fatalf("delivered change order = %v", got)
	}
}

func TestProcessEnvironmentValidatesWholeBatchBeforeMutation(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old")
	manager := NewProcessEnvironmentManager(backend)
	manager.Snapshot()
	_, _, err := manager.Apply([]vfs.ProcessEnvironmentChange{
		{Name: "A", Value: "new"},
		{Name: "NOT-PORTABLE", Value: "bad"},
	})
	if err == nil || len(backend.ops) != 0 {
		t.Fatalf("invalid batch error = %v, operations = %v", err, backend.ops)
	}
	_, _, err = manager.Apply([]vfs.ProcessEnvironmentChange{{Name: "A", Value: "line\nbreak"}})
	if err == nil || len(backend.ops) != 0 {
		t.Fatalf("line-break validation error = %v, operations = %v", err, backend.ops)
	}
	// Value is deliberately ignored for Unset.
	_, _, err = manager.Apply([]vfs.ProcessEnvironmentChange{{Name: "A", Value: "line\nbreak", Unset: true}})
	if err != nil {
		t.Fatalf("unset rejected ignored value: %v", err)
	}
}

func TestProcessEnvironmentRejectsUntransportableWindowsValueBeforeMutation(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe live-update limit")
	}
	const name = "BOUNDARY"
	allowed := strings.Repeat("x", WindowsCmdEnvironmentAssignmentLimit-len(name))
	allowedBackend := newFakeProcessEnvironmentBackend()
	allowedManager := NewProcessEnvironmentManager(allowedBackend)
	allowedManager.Snapshot()
	if _, _, err := allowedManager.Apply([]vfs.ProcessEnvironmentChange{{Name: name, Value: allowed}}); err != nil {
		t.Fatalf("exact cmd.exe boundary rejected: %v", err)
	}
	if len(allowedBackend.ops) != 1 {
		t.Fatalf("exact boundary operations = %v", allowedBackend.ops)
	}

	backend := newFakeProcessEnvironmentBackend("A=old")
	manager := NewProcessEnvironmentManager(backend)
	manager.Snapshot()
	_, _, err := manager.Apply([]vfs.ProcessEnvironmentChange{{Name: name, Value: allowed + "x"}})
	if err == nil {
		t.Fatal("expected cmd.exe length validation error")
	}
	if len(backend.ops) != 0 {
		t.Fatalf("overlong value mutated backend: %v", backend.ops)
	}
}

func TestProcessEnvironmentRollsBackInReverseOrder(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old-a", "B=old-b")
	backend.fail = map[int]error{3: errors.New("injected Apply failure")}
	manager := NewProcessEnvironmentManager(backend)
	manager.Snapshot()
	Snapshot, records, err := manager.Apply([]vfs.ProcessEnvironmentChange{
		{Name: "A", Value: "new-a"},
		{Name: "B", Unset: true},
		{Name: "C", Value: "new-c"},
	})
	if err == nil {
		t.Fatal("expected Apply error")
	}
	if got := strings.Join(backend.ops, ","); got != "set:A,unset:B,set:C,set:B,set:A" {
		t.Fatalf("operation/rollback order = %s", got)
	}
	if Snapshot.Generation != 0 || len(records) != 0 {
		t.Fatalf("rolled-back Generation = %d, records %#v", Snapshot.Generation, records)
	}
	if got, _ := snapshotVariable(Snapshot, "A"); got != "old-a" {
		t.Fatalf("A after rollback = %q", got)
	}
	if got, _ := snapshotVariable(Snapshot, "B"); got != "old-b" {
		t.Fatalf("B after rollback = %q", got)
	}
}

func TestProcessEnvironmentReportsAndDistributesRollbackDrift(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old")
	backend.fail = map[int]error{
		2: errors.New("injected Apply failure"),
		3: errors.New("injected rollback failure"),
	}
	manager := NewProcessEnvironmentManager(backend)
	manager.Snapshot()
	Snapshot, records, err := manager.Apply([]vfs.ProcessEnvironmentChange{
		{Name: "A", Value: "new"},
		{Name: "B", Value: "fail"},
	})
	if err == nil || !strings.Contains(err.Error(), "roll back") {
		t.Fatalf("rollback error = %v", err)
	}
	if Snapshot.Generation != 1 || len(records) != 1 || len(records[0].Changes) != 1 {
		t.Fatalf("rollback drift = Snapshot %d, records %#v", Snapshot.Generation, records)
	}
	if got, _ := snapshotVariable(Snapshot, "A"); got != "new" {
		t.Fatalf("actual A after failed rollback = %q", got)
	}
}

func TestProcessEnvironmentExternalDriftIsNeverInApplyDelivery(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend("A=old", "PROMPT=original")
	manager := NewProcessEnvironmentManager(backend)
	manager.Snapshot()
	if err := backend.Setenv("PROMPT", "external"); err != nil {
		t.Fatal(err)
	}
	backend.ops = nil
	Snapshot, records, err := manager.Apply([]vfs.ProcessEnvironmentChange{{Name: "A", Value: "new"}})
	if err != nil {
		t.Fatal(err)
	}
	if Snapshot.Generation != 2 || len(records) != 1 || len(records[0].Changes) != 1 || records[0].Changes[0].Name != "A" {
		t.Fatalf("external drift leaked into delivery: Snapshot %d, records %#v", Snapshot.Generation, records)
	}
}

func TestProcessEnvironmentHistoryUsesManagedFallback(t *testing.T) {
	backend := newFakeProcessEnvironmentBackend()
	manager := NewProcessEnvironmentManager(backend)
	manager.Snapshot()
	for i := 0; i < processEnvironmentHistoryLimit+2; i++ {
		name := "V" + string(rune('A'+i%26))
		value := string(rune('0' + i%10))
		if _, _, err := manager.Apply([]vfs.ProcessEnvironmentChange{{Name: name, Value: value}}); err != nil {
			t.Fatal(err)
		}
	}
	Generation, changes := manager.ChangesSince(0)
	if Generation == 0 || len(changes) == 0 || manager.historyFloor == 0 {
		t.Fatalf("fallback Generation %d, changes %d, floor %d", Generation, len(changes), manager.historyFloor)
	}
}
