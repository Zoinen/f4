package visren

import (
	"context"
	"errors"
	"os"

	"github.com/unxed/f4/vfs"
)

type ErrorAction int

const (
	Retry ErrorAction = iota
	Skip
	SkipAll
	Cancel
)

type RenamePair struct {
	Old string
	New string
}

type RenameResult struct {
	Succeeded []RenamePair
	Pending   []string
	Canceled  bool
}

type ErrorHandler func(source, destination string, err error) ErrorAction

func ExecuteRenames(ctx context.Context, fs *vfs.OSVFS, dir string, rows []Preview, onError ErrorHandler) RenameResult {
	var result RenameResult
	skipAll := false
	for idx, row := range rows {
		if ctx.Err() != nil {
			result.Canceled = true
			result.Pending = append(result.Pending, sourcesFrom(rows[idx:])...)
			break
		}
		if row.Err != nil {
			result.Pending = append(result.Pending, row.Item.Source)
			continue
		}
		if row.Item.Source == row.Destination {
			continue
		}
		oldPath := fs.Join(dir, row.Item.Source)
		newPath := fs.Join(dir, row.Destination)
		if _, err := os.Lstat(oldPath); errors.Is(err, os.ErrNotExist) {
			continue
		}
		for {
			err := fs.RenameNoReplace(ctx, oldPath, newPath)
			if err == nil {
				result.Succeeded = append(result.Succeeded, RenamePair{Old: row.Item.Source, New: row.Destination})
				break
			}
			if skipAll {
				result.Pending = append(result.Pending, row.Item.Source)
				break
			}
			action := Skip
			if onError != nil {
				action = onError(row.Item.Source, row.Destination, err)
			}
			switch action {
			case Retry:
				continue
			case Skip:
				result.Pending = append(result.Pending, row.Item.Source)
			case SkipAll:
				skipAll = true
				result.Pending = append(result.Pending, row.Item.Source)
			case Cancel:
				result.Canceled = true
				result.Pending = append(result.Pending, sourcesFrom(rows[idx:])...)
			}
			break
		}
		if result.Canceled {
			break
		}
	}
	return result
}

func ExecuteUndo(ctx context.Context, fs *vfs.OSVFS, dir string, log []RenamePair, onError ErrorHandler) RenameResult {
	rows := make([]Preview, 0, len(log))
	for idx := len(log) - 1; idx >= 0; idx-- {
		pair := log[idx]
		rows = append(rows, Preview{Item: &Item{Source: pair.New}, Destination: pair.Old})
	}
	return ExecuteRenames(ctx, fs, dir, rows, onError)
}

func sourcesFrom(rows []Preview) []string {
	result := make([]string, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.Item.Source)
	}
	return result
}
