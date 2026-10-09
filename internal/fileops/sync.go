package fileops

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/numeric"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Folder synchronization, as Total Commander offers it under "Synchronize
// dirs": the two panel folders are compared file by file, every pair gets a
// direction, the user corrects the ones they disagree with, and only then
// does anything move.
//
// The comparison itself is the one in compare.go — the same criteria, the
// same two-second slack, the same content reader — so that the two commands
// can never disagree about whether two files are the same file. What this
// file adds is the pairing that survives as a list (folder comparison only
// needs to mark panel entries, synchronizing needs every pair), the default
// direction Total Commander's rules give each pair, and the execution.
//
// Everything here is free of UI: the dialogs live in the app layer, so the
// planning can be tested against a temporary directory.

// SyncState is what the comparison found about one pair — the middle
// column of Total Commander's window before the user touches it.
type SyncState int

const (
	// SyncEqual is "=": the two files satisfy every enabled criterion.
	SyncEqual SyncState = iota
	// SyncLeftOnly and SyncRightOnly are files only one side has.
	SyncLeftOnly
	SyncRightOnly
	// SyncLeftNewer and SyncRightNewer are pairs whose difference says
	// which copy is the one worth keeping.
	SyncLeftNewer
	SyncRightNewer
	// SyncDiffers is "!=": the files differ, but nothing in the result
	// says which way to copy. Ignoring dates produces these by
	// definition, and so does a pair of files stamped at the same second
	// with different sizes.
	SyncDiffers
)

// SyncAction is what will be done to a pair when the plan runs.
type SyncAction int

const (
	// SyncSkip leaves both sides alone.
	SyncSkip SyncAction = iota
	SyncCopyToRight
	SyncCopyToLeft
	SyncDeleteRight
	SyncDeleteLeft
)

// SyncPair is one row of the synchronize window: one relative path, and
// what each side has under it.
type SyncPair struct {
	// Rel is the path below both folders, always slash separated.
	Rel string
	// HasLeft and HasRight say which sides hold the path at all; Left
	// and Right are the listing entries of those that do.
	HasLeft, HasRight bool
	Left, Right       vfs.VFSItem
	// Locked marks a pair nothing can be done about automatically: one
	// side has a file where the other has a folder. Copying either way
	// would have to remove the other side's tree first, which is not a
	// decision a synchronize run gets to make on its own.
	Locked bool

	State  SyncState
	Action SyncAction
}

// Size is the number of bytes the pair's action will transfer.
func (p SyncPair) Size() int64 {
	switch p.Action {
	case SyncCopyToRight:
		return p.Left.Size
	case SyncCopyToLeft:
		return p.Right.Size
	}
	return 0
}

// AllowedActions lists the actions that make sense for this pair, in the
// order the window cycles through them. Skip is always first, so that a
// row a user cycles past comes back to doing nothing.
func (p SyncPair) AllowedActions() []SyncAction {
	if p.Locked {
		return []SyncAction{SyncSkip}
	}
	out := []SyncAction{SyncSkip}
	if p.HasLeft {
		out = append(out, SyncCopyToRight)
	}
	if p.HasRight {
		out = append(out, SyncCopyToLeft)
	}
	if p.HasRight {
		out = append(out, SyncDeleteRight)
	}
	if p.HasLeft {
		out = append(out, SyncDeleteLeft)
	}
	return out
}

// SyncSides is the pair of folders a comparison or a plan runs against.
type SyncSides struct {
	LeftFS    vfs.VFS
	LeftRoot  string
	RightFS   vfs.VFS
	RightRoot string
}

// SyncTotals is what a plan adds up to, which is what the confirmation
// dialog shows before anything is touched.
type SyncTotals struct {
	ToRight, ToLeft         int
	DeleteRight, DeleteLeft int
	BytesToRight            int64
	BytesToLeft             int64
}

// Empty reports whether the plan would do nothing at all.
func (t SyncTotals) Empty() bool {
	return t.ToRight+t.ToLeft+t.DeleteRight+t.DeleteLeft == 0
}

// SyncPlanTotals counts the actions currently set on a plan.
func SyncPlanTotals(pairs []SyncPair) SyncTotals {
	var t SyncTotals
	for _, p := range pairs {
		switch p.Action {
		case SyncCopyToRight:
			t.ToRight++
			t.BytesToRight += p.Left.Size
		case SyncCopyToLeft:
			t.ToLeft++
			t.BytesToLeft += p.Right.Size
		case SyncDeleteRight:
			t.DeleteRight++
		case SyncDeleteLeft:
			t.DeleteLeft++
		}
	}
	return t
}

