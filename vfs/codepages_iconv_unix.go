//go:build !windows
// +build !windows

package vfs

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/encoding"
	"golang.org/x/text/transform"
)

var iconvCommand struct {
	sync.Once
	path string
}

func iconvPath() string {
	iconvCommand.Do(func() {
		iconvCommand.path, _ = exec.LookPath("iconv")
	})
	return iconvCommand.path
}

func discoverIconvCodepages() []Codepage {
	path := iconvPath()
	if path == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--list")
	output, err := cmd.Output()
	if err != nil {
		cmd = exec.CommandContext(ctx, path, "-l")
		output, err = cmd.Output()
	}
	if err != nil {
		return nil
	}

	canonical := make(map[string]struct{})
	for _, line := range strings.Split(string(output), "\n") {
		name := strings.TrimSpace(strings.TrimSuffix(line, "//"))
		if name = canonicalIconvName(name); name != "" {
			canonical[name] = struct{}{}
		}
	}

	result := make([]Codepage, 0, len(canonical))
	for name := range canonical {
		id := iconvCodepageID(name)
		if id >= 0 {
			if _, exists := FindCodepage(id); exists {
				continue
			}
		} else if id == CodepageAutoDetect {
			continue
		}
		result = append(result, Codepage{
			ID:    id,
			Name:  name + " (iconv)",
			Enc:   iconvEncoding{name: name},
			group: codepageIconv,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ID >= 0 && result[j].ID >= 0 {
			return result[i].ID < result[j].ID
		}
		if result[i].ID >= 0 {
			return true
		}
		if result[j].ID >= 0 {
			return false
		}
		return result[i].Name < result[j].Name
	})
	return result
}

func canonicalIconvName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t") {
		return ""
	}
	upper := strings.ToUpper(name)

	if digits, ok := iconvDigits(upper); ok {
		return "CP" + digits
	}
	if strings.HasPrefix(upper, "CP") {
		if digits, ok := iconvDigits(strings.TrimPrefix(upper, "CP")); ok {
			return "CP" + digits
		}
	}
	if strings.HasPrefix(upper, "WINDOWS-") {
		if digits, ok := iconvDigits(strings.TrimPrefix(upper, "WINDOWS-")); ok {
			return "CP" + digits
		}
	}
	if strings.HasPrefix(upper, "ISO-8859-") {
		if digits, ok := iconvDigits(strings.TrimPrefix(upper, "ISO-8859-")); ok {
			return "ISO-8859-" + digits
		}
	}
	if strings.HasPrefix(upper, "ISO_8859-") {
		if digits, ok := iconvDigits(strings.TrimPrefix(upper, "ISO_8859-")); ok {
			return "ISO-8859-" + digits
		}
	}
	if strings.HasPrefix(upper, "8859_") {
		if digits, ok := iconvDigits(strings.TrimPrefix(upper, "8859_")); ok {
			return "ISO-8859-" + digits
		}
	}

	switch upper {
	case "ASCII", "US-ASCII", "ANSI_X3.4-1968":
		return "US-ASCII"
	case "UTF8", "UTF-8":
		return "UTF-8"
	case "UTF7", "UTF-7":
		return "UTF-7"
	case "UTF16", "UTF-16":
		return "UTF-16"
	case "UTF16LE", "UTF-16LE":
		return "UTF-16LE"
	case "UTF16BE", "UTF-16BE":
		return "UTF-16BE"
	case "UTF32", "UTF-32":
		return "UTF-32"
	case "UTF32LE", "UTF-32LE":
		return "UTF-32LE"
	case "UTF32BE", "UTF-32BE":
		return "UTF-32BE"
	case "BIG-5", "BIG-FIVE", "BIG5":
		return "BIG5"
	case "BIG5-HKSCS", "BIG5HKSCS":
		return "BIG5-HKSCS"
	case "EUC-CN", "EUCCN", "GB2312":
		return "GB2312"
	case "EUC-JP", "EUCJP":
		return "EUC-JP"
	case "EUC-KR", "EUCKR":
		return "EUC-KR"
	case "EUC-TW", "EUCTW":
		return "EUC-TW"
	case "GB18030":
		return "GB18030"
	case "GBK":
		return "GBK"
	case "KOI8-R", "KOI8R":
		return "KOI8-R"
	case "KOI8-U", "KOI8U":
		return "KOI8-U"
	case "KOI8-T":
		return "KOI8-T"
	case "SHIFT-JIS", "SHIFTJIS", "SHIFT_JIS", "WINDOWS-31J":
		return "SHIFT-JIS"
	case "ISO-2022-JP", "ISO2022JP":
		return "ISO-2022-JP"
	case "TIS-620", "TIS620":
		return "TIS-620"
	case "MACINTOSH", "MAC":
		return "MACINTOSH"
	case "MAC-CYRILLIC", "MACCYRILLIC":
		return "MAC-CYRILLIC"
	case "VISCII":
		return "VISCII"
	case "ARMSCII-8", "ARMSCII8":
		return "ARMSCII-8"
	case "JOHAB":
		return "JOHAB"
	}

	// iconv exposes a much wider set than the x/text packages. Keep the
	// registered names that are not obvious aliases, so a platform can offer
	// its own regional and historical encodings without a hardcoded table.
	if strings.HasPrefix(upper, "CS") || strings.HasPrefix(upper, "IBM") || strings.Contains(upper, ":") {
		return ""
	}
	for _, suffix := range []string{"V1", "-1986", "-1987", "-1988", "-1989", "-1990", "-1991", "-1992", "-1993", "-1994", "-1995", "-2000", "-2001"} {
		if strings.HasSuffix(upper, suffix) {
			return ""
		}
	}
	return upper
}

