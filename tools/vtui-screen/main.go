// Command vtui-screen decodes a vtui.ScreenBuf.Dump file into a compact PPM
// bitmap. The bitmap uses one small tile per terminal cell: empty cells keep
// their background colour, while occupied cells get a small foreground mark.
// The exact glyphs remain in the text preview, so this output is deliberately
// a layout and colour map rather than a second font renderer.
package main

import (
	"bufio"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"
)

const (
	dumpHeader     = "VTUI_SCREEN_DUMP_V1 "
	textHeader     = "--- TEXT PREVIEW ---"
	metadataHeader = "--- CELL METADATA (RLE) ---"
	metadataFormat = "Format: [AttrHex]xRepeatCount ..."
	maxDimension   = 10000
	maxCells       = 50_000_000

	isFgRGB          uint64 = 0x0100
	isBgRGB          uint64 = 0x0200
	foregroundDim    uint64 = 0x1000
	commonLvbReverse uint64 = 0x4000
)

// ScreenDump is the decoded, cell-oriented representation of a vtui dump.
// Text is kept as raw rows because a combining sequence can occupy more than
// one Unicode code point while still belonging to a single terminal cell.
type ScreenDump struct {
	Width  int
	Height int
	Text   []string
	Attrs  [][]uint64
}

// Parse reads the V1 dump format emitted by vtui.ScreenBuf.Dump.
func Parse(r io.Reader) (ScreenDump, error) {
	var dump ScreenDump
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 16*1024*1024)

	next := func(what string) (string, error) {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return "", fmt.Errorf("read %s: %w", what, err)
			}
			return "", fmt.Errorf("missing %s", what)
		}
		return scanner.Text(), nil
	}

	line, err := next("header")
	if err != nil {
		return dump, err
	}
	if !strings.HasPrefix(line, dumpHeader) {
		return dump, fmt.Errorf("invalid header %q", line)
	}
	dimensions := strings.Split(strings.TrimPrefix(line, dumpHeader), "x")
	if len(dimensions) != 2 {
		return dump, fmt.Errorf("invalid dimensions %q", strings.TrimPrefix(line, dumpHeader))
	}
	if dump.Width, err = positiveInt(dimensions[0], "width"); err != nil {
		return dump, err
	}
	if dump.Height, err = positiveInt(dimensions[1], "height"); err != nil {
		return dump, err
	}
	if dump.Width > maxDimension || dump.Height > maxDimension || dump.Width > maxCells/dump.Height {
		return dump, fmt.Errorf("screen dimensions %dx%d are too large", dump.Width, dump.Height)
	}

	if line, err = next("text preview header"); err != nil {
		return dump, err
	}
	if line != textHeader {
		return dump, fmt.Errorf("expected %q, got %q", textHeader, line)
	}
	dump.Text = make([]string, dump.Height)
	for y := range dump.Text {
		if dump.Text[y], err = next(fmt.Sprintf("text row %d", y)); err != nil {
			return dump, err
		}
	}

	if line, err = next("metadata header"); err != nil {
		return dump, err
	}
	if line != metadataHeader {
		return dump, fmt.Errorf("expected %q, got %q", metadataHeader, line)
	}
	if line, err = next("metadata format"); err != nil {
		return dump, err
	}
	if line != metadataFormat {
		return dump, fmt.Errorf("expected %q, got %q", metadataFormat, line)
	}

	dump.Attrs = make([][]uint64, dump.Height)
	for y := range dump.Attrs {
		line, err = next(fmt.Sprintf("metadata row %d", y))
		if err != nil {
			return dump, err
		}
		prefix := fmt.Sprintf("R%d:", y)
		if !strings.HasPrefix(line, prefix) {
			return dump, fmt.Errorf("metadata row %d has invalid prefix %q", y, line)
		}
		fields := strings.Fields(strings.TrimSpace(strings.TrimPrefix(line, prefix)))
		if len(fields) == 0 {
			return dump, fmt.Errorf("metadata row %d has no RLE runs", y)
		}
		row := make([]uint64, 0, dump.Width)
		for _, field := range fields {
			attr, repeat, parseErr := parseRun(field)
			if parseErr != nil {
				return dump, fmt.Errorf("metadata row %d: %w", y, parseErr)
			}
			if repeat > dump.Width-len(row) {
				return dump, fmt.Errorf("metadata row %d expands past width %d", y, dump.Width)
			}
			for i := 0; i < repeat; i++ {
				row = append(row, attr)
			}
		}
		if len(row) != dump.Width {
			return dump, fmt.Errorf("metadata row %d expands to %d cells, want %d", y, len(row), dump.Width)
		}
		dump.Attrs[y] = row
	}

	return dump, nil
}

func positiveInt(value, name string) (int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid %s %q", name, value)
	}
	return n, nil
}

func parseRun(value string) (uint64, int, error) {
	if !strings.HasPrefix(value, "[") {
		return 0, 0, fmt.Errorf("invalid RLE run %q", value)
	}
	separator := strings.Index(value, "]x")
	if separator <= 1 || separator+2 >= len(value) {
		return 0, 0, fmt.Errorf("invalid RLE run %q", value)
	}
	attr, err := strconv.ParseUint(value[1:separator], 16, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid attribute in %q", value)
	}
	repeat, err := strconv.Atoi(value[separator+2:])
	if err != nil || repeat <= 0 {
		return 0, 0, fmt.Errorf("invalid repeat count in %q", value)
	}
	return attr, repeat, nil
}