// DefaultSyncAction is the direction Total Commander gives a pair before
// the user changes it.
//
// Symmetrically the two folders are peers: whatever one side is missing or
// has an older copy of is fetched from the other, and a difference with no
// direction in it ("!=") waits for the user.
//
// Asymmetrically the left folder is the master and the right one is made
// to look like it: everything missing or different goes right, and what
// only the right side has is deleted there. That is the mirror Total
// Commander's help describes, and the reason the option is worth having at
// all — nothing else in the dialog can produce a deletion.
func DefaultSyncAction(state SyncState, asymmetric bool) SyncAction {
	if asymmetric {
		switch state {
		case SyncEqual:
			return SyncSkip
		case SyncRightOnly:
			return SyncDeleteRight
		default:
			return SyncCopyToRight
		}
	}
	switch state {
	case SyncLeftOnly, SyncLeftNewer:
		return SyncCopyToRight
	case SyncRightOnly, SyncRightNewer:
		return SyncCopyToLeft
	default:
		return SyncSkip
	}
}

// syncNameFilter decides whether a file takes part in the comparison at
// all. It is supplied by the caller because file masks are a panel-layer
// notion, and fileops sits below it.
type syncNameFilter func(name string) bool

// BuildSyncPairs compares the two folders and returns one row per path
// either of them holds, sorted by path.
//
// Folders themselves are not rows. Total Commander compares files: a
// folder carries no content of its own, and the folders a copy needs are
// created on the way. A folder facing a file of the same name is the one
// exception — that pair is reported, locked, so the user can see why
// nothing will happen to it.
func BuildSyncPairs(ctx context.Context, sides SyncSides, opts config.SyncOptions, keep syncNameFilter,
	scanProgress func(string), cmpProgress func(path string, done, total int)) ([]SyncPair, error) {
	cmp := opts.CompareOptions()

	left, err := CollectCompareSide(ctx, sides.LeftFS, sides.LeftRoot, nil, cmp, scanProgress)
	if err != nil {
		return nil, err
	}
	right, err := CollectCompareSide(ctx, sides.RightFS, sides.RightRoot, nil, cmp, scanProgress)
	if err != nil {
		return nil, err
	}

	keys := make([]string, 0, len(left)+len(right))
	for rel := range left {
		keys = append(keys, rel)
	}
	for rel := range right {
		if _, both := left[rel]; !both {
			keys = append(keys, rel)
		}
	}
	sort.Strings(keys)

	skip := compareSkipMode(cmp)
	pairs := make([]SyncPair, 0, len(keys))
	var readErr error
	for i, rel := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if cmpProgress != nil {
			cmpProgress(rel, i, len(keys))
		}

		l, hasLeft := left[rel]
		r, hasRight := right[rel]
		if hasLeft && hasRight && l.item.IsDir && r.item.IsDir {
			// The container of the comparison, not a subject of it.
			continue
		}
		if keep != nil && !keep(syncBaseName(rel)) {
			continue
		}

		pair := SyncPair{Rel: rel, HasLeft: hasLeft, HasRight: hasRight}
		if hasLeft {
			pair.Left = l.item
		}
		if hasRight {
			pair.Right = r.item
		}

		switch {
		case hasLeft && hasRight && l.item.IsDir != r.item.IsDir:
			pair.State, pair.Locked = SyncDiffers, true
		case hasLeft && !hasRight:
			if l.item.IsDir {
				continue
			}
			pair.State = SyncLeftOnly
		case hasRight && !hasLeft:
			if r.item.IsDir {
				continue
			}
			pair.State = SyncRightOnly
		default:
			state, err := syncComparePair(ctx, sides, l, r, cmp, skip)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				if readErr == nil {
					readErr = err
				}
			}
			pair.State = state
		}

		pair.Action = DefaultSyncAction(pair.State, opts.Asymmetric)
		if pair.Locked {
			pair.Action = SyncSkip
		}
		pairs = append(pairs, pair)
	}
	if readErr != nil {
		return pairs, readErr
	}
	return pairs, nil
}

// syncBaseName is the file name inside a slash separated relative path.
func syncBaseName(rel string) string {
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		return rel[i+1:]
	}
	return rel
}

