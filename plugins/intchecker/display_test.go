package intchecker

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// answerAppMock answers every message with answer and records the texts.
type answerAppMock struct {
	appMock
	answer   int
	messages []string
}

func (a *answerAppMock) Message(_, msg string, _ []string) int {
	a.messages = append(a.messages, msg)
	return a.answer
}

func TestConfirmOverwrite(t *testing.T) {
	for _, tc := range []struct {
		mode      outputMode
		existing  []string
		answer    int
		ok        bool
		overwrite bool
		skip      bool
		question  string
	}{
		{outputSingle, nil, 0, true, false, false, ""},
		{outputSingle, []string{"x.md5"}, 0, true, true, false, fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), "x.md5")},
		{outputSingle, []string{"x.md5"}, 1, false, false, false, fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), "x.md5")},
		{outputSeparate, []string{"a.md5"}, 1, true, false, true, fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), "a.md5")},
		{outputSeparate, []string{"a.md5", "b.md5"}, 0, true, true, false, fmt.Sprintf(vtui.Msg("IntChecker.OverwriteManyQuestion"), 2, "a.md5")},
		{outputDirectory, []string{"d.md5"}, 2, false, false, false, fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), "d.md5")},
		{outputDirectory, []string{"d.md5"}, -1, false, false, false, fmt.Sprintf(vtui.Msg("IntChecker.OverwriteQuestion"), "d.md5")},
	} {
		app := &answerAppMock{answer: tc.answer}
		job := generateJob{mode: tc.mode}
		ok := confirmOverwrite(app, &job, tc.existing)
		if ok != tc.ok || job.overwrite != tc.overwrite || job.skipExisting != tc.skip {
			t.Errorf("%+v: ok=%v overwrite=%v skip=%v", tc, ok, job.overwrite, job.skipExisting)
		}
		if tc.question == "" && len(app.messages) != 0 || tc.question != "" && (len(app.messages) != 1 || app.messages[0] != tc.question) {
			t.Errorf("%+v: messages = %q", tc, app.messages)
		}
	}
}

func TestGenerateReport(t *testing.T) {
	if got := generateReport(generateJob{mode: outputSeparate}, generateResult{Outputs: []string{"a.md5"}}); got != "" {
		t.Fatalf("clean run reported %q", got)
	}
	res := generateResult{
		Outputs:         []string{"a.md5"},
		SkippedExisting: []string{"b.md5", "c.md5"},
		WriteFailures:   []fileFailure{{Name: "d.md5", Err: errors.New("denied")}},
		Failures:        []fileFailure{{Name: "e", Err: errors.New("io")}},
	}
	report := generateReport(generateJob{mode: outputSeparate}, res)
	for _, want := range []string{
		fmt.Sprintf(vtui.Msg("IntChecker.FilesWritten"), 1),
		fmt.Sprintf(vtui.Msg("IntChecker.SkippedExisting"), 2),
		fmt.Sprintf(vtui.Msg("IntChecker.WriteErrors"), 1),
		"d.md5: denied",
		fmt.Sprintf(vtui.Msg("IntChecker.ReadErrors"), 1),
		"e: io",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report lacks %q:\n%s", want, report)
		}
	}
	single := generateReport(generateJob{mode: outputSingle}, generateResult{Outputs: []string{"x"}, Failures: res.Failures})
	if strings.Contains(single, fmt.Sprintf(vtui.Msg("IntChecker.FilesWritten"), 1)) || !strings.Contains(single, "e: io") {
		t.Fatalf("single file report = %q", single)
	}
}

func TestResolveSavePath(t *testing.T) {
	dir := t.TempDir()
	fs := vfs.NewOSVFS(dir)
	if _, ok := resolveSavePath(fs, dir, "  "); ok {
		t.Fatal("empty name accepted")
	}
	if got, ok := resolveSavePath(fs, dir, " list.md5 "); !ok || got != filepath.Join(dir, "list.md5") {
		t.Fatalf("relative: %q %v", got, ok)
	}
	abs := filepath.Join(t.TempDir(), "x.sfv")
	if got, ok := resolveSavePath(fs, dir, abs); !ok || got != abs {
		t.Fatalf("absolute: %q %v", got, ok)
	}
}

