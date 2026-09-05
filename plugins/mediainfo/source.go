package mediainfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// ContextReaderAt is implemented by vfs.ReadAtCloser and by test sources.
type ContextReaderAt interface {
	ReadAt(context.Context, []byte, int64) (int, error)
}

// Source identifies a bounded random-access input. Reader ownership remains
// with the caller; Analyze never closes it.
type Source struct {
	Name    string
	Size    int64
	ModTime time.Time
	Reader  ContextReaderAt
}

// Options controls parser resource use. Zero values are filled from the
// selected mode by normalized().
type Options struct {
	Mode                   Mode
	MaxReadBytes           int64
	MaxReadOps             int
	MaxElements            int
	MaxSingleMetadataBytes int64
	MaxStreams             int
	MaxTags                int
	MaxChapters            int
	MaxTextBytes           int64
	MaxValueBytes          int
}

// DefaultOptions returns conservative defaults for interactive use.
func DefaultOptions(mode Mode) Options {
	if mode == ModeDetailed {
		return Options{Mode: mode, MaxReadBytes: 64 << 20, MaxReadOps: 500000,
			MaxElements: 250000, MaxSingleMetadataBytes: 8 << 20,
			MaxStreams: 256, MaxTags: 4096, MaxChapters: 50000,
			MaxTextBytes: 32 << 20, MaxValueBytes: 64 << 10}
	}
	return Options{Mode: ModeFast, MaxReadBytes: 8 << 20, MaxReadOps: 50000,
		MaxElements: 20000, MaxSingleMetadataBytes: 512 << 10,
		MaxStreams: 64, MaxTags: 256, MaxChapters: 512,
		MaxTextBytes: 2 << 20, MaxValueBytes: 16 << 10}
}

func (o Options) normalized() Options {
	d := DefaultOptions(o.Mode)
	if o.MaxReadBytes <= 0 {
		o.MaxReadBytes = d.MaxReadBytes
	}
	if o.MaxReadOps <= 0 {
		o.MaxReadOps = d.MaxReadOps
	}
	if o.MaxElements <= 0 {
		o.MaxElements = d.MaxElements
	}
	if o.MaxSingleMetadataBytes <= 0 {
		o.MaxSingleMetadataBytes = d.MaxSingleMetadataBytes
	}
	if o.MaxStreams <= 0 {
		o.MaxStreams = d.MaxStreams
	}
	if o.MaxTags <= 0 {
		o.MaxTags = d.MaxTags
	}
	if o.MaxChapters <= 0 {
		o.MaxChapters = d.MaxChapters
	}
	if o.MaxTextBytes <= 0 {
		o.MaxTextBytes = d.MaxTextBytes
	}
	if o.MaxValueBytes <= 0 {
		o.MaxValueBytes = d.MaxValueBytes
	}
	return o
}

var (
	// ErrUnsupported means no parser confidently recognized the source.
	ErrUnsupported = errors.New("media format unsupported")
	// ErrLimit is used internally when a configured I/O cap is exhausted.
	ErrLimit = errors.New("media probe resource limit reached")
)

// ParseError reports structural damage in an otherwise identified format.
type ParseError struct {
	Format string
	Offset int64
	Err    error
}

func (e *ParseError) Error() string {
	if e.Offset >= 0 {
		return fmt.Sprintf("%s parse error at %d: %v", e.Format, e.Offset, e.Err)
	}
	return fmt.Sprintf("%s parse error: %v", e.Format, e.Err)
}
func (e *ParseError) Unwrap() error { return e.Err }

type boundedReaderAt struct {
	ctx      context.Context
	src      Source
	maxBytes int64
	maxOps   int
	mu       sync.Mutex
	bytes    int64
	ops      int
}

func (r *boundedReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 || off >= r.src.Size {
		if off == r.src.Size && len(p) == 0 {
			return 0, nil
		}
		return 0, io.EOF
	}
	r.mu.Lock()
	if r.ops >= r.maxOps || r.bytes >= r.maxBytes {
		r.mu.Unlock()
		return 0, ErrLimit
	}
	r.ops++
	remaining := r.maxBytes - r.bytes
	limited := false
	if int64(len(p)) > remaining {
		p = p[:remaining]
		limited = true
	}
	r.mu.Unlock()
	if len(p) == 0 {
		return 0, ErrLimit
	}
	n, err := r.src.Reader.ReadAt(r.ctx, p, off)
	r.mu.Lock()
	r.bytes += int64(n)
	r.mu.Unlock()
	if err == nil && n < len(p) {
		err = io.ErrUnexpectedEOF
	}
	if limited && (err == nil || errors.Is(err, io.EOF)) {
		err = ErrLimit
	}
	return n, err
}

