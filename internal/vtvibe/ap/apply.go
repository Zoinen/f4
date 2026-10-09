package ap

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Status is the outcome of an Apply call.
type Status string

const (
	StatusSuccess Status = "SUCCESS"
	StatusPartial Status = "PARTIAL"
	StatusFailed  Status = "FAILED"
)

// Options controls how a patch is applied.
type Options struct {
	// Strict enables Strict Mode (§3.0): the whole patch is one atomic
	// transaction (any failure aborts with nothing written), no tolerant
	// heuristics run, and no afailed.ap/afailed.md is produced on failure
	// beyond the single fatal error.
	Strict bool
	// Silent suppresses the human-readable progress/warning text that
	// would otherwise go to Out (successes, "[TOLERANT] ..." notes,
	// idempotency skips, the structural-balance warning).
	Silent bool
	// DryRun computes the same write plan (and, on partial failure, the
	// same afailed.ap/afailed.md as a real run) but never touches
	// projectDir: the reference's --dry-run (§ CLI) skips only the OS
	// write phase, nothing about validation or reporting. When !Silent,
	// Out gets one line per planned change instead of the write actually
	// happening.
	DryRun bool
	// Only, when non-nil, restricts the run to the modifications it maps
	// to true - the engine side of the patch review screen's "apply the
	// checked edits" (docs/VTVIBE.md §7.3). Keys are ModificationResult.Key()
	// values from an earlier run of the same patch on the same tree
	// (normally the dry run the review screen was built from), so FilePath
	// is the already-resolved path and a whole-file RENAME is ModIdx -1.
	// nil applies everything, as before; a non-nil empty map applies
	// nothing. A key that matches no modification is ignored - it cannot
	// widen the selection, so a stale or mistyped key fails safe.
	//
	// Every modification left out is reported as ModExcluded and is
	// otherwise invisible to the run: it is never searched for, never
	// fails, never lands in afailed.ap/afailed.md, and a FILE block with
	// nothing selected is not even checked for existence or path safety.
	// Everything else (tolerant retry, strict atomicity, DryRun, the
	// afailed.* reports) works on the selected modifications exactly as it
	// does on a whole patch.
	//
	// Dependencies between modifications are deliberately not inferred.
	// If B's snippet only appears once A (earlier in the same file) has
	// been applied, selecting B without A makes B fail honestly with
	// SNIPPET_NOT_FOUND - PARTIAL plus B in afailed.ap in tolerant mode, a
	// fatal error in strict mode - rather than being dropped silently or
	// pulling A in behind the user's back: the engine cannot tell "B needs
	// A" from "B is simply wrong", and a dry run with the same Only shows
	// the failure before anything is written. Selecting both keeps the
	// usual multi-pass retry, so their order in the patch still does not
	// matter. Likewise, a locator repeated within a FILE block keeps
	// meaning "successive occurrences" whatever is selected: the selected
	// ones take the next occurrences after the sequential cursor, which a
	// deselected one does not advance.
	Only map[ModKey]bool
	// Out receives progress/warning text when !Silent. Defaults to
	// io.Discard.
	Out io.Writer
}

// Result is the outcome of Apply, mirroring the reference's returned dict.
type Result struct {
	Status Status
	// Error is set for StatusFailed.
	Error *AppError
	// FilePath/HasFilePath identify which file a StatusFailed error
	// happened in, when applicable (a patch-level parse error has none).
	FilePath    string
	HasFilePath bool
	// ModIdx/HasModIdx identify which modification (0-based) failed, for
	// per-modification StatusFailed errors in strict mode.
	ModIdx    int
	HasModIdx bool
	// FailedFiles lists the files with at least one failed modification,
	// for StatusPartial (tolerant mode).
	FailedFiles []string
	// ModificationResults is a per-Modification report, one entry for
	// every Modification Apply actually reasoned about (in patch order,
	// across every FILE block), regardless of the outcome. It is a finer
	// grain than FailedFiles/FilePath/ModIdx above: those only ever name
	// the *first* fatal failure (strict mode) or the set of files that had
	// *some* failure (tolerant mode), which is enough to report a failure
	// but not enough to build a patch review screen that lists every hunk
	// with its own status (docs/VTVIBE.md §17 sketches such a screen
	// around a similar Hunk/Status shape - this is the data for it, not
	// the screen itself, see f4#1606). Populated best-effort even when
	// Apply aborts early in strict mode: it holds whatever modifications
	// were already resolved before the abort.
	ModificationResults []ModificationResult
	// Undo is the transaction journal of a real run that wrote something
	// (StatusSuccess or StatusPartial): Undo.Revert puts every touched path
	// back as it was before this call, unless something changed it since.
	// nil for a dry run, a failed run (a write that fails midway is rolled
	// back before Apply returns) and a run with nothing to write.
	Undo *Undo
}