func iconvDigits(value string) (string, bool) {
	if value == "" {
		return "", false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return "", false
		}
	}
	return value, true
}

func iconvCodepageID(name string) int {
	if strings.HasPrefix(name, "CP") {
		id, err := strconv.Atoi(strings.TrimPrefix(name, "CP"))
		if err == nil {
			return id
		}
	}

	switch name {
	case "US-ASCII":
		return 20127
	case "UTF-8":
		return 65001
	case "UTF-16", "UTF-16LE":
		return 1200
	case "UTF-16BE":
		return 1201
	case "UTF-32LE":
		return 12000
	case "UTF-32BE":
		return 12001
	case "UTF-32":
		return 12000
	case "BIG5":
		return 950
	case "EUC-JP":
		return 51932
	case "EUC-KR":
		return 51949
	case "GB18030":
		return 54936
	case "GBK":
		return 936
	case "KOI8-R":
		return 20866
	case "KOI8-U":
		return 21866
	case "SHIFT-JIS":
		return 932
	case "ISO-2022-JP":
		return 50220
	case "ISO-8859-1", "ISO-8859-2", "ISO-8859-3", "ISO-8859-4", "ISO-8859-5", "ISO-8859-6", "ISO-8859-7", "ISO-8859-8", "ISO-8859-9", "ISO-8859-10", "ISO-8859-13", "ISO-8859-14", "ISO-8859-15", "ISO-8859-16":
		part := strings.TrimPrefix(name, "ISO-8859-")
		partID, err := strconv.Atoi(part)
		if err == nil {
			if partID <= 10 {
				return 28590 + partID
			}
			return 28590 + partID
		}
	}

	// Keep iconv-only names stable across runs while leaving Far's negative
	// virtual values and the old compatibility values untouched.
	var hash uint32 = 2166136261
	for i := 0; i < len(name); i++ {
		hash ^= uint32(name[i])
		hash *= 16777619
	}
	return -100000 - int(hash%900000000)
}

type iconvEncoding struct {
	name string
}

func (e iconvEncoding) NewDecoder() *encoding.Decoder {
	return &encoding.Decoder{Transformer: iconvTransformer{from: e.name, to: "UTF-8"}}
}

func (e iconvEncoding) NewEncoder() *encoding.Encoder {
	return &encoding.Encoder{Transformer: iconvTransformer{from: "UTF-8", to: e.name}}
}

type iconvTransformer struct {
	from string
	to   string
}

func (iconvTransformer) Reset() {}

func (t iconvTransformer) Transform(dst, src []byte, atEOF bool) (int, int, error) {
	if len(src) == 0 {
		return 0, 0, nil
	}
	out, err := runIconv(t.from, t.to, src)
	if err != nil {
		if !atEOF {
			return 0, 0, transform.ErrShortSrc
		}
		return 0, 0, err
	}
	if len(out) > len(dst) {
		return 0, 0, transform.ErrShortDst
	}
	return copy(dst, out), len(src), nil
}

func runIconv(from, to string, input []byte) ([]byte, error) {
	path := iconvPath()
	if path == "" {
		return nil, fmt.Errorf("iconv is not installed")
	}
	cmd := exec.CommandContext(context.Background(), path, "-f", from, "-t", to)
	cmd.Stdin = bytes.NewReader(input)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("iconv %s -> %s: %s", from, to, message)
		}
		return nil, fmt.Errorf("iconv %s -> %s: %w", from, to, err)
	}
	return out, nil
}
