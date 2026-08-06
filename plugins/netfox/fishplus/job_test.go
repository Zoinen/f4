package fishplus

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestParseJobStatus(t *testing.T) {
	for _, tc := range []struct {
		line  string
		state JobState
		exit  int
		msg   string
	}{
		{"S run -", JobRunning, -1, ""},
		{"S done 0", JobDone, 0, ""},
		{"S done 1 not a directory", JobDone, 1, "not a directory"},
		{"S kill -", JobKilled, -1, ""},
	} {
		st, err := parseJobStatus(tc.line)
		if err != nil {
			t.Fatalf("%q: %v", tc.line, err)
		}
		if st.State != tc.state || st.Exit != tc.exit || st.Msg != tc.msg {
			t.Errorf("%q: got %+v", tc.line, st)
		}
	}
	for _, bad := range []string{"", "S", "S run", "run - ", "S weird -", "S done x"} {
		if _, err := parseJobStatus(bad); err == nil {
			t.Errorf("bad status line %q accepted", bad)
		}
	}
}

func TestParseScanLine(t *testing.T) {
	p, total, err := parseScanLine("P 100 4096 3 1 /srv/a dir/with a space")
	if err != nil {
		t.Fatalf("progress line: %v", err)
	}
	if total {
		t.Error("a progress line was taken for a total")
	}
	want := ScanStats{Bytes: 100, DirBytes: 4096, Files: 3, Dirs: 1}
	if p.ScanStats != want || p.Path != "/srv/a dir/with a space" {
		t.Errorf("got %+v", p)
	}

	p, total, err = parseScanLine("T 250 8192 9 2")
	if err != nil {
		t.Fatalf("total line: %v", err)
	}
	if !total {
		t.Error("a total was taken for a progress line")
	}
	if (p.ScanStats != ScanStats{Bytes: 250, DirBytes: 8192, Files: 9, Dirs: 2}) || p.Path != "" {
		t.Errorf("got %+v", p)
	}

	for _, bad := range []string{"", "P", "X 1 2 3 4", "P 1 2 3", "T 1 2 x 4"} {
		if _, _, err := parseScanLine(bad); err == nil {
			t.Errorf("bad scan line %q accepted", bad)
		}
	}
}

// walkStats counts a tree the way the remote helper does: every entry is
// looked at without following symlinks, the root included, and a directory
// contributes its own size to DirBytes rather than to Bytes.
func walkStats(t *testing.T, root string) ScanStats {
	t.Helper()
	var stats ScanStats
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.IsDir() {
			stats.Dirs++
			stats.DirBytes += info.Size()
			return nil
		}
		stats.Files++
		stats.Bytes += info.Size()
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	return stats
}

