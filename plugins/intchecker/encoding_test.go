package intchecker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"golang.org/x/text/encoding/unicode"
)

// cyrillicNames avoid letters with a decomposed form (й, ё), which a macOS
// file system may hand back in another Unicode normalization.
var cyrillicNames = []string{"a.txt", "данные.txt", "Фото на море.txt", "sub/отчет за квартал.txt"}

func TestEncodingChoices(t *testing.T) {
	write := writeEncodingChoices()
	if len(write) < 2 || write[0].encoding != (fileEncoding{Codepage: utf8Codepage}) || write[0].label != "UTF-8" ||
		write[1].encoding != (fileEncoding{Codepage: utf8Codepage, BOM: true}) {
		t.Fatalf("write choices = %+v", write)
	}
	read := readEncodingChoices()
	if len(read) < 2 || read[0].encoding != autoDetectEncoding || read[1].encoding.codepage() != utf8Codepage {
		t.Fatalf("read choices = %+v", read)
	}
	for _, list := range [][]encodingChoice{write, read} {
		seen := map[fileEncoding]bool{}
		for _, c := range list {
			if seen[c.encoding] || c.label == "" {
				t.Fatalf("duplicate or unnamed choice %+v in %+v", c, list)
			}
			seen[c.encoding] = true
		}
	}
	for _, c := range systemEncodingChoices() {
		if c.encoding.isUTF8() {
			t.Fatalf("system choice %+v repeats UTF-8", c)
		}
		if _, ok := vfs.FindCodepage(c.encoding.Codepage); !ok {
			t.Fatalf("system choice %+v is not a known codepage", c)
		}
	}
}

func TestEncodingComboKeepsTheChoice(t *testing.T) {
	initValidateTestScreen(t)
	choices := []encodingChoice{
		{label: "UTF-8", encoding: fileEncoding{Codepage: utf8Codepage}},
		{label: "1251", encoding: fileEncoding{Codepage: 1251}},
	}
	combo := newEncodingCombo(20, choices, fileEncoding{Codepage: 1251})
	if combo.selected() != choices[1].encoding || combo.box.Edit.GetText() != "1251" {
		t.Fatalf("preselected %+v %q", combo.selected(), combo.box.Edit.GetText())
	}
	combo = newEncodingCombo(20, choices, fileEncoding{Codepage: 866})
	if combo.selected() != choices[0].encoding {
		t.Fatalf("unknown current: %+v", combo.selected())
	}
}

func TestFileEncodingEncode(t *testing.T) {
	if got, _ := (fileEncoding{}).encode("x"); string(got) != "x" {
		t.Fatalf("zero value = %q, want plain UTF-8", got)
	}
	if got, _ := (fileEncoding{Codepage: utf8Codepage, BOM: true}).encode("x"); string(got) != "\xef\xbb\xbfx" {
		t.Fatalf("UTF-8 with BOM = %q", got)
	}
	got, err := (fileEncoding{Codepage: 1251}).encode("данные")
	if err != nil || !bytes.Equal(got, []byte{0xe4, 0xe0, 0xed, 0xed, 0xfb, 0xe5}) {
		t.Fatalf("1251 = % x, %v", got, err)
	}
	if _, err := (fileEncoding{Codepage: 1251}).encode("日本"); !errors.Is(err, errUnencodableName) {
		t.Fatalf("unencodable: %v", err)
	}
	if (fileEncoding{Codepage: 866}).canStore("日本.txt") || !(fileEncoding{Codepage: 866}).canStore("данные.txt") {
		t.Fatal("canStore disagrees with CP866")
	}
}