// ModStatus is one Modification's outcome, the level of detail
// ModificationResult reports. It intentionally stays coarser than the
// ErrCode taxonomy in errors.go (SNIPPET_NOT_FOUND, AMBIGUOUS_MATCH, ...):
// a failed ModificationResult's Err already carries that finer reason, so a
// caller that wants a VTVIBE.md-style StatusAmbiguous/StatusNotFound/...
// breakdown can derive it from Err.Code instead of this package
// duplicating that taxonomy a second time.
type ModStatus int

const (
	// ModOK means the modification was applied: it changed the file (or
	// created/renamed/deleted it) as instructed.
	ModOK ModStatus = iota
	// ModSkipped means the modification was not applied because the
	// target was already in the requested state (idempotency, per the ap
	// spec) - not a failure.
	ModSkipped
	// ModFailed means the modification could not be applied; Err explains
	// why.
	ModFailed
	// ModExcluded means the caller left the modification out of
	// Options.Only (the user unchecked it on the review screen), so it was
	// not even attempted - distinct from ModSkipped, which says the file
	// already reads as requested.
	ModExcluded
)

func (s ModStatus) String() string {
	switch s {
	case ModOK:
		return "OK"
	case ModSkipped:
		return "SKIPPED"
	case ModFailed:
		return "FAILED"
	case ModExcluded:
		return "EXCLUDED"
	default:
		return "UNKNOWN"
	}
}

// ModificationResult is one Modification's outcome within an Apply call.
type ModificationResult struct {
	// FilePath is the FILE block's path this modification belongs to,
	// already resolved the same way FailedFiles/Result.FilePath are
	// (resolvePathPrefix applied, "/", "\\" trimmed).
	FilePath string
	// ModIdx is the 0-based index of this modification within its FILE
	// block's Modifications, matching Result.ModIdx and the "Mod #N"
	// (N = ModIdx+1) progress text. -1 for a whole-file operation that
	// carries no Modification of its own (currently: RENAME).
	ModIdx int
	// Action is the modification's directive (REPLACE, INSERT_AFTER,
	// DELETE, CREATE, RECREATE, RENAME, ...), "" if never set.
	Action string
	// Locator is the text the modification searched for, for display
	// without re-parsing the patch: the snippet if given, else the
	// anchor; for RENAME (ModIdx -1) the destination path instead; ""
	// for a bare DELETE/CREATE, which carries no locator at all.
	Locator string
	Status  ModStatus
	// Err is set when Status is ModFailed; nil otherwise.
	Err *AppError
	// Preview is this modification's own edit - the lines it changed
	// plus a little context, before and after - for the review screen's
	// per-row diff (docs/VTVIBE.md §7.3). Set for a ModOK modification
	// that changed the file's text (REPLACE, DELETE, INSERT_*, RECREATE,
	// CREATE of a file), in a dry run and a real one alike; nil for every
	// other status and for operations with no text of their own (RENAME,
	// a whole-file/directory DELETE, CREATE of a directory). See Preview.
	Preview *Preview
}

// ModKey identifies one modification of a patch stably across Apply calls
// on the same patch and tree: the FILE block's resolved path plus the
// modification's index in it (-1 for RENAME), as ModificationResult reports
// them. It is what Options.Only is keyed by.
type ModKey struct {
	FilePath string
	ModIdx   int
}

// Key is r's ModKey, for building Options.Only from a dry run's results.
func (r ModificationResult) Key() ModKey { return ModKey{FilePath: r.FilePath, ModIdx: r.ModIdx} }