// scanTree builds a tree whose shape exercises the parts that usually
// break: a name with a space, a nested directory, and a symlink, which the
// remote walk must count as a leaf with the size of the target path rather
// than follow.
func scanTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "tree")
	sub := filepath.Join(root, "a sub dir")
	if err := os.MkdirAll(filepath.Join(sub, "deeper"), 0755); err != nil {
		t.Fatal(err)
	}
	files := []struct {
		name string
		size int
	}{
		{filepath.Join(root, "top.bin"), 5000},
		{filepath.Join(sub, "a file.txt"), 1234},
		{filepath.Join(sub, "deeper", "smallest"), 7},
	}
	for _, f := range files {
		if err := os.WriteFile(f.name, make([]byte, f.size), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("top.bin", filepath.Join(root, "link")); err != nil {
		t.Skipf("no symlinks on this host: %v", err)
	}
	return root
}

// TestScanAgainstLocalShell drives the scan over every listing backend the
// test machine provides and checks the numbers against a walk done here.
// The point is not that the helper counts, it is that both sides agree on
// what counts as a directory and what a symlink weighs.
func TestScanAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanScan() {
		t.Skip("this host cannot run scan jobs")
	}
	root := scanTree(t)
	want := walkStats(t, root)

	feats := c.Session().Features()
	for _, mode := range ListingModes {
		if !feats.Has(mode) {
			continue
		}
		t.Run(mode, func(t *testing.T) {
			if err := c.SetListingMode(ctx, mode); err != nil {
				t.Fatalf("mode %s: %v", mode, err)
			}
			got, err := c.Scan(ctx, root, nil)
			if err != nil {
				t.Fatalf("scan: %v", err)
			}
			if got != want {
				t.Errorf("scan = %+v, want %+v", got, want)
			}
		})
	}

	// A finished scan leaves nothing behind on the remote host.
	jobs, err := c.ListJobs(ctx)
	if err != nil {
		t.Fatalf("jlist: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("jobs left after the scans: %+v", jobs)
	}
}

// bigTree builds a tree with more entries than the remote awk reports at
// once, which is the only way to see a progress line at all.
func bigTree(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "big")
	for i := 0; i < 10; i++ {
		dir := filepath.Join(root, "d"+strconv.Itoa(i))
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 300; j++ {
			name := filepath.Join(dir, "f"+strconv.Itoa(j)+".txt")
			if err := os.WriteFile(name, make([]byte, j%16), 0644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

// TestScanProgressAgainstLocalShell needs a tree big enough for the remote
// awk to report before it is done, which is the only way to find out that
// the progress lines are flushed rather than sitting in a buffer until the
// end.
func TestScanProgressAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanScan() {
		t.Skip("this host cannot run scan jobs")
	}
	if !c.Session().Features().Has("awkflush") {
		t.Skip("the remote awk cannot flush, so it reports only at the end")
	}
	root := bigTree(t)
	want := walkStats(t, root)

	var last ScanProgress
	reports := 0
	got, err := c.Scan(ctx, root, func(p ScanProgress) {
		reports++
		last = p
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got != want {
		t.Errorf("scan = %+v, want %+v", got, want)
	}
	if reports == 0 {
		t.Fatal("a scan over three thousand entries reported no progress at all")
	}
	if last.Files == 0 || last.Files > want.Files {
		t.Errorf("last progress report = %+v, want something below %+v", last, want)
	}
	if last.Path == "" {
		t.Error("a progress report without the entry it had reached")
	}
}

// TestScanCancelledFromTheCallback covers the policy Scan implements: what
// the user stopped watching stops running, and the session it ran over
// stays usable. Cancelling from a progress report is deterministic because
// it happens between two polls, with nothing of ours on the wire.
func TestScanCancelledFromTheCallback(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanScan() {
		t.Skip("this host cannot run scan jobs")
	}
	if !c.Session().Features().Has("awkflush") {
		t.Skip("the remote awk cannot flush, so there is no report to cancel from")
	}
	root := bigTree(t)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	_, err := c.Scan(cancelCtx, root, func(ScanProgress) { cancel() })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled scan = %v, want context.Canceled", err)
	}
	jobs, err := c.ListJobs(ctx)
	if err != nil {
		t.Fatalf("jlist: %v", err)
	}
	if len(jobs) != 0 {
		t.Errorf("jobs after a cancelled scan = %+v, want none", jobs)
	}
	if got, err := c.Session().Ping(ctx, "alive"); err != nil || got != "alive" {
		t.Fatalf("ping after a cancelled scan = %q (%v)", got, err)
	}
}

// TestFollowScanLeavesTheJobAlone pins down the difference between the two
// ways out of a scan: Scan cancels the remote job, FollowScan on its own
// does not, which is what a job that outlives its dialog will be built on.
func TestFollowScanLeavesTheJobAlone(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanScan() {
		t.Skip("this host cannot run scan jobs")
	}
	root := scanTree(t)

	id, err := c.StartScan(ctx, root)
	if err != nil {
		t.Fatalf("jstart: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := c.FollowScan(cancelled, id, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("FollowScan on a cancelled context = %v, want context.Canceled", err)
	}
	jobs, err := c.ListJobs(ctx)
	if err != nil {
		t.Fatalf("jlist: %v", err)
	}
	if len(jobs) != 1 || jobs[0].ID != id || jobs[0].Kind != "scan" {
		t.Fatalf("jobs = %+v, want the scan still there", jobs)
	}

	if err := c.DropJob(ctx, id); err != nil {
		t.Fatalf("jdrop: %v", err)
	}
	if jobs, err = c.ListJobs(ctx); err != nil || len(jobs) != 0 {
		t.Fatalf("jobs after the drop = %+v (%v)", jobs, err)
	}
	// Dropping twice is how a cancelled scan and a job list can race, so it
	// has to be harmless.
	if err := c.DropJob(ctx, id); err != nil {
		t.Errorf("dropping a job that is gone: %v", err)
	}
}

// TestKilledScanIsReported checks that a killed job is reported as killed
// even when it had already finished: the kill marker is looked at first, so
// the answer does not depend on winning a race with the remote walk.
func TestKilledScanIsReported(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanScan() {
		t.Skip("this host cannot run scan jobs")
	}
	id, err := c.StartScan(ctx, scanTree(t))
	if err != nil {
		t.Fatalf("jstart: %v", err)
	}
	if err := c.KillJob(ctx, id); err != nil {
		t.Fatalf("jkill: %v", err)
	}
	if _, err := c.FollowScan(ctx, id, nil); !errors.Is(err, ErrJobKilled) {
		t.Fatalf("FollowScan after a kill = %v, want ErrJobKilled", err)
	}
	jobs, err := c.ListJobs(ctx)
	if err != nil {
		t.Fatalf("jlist: %v", err)
	}
	if len(jobs) != 1 || jobs[0].State != JobKilled {
		t.Errorf("jobs = %+v, want one killed job", jobs)
	}
	if err := c.DropJob(ctx, id); err != nil {
		t.Errorf("jdrop: %v", err)
	}
}

// TestScanFailureIsReported checks that a job which failed carries the
// reason back rather than an empty answer, and that the ping after it still
// works: a job that ends badly must not take the session with it.
func TestScanFailureIsReported(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanScan() {
		t.Skip("this host cannot run scan jobs")
	}
	file := filepath.Join(t.TempDir(), "not a directory.txt")
	if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := c.Scan(ctx, file, nil)
	if err == nil {
		t.Fatal("scanning a plain file succeeded")
	}
	var remote *RemoteError
	if !errors.As(err, &remote) {
		t.Fatalf("scan error = %v, want a remote error", err)
	}
	if got, err := c.Session().Ping(ctx, "alive"); err != nil || got != "alive" {
		t.Fatalf("ping after a failed scan = %q (%v)", got, err)
	}
}

// TestRefusedJobKeepsTheStreamInSync is the reason the request carries its
// own path count. A helper that rejected the job before reading the paths
// would leave them in the stream, and the next request would be parsed out
// of somebody's directory name. Only the ping afterwards can show it.
func TestRefusedJobKeepsTheStreamInSync(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	if !c.CanRunJobs() {
		t.Skip("this host cannot run background jobs")
	}
	if _, err := c.StartJob(ctx, "nosuchkind", []string{"/tmp"}); err == nil {
		t.Fatal("an unknown job kind was accepted")
	}
	if got, err := c.Session().Ping(ctx, "in sync"); err != nil || got != "in sync" {
		t.Fatalf("ping after a refused job = %q (%v)", got, err)
	}
	if _, err := c.StartJob(ctx, "scan", []string{"/tmp", "/tmp"}); err == nil {
		t.Fatal("a scan job with two paths was accepted")
	}
	if got, err := c.Session().Ping(ctx, "still in sync"); err != nil || got != "still in sync" {
		t.Fatalf("ping after a refused path count = %q (%v)", got, err)
	}
}
