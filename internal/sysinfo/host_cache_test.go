package sysinfo

import (
	"context"
	"testing"
	"time"
)

func TestHostCacheSnapshotWhileRefreshing(t *testing.T) {
	c := NewHostCache()
	calls := 0
	c.probe = func(context.Context) (HostInfo, error) {
		calls++
		return HostInfo{Model: "MacBook Pro", Displays: []DisplayInfo{{Name: "Built-in"}}}, nil
	}
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, fresh := c.Snapshot()
	if !fresh || info.Model != "MacBook Pro" {
		t.Fatalf("snapshot missing: %+v", info)
	}
	info.Displays[0].Name = "mutated"
	if err := c.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, _ = c.Snapshot()
	if calls != 1 || info.Displays[0].Name != "Built-in" {
		t.Fatal("cache resampled or mutated")
	}
	c.mu.Lock()
	c.updated = time.Now().Add(-time.Hour)
	c.mu.Unlock()
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	c.probe = func(context.Context) (HostInfo, error) {
		close(entered)
		<-release
		return HostInfo{Model: "updated"}, nil
	}
	go func() { done <- c.Refresh(context.Background()) }()
	<-entered
	// Snapshot must not wait for the ongoing OS probe.
	read := make(chan HostInfo, 1)
	go func() { got, _ := c.Snapshot(); read <- got }()
	select {
	case got := <-read:
		if got.Model != "MacBook Pro" {
			t.Errorf("lost stale data: %+v", got)
		}
	case <-time.After(time.Second):
		t.Error("snapshot blocked on probe")
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func BenchmarkHostCacheSnapshot(b *testing.B) {
	c := NewHostCache()
	c.info = HostInfo{Model: "MacBook Pro", Displays: []DisplayInfo{{Name: "Built-in"}}}
	c.updated = time.Now()
	b.ReportAllocs()
	for b.Loop() {
		c.Snapshot()
	}
}
