package viewer

import (
	"fmt"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"net/url"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// UrlLink is a byte range in the text that contains one externally
// launchable URL. End is exclusive.
type UrlLink struct {
	Start int
	End   int
	URL   string
}

const MaxURLScanBytes = 64 * 1024

// URL-looking text is deliberately limited to web links. Terminal output is
// untrusted input, so accepting arbitrary schemes here would make a click a
// way to invoke custom URI handlers unexpectedly.
var urlLinkPattern = regexp.MustCompile(`(?i)(?:https?://|www\.)[^\s<>"']+`)

func FindURLLinks(text string) []UrlLink {
	matches := urlLinkPattern.FindAllStringIndex(text, -1)
	if len(matches) == 0 {
		return nil
	}

	links := make([]UrlLink, 0, len(matches))
	for _, match := range matches {
		start, end := match[0], match[1]
		candidate := strings.TrimRight(text[start:end], ".,;:!?)]}")
		if candidate == "" {
			continue
		}
		if len(candidate) < end-start {
			end = start + len(candidate)
		}
		launchURL := candidate
		if strings.HasPrefix(strings.ToLower(launchURL), "www.") {
			launchURL = "https://" + launchURL
		}
		if !validExternalURL(launchURL) {
			continue
		}
		links = append(links, UrlLink{Start: start, End: end, URL: launchURL})
	}
	return links
}

func validExternalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return true
	default:
		return false
	}
}

func UrlLinkAt(links []UrlLink, byteOffset int) (UrlLink, bool) {
	for _, link := range links {
		if byteOffset >= link.Start && byteOffset < link.End {
			return link, true
		}
	}
	return UrlLink{}, false
}

// UrlCellRange maps a byte-range link to the cells that display it. The
// offsets slice has one entry per cell and points at that cell's byte offset
// in text. It also works for tabs and wide-character filler cells.
type UrlCellRange struct {
	Start int
	End   int
	URL   string
}

func urlCellRanges(text string, cellByteOffsets []int) []UrlCellRange {
	links := FindURLLinks(text)
	if len(links) == 0 || len(cellByteOffsets) == 0 {
		return nil
	}
	ranges := make([]UrlCellRange, 0, len(links))
	for _, link := range links {
		first, last := -1, -1
		for i, offset := range cellByteOffsets {
			if offset >= link.Start && offset < link.End {
				if first == -1 {
					first = i
				}
				last = i + 1
			}
		}
		if first >= 0 {
			ranges = append(ranges, UrlCellRange{Start: first, End: last, URL: link.URL})
		}
	}
	return ranges
}

func UrlCellRangesFromCells(cells []vtui.CharInfo) []UrlCellRange {
	var text strings.Builder
	offsets := make([]int, 0, len(cells))
	for _, cell := range cells {
		offsets = append(offsets, text.Len())
		text.WriteString(vtui.CellString(cell.Char))
	}
	return urlCellRanges(text.String(), offsets)
}

func ApplyURLHoverAttr(cells []vtui.CharInfo, ranges []UrlCellRange, hoveredURL string) {
	if hoveredURL == "" {
		return
	}
	for _, link := range ranges {
		if link.URL != hoveredURL {
			continue
		}
		start, end := link.Start, link.End
		if start < 0 {
			start = 0
		}
		if end > len(cells) {
			end = len(cells)
		}
		for i := start; i < end; i++ {
			cells[i].Attributes |= vtui.CommonLvbUnderscore
		}
	}
}

func CtrlMouseClick(e *vtinput.InputEvent) bool {
	return e != nil && e.Type == vtinput.MouseEventType &&
		e.ButtonState&vtinput.FromLeft1stButtonPressed != 0 && e.KeyDown &&
		e.MouseEventFlags&vtinput.MouseMoved == 0 &&
		e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
}

// launchExternalURL is a variable so UI tests can verify the full click path
// without starting a real browser.
var launchExternalURL = launchExternalURLDefault

func openExternalURL(raw string) error {
	if !validExternalURL(raw) {
		return fmt.Errorf("unsupported URL")
	}
	return launchExternalURL(raw)
}

func launchExternalURLDefault(raw string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", raw)
	case "darwin":
		cmd = exec.Command("open", raw)
	default:
		cmd = exec.Command("xdg-open", raw)
	}
	return cmd.Start()
}

func OpenExternalURLAsync(raw string) {
	go func() {
		if err := openExternalURL(raw); err != nil {
			toast.Show(fmt.Sprintf("Cannot open URL: %v", err), 3*time.Second)
		}
	}()
}
