package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/unxed/vtui"
)

const f4HistoryPersistenceDebounce = 100 * time.Millisecond

type HistoryRecord struct {
	Name string `json:"name"`
	Dir  string `json:"dir,omitempty"`
	// Extra is the pre-rich-history spelling of Dir. Keep reading and
	// writing it for imported/older records, but use Dir for new records.
	Extra     string    `json:"extra,omitempty"`
	Timestamp time.Time `json:"timestamp,omitempty"`
	Lock      bool      `json:"lock,omitempty"`
}

const (
	historyTypeCommands = iota
	historyTypeFolders
	historyTypeViewEdit
	historyTypeCount
)

const (
	historyShowDateTime = iota
	historyShowDate
	historyShowNone
)

func (r HistoryRecord) directory() string {
	if r.Dir != "" {
		return r.Dir
	}
	return r.Extra
}

func (r HistoryRecord) DisplayText() string {
	res := ""
	if !r.Timestamp.IsZero() {
		res += r.Timestamp.Format("15:04:05 ")
	}
	if extra := r.directory(); extra != "" {
		if len(extra) > 15 {
			extra = "..." + extra[len(extra)-12:]
		}
		if !strings.HasSuffix(extra, "/") && !strings.HasSuffix(extra, "\\") {
			res += extra + "/ "
		} else {
			res += extra + " "
		}
	}
	res += r.Name
	return res
}

type F4HistoryProvider struct {
	mu        sync.Mutex
	controlMu sync.Mutex
	path      string
	data      map[string][]string
	rich      map[string][]HistoryRecord

	revision          uint64
	persistedRevision uint64
	lastPersistErr    error
	pendingKind       string
	pendingID         string
	pendingTrace      *navigationBenchmarkTrace
	worker            *historyPersistenceWorker
	closing           bool
	closed            bool
	closeDone         chan struct{}
	closeErr          error

	// Tests can lengthen the debounce and intercept the actual write without
	// changing production timing or package-global state.
	saveDebounce time.Duration
	writeFile    func(string, []byte, os.FileMode) error
}

type historyPersistenceWorker struct {
	wake    chan struct{}
	control chan historyPersistenceRequest
}

type historyPersistenceRequest struct {
	revision uint64
	stop     bool
	done     chan error
}

type historyPersistenceSnapshot struct {
	revision uint64
	path     string
	data     map[string][]string
	rich     map[string][]HistoryRecord
	kind     string
	id       string
	trace    *navigationBenchmarkTrace
	write    func(string, []byte, os.FileMode) error
}

func NewF4HistoryProvider() *F4HistoryProvider {
	p := filepath.Join(GetF4ConfigDir(), "history.json")
	hp := &F4HistoryProvider{
		path: p,
		data: make(map[string][]string),
		rich: make(map[string][]HistoryRecord),
	}
	hp.load()
	return hp
}

func (hp *F4HistoryProvider) ensureMapsLocked() {
	if hp.data == nil {
		hp.data = make(map[string][]string)
	}
	if hp.rich == nil {
		hp.rich = make(map[string][]HistoryRecord)
	}
}

// lockForMutation serializes a rare save racing with Close. Close is terminal
// for the current worker, but a later caller may safely reuse the provider and
// starts a fresh worker rather than writing concurrently with the old one.
func (hp *F4HistoryProvider) lockForMutation() {
	for {
		hp.mu.Lock()
		if !hp.closing {
			if hp.closed {
				hp.closed = false
				hp.closeErr = nil
			}
			hp.ensureMapsLocked()
			return
		}
		done := hp.closeDone
		hp.mu.Unlock()
		if done != nil {
			<-done
		}
	}
}

func (hp *F4HistoryProvider) debounceLocked() time.Duration {
	if hp.saveDebounce > 0 {
		return hp.saveDebounce
	}
	return f4HistoryPersistenceDebounce
}

func (hp *F4HistoryProvider) ensureWorkerLocked() *historyPersistenceWorker {
	if hp.worker != nil {
		return hp.worker
	}
	worker := &historyPersistenceWorker{
		wake:    make(chan struct{}, 1),
		control: make(chan historyPersistenceRequest),
	}
	hp.worker = worker
	go hp.runPersistenceWorker(worker)
	return worker
}