// Apply applies the ap-format patch at patchFile to the tree rooted at
// projectDir, per §3 of the specification.
func Apply(patchFile, projectDir string, opts Options) *Result {
	out := opts.Out
	if out == nil {
		out = io.Discard
	}
	e := &engine{strict: opts.Strict, silent: opts.Silent, dryRun: opts.DryRun, only: opts.Only, out: out, projectDir: projectDir}
	e.afailedMDPath = filepath.Join(projectDir, "afailed.md")

	patchBytes, _ := os.ReadFile(patchFile)
	e.patchContent = string(patchBytes)

	afailedAPPath := filepath.Join(projectDir, "afailed.ap")
	if !e.strict {
		if pathExists(afailedAPPath) {
			return e.fatal(StatusFailed, "", false, 0, false, &AppError{
				Code:    ErrAfailedExists,
				Message: fmt.Sprintf("afailed.ap exists at %s. Please remove or rename it before running.", afailedAPPath),
			})
		}
	}

	var warnFn func(string)
	if !e.silent {
		warnFn = e.warn
	}
	data, perr := Parse(e.patchContent, e.strict, warnFn)
	if perr != nil {
		return e.fatal(StatusFailed, "", false, 0, false, &AppError{Code: ErrInvalidPatchFile, Message: perr.Error()})
	}

	patchIDStr := data.PatchID
	if patchIDStr == "" {
		patchIDStr = "00000000"
	}

	for _, change := range data.Changes {
		if res := e.processChange(change); res != nil {
			return res
		}
	}

	if !e.strict && len(e.failedChangesOutput) > 0 {
		_ = writeAfailedAP(afailedAPPath, patchIDStr, e.failedChangesOutput)
		_ = writeLLMReport(e.afailedMDPath, e.patchContent, e.llmFileReports, nil, false)
		e.printf("\nWARNING: Some changes failed and were written to %s\n", afailedAPPath)
		e.printf("         A briefing for the generating model is in %s\n", e.afailedMDPath)
	}

	var undo *Undo
	if e.dryRun {
		e.reportDryRun()
	} else if len(e.writePlan) > 0 {
		// Transaction (docs/VTVIBE.md §7.4): snapshot everything the plan
		// touches, write, and either roll the snapshot back at once if a
		// write fails or hand it to the caller as Result.Undo.
		u, err := beginUndo(projectDir, e.writePlan)
		if err != nil {
			return e.fatal(StatusFailed, "", false, 0, false, &AppError{
				Code:    ErrSnapshotError,
				Message: "Cannot snapshot the files the patch touches, nothing was written: " + err.Error(),
			})
		}
		if res := e.commit(); res != nil {
			note := "Everything already written was rolled back."
			if rerr := u.restore(); rerr != nil {
				note = "Rolling back what was already written failed too: " + rerr.Error()
			}
			res.Error.Message += " " + note
			e.printf("%s\n", note)
			return res
		}
		if err := u.seal(); err != nil {
			e.printf("  ! WARNING: the patch cannot be undone: %v\n", err)
		} else {
			undo = u
		}
	}

	if !e.dryRun && len(e.failedChangesOutput) == 0 && pathExists(e.afailedMDPath) {
		_ = os.Remove(e.afailedMDPath)
	}

	if len(e.failedChangesOutput) > 0 {
		var failedFiles []string
		for _, b := range e.failedChangesOutput {
			failedFiles = append(failedFiles, b.FilePath)
		}
		return &Result{Status: StatusPartial, FailedFiles: failedFiles, ModificationResults: e.modResults, Undo: undo}
	}
	return &Result{Status: StatusSuccess, ModificationResults: e.modResults, Undo: undo}
}

// --- engine: per-Apply-call mutable state ----------------------------

type engine struct {
	strict     bool
	silent     bool
	dryRun     bool
	only       map[ModKey]bool
	out        io.Writer
	projectDir string

	patchContent  string
	afailedMDPath string

	llmFileReports      []fileReport
	failedChangesOutput []*failedFileBlock
	writePlan           []writeOp
	modResults          []ModificationResult
}

func (e *engine) printf(format string, args ...any) {
	if !e.silent {
		_, _ = fmt.Fprintf(e.out, format, args...) // Progress output has nowhere else to be reported.
	}
}

func (e *engine) warn(msg string) {
	if !e.silent {
		_, _ = fmt.Fprintf(e.out, "  [TOLERANT] %s\n", msg) // Progress output has nowhere else to be reported.
	}
}

