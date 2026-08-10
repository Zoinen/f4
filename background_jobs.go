package main

import (
	"sort"
	"sync"
	"time"
)

// A long operation on a remote host runs there whether or not anybody is
// watching: the FISH+ job machinery already lets one be started, polled and
// dropped by whoever cares to. What has been missing is a place to keep the
// ones that are running, so that a dialog can be closed without either
// killing the work or losing the way back to it.
//
// The registry holds no session and no context of its own. It knows how to
// ask a job to stop and how to say what it is doing; everything else stays
// with the task that started it.

// BackgroundJobState is what the list shows about a job.
type BackgroundJobState struct {
	ID      int
	Title   string
	Status  string
	Started time.Time
	// Done is set once the work has ended and left something to look at.
	// A job that ended with nothing to show simply leaves the list.
	Done bool
	// Result is the line describing what came out of it.
	Result string
}

// BackgroundJob is the handle the task that started the work keeps.
type BackgroundJob struct {
	id       int
	registry *BackgroundJobRegistry
}

// BackgroundJobRegistry is the list of jobs that are running. It is safe for
// use from several goroutines, which it has to be: a job reports progress
// from its own task while the interface reads the list from the UI thread.
type BackgroundJobRegistry struct {
	mu     sync.Mutex
	next   int
	jobs   map[int]*backgroundJobEntry
	notify []func()
}

type backgroundJobEntry struct {
	state  BackgroundJobState
	cancel func()
	open   func()
	// owner identifies the connection the work runs on, for work that runs
	// somewhere else. It is compared and never read: see vfs.SessionIdentity.
	// A nil owner is work running here, which no reconnect can take away.
	owner any
}

// GlobalBackgroundJobs is the registry the interface shows.
var GlobalBackgroundJobs = NewBackgroundJobRegistry()

func NewBackgroundJobRegistry() *BackgroundJobRegistry {
	return &BackgroundJobRegistry{jobs: make(map[int]*backgroundJobEntry)}
}

// Start records a job that has begun. cancel may be nil for work that cannot
// be stopped, in which case the list shows it and offers nothing.
func (r *BackgroundJobRegistry) Start(title string, cancel func()) *BackgroundJob {
	return r.StartOn(nil, title, cancel)
}

// StartOn records a job that runs on somebody else's machine, together with
// the connection it runs on. That is what lets SessionLost find it later: the
// far side of a session that dropped keeps nothing, so a job started through
// it is gone whether or not anything here noticed.
func (r *BackgroundJobRegistry) StartOn(owner any, title string, cancel func()) *BackgroundJob {
	r.mu.Lock()
	r.next++
	id := r.next
	r.jobs[id] = &backgroundJobEntry{
		state:  BackgroundJobState{ID: id, Title: title, Started: time.Now()},
		cancel: cancel,
		owner:  owner,
	}
	r.mu.Unlock()
	r.changed()
	return &BackgroundJob{id: id, registry: r}
}

// SessionLost tells the registry that a connection is gone and everything
// that was running on it with it. It reports how many jobs that was.
//
// The jobs are marked as ended rather than removed, because the user asked
// for that work and is owed the news that it will not arrive: a scan that
// silently vanished from the list would look like one that finished. There is
// nothing left to cancel and nothing to open, so both are dropped.
//
// A job that had already finished keeps its result. It was computed while the
// session was alive and is no less true now that the session is not.
//
// An owner of nil matches nothing: that is work running here, which no
// reconnect can take away.
func (r *BackgroundJobRegistry) SessionLost(owner any) int {
	if owner == nil {
		return 0
	}
	lost := 0
	r.mu.Lock()
	for _, e := range r.jobs {
		if e.owner != owner || e.state.Done {
			continue
		}
		e.state.Done = true
		e.state.Status = backgroundJobLostText
		e.state.Result = backgroundJobLostText
		e.cancel = nil
		e.open = nil
		lost++
	}
	r.mu.Unlock()
	if lost > 0 {
		r.changed()
	}
	return lost
}

// backgroundJobLostText is what the list says about work that died with its
// connection. It is one line because that is all the list has room for, and
// it says gone rather than failed: nothing went wrong with the work itself.
const backgroundJobLostText = "lost with the connection"