type probe struct {
	ctx      context.Context
	src      Source
	opts     Options
	r        *boundedReaderAt
	report   Report
	elements int
}

func newProbe(ctx context.Context, src Source, opts Options) (*probe, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if src.Reader == nil || src.Size < 0 {
		return nil, fmt.Errorf("invalid media source")
	}
	opts = opts.normalized()
	br := &boundedReaderAt{ctx: ctx, src: src, maxBytes: opts.MaxReadBytes, maxOps: opts.MaxReadOps}
	p := &probe{ctx: ctx, src: src, opts: opts, r: br}
	p.report.Mode = opts.Mode
	p.report.General.FileName = src.Name
	p.report.General.FileSize = src.Size
	return p, nil
}

func (p *probe) readAt(off int64, n int) ([]byte, error) {
	if n < 0 || int64(n) > p.opts.MaxSingleMetadataBytes {
		return nil, ErrLimit
	}
	if off < 0 || off > p.src.Size || int64(n) > p.src.Size-off {
		return nil, io.ErrUnexpectedEOF
	}
	b := make([]byte, n)
	read, err := p.r.ReadAt(b, off)
	if err != nil && (err != io.EOF || read != n) {
		return b[:read], err
	}
	if read != n {
		return b[:read], io.ErrUnexpectedEOF
	}
	return b, nil
}

func (p *probe) readBounded(off, n int64) ([]byte, error) {
	if off < 0 || n < 0 || off > p.src.Size || n > p.src.Size-off {
		return nil, io.ErrUnexpectedEOF
	}
	var out bytes.Buffer
	for n > 0 {
		if err := p.ctx.Err(); err != nil {
			return nil, err
		}
		chunk := n
		if chunk > 64<<10 {
			chunk = 64 << 10
		}
		b := make([]byte, int(chunk))
		got, err := p.r.ReadAt(b, off)
		out.Write(b[:got])
		off += int64(got)
		n -= int64(got)
		if err != nil {
			return out.Bytes(), err
		}
		if got == 0 {
			return out.Bytes(), io.ErrNoProgress
		}
	}
	return out.Bytes(), nil
}

func (p *probe) section(off, n int64) (*io.SectionReader, error) {
	if off < 0 || n < 0 || off > p.src.Size || n > p.src.Size-off {
		return nil, io.ErrUnexpectedEOF
	}
	return io.NewSectionReader(p.r, off, n), nil
}

func (p *probe) step() error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	p.elements++
	if p.elements > p.opts.MaxElements {
		return ErrLimit
	}
	return nil
}

func (p *probe) warn(code, message string, off int64) {
	p.report.Warnings = append(p.report.Warnings, Warning{Code: code, Message: message, Offset: off})
}

func (p *probe) addTag(target, name, value string) {
	if name == "" || value == "" {
		return
	}
	if len(p.report.Tags) >= p.opts.MaxTags {
		p.report.Truncated = true
		return
	}
	if len(value) > p.opts.MaxValueBytes {
		value = value[:p.opts.MaxValueBytes]
		p.report.Truncated = true
	}
	const maxTagIdentityBytes = 1024
	if len(name) > maxTagIdentityBytes {
		name = name[:maxTagIdentityBytes]
		p.report.Truncated = true
	}
	if len(target) > maxTagIdentityBytes {
		target = target[:maxTagIdentityBytes]
		p.report.Truncated = true
	}
	// Parsers often derive a short key or value by slicing a much larger
	// metadata string. Own every accepted field so the report cannot retain
	// that backing allocation after the parser returns.
	target = strings.Clone(target)
	name = strings.Clone(name)
	value = strings.Clone(value)
	p.report.Tags = append(p.report.Tags, Field{Target: target, Name: name, Value: value})
}