func (e *engine) reportIdempotencySkip(reason string) {
	e.printf("  ~ SKIPPED (Idempotency): Looks like it's already applied. Reason: %s\n", reason)
}

func (e *engine) fatal(status Status, filePath string, hasFilePath bool, modIdx int, hasModIdx bool, aerr *AppError) *Result {
	fatal := &fatalInfo{HasFilePath: hasFilePath, FilePath: filePath, Err: aerr}
	_ = writeLLMReport(e.afailedMDPath, e.patchContent, e.llmFileReports, fatal, e.strict)
	e.printf("\nERROR: %s\n", aerr.Message)
	return &Result{
		Status: status, Error: aerr, FilePath: filePath, HasFilePath: hasFilePath, ModIdx: modIdx, HasModIdx: hasModIdx,
		ModificationResults: e.modResults,
	}
}

// included reports whether Options.Only selects the modification modIdx
// (-1: RENAME) of the FILE block at the resolved path relativePath.
func (e *engine) included(relativePath string, modIdx int) bool {
	return e.only == nil || e.only[ModKey{FilePath: relativePath, ModIdx: modIdx}]
}

// selectedMods is mods restricted to the ones Options.Only selects, in
// patch order - what afailed.ap gets when a whole FILE block fails, so a
// retry never brings back an edit the user turned down.
func (e *engine) selectedMods(relativePath string, mods []*Modification) []*Modification {
	if e.only == nil {
		return mods
	}
	var sel []*Modification
	for i, m := range mods {
		if e.included(relativePath, i) {
			sel = append(sel, m)
		}
	}
	return sel
}

// blockModIdxs lists the ModIdx values a FILE block is reported under:
// 0 for a contextual whole-file DELETE, -1 for RENAME, otherwise one per
// modification - the same keys processChange's branches record.
func blockModIdxs(change *FileChange) []int {
	mods := change.Modifications
	switch {
	case len(mods) == 1 && mods[0].Action == "DELETE" && isBareDelete(mods[0]):
		return []int{0}
	case change.RenameTo != nil:
		return []int{-1}
	}
	idxs := make([]int, len(mods))
	for i := range idxs {
		idxs[i] = i
	}
	return idxs
}

// skipExcludedBlock handles a FILE block none of whose modifications
// Options.Only selects: each is reported ModExcluded and nothing else
// happens (no existence or path check - the user said "not this file").
// It returns false, doing nothing, when anything in the block is selected.
func (e *engine) skipExcludedBlock(change *FileChange, relativePath string) bool {
	if e.only == nil {
		return false
	}
	idxs := blockModIdxs(change)
	if len(idxs) == 0 {
		return false
	}
	for _, i := range idxs {
		if e.included(relativePath, i) {
			return false
		}
	}
	e.printf("\nFile: %s\n  = EXCLUDED: not selected.\n", relativePath)
	for _, i := range idxs {
		if i < 0 {
			e.recordFileResult(relativePath, "RENAME", *change.RenameTo, ModExcluded, nil)
		} else {
			e.recordModResult(relativePath, i, change.Modifications[i], ModExcluded, nil)
		}
	}
	return true
}

// recordModResult appends one Modification's outcome to the per-Apply-call
// report (Result.ModificationResults). mod may be nil (a patch-level
// failure with no specific modification, e.g. an unparseable FILE block);
// Action/Locator are then left empty.
func (e *engine) recordModResult(filePath string, modIdx int, mod *Modification, status ModStatus, aerr *AppError) {
	var action, locator string
	if mod != nil {
		action = mod.Action
		locator = modLocator(mod)
	}
	e.modResults = append(e.modResults, ModificationResult{
		FilePath: filePath, ModIdx: modIdx, Action: action, Locator: locator, Status: status, Err: aerr,
	})
}

// recordFileResult is recordModResult's counterpart for a whole-FILE-block
// operation that carries no Modification of its own - currently RENAME
// only, since a bare whole-file DELETE still has a real Modification
// (mods[0]) to key off. ModIdx -1 marks it as such to callers.
func (e *engine) recordFileResult(filePath, action, locator string, status ModStatus, aerr *AppError) {
	e.modResults = append(e.modResults, ModificationResult{
		FilePath: filePath, ModIdx: -1, Action: action, Locator: locator, Status: status, Err: aerr,
	})
}