// TestGeneratedInEncodingValidates is the step's end-to-end check: generate a
// checksum file with non-ASCII names in an encoding, make sure the bytes on
// disk really are in that encoding, then validate it read in the same
// encoding (and, where it can tell, auto-detected).
func TestGeneratedInEncodingValidates(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, cyrillicNames...)
	fs := vfs.NewOSVFS(dir)
	for _, tc := range []struct {
		label     string
		enc       fileEncoding
		algorithm Algorithm
		autoCP    int // what detection must find; 0: not checked
	}{
		{"utf-8", fileEncoding{Codepage: utf8Codepage}, AlgMD5, utf8Codepage},
		{"utf-8 bom", fileEncoding{Codepage: utf8Codepage, BOM: true}, AlgSHA256, utf8Codepage},
		{"windows-1251", fileEncoding{Codepage: 1251}, AlgSHA1, 0},
		{"cp866", fileEncoding{Codepage: 866}, AlgCRC32, 0},
		{"koi8-r", fileEncoding{Codepage: 20866}, AlgSHA512, 0},
	} {
		output := "list" + tc.algorithm.Extension()
		job := generateJob{fs: fs, dir: dir, names: []string{"a.txt", "данные.txt", "Фото на море.txt", "sub"},
			algorithm: tc.algorithm, mode: outputSingle, output: output, recursive: true, overwrite: true,
			mask: "*.txt", encoding: tc.enc}
		res, err := runGenerate(context.Background(), job, &recordingReporter{})
		if err != nil || len(res.Unencodable) != 0 || res.Written != len(cyrillicNames) {
			t.Fatalf("%s: generate = %+v, %v", tc.label, res, err)
		}
		data, err := os.ReadFile(filepath.Join(dir, output))
		if err != nil {
			t.Fatal(err)
		}
		hasBOM := bytes.HasPrefix(data, utf8BOM)
		if hasBOM != tc.enc.BOM {
			t.Fatalf("%s: BOM = %v\n% x", tc.label, hasBOM, data)
		}
		if tc.enc.isUTF8() != utf8.Valid(data) || tc.enc.isUTF8() != bytes.Contains(data, []byte("данные.txt")) {
			t.Fatalf("%s: the file is not in the chosen encoding:\n% x", tc.label, data)
		}

		encodings := []fileEncoding{tc.enc}
		if tc.autoCP != 0 {
			encodings = append(encodings, autoDetectEncoding)
		}
		for _, readAs := range encodings {
			text, cp, err := decodeChecksumFile(data, readAs.Codepage)
			if err != nil {
				t.Fatalf("%s: decode as %d: %v", tc.label, readAs.Codepage, err)
			}
			if readAs == autoDetectEncoding && cp != tc.autoCP {
				t.Fatalf("%s: detected %d, want %d", tc.label, cp, tc.autoCP)
			}
			file, err := ParseHashFile(output, text)
			if err != nil || file.Malformed != 0 || file.Algorithm != tc.algorithm {
				t.Fatalf("%s: parse = %+v, %v\n%s", tc.label, file, err, text)
			}
			var names []string
			for _, e := range file.Entries {
				names = append(names, e.Name)
			}
			want := append([]string(nil), cyrillicNames...)
			sort.Strings(want)
			if strings.Join(names, "|") != strings.Join(want, "|") {
				t.Fatalf("%s: names = %q, want %q", tc.label, names, want)
			}
			vres, err := runValidate(context.Background(), validateJob{fs: fs, dir: dir, file: file}, &recordingReporter{})
			if err != nil || vres.Counts[statusOK] != len(cyrillicNames) {
				t.Fatalf("%s: validate = %+v, %v", tc.label, vres, err)
			}
		}
	}
}

// TestLegacyChecksumFileIsNotReadAsUTF8 makes sure auto-detection does not
// take a Windows-1251 file for UTF-8: its Cyrillic bytes are not valid UTF-8,
// so they must go through the legacy codepage detection.
func TestLegacyChecksumFileIsNotReadAsUTF8(t *testing.T) {
	if vfs.NormalizeCodepageID(vfs.SystemANSICodepage()) == utf8Codepage {
		t.Skip("the system ANSI codepage is UTF-8 here, so it is the fallback")
	}
	data, err := (fileEncoding{Codepage: 1251}).encode(abcDigests[AlgMD5] + " *Фото на море.txt\n")
	if err != nil {
		t.Fatal(err)
	}
	text, cp, err := decodeChecksumFile(data, vfs.CodepageAutoDetect)
	if err != nil || cp == utf8Codepage || !utf8.Valid(text) {
		t.Fatalf("detected %d, %v: %q", cp, err, text)
	}
}

