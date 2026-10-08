package mediainfo

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestReportDialogExportsFullScrollableReport(t *testing.T) {
	lines := []string{"General", "Title : A&B <movie>", "", "Video", "Format : AVC"}
	for index := range 100 {
		lines = append(lines, fmt.Sprintf("Field %d : value", index))
	}
	lines[len(lines)-1] += strings.Repeat(" long metadata value", 30)
	view := newReportTextView(2, 2, 84, 18, lines, true)
	window := &reportWindow{Window: vtui.NewWindow(0, 0, 88, 24, "MediaInfo")}
	window.AddItem(view)
	window.SetFocusedItem(view)
	if window.SemanticNode(nil)["sizeKey"] != "mediainfo.report" {
		t.Fatal("report size identity must be shared across files")
	}
	children := window.SemanticNode(nil)["children"].([]map[string]any)
	if len(children) != 1 || children[0]["kind"] != "listBox" {
		t.Fatalf("report projected as %v; want a populated listBox", children)
	}
	node := children[0]
	if node["id"] != vtui.SemanticID(view) || node["focused"] != true || node["wrapText"] != true {
		t.Fatalf("report identity or presentation lost: %v", node)
	}
	if !reflect.DeepEqual(node["items"], lines) {
		t.Fatalf("full report lost: items=%v", node["items"])
	}
	fields := node["itemFields"].([]map[string]any)
	if len(fields) != len(lines) || fields[0]["name"] != "" || fields[1]["name"] != "Title" || fields[1]["value"] != "A&B <movie>" || node["fieldColumnWidth"] != 11 {
		t.Fatalf("field/value columns lost: %v", node)
	}
	if node["h"] != 18 || node["w"] != 84 {
		t.Fatalf("viewport geometry lost: %v", node)
	}
	if path := os.Getenv("F4_MEDIAINFO_REPORT_SCENE"); path != "" {
		data, err := json.Marshal(window.SemanticNode(nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	items := node["items"].([]string)
	items[0] = "changed snapshot"
	if view.lines[0].text != lines[0] {
		t.Fatal("semantic snapshot mutated the report")
	}
}

func TestReportTemplatePreservesLiteralLines(t *testing.T) {
	view := newReportTextView(2, 2, 84, 3, []string{"Custom : literal"}, false)
	node := view.SemanticNode(nil)
	fields := node["itemFields"].([]map[string]any)
	if fields[0]["name"] != "" || fields[0]["value"] != "" || node["fieldColumnWidth"] != 0 {
		t.Fatalf("custom template split into columns: %v", node)
	}
}

func TestReportDialogRoutesSemanticSelection(t *testing.T) {
	view := newReportTextView(2, 2, 84, 3, []string{"General", "Title : A&B", "", "Video", "Format : AVC"}, false)
	window := vtui.NewWindow(0, 0, 88, 12, "MediaInfo")
	window.AddItem(view)
	action := map[string]any{"target": vtui.SemanticID(view), "action": "control.select", "index": 4}
	if !window.HandleSemanticAction(action) || view.SelectPos != 4 || view.TopPos != 2 {
		t.Fatalf("Qt selection not applied: cursor=%d top=%d", view.SelectPos, view.TopPos)
	}
	action["index"] = 5
	if window.HandleSemanticAction(action) {
		t.Fatal("accepted selection outside report")
	}
	view.SetDisabled(true)
	action["index"] = 0
	if window.HandleSemanticAction(action) || view.SelectPos != 4 {
		t.Fatal("disabled report accepted selection")
	}
}
