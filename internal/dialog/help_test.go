package dialog

import (
	runewidth "github.com/mattn/go-runewidth"
	"github.com/unxed/vtui"
	"os"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestVisRenHelpReference(t *testing.T) {
	ruHelp, err := os.ReadFile("help/ru.hlf")
	if err != nil {
		t.Fatal(err)
	}
	esHelp, err := os.ReadFile("help/es.hlf")
	if err != nil {
		t.Fatal(err)
	}
	arHelp, err := os.ReadFile("help/ar.hlf")
	if err != nil {
		t.Fatal(err)
	}
	heHelp, err := os.ReadFile("help/he.hlf")
	if err != nil {
		t.Fatal(err)
	}
	trHelp, err := os.ReadFile("help/tr.hlf")
	if err != nil {
		t.Fatal(err)
	}
	hiHelp, err := os.ReadFile("help/hi.hlf")
	if err != nil {
		t.Fatal(err)
	}
	topics := []string{
		"VisRen", "VisRenQuickStart", "VisRenMasks", "VisRenTransforms",
		"VisRenMetadata", "VisRenSearch", "VisRenPreview", "VisRenEditor",
		"VisRenRename", "VisRenSafety", "VisRenExamples",
	}
	linkMarkup := regexp.MustCompile(`~([^~]+)~[^@]+@`)
	for _, tc := range []struct {
		name string
		data string
	}{
		{name: "English", data: DefaultHelpData},
		{name: "Russian", data: string(ruHelp)},
		{name: "Spanish", data: string(esHelp)},
		{name: "Arabic", data: string(arHelp)},
		{name: "Hebrew", data: string(heHelp)},
		{name: "Turkish", data: string(trHelp)},
		{name: "Hindi", data: string(hiHelp)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !utf8.ValidString(tc.data) {
				t.Fatal("help is not valid UTF-8")
			}
			if strings.HasPrefix(tc.data, "\uFEFF") {
				t.Fatal("help must not contain a UTF-8 BOM")
			}
			seenTopics := make(map[string]bool)
			for lineNo, line := range strings.Split(tc.data, "\n") {
				if strings.HasPrefix(line, "@") && !strings.Contains(line, "~") {
					name := strings.TrimPrefix(strings.TrimSuffix(line, "\r"), "@")
					if seenTopics[name] {
						t.Fatalf("duplicate topic %q on line %d", name, lineNo+1)
					}
					seenTopics[name] = true
				}
			}
			Engine := vtui.NewHelpEngine(NewMemoryHelpVFS(map[string]string{"visren.hlf": tc.data}))
			if err := Engine.LoadFile("visren.hlf"); err != nil {
				t.Fatal(err)
			}
			FlattenVisRenHelp(Engine)
			for _, name := range topics {
				topic := Engine.GetTopic(name)
				if topic == nil {
					t.Fatalf("topic %q is missing", name)
				}
				if topic.StickyRows != 1 {
					t.Errorf("topic %q has %d sticky rows, want 1", name, topic.StickyRows)
				}
				if name != "VisRen" && len(topic.Links) != 0 {
					t.Errorf("detail topic %q contains links that capture Up/Down scrolling", name)
				}
				for lineNo, line := range topic.Lines {
					if strings.Count(line, "#")%2 != 0 {
						t.Errorf("topic %q line %d has unbalanced bold markup: %q", name, lineNo+1, line)
					}
					display := linkMarkup.ReplaceAllString(line, "$1")
					display = strings.ReplaceAll(strings.TrimPrefix(display, "^"), "#", "")
					if width := runewidth.StringWidth(display); width > 73 {
						t.Errorf("topic %q line %d is %d cells wide: %q", name, lineNo+1, width, display)
					}
				}
			}
			index := Engine.GetTopic("VisRen")
			for _, name := range topics[1:] {
				section := Engine.GetTopic(name)
				if section == nil || len(section.Lines) == 0 {
					continue
				}
				title := strings.TrimSpace(section.Lines[0])
				if !strings.Contains(strings.Join(index.Lines, "\n"), "#"+title+"#") {
					t.Errorf("VisRen does not contain flattened section %q", title)
				}
			}
			if len(index.Links) != len(topics)-1 {
				t.Fatalf("VisRen index has %d links, want %d", len(index.Links), len(topics)-1)
			}
			for _, link := range index.Links {
				if Engine.GetTopic(link.Target) == nil {
					t.Errorf("VisRen link %q targets missing topic %q", link.Text, link.Target)
				}
			}
			contents := Engine.GetTopic("Contents")
			linkedFromContents := false
			for _, link := range contents.Links {
				linkedFromContents = linkedFromContents || link.Target == "VisRen"
			}
			if !linkedFromContents {
				t.Error("Contents does not link to VisRen")
			}
			for _, syntax := range []string{"[N2--5]", "[C001+5]", "[TL]", "[DM]", "[V]", "<Error!>"} {
				if !strings.Contains(tc.data, syntax) {
					t.Errorf("help does not document %s", syntax)
				}
			}
		})
	}
}