// SetStatus replaces the line the list shows for a job. It is called from
// the job's own progress callback, so it does nothing expensive.
func (j *BackgroundJob) SetStatus(status string) {
	if j == nil {
		return
	}
	r := j.registry
	r.mu.Lock()
	if e := r.jobs[j.id]; e != nil {
		e.state.Status = status
	}
	r.mu.Unlock()
	r.changed()
}

// Finish marks a job as ended and takes it out of the list. A job that
// failed is finished too: the error belongs to whoever was waiting for it.
func (j *BackgroundJob) Finish() {
	if j == nil {
		return
	}
	r := j.registry
	r.mu.Lock()
	delete(r.jobs, j.id)
	r.mu.Unlock()
	r.changed()
}

// ID identifies the job in the list.
func (j *BackgroundJob) ID() int {
	if j == nil {
		return 0
	}
	return j.id
}

// FinishWith ends a job and leaves its answer in the list instead of
// showing it. A window that has been sent to the background must not come
// back on its own: the user asked to stop watching, and an answer that
// arrives half an hour later on top of whatever they are doing now is not
// what stopping watching means. The job waits in the list with a line
// saying what it found, and open is what shows it.
func (j *BackgroundJob) FinishWith(result string, open func()) {
	if j == nil {
		return
	}
	r := j.registry
	r.mu.Lock()
	if e := r.jobs[j.id]; e != nil {
		e.state.Done = true
		e.state.Status = result
		e.state.Result = result
		e.cancel = nil
		e.open = open
	}
	r.mu.Unlock()
	r.changed()
}

// Open shows the result of a finished job and takes it out of the list.
// It reports whether there was anything to show.
func (r *BackgroundJobRegistry) Open(id int) bool {
	r.mu.Lock()
	e := r.jobs[id]
	var open func()
	if e != nil && e.state.Done {
		open = e.open
		delete(r.jobs, id)
	}
	r.mu.Unlock()
	if e == nil || !e.state.Done {
		return false
	}
	r.changed()
	if open != nil {
		open()
	}
	return true
}

// Forget drops a job from the list without showing anything, for a result
// the user does not want after all.
func (r *BackgroundJobRegistry) Forget(id int) {
	r.mu.Lock()
	delete(r.jobs, id)
	r.mu.Unlock()
	r.changed()
}

// List returns what is running, oldest first, which is the order somebody
// watching a list expects things to have started in.
func (r *BackgroundJobRegistry) List() []BackgroundJobState {
	r.mu.Lock()
	out := make([]BackgroundJobState, 0, len(r.jobs))
	for _, e := range r.jobs {
		out = append(out, e.state)
	}
	r.mu.Unlock()
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// Cancel asks a job to stop and reports whether there was one to ask. The
// job is not removed here: it disappears from the list when the work it was
// doing actually ends, so that a cancel that takes a while is still visible.
func (r *BackgroundJobRegistry) Cancel(id int) bool {
	r.mu.Lock()
	e := r.jobs[id]
	var cancel func()
	if e != nil && !e.state.Done {
		cancel = e.cancel
		e.state.Status = "cancelling"
	}
	r.mu.Unlock()
	if e == nil || e.state.Done {
		return false
	}
	r.changed()
	if cancel != nil {
		cancel()
	}
	return true
}

// CancelAll stops everything, for a quit that must not leave remote work
// running on somebody's server.
func (r *BackgroundJobRegistry) CancelAll() {
	for _, s := range r.List() {
		r.Cancel(s.ID)
	}
}

func (r *BackgroundJobRegistry) ActiveCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, entry := range r.jobs {
		if !entry.state.Done {
			count++
		}
	}
	return count
}

// OnChange registers a callback fired whenever the list changes, so a window
// showing it can redraw. It is called with no lock held.
func (r *BackgroundJobRegistry) OnChange(fn func()) {
	r.mu.Lock()
	r.notify = append(r.notify, fn)
	r.mu.Unlock()
}

func (r *BackgroundJobRegistry) changed() {
	r.mu.Lock()
	fns := append([]func(){}, r.notify...)
	r.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
}
