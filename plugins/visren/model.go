// Package visren implements visual batch renaming for F4.
//
// Its mask language and user-facing behaviour are based on FarPlugins/VisRen
// 3.2.0 (BSD-3-Clause), commit a31e48d7dd154c4fde86aef593810343a1289356.
package visren

import (
	"fmt"
	"math/rand/v2"
	"path/filepath"
	"sync"
	"time"
)

const errorPreview = "<Error!>"

type Metadata struct {
	Track, Title, Artist, Album, Year, Genre string
	CameraMake, CameraModel                  string
	ImageDate                                string
	Width, Height                            int
	Version                                  string
}

type Item struct {
	Source string
	MTime  time.Time
	IsDir  bool
	Random int

	path     string
	metaOnce sync.Once
	meta     Metadata
	metaErr  error
}

func NewItem(path, name string, mtime time.Time, isDir bool) *Item {
	return &Item{
		Source: name,
		MTime:  mtime,
		IsDir:  isDir,
		// #nosec G404 -- this value expands the user's random rename mask and has no security role.
		Random: rand.IntN(32768),
		path:   filepath.Join(path, name),
	}
}

func (i *Item) Metadata() (Metadata, error) {
	i.metaOnce.Do(func() { i.meta, i.metaErr = readMetadata(i.path, i.MTime) })
	return i.meta, i.metaErr
}

type Options struct {
	NameMask      string
	ExtMask       string
	Search        string
	Replace       string
	CaseSensitive bool
	Regex         bool
	WordDiv       string
}

type Preview struct {
	Item               *Item
	Destination        string
	Err                error
	Matches            []TextRange
	ReplacementMatches []TextRange
}

// TextRange is a half-open range of Unicode code points in a displayed name.
type TextRange struct {
	Start int
	End   int
}

type Engine struct {
	Items []*Item
}

func (e *Engine) Build(opts Options) ([]Preview, error) {
	if opts.WordDiv == "" {
		opts.WordDiv = "-. _&"
	}
	compiled, err := compileReplacement(opts)
	if err != nil {
		return errorPreviews(e.Items, err), err
	}

	result := make([]Preview, len(e.Items))
	var firstErr error
	for idx, item := range e.Items {
		dest, replacementMatches, err := renderItem(item, idx, opts, compiled)
		if err == nil && dest != item.Source {
			err = ValidateFilename(dest)
		}
		result[idx] = Preview{Item: item, Destination: dest, Err: err, Matches: compiled.matches(item.Source), ReplacementMatches: replacementMatches}
		if err != nil {
			result[idx].Destination = errorPreview
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", item.Source, err)
			}
		}
	}
	if firstErr != nil {
		for idx := range result {
			result[idx].Destination = errorPreview
			if result[idx].Err == nil {
				result[idx].Err = firstErr
			}
		}
	}
	return result, firstErr
}

func errorPreviews(items []*Item, err error) []Preview {
	result := make([]Preview, len(items))
	for idx, item := range items {
		result[idx] = Preview{Item: item, Destination: errorPreview, Err: err}
	}
	return result
}
