package settings

import (
	"reflect"
	"testing"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func TestFileGroupSortListExpandsWithConfiguredDraftNumbers(t *testing.T) {
	parent, overview := setupFileGroupColorChild(t)
	parent.Records["filegroups"][0].Values["group.Sort.Number"] = "7"
	parent.Records["filegroups"] = append(parent.Records["filegroups"],
		f4settings.Record{ID: "photos", Values: map[string]string{
			"group.Name": "Photos", "group.Sort.Number": "100", "group.Sort.Enabled": "true",
		}},
		f4settings.Record{ID: "pdfs", Values: map[string]string{
			"group.Name": "PDFs", "group.Sort.Number": "20", "group.Sort.Enabled": "true",
		}},
		f4settings.Record{ID: "archives", Values: map[string]string{
			"group.Name": "Archives", "group.Sort.Number": "-5",
		}},
		f4settings.Record{ID: "no-number", Values: map[string]string{
			"group.Name": "No number", "group.Sort.Enabled": "true",
		}},
	)
	editor := overview.childEditor(0)
	defer editor.sessions[0].draft.Close()
	editor.SetPosition(0, 0, 118, 32)

	sortBox := func() map[string]map[string]any {
		t.Helper()
		for _, group := range semantic.AppMapSlice(editor.page.SemanticNode(&vtui.SemanticContext{})["children"]) {
			if group["title"] != "Sort priority" {
				continue
			}
			controls := map[string]map[string]any{}
			for _, child := range semantic.AppMapSlice(group["children"]) {
				controls[semantic.String(child["id"])] = child
			}
			return controls
		}
		t.Fatal("sort priority box missing")
		return nil
	}
	const tableID = "id:filegroups.sort-list"
	buttonID := "id:" + fileGroupSortListToggle
	collapsed := sortBox()
	if collapsed[tableID] != nil || collapsed[buttonID]["text"] != "View existing groups" {
		t.Fatalf("wrong collapsed state: %v", collapsed)
	}
	number := collapsed["id:setting:group.Sort.Number"]
	button := collapsed[buttonID]
	if button["y"] != number["y"] || semantic.Int(button["x"]) <= semantic.Int(number["x"])+semantic.Int(number["w"]) {
		t.Fatalf("button is not beside number field: %v, %v", button, number)
	}

	action := map[string]any{"target": buttonID, "action": "control.activate"}
	if !editor.HandleSemanticAction(action) {
		t.Fatal("sort list button did not activate")
	}
	expanded := sortBox()
	table := expanded[tableID]
	if expanded[buttonID]["text"] != "Hide existing groups" || table == nil {
		t.Fatalf("wrong expanded state: %v", expanded)
	}
	var values [][]string
	for _, row := range semantic.AppMapSlice(table["rows"]) {
		values = append(values, semantic.AppStringSlice(row["cells"]))
	}
	if want := [][]string{{"Archives (disabled)", "-5"}, {"PDFs", "20"}, {"Photos", "100"}}; !reflect.DeepEqual(values, want) {
		t.Fatalf("sort list = %v, want %v", values, want)
	}
	if !editor.HandleSemanticAction(action) || sortBox()[tableID] != nil {
		t.Fatal("sort list did not collapse")
	}
}
