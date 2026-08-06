package main

import (
	"sync"
	"testing"
)

func TestBackgroundJobRegistry(t *testing.T) {
	r := NewBackgroundJobRegistry()
	changes := 0
	r.OnChange(func() { changes++ })

	cancelled := false
	first := r.Start("hashing /var/log", func() { cancelled = true })
	second := r.Start("scanning /home", nil)

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("%d jobs listed, want 2", len(list))
	}
	if list[0].ID >= list[1].ID {
		t.Error("the list is not in the order the jobs started")
	}
	if list[0].Title != "hashing /var/log" {
		t.Errorf("first job is %q", list[0].Title)
	}
	if list[0].Started.IsZero() {
		t.Error("a job was recorded without a start time")
	}

	first.SetStatus("41 of 900 files")
	if s := r.List()[0].Status; s != "41 of 900 files" {
		t.Errorf("status = %q", s)
	}

	if !r.Cancel(first.ID()) {
		t.Error("cancelling a running job reported nothing to cancel")
	}
	if !cancelled {
		t.Error("the cancel never reached the job")
	}
	// The job stays listed until the work it was doing actually stops, so
	// that a cancel which takes a while does not look like nothing happened.
	if len(r.List()) != 2 {
		t.Error("a cancelled job left the list before it had finished")
	}
	if s := r.List()[0].Status; s != "cancelling" {
		t.Errorf("a cancelled job shows %q", s)
	}

	// A job with no way to stop is listed and simply offers nothing.
	if !r.Cancel(second.ID()) {
		t.Error("a job without a cancel function could not be asked")
	}

	first.Finish()
	second.Finish()
	if len(r.List()) != 0 {
		t.Errorf("%d jobs left after both finished", len(r.List()))
	}
	if r.Cancel(first.ID()) {
		t.Error("cancelling a finished job reported something to cancel")
	}
	if changes == 0 {
		t.Error("no change was ever reported")
	}

	// Finishing twice, which happens when a task cleans up after an error
	// it already reported, must not panic or resurrect anything.
	first.Finish()
	var nilJob *BackgroundJob
	nilJob.SetStatus("ignored")
	nilJob.Finish()
}

func TestBackgroundJobResultWaits(t *testing.T) {
	r := NewBackgroundJobRegistry()
	job := r.Start("duplicates in /srv", func() {})

	shown := 0
	job.FinishWith("3 groups found", func() { shown++ })

	// The answer waits in the list rather than appearing over whatever the
	// user is doing now, which is what sending it to the background meant.
	list := r.List()
	if len(list) != 1 {
		t.Fatalf("%d jobs listed, want the finished one", len(list))
	}
	if !list[0].Done || list[0].Result != "3 groups found" {
		t.Errorf("the finished job reads %+v", list[0])
	}
	if shown != 0 {
		t.Error("the result showed itself without being asked")
	}
	if r.Cancel(job.ID()) {
		t.Error("a finished job accepted a cancel")
	}

	if !r.Open(job.ID()) {
		t.Error("opening the result found nothing")
	}
	if shown != 1 {
		t.Errorf("the result was shown %d times", shown)
	}
	if len(r.List()) != 0 {
		t.Error("an opened result stayed in the list")
	}
	if r.Open(job.ID()) {
		t.Error("the result could be opened twice")
	}

	// A result nobody wants can be dropped, and a job still running can be
	// dropped from the list too without pretending it produced anything.
	other := r.Start("scanning", nil)
	other.FinishWith("nothing found", nil)
	r.Forget(other.ID())
	if len(r.List()) != 0 {
		t.Error("a forgotten job stayed in the list")
	}
}
func TestBackgroundJobRegistryIsConcurrent(t *testing.T) {
	r := NewBackgroundJobRegistry()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			j := r.Start("work", func() {})
			for k := 0; k < 20; k++ {
				j.SetStatus("running")
				r.List()
			}
			j.Finish()
		}()
	}
	wg.Wait()
	if len(r.List()) != 0 {
		t.Errorf("%d jobs left behind", len(r.List()))
	}
}