// syncComparePair decides the state of two files present on both sides.
//
// A file that cannot be read counts as differing and the error is passed
// up: an unreadable file is not a match, and saying it is would let a
// synchronize run skip exactly the file that needs attention.
func syncComparePair(ctx context.Context, sides SyncSides, l, r CompareItem, cmp config.CompareOptions, skip int) (SyncState, error) {
	sizeDiffers := l.item.Size != r.item.Size
	if cmp.ByTime {
		differs, timeOnly, newer := compareMetadata(l.item, r.item, cmp)
		if differs {
			switch {
			case timeOnly && newer > 0:
				return SyncLeftNewer, nil
			case timeOnly && newer < 0:
				return SyncRightNewer, nil
			case newer > 0 && !sizeDiffers:
				return SyncLeftNewer, nil
			case newer < 0 && !sizeDiffers:
				return SyncRightNewer, nil
			case newer > 0:
				// Different size and a later time on one side: that
				// side is still the edited one, which is the answer
				// Total Commander gives too.
				return SyncLeftNewer, nil
			case newer < 0:
				return SyncRightNewer, nil
			default:
				return SyncDiffers, nil
			}
		}
	} else if sizeDiffers {
		// Ignoring dates leaves size as the only metadata, and a size
		// difference says nothing about which copy to keep.
		return SyncDiffers, nil
	}

	if cmp.ByContent {
		// Total Commander reads the files that already look equal, to
		// find the ones that only look it.
		equal, err := compareContents(ctx, sides.LeftFS, l, sides.RightFS, r, skip)
		if err != nil {
			return SyncDiffers, err
		}
		if !equal {
			return SyncDiffers, nil
		}
	}
	return SyncEqual, nil
}

// syncJoin turns a slash separated relative path into a path in the given
// file system. Joining the whole string at once would leave the separators
// of one file system inside a path belonging to another.
func syncJoin(v vfs.VFS, root, rel string) string {
	full := root
	for _, part := range strings.Split(rel, "/") {
		if part == "" {
			continue
		}
		full = v.Join(full, part)
	}
	return full
}

// syncEnsureDir creates the folder a copy is about to land in, and every
// missing folder above it. VFS has no MkdirAll: each implementation only
// promises the single level.
func syncEnsureDir(ctx context.Context, v vfs.VFS, root, relDir string) error {
	if relDir == "" {
		return nil
	}
	full := root
	for _, part := range strings.Split(relDir, "/") {
		if part == "" {
			continue
		}
		full = v.Join(full, part)
		if st, err := v.Stat(ctx, full); err == nil {
			if !st.IsDir {
				return fmt.Errorf("%s: %w", full, errSyncNotADirectory)
			}
			continue
		}
		if err := v.MkDir(ctx, full); err != nil {
			// Another item of the same plan may have created it in the
			// meantime; only a folder that is still missing is an error.
			if st, statErr := v.Stat(ctx, full); statErr != nil || !st.IsDir {
				return err
			}
		}
	}
	return nil
}

var errSyncNotADirectory = errors.New("not a directory")

// syncRelDir is everything above the file name in a relative path.
func syncRelDir(rel string) string {
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return ""
}

// syncProgressInterval is how often the progress dialog is refreshed while
// a plan runs.
const syncProgressInterval = 100 * time.Millisecond

// ExecuteSyncPlan carries out the actions set on the pairs, in one
// foreground operation with a progress dialog and a cancel button.
//
// The plan already knows every file and its size, so unlike a copy there
// is nothing to scan first: the run starts moving bytes immediately.
func ExecuteSyncPlan(sides SyncSides, pairs []SyncPair, disposition vfs.DeleteDisposition, onComplete func()) {
	plan := make([]SyncPair, 0, len(pairs))
	var total vfs.OpStats
	for _, p := range pairs {
		if p.Action == SyncSkip || p.Locked {
			continue
		}
		plan = append(plan, p)
		total.Files++
		total.Bytes += p.Size()
	}
	if len(plan) == 0 {
		if onComplete != nil {
			onComplete()
		}
		return
	}

	dlg := NewFileOpProgressDialog(i18n.Msg("Sync.ProgressTitle"))
	var taskCtx atomic.Pointer[vtui.TaskContext]
	dlg.btnCancel.OnClick = func() { dlg.SetExitCode(1) }
	dlg.OnResult = func(int) {
		if ctx := taskCtx.Load(); ctx != nil {
			ctx.Cancel()
		}
	}
	reporter := NewDialogReporter(dlg)

	vtui.FrameManager.PostTask(func() {
		vtui.FrameManager.AddScreenHeadless(dlg)
	})

	taskCtx.Store(vtui.RunAsync(func(task *vtui.TaskContext) {
		err := runSyncPlan(task.Context, sides, plan, total, disposition, reporter, dlg)
		task.RunOnUI(func() {
			reporter.Stop()
			dlg.Close()
			if onComplete != nil {
				onComplete()
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				vtui.ShowMessage(i18n.Msg("Sync.Title"),
					fmt.Sprintf(i18n.Msg("Sync.Failed"), err.Error()), []string{"&Ok"})
			}
		})
	}))
}