func (hp *F4HistoryProvider) markDirtyLocked(kind, id string, trace *navigationBenchmarkTrace) uint64 {
	hp.revision++
	hp.pendingKind = kind
	hp.pendingID = id
	hp.pendingTrace = trace
	worker := hp.ensureWorkerLocked()
	select {
	case worker.wake <- struct{}{}:
	default:
	}
	return hp.revision
}

func clonePlainHistory(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string][]string, len(source))
	for id, history := range source {
		result[id] = append([]string(nil), history...)
	}
	return result
}

func cloneRichHistory(source map[string][]HistoryRecord) map[string][]HistoryRecord {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string][]HistoryRecord, len(source))
	for id, history := range source {
		result[id] = append([]HistoryRecord(nil), history...)
	}
	return result
}

func (hp *F4HistoryProvider) pendingSnapshot(target uint64) (historyPersistenceSnapshot, bool) {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if hp.persistedRevision >= target || hp.persistedRevision >= hp.revision {
		return historyPersistenceSnapshot{}, false
	}
	write := hp.writeFile
	if write == nil {
		write = os.WriteFile
	}
	return historyPersistenceSnapshot{
		revision: hp.revision,
		path:     hp.path,
		data:     clonePlainHistory(hp.data),
		rich:     cloneRichHistory(hp.rich),
		kind:     hp.pendingKind,
		id:       hp.pendingID,
		trace:    hp.pendingTrace,
		write:    write,
	}, true
}

func persistHistorySnapshot(snapshot historyPersistenceSnapshot) error {
	benchmark := snapshot.trace
	if benchmark != nil {
		benchmark.event("history.mkdir.begin", "go.history", "kind", snapshot.kind,
			"historyId", snapshot.id, "revision", snapshot.revision,
			"path", filepath.Dir(snapshot.path))
	}
	mkdirErr := os.MkdirAll(filepath.Dir(snapshot.path), 0755)
	if benchmark != nil {
		fields := []any{"kind", snapshot.kind, "historyId", snapshot.id,
			"revision", snapshot.revision, "path", filepath.Dir(snapshot.path), "ok", mkdirErr == nil}
		if mkdirErr != nil {
			fields = append(fields, "error", mkdirErr.Error())
		}
		benchmark.event("history.mkdir.end", "go.history", fields...)
	}
	if mkdirErr != nil {
		return mkdirErr
	}

	wrapper := struct {
		Data map[string][]string        `json:"data,omitempty"`
		Rich map[string][]HistoryRecord `json:"rich,omitempty"`
	}{Data: snapshot.data, Rich: snapshot.rich}
	if benchmark != nil {
		benchmark.event("history.marshal.begin", "go.history", "kind", snapshot.kind,
			"historyId", snapshot.id, "revision", snapshot.revision,
			"plainGroups", len(snapshot.data), "richGroups", len(snapshot.rich))
	}
	file, err := json.MarshalIndent(wrapper, "", "  ")
	if benchmark != nil {
		fields := []any{"kind", snapshot.kind, "historyId", snapshot.id,
			"revision", snapshot.revision, "ok", err == nil, "payloadBytes", len(file)}
		if err != nil {
			fields = append(fields, "error", err.Error())
		}
		benchmark.event("history.marshal.end", "go.history", fields...)
	}
	if err != nil {
		return err
	}

	if benchmark != nil {
		benchmark.event("history.write.begin", "go.history", "kind", snapshot.kind,
			"historyId", snapshot.id, "revision", snapshot.revision,
			"path", snapshot.path, "payloadBytes", len(file))
	}
	err = snapshot.write(snapshot.path, file, 0644)
	if benchmark != nil {
		fields := []any{"kind", snapshot.kind, "historyId", snapshot.id,
			"revision", snapshot.revision, "path", snapshot.path,
			"payloadBytes", len(file), "ok", err == nil}
		if err != nil {
			fields = append(fields, "error", err.Error())
		}
		benchmark.event("history.write.end", "go.history", fields...)
	}
	return err
}

