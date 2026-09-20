package fileops

// ExistingFilesMode is the "Already existing files" choice of the F5/F6 dialog
// (#722): what happens when a file being copied or moved meets one of the same
// name.
type ExistingFilesMode int

const (
	// ExistingFilesAsk asks about every conflict, which is what f4 always did.
	ExistingFilesAsk ExistingFilesMode = iota
	// ExistingFilesOverwrite replaces every existing file without asking.
	ExistingFilesOverwrite
	// ExistingFilesSkip leaves every existing file alone without asking.
	ExistingFilesSkip
)

// ExistingFilesModeFromChoice maps the position of the dialog's list onto a
// mode. Anything it does not know asks, the one choice that cannot lose data.
func ExistingFilesModeFromChoice(pos int) ExistingFilesMode {
	switch ExistingFilesMode(pos) {
	case ExistingFilesOverwrite:
		return ExistingFilesOverwrite
	case ExistingFilesSkip:
		return ExistingFilesSkip
	default:
		return ExistingFilesAsk
	}
}

// MaxReadAttempts bounds the "Read attempts" field. A read that failed a
// hundred times in a row is not going to succeed on the next one, and an
// unbounded count would let a typo stall a copy on one bad sector for hours.
const MaxReadAttempts = 100

// FileOpOptions carries the choices made in the copy/move dialog into the
// operation. Its zero value is the behaviour f4 had before the dialog offered
// any of them.
type FileOpOptions struct {
	AccessRights  AccessRightsMode
	ExistingFiles ExistingFilesMode
	// SymlinksAsLinks copies a symbolic link as a link. False follows the link
	// and copies what it points at, which is what f4 always did.
	SymlinksAsLinks bool
	// IgnoreReadErrors and IgnoreWriteErrors let the operation go on past a
	// file it cannot read or write: the file is recorded as failed and the
	// operation ends with a summary and a log instead of a question.
	IgnoreReadErrors  bool
	IgnoreWriteErrors bool
	// ReadAttempts is how many reads in a row a file gets before an ignored
	// read error gives up on it. Below 1 means 1: no retry.
	ReadAttempts int
}

// Tolerant reports whether the operation goes on past errors, and therefore
// has to end with a summary and a log.
func (o FileOpOptions) Tolerant() bool {
	return o.IgnoreReadErrors || o.IgnoreWriteErrors
}

// bulkCompatible reports whether a source's bulk copier may serve the
// operation. A bulk copier extracts on its own terms: it consults none of the
// choices above and reports no per-file outcome to count, so any choice other
// than the defaults sends the operation down the per-file path that honours
// it.
func (o FileOpOptions) bulkCompatible() bool {
	return o.AccessRights == AccessRightsDefault && o.ExistingFiles == ExistingFilesAsk && !o.Tolerant()
}

func (o FileOpOptions) readAttempts() int {
	if o.ReadAttempts < 1 {
		return 1
	}
	return min(o.ReadAttempts, MaxReadAttempts)
}

// applyTo puts the options into the state of one operation.
func (o FileOpOptions) applyTo(state *FileOpState, isMove bool) {
	state.AccessRights = o.AccessRights
	state.OverwriteAll = o.ExistingFiles == ExistingFilesOverwrite
	state.SkipAll = o.ExistingFiles == ExistingFilesSkip
	// A move carries a link as a link: moving it is not a request to replace
	// it with a copy of whatever it points at. That is why the dialog offers
	// the choice for a copy only.
	state.CopySymlinksAsLinks = o.SymlinksAsLinks || isMove
	state.IgnoreReadErrors = o.IgnoreReadErrors
	state.IgnoreWriteErrors = o.IgnoreWriteErrors
	state.ReadAttempts = o.readAttempts()
}