func TestDecodeChecksumFileByteOrderMarkWins(t *testing.T) {
	line := abcDigests[AlgMD5] + " *данные.txt\n"
	text, cp, err := decodeChecksumFile(append(append([]byte{}, utf8BOM...), line...), 1251)
	if err != nil || cp != utf8Codepage || strings.TrimPrefix(string(text), "\ufeff") != line {
		t.Fatalf("UTF-8 BOM read as 1251: %d %v %q", cp, err, text)
	}
	utf16, err := unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes([]byte(line))
	if err != nil {
		t.Fatal(err)
	}
	text, cp, err = decodeChecksumFile(utf16, utf8Codepage)
	if err != nil || cp != 1200 || string(text) != line {
		t.Fatalf("UTF-16 LE: %d %v %q", cp, err, text)
	}
	if text, cp, _ = decodeChecksumFile([]byte("\xff name"), 0); cp != utf8Codepage || string(text) != "\xff name" {
		t.Fatalf("UTF-8 is not passed through: %d %q", cp, text)
	}
}

func TestStartValidateReadsChosenEncoding(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "данные.txt")
	data, err := (fileEncoding{Codepage: 866}).encode(abcDigests[AlgMD5] + " *данные.txt\n")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "list.md5"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	app := &taskAppMock{messages: make(chan string, 1)}
	startValidate(app, vfs.NewOSVFS(dir), filepath.Join(dir, "list.md5"), dir, validateOptions{encoding: fileEncoding{Codepage: 866}}, nil)
	if got, want := <-app.messages, fmt.Sprintf(vtui.Msg("IntChecker.AllOK"), 1); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
}

func TestUnencodableNamesAreLeftOutAndReported(t *testing.T) {
	dir := t.TempDir()
	makeTree(t, dir, "a.txt", "данные.txt", "日本.txt", "far/東京.txt")
	fs := vfs.NewOSVFS(dir)
	enc := fileEncoding{Codepage: 1251}

	job := generateJob{fs: fs, dir: dir, names: []string{"a.txt", "данные.txt", "日本.txt", "far"}, algorithm: AlgMD5,
		mode: outputSingle, output: "list.md5", recursive: true, encoding: enc}
	res, err := runGenerate(context.Background(), job, &recordingReporter{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(res.Unencodable, "|") != "far/東京.txt|日本.txt" || res.Written != 2 {
		t.Fatalf("result = %+v", res)
	}
	data, err := os.ReadFile(filepath.Join(dir, "list.md5"))
	if err != nil {
		t.Fatal(err)
	}
	text, _, err := decodeChecksumFile(data, 1251)
	if err != nil {
		t.Fatal(err)
	}
	if names := storedNames(t, "list.md5", string(text)); strings.Join(names, "|") != "a.txt|данные.txt" {
		t.Fatalf("stored names = %q", names)
	}
	report := generateReport(job, res)
	for _, want := range []string{"日本.txt", "far/東京.txt", enc.name()} {
		if !strings.Contains(report, want) {
			t.Fatalf("report lacks %q:\n%s", want, report)
		}
	}

	// A directory whose every name is unencodable gets no checksum file,
	// and the others are still written.
	job.mode, job.overwrite = outputDirectory, true
	res, err = runGenerate(context.Background(), job, &recordingReporter{})
	if err != nil {
		t.Fatal(err)
	}
	base := defaultOutputName(fs.Base(dir), AlgMD5)
	if strings.Join(res.Outputs, "|") != base || len(res.Unencodable) != 2 {
		t.Fatalf("per directory = %+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "far", "far.md5")); !os.IsNotExist(err) {
		t.Fatalf("far/far.md5 written: %v", err)
	}

	// The display mode writes nothing, so every name is shown.
	job.mode = outputDisplay
	res, err = runGenerate(context.Background(), job, &recordingReporter{})
	if err != nil || len(res.Unencodable) != 0 || !strings.Contains(res.Text, "日本.txt") {
		t.Fatalf("display = %+v, %v", res, err)
	}
}
