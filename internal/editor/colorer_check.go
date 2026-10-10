//go:build !lite

package editor

import (
	"context"
	"errors"
	"fmt"

	colorer "github.com/unxed/colorer4go"
)

// maxColorerCheckReports bounds how many of Colorer's messages a check keeps.
// The first ones name the broken file; a long tail only repeats that.
const maxColorerCheckReports = 20

// checkAllBatchSize bounds how many file types CheckColorerSource loads in one
// colorer4go session before it closes that session and opens a fresh one.
//
// colorer4go's HRC engine keeps every scheme it has ever loaded, in the same
// session, in one namespace, and re-links and re-builds the search dispatch
// table of *all* of them — not just the one just loaded — after each single
// LoadFileType call (HrcLibrary::Impl::updateLinks, called from parseHRC).
// That cost is invisible in ordinary editing, where a session sees only the
// handful of types its open files use, but "Check all schemes" loads every
// type the catalog has, one after another, in the same session: each call
// re-processes everything loaded before it, so the whole run is quadratic in
// the number of types, and gets slower call after call instead of costing
// roughly the same each time (#277). Recreating the session every
// checkAllBatchSize types bounds how much accumulated state any single
// LoadFileType call has to re-process, which turns the total cost back into
// something that scales with the number of types, not its square.
const checkAllBatchSize = 32

// ColorerCheck is what loading a Colorer configuration found.
type ColorerCheck struct {
	// Err is why an editor could not highlight with this configuration: the
	// session, the colour style or a scheme failed to load. Nil when it can.
	Err error
	// Reports are the errors and warnings Colorer reported on the way, the ones
	// it survived included, oldest first: a file and line where Err names only
	// a throw site, a user path that was skipped, a scheme link that does not
	// resolve.
	Reports []string
	// Types is how many file types were loaded; zero without allTypes.
	Types int
	// sessions is how many colorer4go sessions the check opened: one, plus one
	// more per checkAllBatchSize types beyond the first. Tests use it to catch
	// a regression to a single session for every type (#277) without timing
	// anything, since colorer4go's own per-type cost is what actually grows.
	sessions int
}

// Clean reports whether nothing failed and nothing was reported.
func (c ColorerCheck) Clean() bool {
	return c.Err == nil && len(c.Reports) == 0
}

// CheckColorerSource loads the configuration src names the way an editor does
// when it starts Colorer — the catalog, the user's colour styles and schemes,
// the colour style — and, with allTypes, the scheme of every file type as
// well, which is where a broken scheme otherwise shows up only once a file of
// its type is opened. It is FarColorer's TestLoadBase, except that allTypes
// really loads each type: TestLoadBase calls getBaseScheme(), which in this
// Colorer version does not load anything.
//
// progress, if not nil, is called before each type is loaded. Cancelling ctx
// stops between types and is reported as Err.
func CheckColorerSource(ctx context.Context, src ColorerSource, scheme string, allTypes bool, progress func(done, total int, label string)) ColorerCheck {
	var check ColorerCheck
	if err := colorerRuntimeCheck(); err != nil {
		check.Err = err
		return check
	}
	logLevel, logging := colorerDiagnosticsLevel()
	level := colorer.LevelWarn
	if logging && logLevel > level {
		level = logLevel
	}
	collect := func(d colorer.Diagnostic) {
		if logging && d.Level <= logLevel {
			logColorerDiagnostic(d)
		}
		if d.Level <= colorer.LevelWarn && len(check.Reports) < maxColorerCheckReports {
			check.Reports = append(check.Reports, d.String())
		}
	}

	ensureRadiolaSchema(src.ConfigsDir)
	opts := append([]colorer.Option{colorer.WithDiagnostics(level, collect)}, src.userOptions()...)
	if scheme == "" {
		scheme = "default"
	}
	open := func() (*colorer.Session, error) {
		s, err := colorer.NewSession(ctx, "/base/catalog.xml", src.ConfigsDir, opts...)
		if err != nil {
			return nil, err
		}
		if err := s.SetHRD("rgb", scheme); err != nil {
			s.Close()
			return nil, fmt.Errorf("colour style %q: %w", scheme, err)
		}
		check.sessions++
		return s, nil
	}

	session, err := open()
	if err != nil {
		check.Err = err
		return check
	}
	defer func() {
		if session != nil {
			session.Close()
		}
	}()

	if !allTypes {
		return check
	}

	types, err := session.FileTypes()
	if err != nil {
		check.Err = err
		return check
	}
	for i, ft := range types {
		if err := ctx.Err(); err != nil {
			check.Err = err
			return check
		}
		if i > 0 && i%checkAllBatchSize == 0 {
			session.Close()
			session = nil
			session, err = open()
			if err != nil {
				check.Err = err
				return check
			}
		}
		if progress != nil {
			progress(i, len(types), ft.Group+": "+ft.Description)
		}
		ok, err := session.LoadFileType(ft.Name)
		if err != nil {
			check.Err = fmt.Errorf("file type %q (%s): %w", ft.Name, ft.Description, err)
			return check
		}
		check.Types++
		if !ok && len(check.Reports) < maxColorerCheckReports {
			check.Reports = append(check.Reports, fmt.Sprintf("file type %q (%s) has no scheme", ft.Name, ft.Description))
		}
	}
	return check
}

// IsColorerCheckCancelled tells a check the user stopped from one that failed.
func IsColorerCheckCancelled(c ColorerCheck) bool {
	return errors.Is(c.Err, context.Canceled)
}
