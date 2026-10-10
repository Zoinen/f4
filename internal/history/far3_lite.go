//go:build lite

package history

import (
	"context"
	"errors"
)

var errFar3ImportUnavailable = errors.New("Far3 history import is unavailable in lite builds")

// ReadFar3History reports that the SQLite-backed importer is not linked into
// lite builds. It never opens or creates the source database. Snapshot merging
// and the application's JSON history store remain available.
func ReadFar3History(ctx context.Context, path string) (Far3History, error) {
	if err := ctx.Err(); err != nil {
		return Far3History{}, err
	}
	return Far3History{}, errFar3ImportUnavailable
}