// modLocator is the text a Modification searched for, preferring snippet
// over anchor - the same precedence Apply itself uses when both may be
// given (see the "TOLERANT: Using 'anchor' as snippet" fallback).
func modLocator(m *Modification) string {
	if s := derefOr(m.Snippet, ""); s != "" {
		return s
	}
	return derefOr(m.Anchor, "")
}

func (e *engine) addFailedWholeChange(relativePath, newline string, mods []*Modification) {
	e.failedChangesOutput = append(e.failedChangesOutput, &failedFileBlock{
		FilePath: relativePath, Newline: newline, Modifications: mods,
	})
}

func (e *engine) findOrCreateFailedBlock(relativePath, newline string) *failedFileBlock {
	for _, b := range e.failedChangesOutput {
		if b.FilePath == relativePath {
			return b
		}
	}
	b := &failedFileBlock{FilePath: relativePath, Newline: newline}
	e.failedChangesOutput = append(e.failedChangesOutput, b)
	return b
}

// --- write plan --------------------------------------------------------

type writeOpKind int

const (
	opDeletePath writeOpKind = iota
	opRename
	opCreateDir
	opWrite
)

type writeOp struct {
	kind    writeOpKind
	path    string
	newPath string
	content string
	relPath string
}

func (e *engine) commit() *Result {
	var deletes, renames, dirs, writes []writeOp
	for _, op := range e.writePlan {
		switch op.kind {
		case opDeletePath:
			deletes = append(deletes, op)
		case opRename:
			renames = append(renames, op)
		case opCreateDir:
			dirs = append(dirs, op)
		case opWrite:
			writes = append(writes, op)
		}
	}
	for _, op := range deletes {
		fi, err := os.Stat(op.path)
		if err != nil {
			continue
		}
		if fi.IsDir() {
			err = os.RemoveAll(op.path)
		} else {
			err = os.Remove(op.path)
		}
		if err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileDeleteError, Message: err.Error()})
		}
	}
	for _, op := range renames {
		if !pathExists(op.path) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(op.newPath), 0o755); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileRenameError, Message: err.Error()})
		}
		if err := os.Rename(op.path, op.newPath); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileRenameError, Message: err.Error()})
		}
	}
	for _, op := range dirs {
		if err := os.MkdirAll(op.path, 0o755); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrDirCreateError, Message: err.Error()})
		}
	}
	for _, op := range writes {
		if err := os.MkdirAll(filepath.Dir(op.path), 0o755); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileWriteError, Message: err.Error()})
		}
		if err := os.WriteFile(op.path, []byte(op.content), 0o600); err != nil {
			return e.fatal(StatusFailed, op.relPath, true, 0, false, &AppError{Code: ErrFileWriteError, Message: err.Error()})
		}
	}
	return nil
}

// reportDryRun prints one summary line per planned change instead of
// applying it, the DryRun counterpart of commit(). It mirrors the
// reference's "--- DRY RUN: planned changes ---" block (kind and path per
// op), but without the reference's per-file unified diff: the caller
// already has the confirmation dialog's own file list, and a paragraph of
// diff text does not fit a modal message box any better than this does.
func (e *engine) reportDryRun() {
	if len(e.writePlan) == 0 {
		return
	}
	e.printf("\n--- DRY RUN: planned changes ---\n")
	for _, op := range e.writePlan {
		switch op.kind {
		case opDeletePath:
			e.printf("delete %s\n", op.relPath)
		case opRename:
			newRel, err := filepath.Rel(e.projectDir, op.newPath)
			if err != nil {
				newRel = op.newPath
			}
			e.printf("rename %s -> %s\n", op.relPath, newRel)
		case opCreateDir:
			e.printf("create dir %s\n", op.relPath)
		case opWrite:
			e.printf("write %s\n", op.relPath)
		}
	}
}

// --- per-FILE-block dispatch --------------------------------------------

func isBareDelete(m *Modification) bool {
	return m.Snippet == nil && m.Anchor == nil && m.Content == nil && m.SnippetTail == nil &&
		m.IncludeLeadingBlankLines == 0 && m.IncludeTrailingBlankLines == 0 && m.ScopeEnd == 0
}

