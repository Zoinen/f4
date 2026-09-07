package mediainfo

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type parserFunc func(*probe, []byte) error

// Analyze probes src and returns a normalized report. A non-nil partial report
// is returned only for non-fatal resource limits; cancellation always wins.
func Analyze(ctx context.Context, src Source, opts Options) (report Report, err error) {
	p, err := newProbe(ctx, src, opts)
	if err != nil {
		return Report{}, err
	}
	defer func() {
		if v := recover(); v != nil {
			report = Report{}
			err = &ParseError{Format: p.report.General.Format, Offset: -1, Err: fmt.Errorf("parser panic: %v", v)}
		}
	}()

	headLen := int64(64 << 10)
	if src.Size < headLen {
		headLen = src.Size
	}
	head, err := p.readAt(0, int(headLen))
	if err != nil && !errors.Is(err, ErrLimit) {
		return Report{}, err
	}
	parser := chooseParser(src.Name, head)
	if parser == nil {
		return Report{}, ErrUnsupported
	}
	if err := parser(p, head); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Report{}, err
		}
		if errors.Is(err, ErrLimit) {
			p.report.Truncated = true
			p.warn("limit", "metadata scan stopped at the configured resource limit", -1)
			return p.report, nil
		}
		return Report{}, err
	}
	// Some optional metadata walkers intentionally stop on a failed read and
	// retain the already decoded primary stream. Cancellation must never be
	// mistaken for that best-effort behavior.
	if err := p.ctx.Err(); err != nil {
		return Report{}, err
	}
	if p.report.General.Duration > 0 && p.report.General.FileSize > 0 && p.report.General.OverallBitRate == 0 {
		p.report.General.OverallBitRate = int64(float64(p.report.General.FileSize*8) / p.report.General.Duration.Seconds())
		p.report.General.OverallBitRateEstimated = true
	}
	return p.report, nil
}

func chooseParser(name string, h []byte) parserFunc {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".stl" {
		return parseEBUSTL
	}
	if len(h) >= 12 {
		if bytes.Equal(h[:4], []byte("RIFF")) && bytes.Equal(h[8:12], []byte("WEBP")) {
			return parseImage
		}
		if bytes.Equal(h[:4], []byte("RIFF")) || bytes.Equal(h[:4], []byte("RIFX")) || bytes.Equal(h[:4], []byte("RF64")) || bytes.Equal(h[:4], []byte("BW64")) {
			return parseRIFF
		}
		if bytes.Equal(h[:4], []byte("FORM")) && (bytes.Equal(h[8:12], []byte("AIFF")) || bytes.Equal(h[8:12], []byte("AIFC"))) {
			return parseAIFF
		}
		if bytes.Equal(h[4:8], []byte("ftyp")) && isHEIFSource(name, h) {
			return parseHEIF
		}
		if bytes.Equal(h[4:8], []byte("ftyp")) || bytes.Equal(h[4:8], []byte("styp")) || bytes.Equal(h[4:8], []byte("moov")) || bytes.Equal(h[4:8], []byte("moof")) {
			return parseISOBaseMedia
		}
	}
	if len(h) >= 4 {
		switch {
		case bytes.Equal(h[:4], []byte{0x1a, 0x45, 0xdf, 0xa3}):
			return parseMatroska
		case bytes.Equal(h[:4], []byte("fLaC")):
			return parseFLAC
		case bytes.Equal(h[:4], []byte("OggS")):
			return parseOgg
		}
	}
	if isTIFFMagic(h) {
		return parseTIFFImage
	}
	if isImageMagic(h) {
		return parseImage
	}
	if looksLikeMPEGAudio(h) || (len(h) >= 3 && bytes.Equal(h[:3], []byte("ID3"))) {
		return parseMPEGAudio
	}
	if isSubtitleExtension(ext) && looksLikeText(h) {
		return parseSubtitle
	}
	return nil
}
