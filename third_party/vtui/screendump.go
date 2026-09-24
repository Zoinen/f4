package vtui

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ScreenDumpVersion identifies the text format emitted by ScreenBuf.Dump.
const ScreenDumpVersion = "VTUI_SCREEN_DUMP_V1"

const (
	screenDumpTextMarker = "--- TEXT PREVIEW ---"
	screenDumpCellMarker = "--- CELL METADATA (RLE) ---"
	screenDumpFormatLine = "Format: [AttrHex]xRepeatCount ..."
)

// ScreenDump is the decoded logical screen bitmap produced by ScreenBuf.Dump.
//
// Text is intentionally kept as the human-readable row emitted by the dump,
// rather than split into runes: a terminal cell may contain a grapheme cluster
// and a wide character occupies a second filler cell that has no text of its
// own. Attributes, on the other hand, are expanded to exactly one value per
// screen cell, so callers can align the colour map with the declared width.
type ScreenDump struct {
	Width  int
	Height int
	Rows   []ScreenDumpRow
}

// ScreenDumpRow contains one text-preview row and its expanded attribute map.
type ScreenDumpRow struct {
	Text       string
	Attributes []uint64
}

// DecodeScreenDump decodes the VTUI_SCREEN_DUMP_V1 format written by
// ScreenBuf.Dump. It validates the dimensions, row numbering, RLE syntax and
// the invariant that every attribute row expands to exactly Width cells.
func DecodeScreenDump(r io.Reader) (*ScreenDump, error) {
	if r == nil {
		return nil, fmt.Errorf("screen dump: nil reader")
	}
	reader := bufio.NewReader(r)

	header, err := readScreenDumpLine(reader)
	if err != nil {
		return nil, fmt.Errorf("screen dump header: %w", err)
	}
	fields := strings.Fields(header)
	if len(fields) != 2 || fields[0] != ScreenDumpVersion {
		return nil, fmt.Errorf("screen dump header: want %s <width>x<height>, got %q", ScreenDumpVersion, header)
	}
	width, height, err := parseScreenDumpDimensions(fields[1])
	if err != nil {
		return nil, fmt.Errorf("screen dump header: %w", err)
	}

	if err := expectScreenDumpLine(reader, screenDumpTextMarker); err != nil {
		return nil, err
	}
	textRows := make([]string, height)
	for y := range textRows {
		line, err := readScreenDumpLine(reader)
		if err != nil {
			return nil, fmt.Errorf("screen dump text row %d: %w", y, err)
		}
		if !utf8.ValidString(line) {
			return nil, fmt.Errorf("screen dump text row %d: invalid UTF-8", y)
		}
		textRows[y] = line
	}

	if err := expectScreenDumpLine(reader, screenDumpCellMarker); err != nil {
		return nil, err
	}
	if err := expectScreenDumpLine(reader, screenDumpFormatLine); err != nil {
		return nil, err
	}

	dump := &ScreenDump{
		Width:  width,
		Height: height,
		Rows:   make([]ScreenDumpRow, height),
	}
	for y := 0; y < height; y++ {
		line, err := readScreenDumpLine(reader)
		if err != nil {
			return nil, fmt.Errorf("screen dump attribute row %d: %w", y, err)
		}
		attributes, err := decodeScreenDumpAttributes(line, y, width)
		if err != nil {
			return nil, err
		}
		dump.Rows[y] = ScreenDumpRow{
			Text:       textRows[y],
			Attributes: attributes,
		}
	}

	for {
		line, err := readScreenDumpLine(reader)
		if err == io.EOF {
			return dump, nil
		}
		if err != nil {
			return nil, fmt.Errorf("screen dump trailing data: %w", err)
		}
		if strings.TrimSpace(line) != "" {
			return nil, fmt.Errorf("screen dump trailing data: unexpected line %q", line)
		}
	}
}

func readScreenDumpLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	if err == io.EOF && line == "" {
		return "", io.EOF
	}
	line = strings.TrimSuffix(line, "\n")
	line = strings.TrimSuffix(line, "\r")
	return line, nil
}

func expectScreenDumpLine(r *bufio.Reader, want string) error {
	got, err := readScreenDumpLine(r)
	if err != nil {
		return fmt.Errorf("screen dump: expected %q: %w", want, err)
	}
	if got != want {
		return fmt.Errorf("screen dump: expected %q, got %q", want, got)
	}
	return nil
}

func parseScreenDumpDimensions(spec string) (int, int, error) {
	parts := strings.Split(spec, "x")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid dimensions %q", spec)
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil || width < 0 {
		return 0, 0, fmt.Errorf("invalid width %q", parts[0])
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil || height < 0 {
		return 0, 0, fmt.Errorf("invalid height %q", parts[1])
	}
	return width, height, nil
}

func decodeScreenDumpAttributes(line string, row, width int) ([]uint64, error) {
	prefix := fmt.Sprintf("R%d: ", row)
	if !strings.HasPrefix(line, prefix) {
		return nil, fmt.Errorf("screen dump attribute row %d: expected prefix %q, got %q", row, prefix, line)
	}
	payload := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if payload == "" {
		if width == 0 {
			return []uint64{}, nil
		}
		return nil, fmt.Errorf("screen dump attribute row %d: empty RLE data", row)
	}

	attributes := make([]uint64, 0, width)
	for _, token := range strings.Fields(payload) {
		if len(token) < 20 || token[0] != '[' {
			return nil, fmt.Errorf("screen dump attribute row %d: malformed token %q", row, token)
		}
		closeBracket := strings.Index(token, "]x")
		if closeBracket != 17 || len(token) == closeBracket+2 {
			return nil, fmt.Errorf("screen dump attribute row %d: malformed token %q", row, token)
		}
		attribute, err := strconv.ParseUint(token[1:closeBracket], 16, 64)
		if err != nil {
			return nil, fmt.Errorf("screen dump attribute row %d: invalid attribute in %q", row, token)
		}
		repeat, err := strconv.Atoi(token[closeBracket+2:])
		if err != nil || repeat < 0 {
			return nil, fmt.Errorf("screen dump attribute row %d: invalid repeat count in %q", row, token)
		}
		if width > 0 && repeat == 0 {
			return nil, fmt.Errorf("screen dump attribute row %d: zero repeat count in %q", row, token)
		}
		if repeat > width-len(attributes) {
			return nil, fmt.Errorf("screen dump attribute row %d: RLE expands beyond width %d", row, width)
		}
		for i := 0; i < repeat; i++ {
			attributes = append(attributes, attribute)
		}
	}
	if len(attributes) != width {
		return nil, fmt.Errorf("screen dump attribute row %d: RLE expands to %d cells, want %d", row, len(attributes), width)
	}
	return attributes, nil
}