func TestGenerateDialogDisablesFileNameForOtherOutputs(t *testing.T) {
	initValidateTestScreen(t)
	d := newGenerateDialog("photos", DefaultSettings())
	if d.editOutput.IsDisabled() || d.editOutput.GetText() != "photos.md5" {
		t.Fatalf("single file: disabled=%v text=%q", d.editOutput.IsDisabled(), d.editOutput.GetText())
	}
	for _, mode := range []outputMode{outputSeparate, outputDirectory, outputDisplay} {
		d.output.OnChange(int(mode))
		if !d.editOutput.IsDisabled() {
			t.Errorf("mode %d left the file name enabled", mode)
		}
	}
	d.output.OnChange(int(outputSingle))
	if d.editOutput.IsDisabled() {
		t.Fatal("single file left the file name disabled")
	}
}

func TestFinishGenerateDisplayOpensTheList(t *testing.T) {
	initValidateTestScreen(t)
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", "abc")
	app := &taskAppMock{messages: make(chan string, 1)}
	job := generateJob{fs: vfs.NewOSVFS(dir), dir: dir, names: []string{"a.txt"}, algorithm: AlgMD5, mode: outputDisplay}
	startGenerate(app, job)
	if _, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window); !ok {
		t.Fatalf("top frame = %T, want the list window", vtui.FrameManager.GetTopFrame())
	}
	select {
	case msg := <-app.messages:
		t.Fatalf("unexpected message %q", msg)
	default:
	}
}

// TestGenerateDialogDump renders the "Generate hashes" dialog and the list
// window in English and Russian; the dumps go into the commit message.
func TestGenerateDialogDump(t *testing.T) {
	t.Cleanup(func() { i18n.InitLang("", "", "") })
	list := abcDigests[AlgMD5] + " *a.txt\n" + abcDigests[AlgMD5] + " *b.txt\n" + abcDigests[AlgMD5] + " *photo 01.jpg\n"
	for _, lang := range []string{"en", "ru"} {
		i18n.InitLang(lang, "en", "")

		scr := initValidateTestScreen(t)
		d := newGenerateDialog("photos", DefaultSettings())
		d.output.Selected = int(outputSeparate)
		d.output.OnChange(int(outputSeparate))
		d.win.Show(scr)
		var dump bytes.Buffer
		scr.Dump(&dump)
		text, _, _ := strings.Cut(dump.String(), "--- CELL METADATA")
		options := []string{"photos.md5", "SHA-512", strings.ReplaceAll(vtui.Msg("IntChecker.Recursive"), "&", ""),
			strings.ReplaceAll(vtui.Msg("IntChecker.AbsolutePaths"), "&", ""), strings.ReplaceAll(vtui.Msg("IntChecker.FileMask"), "&", "") + " *",
			strings.ReplaceAll(vtui.Msg("IntChecker.FileEncoding"), "&", "") + " UTF-8"}
		for _, want := range append(outputModeNames(), options...) {
			if !strings.Contains(text, want) {
				t.Errorf("%s dialog dump lacks %q:\n%s", lang, want, text)
			}
		}
		t.Logf("%s, generate dialog:\n%s", lang, text)

		scr = initValidateTestScreen(t)
		w := newHashListWindow(list)
		w.win.Show(scr)
		dump.Reset()
		scr.Dump(&dump)
		text, _, _ = strings.Cut(dump.String(), "--- CELL METADATA")
		for _, want := range []string{"*photo 01.jpg", strings.ReplaceAll(vtui.Msg("IntChecker.CopyToClipboard"), "&", ""), strings.ReplaceAll(vtui.Msg("IntChecker.SaveToFile"), "&", "")} {
			if !strings.Contains(text, want) {
				t.Errorf("%s list dump lacks %q:\n%s", lang, want, text)
			}
		}
		t.Logf("%s, list window:\n%s", lang, text)
	}
}
