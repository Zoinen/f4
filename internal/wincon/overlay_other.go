//go:build !windows

package wincon

import "errors"

// There is no Windows console anywhere else, so nothing here finds one. The
// stubs exist so that the geometry above, and everything that calls into this
// package, compiles on every platform f4 is built for.

// ErrNoConsole means this is not Windows, or there is no console window on it.
var ErrNoConsole = errors.New("no Windows console here")

type Overlay struct{}

func ConsoleWindow() (uintptr, Source) { return 0, SourceNone }

func CellSize() (int, int, bool) { return 0, 0, false }

func GridSize() (int, int, bool) { return 0, 0, false }

func New() (*Overlay, error) { return nil, ErrNoConsole }

func (o *Overlay) Place(Rect) error                 { return ErrNoConsole }
func (o *Overlay) Draw([]byte, int, int, int) error { return ErrNoConsole }
func (o *Overlay) SetBounds([]Rect) bool            { return false }
func (o *Overlay) Hide()                            {}
func (o *Overlay) Visible() bool                    { return false }
func (o *Overlay) ClientSize() (int, int, bool)     { return 0, 0, false }
func (o *Overlay) Close()                           {}
func (o *Overlay) Stats() Stats                     { return Stats{} }