func (hp *F4HistoryProvider) persistThrough(target uint64) error {
	for {
		snapshot, pending := hp.pendingSnapshot(target)
		if !pending {
			hp.mu.Lock()
			err := hp.lastPersistErr
			if hp.persistedRevision >= target {
				err = nil
			}
			hp.mu.Unlock()
			return err
		}

		err := persistHistorySnapshot(snapshot)
		hp.mu.Lock()
		if err == nil {
			if snapshot.revision > hp.persistedRevision {
				hp.persistedRevision = snapshot.revision
			}
			hp.lastPersistErr = nil
		} else {
			hp.lastPersistErr = err
		}
		hp.mu.Unlock()
		if err != nil {
			return err
		}
	}
}

func stopAndDrainTimer(timer *time.Timer) {
	if timer == nil {
		return
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
}

func (hp *F4HistoryProvider) runPersistenceWorker(worker *historyPersistenceWorker) {
	var timer *time.Timer
	var timerC <-chan time.Time
	for {
		select {
		case <-worker.wake:
			hp.mu.Lock()
			debounce := hp.debounceLocked()
			hp.mu.Unlock()
			if timer == nil {
				timer = time.NewTimer(debounce)
			} else {
				stopAndDrainTimer(timer)
				timer.Reset(debounce)
			}
			timerC = timer.C
		case <-timerC:
			timerC = nil
			hp.mu.Lock()
			target := hp.revision
			hp.mu.Unlock()
			_ = hp.persistThrough(target)
		case request := <-worker.control:
			stopAndDrainTimer(timer)
			timerC = nil
			err := hp.persistThrough(request.revision)
			request.done <- err
			if request.stop {
				return
			}
		}
	}
}

// Flush synchronously persists every in-memory revision accepted before the
// call. Newer mutations may be folded into the same snapshot, but an older
// snapshot can never overwrite them because one worker owns all disk writes.
func (hp *F4HistoryProvider) Flush() error {
	hp.controlMu.Lock()
	defer hp.controlMu.Unlock()

	for {
		hp.mu.Lock()
		if hp.closing {
			done := hp.closeDone
			hp.mu.Unlock()
			if done != nil {
				<-done
			}
			continue
		}
		target := hp.revision
		if hp.persistedRevision >= target {
			hp.mu.Unlock()
			return nil
		}
		worker := hp.ensureWorkerLocked()
		hp.mu.Unlock()

		done := make(chan error, 1)
		worker.control <- historyPersistenceRequest{revision: target, done: done}
		return <-done
	}
}

// Close is used by the application shutdown defer. It flushes the latest
// revision and stops the provider's worker; a later Save* call can safely
// reopen the provider, which keeps tests and embedded sessions reusable.
func (hp *F4HistoryProvider) Close() error {
	hp.controlMu.Lock()
	defer hp.controlMu.Unlock()

	hp.mu.Lock()
	if hp.closed {
		err := hp.closeErr
		hp.mu.Unlock()
		return err
	}
	hp.closing = true
	hp.closeDone = make(chan struct{})
	target := hp.revision
	worker := hp.worker
	if worker == nil && hp.persistedRevision < target {
		worker = hp.ensureWorkerLocked()
	}
	doneClosing := hp.closeDone
	hp.mu.Unlock()

	var err error
	if worker != nil {
		done := make(chan error, 1)
		worker.control <- historyPersistenceRequest{revision: target, stop: true, done: done}
		err = <-done
	}

	hp.mu.Lock()
	if hp.worker == worker {
		hp.worker = nil
	}
	hp.closing = false
	hp.closed = true
	hp.closeErr = err
	close(doneClosing)
	hp.mu.Unlock()
	return err
}

func (hp *F4HistoryProvider) load() {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	file, err := os.ReadFile(hp.path)
	if err == nil {
		var wrapper struct {
			Data map[string][]string        `json:"data,omitempty"`
			Rich map[string][]HistoryRecord `json:"rich,omitempty"`
		}
		if err := json.Unmarshal(file, &wrapper); err == nil && (wrapper.Data != nil || wrapper.Rich != nil) {
			if wrapper.Data != nil {
				hp.data = wrapper.Data
			}
			if wrapper.Rich != nil {
				hp.rich = wrapper.Rich
			}
		} else {
			var oldData map[string][]string
			if err := json.Unmarshal(file, &oldData); err == nil {
				hp.data = oldData
			}
		}
	}
	if hp.data == nil {
		hp.data = make(map[string][]string)
	}
	if hp.rich == nil {
		hp.rich = make(map[string][]HistoryRecord)
	}
	// A wrapper written before rich history was introduced still has the
	// command and folder buckets in Data. Promote those buckets lazily while
	// retaining Data as the string-compatible view used by vtui.Edit.
	for _, id := range []string{"cmdline", "folders"} {
		if _, ok := hp.rich[id]; ok {
			continue
		}
		if names, ok := hp.data[id]; ok {
			hp.rich[id] = recordsFromNames(names)
		}
	}
	for id, records := range hp.rich {
		if _, ok := hp.data[id]; !ok && (id == "cmdline" || id == "folders") {
			hp.data[id] = extractHistoryNames(records)
		}
	}
}

func (hp *F4HistoryProvider) LoadHistory(id string) []string {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if items, ok := hp.data[id]; ok {
		// Return a copy to avoid concurrent slice modification issues
		res := make([]string, len(items))
		copy(res, items)
		return res
	}
	return nil
}

func (hp *F4HistoryProvider) SaveHistory(id string, history []string) {
	benchmark := navigationBenchmarkCurrentUI()
	hp.lockForMutation()
	hp.data[id] = append([]string(nil), history...)
	if id == "cmdline" || id == "folders" {
		hp.rich[id] = mergeHistoryNames(hp.rich[id], history)
	}
	revision := hp.markDirtyLocked("plain", id, benchmark)
	hp.mu.Unlock()
	if benchmark != nil {
		benchmark.event("history.persistence.scheduled", "go.ui", "kind", "plain",
			"historyId", id, "revision", revision, "items", len(history))
	}
}
func (hp *F4HistoryProvider) LoadRichHistory(id string) []HistoryRecord {
	hp.mu.Lock()
	defer hp.mu.Unlock()
	if items, ok := hp.rich[id]; ok {
		res := make([]HistoryRecord, len(items))
		copy(res, items)
		return res
	}
	return nil
}

func (hp *F4HistoryProvider) SaveRichHistory(id string, history []HistoryRecord) {
	benchmark := navigationBenchmarkCurrentUI()
	hp.lockForMutation()
	hp.rich[id] = append([]HistoryRecord(nil), history...)
	if id == "cmdline" || id == "folders" {
		hp.data[id] = extractHistoryNames(history)
	}
	revision := hp.markDirtyLocked("rich", id, benchmark)
	hp.mu.Unlock()
	if benchmark != nil {
		benchmark.event("history.persistence.scheduled", "go.ui", "kind", "rich",
			"historyId", id, "revision", revision, "items", len(history))
	}
}

func recordsFromNames(names []string) []HistoryRecord {
	if len(names) == 0 {
		return nil
	}
	records := make([]HistoryRecord, 0, len(names))
	for _, name := range names {
		records = append(records, HistoryRecord{Name: name})
	}
	return records
}

func extractHistoryNames(records []HistoryRecord) []string {
	if len(records) == 0 {
		return nil
	}
	names := make([]string, 0, len(records))
	for _, record := range records {
		names = append(names, record.Name)
	}
	return names
}

// mergeHistoryNames updates the string-compatible view without throwing away
// metadata that belongs to an entry which is still present. This is needed
// because vtui.Edit can save a plain []string history after rich history has
// already been loaded.
func mergeHistoryNames(old []HistoryRecord, names []string) []HistoryRecord {
	if len(names) == 0 {
		return nil
	}
	byName := make(map[string]HistoryRecord, len(old))
	for _, record := range old {
		if _, exists := byName[record.Name]; !exists {
			byName[record.Name] = record
		}
	}
	merged := make([]HistoryRecord, 0, len(names))
	for _, name := range names {
		record := byName[name]
		record.Name = name
		merged = append(merged, record)
	}
	return merged
}

func limitRichHistory(history []HistoryRecord, limit int) []HistoryRecord {
	if limit <= 0 || len(history) <= limit {
		return history
	}
	locked := 0
	for _, record := range history {
		if record.Lock {
			locked++
		}
	}
	unlockedBudget := limit - locked
	if unlockedBudget < 0 {
		unlockedBudget = 0
	}
	kept := make([]HistoryRecord, 0, limit)
	for _, record := range history {
		if record.Lock {
			kept = append(kept, record)
			continue
		}
		if unlockedBudget > 0 {
			kept = append(kept, record)
			unlockedBudget--
		}
	}
	return kept
}

func mergeFolderHistoryRecords(plain []string, rich []HistoryRecord) []HistoryRecord {
	richByIdentity := make(map[string]HistoryRecord, len(rich))
	for _, candidate := range rich {
		identity, valid := folderHistoryPathIdentity(candidate.Name)
		if !valid {
			continue
		}
		if existing, present := richByIdentity[identity]; present {
			existing.Lock = existing.Lock || candidate.Lock
			richByIdentity[identity] = existing
		} else {
			richByIdentity[identity] = candidate
		}
	}

	records := make([]HistoryRecord, 0, len(plain))
	recordByIdentity := make(map[string]int, len(plain))
	for _, path := range plain {
		identity, valid := folderHistoryPathIdentity(path)
		record := HistoryRecord{}
		if valid {
			record = richByIdentity[identity]
		}
		if record.Name == "" {
			record.Name = path
		}
		record.Name = path
		if duplicate, present := recordByIdentity[identity]; valid && present {
			records[duplicate].Lock = records[duplicate].Lock || record.Lock
			continue
		}
		if valid {
			recordByIdentity[identity] = len(records)
		}
		records = append(records, record)
	}
	return records
}

func (hp *F4HistoryProvider) SaveFolderHistory(records []HistoryRecord) {
	benchmark := navigationBenchmarkCurrentUI()
	records = append([]HistoryRecord(nil), records...)
	names := extractNames(records)
	hp.lockForMutation()
	hp.rich["folders"] = records
	hp.data["folders"] = names
	revision := hp.markDirtyLocked("folder_combined", "folders", benchmark)
	hp.mu.Unlock()
	if benchmark != nil {
		benchmark.event("history.persistence.scheduled", "go.ui", "kind", "folder_combined",
			"historyId", "folders", "revision", revision, "items", len(records))
	}
}

// addFolderHistory updates the rich records and the plain MRU projection in a
// single critical section. Concurrent callers are ordered by lock acquisition,
// so none can overwrite an entry inserted by an earlier accepted navigation.
func (hp *F4HistoryProvider) addFolderHistory(path string, benchmark *navigationBenchmarkTrace) (before, after int, revision uint64) {
	hp.lockForMutation()
	records := mergeFolderHistoryRecords(hp.data["folders"], hp.rich["folders"])
	before = len(records)
	current := HistoryRecord{Name: path}
	newHistory := []HistoryRecord{current}
	for _, record := range records {
		if sameFolderHistoryPath(record.Name, path) {
			newHistory[0].Lock = newHistory[0].Lock || record.Lock
			continue
		}
		newHistory = append(newHistory, record)
	}
	newHistory = limitRichHistory(newHistory, 100)
	hp.rich["folders"] = append([]HistoryRecord(nil), newHistory...)
	hp.data["folders"] = extractNames(newHistory)
	revision = hp.markDirtyLocked("folder_combined", "folders", benchmark)
	after = len(newHistory)
	hp.mu.Unlock()
	return before, after, revision
}

func loadFolderHistoryRecords(provider vtui.HistoryProvider) ([]HistoryRecord, *F4HistoryProvider) {
	benchmark := navigationBenchmarkCurrentUI()
	hp, _ := provider.(*F4HistoryProvider)
	if hp != nil {
		if benchmark != nil {
			benchmark.event("history.load_plain.begin", "go.ui", "historyId", "folders")
			benchmark.event("history.load_rich.begin", "go.ui", "historyId", "folders")
		}
		hp.mu.Lock()
		plain := append([]string(nil), hp.data["folders"]...)
		rich := append([]HistoryRecord(nil), hp.rich["folders"]...)
		hp.mu.Unlock()
		if benchmark != nil {
			benchmark.event("history.load_plain.end", "go.ui", "historyId", "folders", "items", len(plain))
			benchmark.event("history.load_rich.end", "go.ui", "historyId", "folders", "items", len(rich))
		}
		return mergeFolderHistoryRecords(plain, rich), hp
	}
	if benchmark != nil {
		benchmark.event("history.load_plain.begin", "go.ui", "historyId", "folders")
	}
	plain := provider.LoadHistory("folders")
	if benchmark != nil {
		benchmark.event("history.load_plain.end", "go.ui", "historyId", "folders", "items", len(plain))
	}
	records := make([]HistoryRecord, 0, len(plain))
	for _, path := range plain {
		records = append(records, HistoryRecord{Name: path})
	}
	return records, nil
}

func saveFolderHistoryRecords(hp *F4HistoryProvider, records []HistoryRecord) {
	if hp == nil {
		return
	}
	benchmark := navigationBenchmarkCurrentUI()
	if benchmark != nil {
		benchmark.event("history.save_rich.begin", "go.ui", "historyId", "folders", "items", len(records))
		benchmark.event("history.save_plain.begin", "go.ui", "historyId", "folders", "items", len(records))
		benchmark.event("history.save_combined.begin", "go.ui", "historyId", "folders", "items", len(records))
	}
	hp.SaveFolderHistory(records)
	if benchmark != nil {
		benchmark.event("history.save_combined.end", "go.ui", "historyId", "folders", "items", len(records))
		benchmark.event("history.save_rich.end", "go.ui", "historyId", "folders", "items", len(records))
		benchmark.event("history.save_plain.end", "go.ui", "historyId", "folders", "items", len(records))
	}
}

func AddFolderHistory(path string) {
	if path == "" || path == "." || vtui.GlobalHistoryProvider == nil {
		return
	}
	if hp, ok := vtui.GlobalHistoryProvider.(*F4HistoryProvider); ok {
		benchmark := navigationBenchmarkCurrentUI()
		if benchmark != nil {
			benchmark.event("history.load_plain.begin", "go.ui", "historyId", "folders")
			benchmark.event("history.load_rich.begin", "go.ui", "historyId", "folders")
			benchmark.event("history.update.begin", "go.ui", "historyId", "folders",
				"path", path)
		}
		before, after, revision := hp.addFolderHistory(path, benchmark)
		if benchmark != nil {
			benchmark.event("history.load_plain.end", "go.ui", "historyId", "folders", "items", before)
			benchmark.event("history.load_rich.end", "go.ui", "historyId", "folders", "items", before)
			benchmark.event("history.update.end", "go.ui", "historyId", "folders",
				"path", path, "itemsBefore", before, "itemsAfter", after)
			benchmark.event("history.save_rich.begin", "go.ui", "historyId", "folders", "items", after)
			benchmark.event("history.save_plain.begin", "go.ui", "historyId", "folders", "items", after)
			benchmark.event("history.save_combined.begin", "go.ui", "historyId", "folders", "items", after)
			benchmark.event("history.persistence.scheduled", "go.ui", "kind", "folder_combined",
				"historyId", "folders", "revision", revision, "items", after)
			benchmark.event("history.save_combined.end", "go.ui", "historyId", "folders", "items", after)
			benchmark.event("history.save_rich.end", "go.ui", "historyId", "folders", "items", after)
			benchmark.event("history.save_plain.end", "go.ui", "historyId", "folders", "items", after)
		}
		return
	}
	benchmark := navigationBenchmarkCurrentUI()
	if benchmark != nil {
		benchmark.event("history.update.begin", "go.ui", "historyId", "folders", "path", path)
	}
	h := vtui.GlobalHistoryProvider.LoadHistory("folders")
	// Deduplicate and move to top
	newHist := []string{path}
	for _, item := range h {
		if !sameFolderHistoryPath(item, path) {
			newHist = append(newHist, item)
		}
	}
	// Limit to 100 items
	if len(newHist) > 100 {
		newHist = newHist[:100]
	}
	if benchmark != nil {
		benchmark.event("history.update.end", "go.ui", "historyId", "folders",
			"path", path, "itemsBefore", len(h), "itemsAfter", len(newHist))
		benchmark.event("history.save_plain.begin", "go.ui", "historyId", "folders", "items", len(newHist))
	}
	vtui.GlobalHistoryProvider.SaveHistory("folders", newHist)
	if benchmark != nil {
		benchmark.event("history.save_plain.end", "go.ui", "historyId", "folders", "items", len(newHist))
	}
}
