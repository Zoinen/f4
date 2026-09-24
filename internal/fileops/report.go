package fileops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// OpenLog shows the log of a finished operation. The root wires it to f4's
// viewer; left unwired, the summary simply offers no way into the log.
var OpenLog func(path string)

// opReport is what an operation that ignores errors (#722) keeps about itself:
// a log in the temporary folder and the tallies its summary shows. The log is
// written line by line, not buffered, so what a crash leaves behind is still
// readable.
type opReport struct {
	path    string
	file    *os.File
	logErr  error
	copied  int
	failed  int
	skipped int
}

func openOpReport(action string, opts FileOpOptions, srcBase string, names []string, dest string) *opReport {
	r := &opReport{}
	f, err := os.CreateTemp("", "f4-"+strings.ToLower(action)+"-*.log")
	if err != nil {
		r.logErr = err
		vtui.DebugLog("FILEOP: cannot create the operation log: %v", err)
		return r
	}
	r.file, r.path = f, f.Name()
	r.printf("%s started", action)
	r.printf("source folder: %s", srcBase)
	for _, name := range names {
		r.printf("item: %s", name)
	}
	r.printf("destination: %s", dest)
	r.printf("ignore read errors: %v, read attempts: %d, ignore write errors: %v",
		opts.IgnoreReadErrors, opts.readAttempts(), opts.IgnoreWriteErrors)
	r.printf("already existing files: %d, access rights: %d, symbolic links as links: %v",
		opts.ExistingFiles, opts.AccessRights, opts.SymlinksAsLinks)
	return r
}

func (r *opReport) printf(format string, args ...any) {
	if r == nil || r.file == nil {
		return
	}
	line := time.Now().Format("2006-01-02 15:04:05") + "  " + fmt.Sprintf(format, args...) + "\n"
	if _, err := r.file.WriteString(line); err != nil && r.logErr == nil {
		r.logErr = err
	}
}

func (r *opReport) close(opErr error) {
	if r == nil {
		return
	}
	if opErr != nil {
		r.printf("stopped: %v", opErr)
	}
	r.printf("finished: %d done, %d failed, %d skipped", r.copied, r.failed, r.skipped)
	if r.file != nil {
		if err := r.file.Close(); err != nil && r.logErr == nil {
			r.logErr = err
		}
		r.file = nil
	}
}

// showOpSummary ends an operation that ignored errors with what it did: the
// issue asks for the counts and a way into the detailed log, whether the
// operation completed or not.
func showOpSummary(isMove bool, r *opReport, opErr error) {
	if r == nil || vtui.FrameManager == nil {
		return
	}
	title := i18n.Msg("Copy.Title")
	done := fmt.Sprintf(i18n.Msg("FileOp.Summary.Copied"), r.copied)
	if isMove {
		title = i18n.Msg("Move.Title")
		done = fmt.Sprintf(i18n.Msg("FileOp.Summary.Moved"), r.copied)
	}
	lines := []string{done, fmt.Sprintf(i18n.Msg("FileOp.Summary.Failed"), r.failed)}
	if r.skipped > 0 {
		lines = append(lines, fmt.Sprintf(i18n.Msg("FileOp.Summary.Skipped"), r.skipped))
	}
	switch {
	case opErr == nil:
	case errors.Is(opErr, context.Canceled):
		lines = append(lines, i18n.Msg("FileOp.Summary.Cancelled"))
	default:
		lines = append(lines, fmt.Sprintf(i18n.Msg("FileOp.Summary.Stopped"), opErr))
	}
	if r.logErr != nil {
		lines = append(lines, fmt.Sprintf(i18n.Msg("FileOp.Summary.NoLog"), r.logErr))
	}
	buttons := []string{i18n.Msg("vtui.Ok")}
	logPath := r.path
	if logPath != "" {
		buttons = append(buttons, i18n.Msg("FileOp.Summary.ViewLog"))
	}
	text := strings.Join(lines, "\n")
	vtui.FrameManager.PostTask(func() {
		dlg := vtui.ShowMessage(title, text, buttons)
		dlg.OnResult = func(code int) {
			if code == 1 && logPath != "" && OpenLog != nil {
				OpenLog(logPath)
			}
		}
	})
}

// note writes a line to the log of an operation that keeps one.
func (s *FileOpState) note(format string, args ...any) {
	if s != nil {
		s.Report.printf(format, args...)
	}
}

func (s *FileOpState) fileCopied(src, dst string) {
	if s == nil || s.Report == nil {
		return
	}
	s.Report.copied++
	s.Report.printf("OK       %s -> %s", src, dst)
}

func (s *FileOpState) skipItem(src, dst string) {
	s.SkippedCount++
	if s.Report != nil {
		s.Report.skipped++
		s.Report.printf("SKIPPED  %s -> %s", src, dst)
	}
	if s.Tracker != nil {
		s.Tracker.FileSkipped()
		if s.UpdateUI != nil {
			s.UpdateUI(true)
		}
	}
}

// tolerate decides whether an error ends the operation. With the matching
// "Ignore ... errors" choice made, the item is recorded as failed, the progress
// moves past it, and true tells the caller to go on with the next item.
// Cancellation and an outcome a provider could not confirm are never tolerated:
// those are the errors that have to reach the user unchanged.
func (s *FileOpState) tolerate(ignore bool, side, path string, err error) bool {
	if s == nil || !ignore || err == nil || OperationMustNotRetry(err) {
		return false
	}
	s.FailedCount++
	if s.Report != nil {
		s.Report.failed++
		s.Report.printf("FAILED   %s: %s error: %v", path, side, err)
	}
	vtui.DebugLog("FILEOP: %s error on %q ignored: %v", side, path, err)
	if s.Tracker != nil {
		s.Tracker.FileSkipped()
		if s.UpdateUI != nil {
			s.UpdateUI(true)
		}
	}
	return true
}

func (s *FileOpState) tolerateRead(path string, err error) bool {
	return s != nil && s.tolerate(s.IgnoreReadErrors, "read", path, err)
}

func (s *FileOpState) tolerateWrite(path string, err error) bool {
	return s != nil && s.tolerate(s.IgnoreWriteErrors, "write", path, err)
}