// processChange handles one FILE block. A non-nil return means a
// strict-mode fatal error occurred and Apply must return immediately
// (nothing gets written, per §3.0 atomicity).
func (e *engine) processChange(change *FileChange) *Result {
	if !change.HasFilePath {
		return e.fatal(StatusFailed, "", false, 0, false, &AppError{
			Code: ErrInvalidPatchFile, Message: "Missing 'file_path' for a change block.",
		})
	}
	originalRelativePath := change.FilePath
	isExplicitDir := strings.HasSuffix(originalRelativePath, "/") || strings.HasSuffix(originalRelativePath, "\\")
	relativePath := strings.TrimRight(originalRelativePath, "/\\")
	if relativePath == "" {
		relativePath = originalRelativePath
	}

	relativePath, strippedPrefix := resolvePathPrefix(e.projectDir, relativePath)
	if e.skipExcludedBlock(change, relativePath) {
		return nil
	}

	filePath, secErr := securePath(e.projectDir, relativePath)
	if secErr != nil {
		aerr := &AppError{Code: ErrInvalidFilePath, Message: "Path traversal detected or invalid path format."}
		if len(change.Modifications) > 0 {
			for i, m := range change.Modifications {
				if e.included(relativePath, i) {
					e.recordModResult(relativePath, i, m, ModFailed, aerr)
				} else {
					e.recordModResult(relativePath, i, m, ModExcluded, nil)
				}
			}
		} else if change.RenameTo != nil {
			e.recordFileResult(relativePath, "RENAME", *change.RenameTo, ModFailed, aerr)
		}
		if e.strict {
			return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
		}
		e.printf("  - FAILED: Path traversal detected or invalid path format.\n")
		e.addFailedWholeChange(relativePath, change.Newline, e.selectedMods(relativePath, change.Modifications))
		return nil
	}

	var newlineChar string
	switch change.Newline {
	case "LF":
		newlineChar = "\n"
	case "CRLF":
		newlineChar = "\r\n"
	case "CR":
		newlineChar = "\r"
	default:
		if pathExists(filePath) {
			newlineChar = detectLineEndings(filePath)
		} else {
			newlineChar = osLineSep()
		}
	}

	e.printf("\nFile: %s\n", relativePath)

	mods := change.Modifications
	// CONTEXTUAL FILE DELETION: a FILE block containing only a bare
	// DELETE (no other directives) deletes the whole file/directory.
	if len(mods) == 1 && mods[0].Action == "DELETE" && isBareDelete(mods[0]) {
		if !pathExists(filePath) {
			e.reportIdempotencySkip(fmt.Sprintf("Path to delete does not exist: %s", filePath))
			e.recordModResult(relativePath, 0, mods[0], ModSkipped, nil)
			return nil
		}
		e.writePlan = append(e.writePlan, writeOp{kind: opDeletePath, path: filePath, relPath: relativePath})
		e.printf("  + SUCCESS: File deleted.\n")
		e.recordModResult(relativePath, 0, mods[0], ModOK, nil)
		return nil
	}

	if change.RenameTo != nil {
		return e.processRename(change, relativePath, filePath, strippedPrefix)
	}

	return e.processModifications(change, relativePath, filePath, newlineChar, isExplicitDir)
}