// runSyncPlan is the body of the operation, off the UI thread.
func runSyncPlan(ctx context.Context, sides SyncSides, plan []SyncPair, total vfs.OpStats,
	disposition vfs.DeleteDisposition, reporter TaskReporter, anchor vtui.Frame) error {
	start := time.Now()
	tracker := NewFileOpTracker(total)
	lastUpdate := start

	getGlobal := func(string) (string, int, string) {
		_, totalPct, _ := tracker.GetProgress()
		processed, totals := tracker.GetStats()
		totalText := fmt.Sprintf(i18n.Msg("Sync.TotalItems"), processed.Files, totals.Files)
		if totals.Bytes > 0 {
			totalText = fmt.Sprintf(i18n.Msg("Sync.TotalBytes"),
				numeric.FormatSize(processed.Bytes), numeric.FormatSize(totals.Bytes))
		}
		elapsed := time.Since(start)
		timeText := fmt.Sprintf(i18n.Msg("Sync.Elapsed"),
			int(elapsed.Hours()), int(elapsed.Minutes())%60, int(elapsed.Seconds())%60)
		return totalText, totalPct, timeText
	}

	var wrapRep *GlobalAwareReporter
	updateUI := func(force bool) {
		now := time.Now()
		if !force && now.Sub(lastUpdate) < syncProgressInterval {
			return
		}
		lastUpdate = now
		filePct, _, currName := tracker.GetProgress()
		totalText, totalPct, timeText := getGlobal("")
		reporter.UpdateTransfer(i18n.Msg("Sync.Action"), currName, filePct, totalText, totalPct, timeText)
	}
	wrapRep = NewGlobalAwareReporter(reporter, getGlobal, tracker, func(int) { updateUI(false) })
	ctx = context.WithValue(ctx, vfs.ReporterKey, wrapRep)

	state := &FileOpState{
		Tracker:     tracker,
		UpdateUI:    updateUI,
		StartFile:   wrapRep.StartFileKnown,
		SetFileSize: wrapRep.SetCurrentSize,
		OnBytes:     wrapRep.UpdateBytes,
		Anchor:      anchor,
		Buffer:      make([]byte, 128*1024),
		// The window already asked, row by row, what should happen to
		// every file. Asking again per file would be asking twice.
		OverwriteAll: true,
	}

	updateUI(true)
	for _, p := range plan {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := runSyncPair(ctx, sides, p, disposition, state, wrapRep); err != nil {
			return err
		}
		updateUI(true)
	}
	return nil
}

// runSyncPair performs one row's action.
func runSyncPair(ctx context.Context, sides SyncSides, p SyncPair, disposition vfs.DeleteDisposition,
	state *FileOpState, rep *GlobalAwareReporter) error {
	switch p.Action {
	case SyncCopyToRight:
		return syncCopy(ctx, sides.LeftFS, sides.LeftRoot, sides.RightFS, sides.RightRoot, p.Rel, state)
	case SyncCopyToLeft:
		return syncCopy(ctx, sides.RightFS, sides.RightRoot, sides.LeftFS, sides.LeftRoot, p.Rel, state)
	case SyncDeleteRight:
		return syncDelete(ctx, sides.RightFS, sides.RightRoot, p.Rel, disposition, rep)
	case SyncDeleteLeft:
		return syncDelete(ctx, sides.LeftFS, sides.LeftRoot, p.Rel, disposition, rep)
	}
	return nil
}

func syncCopy(ctx context.Context, srcFS vfs.VFS, srcRoot string, dstFS vfs.VFS, dstRoot, rel string, state *FileOpState) error {
	if err := syncEnsureDir(ctx, dstFS, dstRoot, syncRelDir(rel)); err != nil {
		return err
	}
	return recursiveCopy(ctx, srcFS, syncJoin(srcFS, srcRoot, rel), dstFS, syncJoin(dstFS, dstRoot, rel), state, 0)
}

func syncDelete(ctx context.Context, v vfs.VFS, root, rel string, disposition vfs.DeleteDisposition, rep *GlobalAwareReporter) error {
	path := syncJoin(v, root, rel)
	if rep != nil {
		rep.StartFileKnown(path, 0, true, 1, 0)
	}
	if err := DeletePathWithDisposition(ctx, v, path, disposition); err != nil {
		return err
	}
	if rep != nil {
		rep.FileDone()
	}
	return nil
}