// WritePPM writes a P6 bitmap with a small tile for each terminal cell.
// cellWidth and cellHeight are deliberately explicit so callers can choose a
// one-pixel cell map for compact machine processing or a larger preview.
func WritePPM(w io.Writer, dump ScreenDump, cellWidth, cellHeight int) error {
	if dump.Width <= 0 || dump.Height <= 0 || len(dump.Text) != dump.Height || len(dump.Attrs) != dump.Height {
		return errors.New("invalid decoded screen dimensions")
	}
	if cellWidth <= 0 || cellHeight <= 0 {
		return errors.New("cell dimensions must be positive")
	}
	if dump.Width > int(^uint(0)>>1)/cellWidth || dump.Height > int(^uint(0)>>1)/cellHeight {
		return errors.New("bitmap dimensions overflow the host integer size")
	}

	width := dump.Width * cellWidth
	height := dump.Height * cellHeight
	writer := bufio.NewWriter(w)
	if _, err := fmt.Fprintf(writer, "P6\n%d %d\n255\n", width, height); err != nil {
		return err
	}
	var pixel [8]byte
	for y := 0; y < dump.Height; y++ {
		if len(dump.Attrs[y]) != dump.Width {
			return fmt.Errorf("row %d has %d attributes, want %d", y, len(dump.Attrs[y]), dump.Width)
		}
		for py := 0; py < cellHeight; py++ {
			for x := 0; x < dump.Width; x++ {
				fg, bg := attrColors(dump.Attrs[y][x])
				ink := cellHasInk(dump.Text[y], x)
				for px := 0; px < cellWidth; px++ {
					color := bg
					if ink && isInkPixel(px, py, cellWidth, cellHeight) {
						color = fg
					}
					binary.BigEndian.PutUint64(pixel[:], color)
					if _, err := writer.Write(pixel[5:]); err != nil {
						return err
					}
				}
			}
		}
	}
	return writer.Flush()
}

func isInkPixel(x, y, width, height int) bool {
	if width < 3 || height < 3 {
		return true
	}
	return x >= width/3 && x < width-width/3 && y >= height/4 && y < height-height/4
}

func cellHasInk(row string, cell int) bool {
	runes := []rune(row)
	if cell < 0 || cell >= len(runes) {
		return false
	}
	return runes[cell] != 0 && !unicode.IsSpace(runes[cell])
}

func attrColors(attr uint64) (fg, bg uint64) {
	if attr&isFgRGB != 0 {
		fg = (attr >> 16) & 0xFFFFFF
	} else {
		fg = xtermColor((attr >> 16) & 0xFF)
	}
	if attr&isBgRGB != 0 {
		bg = (attr >> 40) & 0xFFFFFF
	} else {
		bg = xtermColor((attr >> 40) & 0xFF)
	}
	if attr&foregroundDim != 0 {
		if attr&isFgRGB != 0 {
			fg = (fg >> 1) & 0x7F7F7F
		} else {
			fg = xtermColor(8)
		}
	}
	if attr&commonLvbReverse != 0 {
		fg, bg = bg, fg
	}
	return fg, bg
}

func xtermColor(index uint64) uint64 {
	if index < 16 {
		return [16]uint64{
			0x000000, 0x800000, 0x008000, 0x808000,
			0x000080, 0x800080, 0x008080, 0xC0C0C0,
			0x808080, 0xFF0000, 0x00FF00, 0xFFFF00,
			0x0000FF, 0xFF00FF, 0x00FFFF, 0xFFFFFF,
		}[index]
	}
	if index < 232 {
		value := index - 16
		red := value / 36
		green := (value / 6) % 6
		blue := value % 6
		component := func(value uint64) uint64 {
			if value == 0 {
				return 0
			}
			return 55 + value*40
		}
		return component(red)<<16 | component(green)<<8 | component(blue)
	}
	gray := 8 + (index-232)*10
	return gray<<16 | gray<<8 | gray
}

func openInput(path string) (io.ReadCloser, error) {
	if path == "-" {
		return io.NopCloser(os.Stdin), nil
	}
	return os.Open(path)
}

func openOutput(path string) (io.WriteCloser, error) {
	if path == "-" {
		return nopWriteCloser{Writer: os.Stdout}, nil
	}
	return os.Create(path)
}

type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

func run(args []string, stderr io.Writer) (err error) {
	flags := flag.NewFlagSet("vtui-screen", flag.ContinueOnError)
	flags.SetOutput(stderr)
	cellWidth := flags.Int("cell-width", 4, "bitmap pixels per terminal cell horizontally")
	cellHeight := flags.Int("cell-height", 8, "bitmap pixels per terminal cell vertically")
	if err := flags.Parse(args); err != nil {
		return err
	}
	paths := flags.Args()
	if len(paths) < 1 || len(paths) > 2 {
		return errors.New("usage: vtui-screen [flags] input.dump [output.ppm]")
	}
	outputPath := "-"
	if len(paths) == 2 {
		outputPath = paths[1]
	}
	in, err := openInput(paths[0])
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := in.Close(); err == nil {
			err = closeErr
		}
	}()
	dump, err := Parse(in)
	if err != nil {
		return err
	}
	out, err := openOutput(outputPath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := out.Close(); err == nil {
			err = closeErr
		}
	}()
	return WritePPM(out, dump, *cellWidth, *cellHeight)
}

func main() {
	if err := run(os.Args[1:], os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "vtui-screen:", err)
		os.Exit(1)
	}
}