func (e *engine) processRename(change *FileChange, relativePath, filePath, strippedPrefix string) *Result {
	newRelativePath := *change.RenameTo
	if strippedPrefix != "" {
		newParts := strings.Split(strings.ReplaceAll(newRelativePath, "\\", "/"), "/")
		prefixParts := strings.Split(strippedPrefix, "/")
		if len(newParts) >= len(prefixParts) && stringSlicesEqual(newParts[:len(prefixParts)], prefixParts) {
			newRelativePath = strings.Join(newParts[len(prefixParts):], "/")
		}
	}

	newFilePath, secErr := securePath(e.projectDir, newRelativePath)
	if secErr != nil {
		aerr := &AppError{Code: ErrInvalidFilePath, Message: "Path traversal detected in new rename path."}
		e.recordFileResult(relativePath, "RENAME", newRelativePath, ModFailed, aerr)
		if e.strict {
			return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
		}
		e.printf("  - FAILED: Path traversal detected in new rename path.\n")
		e.addFailedWholeChange(relativePath, change.Newline, change.Modifications)
		return nil
	}

	if pathExists(newFilePath) {
		if !pathExists(filePath) {
			e.reportIdempotencySkip(fmt.Sprintf("Source does not exist, but destination does. Assuming rename complete: %s", newFilePath))
			e.recordFileResult(relativePath, "RENAME", newRelativePath, ModSkipped, nil)
			return nil
		}
		aerr := &AppError{Code: ErrDestinationExists, Message: "Rename destination already exists."}
		e.recordFileResult(relativePath, "RENAME", newRelativePath, ModFailed, aerr)
		if e.strict {
			return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
		}
		e.printf("  - FAILED: Rename destination already exists.\n")
		e.addFailedWholeChange(relativePath, change.Newline, change.Modifications)
		return nil
	}

	if !pathExists(filePath) {
		aerr := &AppError{Code: ErrFileNotFound, Message: "Target for rename not found."}
		e.recordFileResult(relativePath, "RENAME", newRelativePath, ModFailed, aerr)
		if e.strict {
			return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
		}
		e.printf("  - FAILED: Target for rename not found.\n")
		e.addFailedWholeChange(relativePath, change.Newline, change.Modifications)
		return nil
	}

	e.writePlan = append(e.writePlan, writeOp{kind: opRename, path: filePath, newPath: newFilePath, relPath: relativePath})
	e.printf("  + SUCCESS: Renamed to %s\n", newRelativePath)
	e.recordFileResult(relativePath, "RENAME", newRelativePath, ModOK, nil)
	return nil
}

// --- modification loop --------------------------------------------------

type modFailure struct {
	Idx int
	Mod *Modification
	Err *AppError
}

type consumedEntry struct {
	Idx     int
	Removed string
}

type locatorTriple struct{ anchor, snippet, tail string }

func locatorKey(m *Modification) locatorTriple {
	return locatorTriple{
		normalizeBlock(derefOr(m.Anchor, "")),
		normalizeBlock(derefOr(m.Snippet, "")),
		normalizeBlock(derefOr(m.SnippetTail, "")),
	}
}

func actionOrUnknown(a string) string {
	if a == "" {
		return "Unknown"
	}
	return a
}

func (e *engine) processModifications(change *FileChange, relativePath, filePath, newlineChar string, isExplicitDir bool) *Result {
	fileExisted := pathExists(filePath)
	var originalContent string
	if fileExisted && isDirPath(filePath) {
		// A directory at the target path: leave originalContent empty; the
		// logic below (CREATE/RECREATE on it) will surface the right errors.
	} else if fileExisted {
		b, rerr := os.ReadFile(filePath)
		if rerr == nil {
			originalContent = normalizeToLF(string(b))
		}
	} else {
		hasCreateOrRecreate := false
		for _, m := range e.selectedMods(relativePath, change.Modifications) {
			if m.Action == "CREATE" || m.Action == "RECREATE" {
				hasCreateOrRecreate = true
				break
			}
		}
		if !hasCreateOrRecreate {
			aerr := &AppError{Code: ErrFileNotFound, Message: "Target file not found."}
			for i, m := range change.Modifications {
				if e.included(relativePath, i) {
					e.recordModResult(relativePath, i, m, ModFailed, aerr)
				} else {
					e.recordModResult(relativePath, i, m, ModExcluded, nil)
				}
			}
			if e.strict {
				return e.fatal(StatusFailed, relativePath, true, 0, false, aerr)
			}
			e.printf("  - FAILED: Target file not found.\n")
			e.addFailedWholeChange(relativePath, change.Newline, e.selectedMods(relativePath, change.Modifications))
			return nil
		}
	}

	workingContent := originalContent
	initialContent := workingContent
	var consumedLog []consumedEntry

	seen := map[locatorTriple]bool{}
	repeated := map[locatorTriple]bool{}
	for _, m := range change.Modifications {
		if derefOr(m.Snippet, "") == "" && derefOr(m.Anchor, "") == "" {
			continue
		}
		k := locatorKey(m)
		if seen[k] {
			repeated[k] = true
		}
		seen[k] = true
	}

	var dirtyRegions []span
	terminalOpPlanned := false

	// Deselected modifications (Options.Only) never enter the retry loop.
	// The locator-repetition map above still counts them on purpose - see
	// the Only doc comment.
	var pendingIdx []int
	for i, m := range change.Modifications {
		if e.included(relativePath, i) {
			pendingIdx = append(pendingIdx, i)
		} else {
			e.printf("  = EXCLUDED: Mod #%d (%s) not selected.\n", i+1, actionOrUnknown(m.Action))
			e.recordModResult(relativePath, i, m, ModExcluded, nil)
		}
	}
	var finalFailedMods []modFailure
	passNumber := 1

	for len(pendingIdx) > 0 {
		madeProgress := false
		var failedInThisPass []modFailure
		lastModEndPos := 0

		for _, modIdx := range pendingIdx {
			mod := change.Modifications[modIdx]
			before := workingContent
			res, failure, progressed, newLastEnd, stop := e.applyOneModification(
				change, mod, modIdx, relativePath, filePath, isExplicitDir, fileExisted,
				originalContent, &workingContent, initialContent, &dirtyRegions, &consumedLog,
				repeated[locatorKey(mod)], lastModEndPos, passNumber, &terminalOpPlanned)
			if stop {
				return res
			}
			if failure != nil {
				failedInThisPass = append(failedInThisPass, *failure)
				continue
			}
			if progressed {
				madeProgress = true
				e.recordModResult(relativePath, modIdx, mod, ModOK, nil)
				e.modResults[len(e.modResults)-1].Preview = modPreview(before, workingContent)
			} else {
				e.recordModResult(relativePath, modIdx, mod, ModSkipped, nil)
			}
			lastModEndPos = newLastEnd
		}

		if !madeProgress {
			finalFailedMods = failedInThisPass
			break
		}
		pendingIdx = make([]int, len(failedInThisPass))
		for i, f := range failedInThisPass {
			pendingIdx[i] = f.Idx
		}
		passNumber++
	}

	if len(finalFailedMods) > 0 {
		var failedItems []failedItem
		for _, f := range finalFailedMods {
			failedItems = append(failedItems, failedItem{ModIdx: f.Idx, Mod: f.Mod, Err: f.Err})
		}
		e.llmFileReports = append(e.llmFileReports, fileReport{
			FilePath: relativePath, Original: originalContent, Current: workingContent,
			TotalMods: len(change.Modifications), Failed: failedItems,
		})
		for _, f := range finalFailedMods {
			e.printf("  - FAILED: Mod #%d (%s). Reason: %s\n", f.Idx+1, actionOrUnknown(f.Mod.Action), f.Err.Message)
			block := e.findOrCreateFailedBlock(relativePath, change.Newline)
			block.Modifications = append(block.Modifications, f.Mod)
			e.recordModResult(relativePath, f.Idx, f.Mod, ModFailed, f.Err)
		}
	}

	// STRUCTURAL SANITY CHECK: a file balanced before the patch and not
	// balanced after it is almost certainly no longer valid source, even
	// though every modification applied cleanly.
	if fileExisted && workingContent != initialContent && !e.silent {
		before := netBracketDepth(initialContent)
		after := netBracketDepth(workingContent)
		if before == 0 && after != 0 {
			e.printf("  ! WARNING: brackets in %s were balanced before the patch and are off by %+d "+
				"after it. The result is very likely not valid source code - review it before committing.\n",
				relativePath, after)
		}
	}

	if !terminalOpPlanned {
		finalContent := strings.Join(strings.Split(workingContent, "\n"), newlineChar)
		// "Did the patch change this file?" has to be answered in one
		// line-ending domain. originalContent is LF-normalized, so comparing
		// it with finalContent (already re-joined with newlineChar) made
		// every CRLF/CR file look changed: a no-op patch rewrote it and
		// appended the final newline meant only for files it really edits
		// (f4#1606; the reference ap.py has the same slip). The text
		// changed, or an explicit FILE LF/CRLF/CR asks for endings the file
		// does not have yet - otherwise the file is left byte for byte.
		changed := !fileExisted || workingContent != originalContent
		if !changed && change.Newline != "" {
			raw, rerr := os.ReadFile(filePath)
			changed = rerr != nil || string(raw) != finalContent
		}
		if changed {
			if finalContent != "" && !strings.HasSuffix(finalContent, newlineChar) {
				finalContent += newlineChar
			}
			e.writePlan = append(e.writePlan, writeOp{kind: opWrite, path: filePath, content: finalContent, relPath: relativePath})
		}
	}

	return nil
}
