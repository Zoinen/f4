package settings

import (
	"slices"
	"testing"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFileGroupChildSinglePageContainsEverySection(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 99, 29)
	assertSections := func() {
		t.Helper()
		seen := map[string]bool{}
		headers := 0
		fields := map[string]bool{}
		for _, row := range child.page.rows {
			seen[row.field.Category] = true
			fields[row.field.ID] = true
			if row.sectionHeading && row.checkableHeader {
				headers++
			}
		}
		for _, section := range []string{"Conditions", "Highlighting", "Icons", "Sorting", "Associations", "Reset"} {
			if !seen["filegroup-"+section] {
				t.Errorf("single page omits %s", section)
			}
		}
		for _, id := range []string{"group.Name", "group.NormalColor", "group.Sort.Number", "collection:filegroups.commands"} {
			if id == "collection:filegroups.commands" {
				found := false
				for _, row := range child.page.rows {
					found = found || row.control != nil && row.control.GetId() == id
				}
				if !found {
					t.Error("applications list missing from single page")
				}
			} else if !fields[id] {
				t.Errorf("single page omits %s", id)
			}
		}
		if headers != 4 {
			t.Fatalf("single page has %d feature headers, want 4", headers)
		}
	}
	assertSections()
	if child.page.total <= child.page.Y2-child.page.Y1+1 {
		t.Fatal("long property page does not scroll")
	}
	for _, section := range []string{"Highlighting", "Icons", "Sorting", "Associations", "Reset", "Conditions"} {
		child.selectCategory("filegroup-" + section)
		assertSections()
	}
	if view.center.sidebar.IsDisabled() {
		t.Fatal("child changed the parent category navigation")
	}
}

func TestFileGroupChildSinglePageSemanticChromeAndPinnedPreview(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(3, 2, 102, 31)
	for _, control := range []vtui.UIElement{child.search, child.sidebar, child.previous, child.next, child.clearSearch} {
		if !control.IsDisabled() || control.IsVisible() {
			t.Fatalf("hidden navigation remains interactive: %s", control.GetId())
		}
	}
	node := child.SemanticNode(&vtui.SemanticContext{})
	if node["settingsSectionsOnly"] != true {
		t.Fatal("child allowed native settings categories")
	}
	var page, sample map[string]any
	for _, control := range semantic.AppMapSlice(node["children"]) {
		switch semantic.String(control["layoutRole"]) {
		case "navigation", "search", "search-clear", "search-label", "search-previous", "search-next", "search-matches", "content-title":
			t.Fatalf("child exported navigation chrome: %+v", control)
		case "content":
			page = control
		case "editor-preview":
			sample = control
		}
	}
	if len(page) == 0 || page["scrollable"] != true || len(sample) == 0 || sample["fileGroupSample"] != true {
		t.Fatal("missing scrollable page or pinned preview")
	}
	var sections []string
	icons := []string{"list-checks", "palette", "images", "arrow-down-a-z", "plug", "rotate-ccw"}
	boxes := map[string][]string{}
	for i, entry := range semantic.AppMapSlice(page["children"]) {
		if i == 0 {
			if entry["title"] != "Group name" || entry["headerControl"] == nil {
				t.Fatal("the group name box should come before Conditions")
			}
			continue
		}
		title := semantic.String(entry["title"])
		if entry["layoutRole"] == "section-header" {
			if i := len(sections); i >= len(icons) || entry["iconName"] != icons[i] {
				t.Fatalf("unexpected icon for %s: %v", title, entry["iconName"])
			}
			sections = append(sections, title)
			if entry["bordered"] != false {
				t.Fatalf("%s section header is framed", title)
			}
			hasToggle := entry["headerControl"] != nil
			if hasToggle != (title != "Matching conditions" && title != "Reset") {
				t.Fatalf("unexpected checkbox state in %s: %v", title, hasToggle)
			}
			continue
		}
		if len(sections) == 0 || entry["bordered"] != true {
			t.Fatalf("unexpected group box %s", title)
		}
		section := sections[len(sections)-1]
		boxes[section] = append(boxes[section], title)
	}
	if !slices.Equal(sections, []string{"Matching conditions", "Highlighting", "Icons", "Sorting", "Associations", "Reset"}) {
		t.Fatalf("section headings = %v", sections)
	}
	if got := boxes["Matching conditions"]; !slices.Equal(got, []string{"Name", "Attributes", "Size", "Date and time"}) {
		t.Fatalf("matching condition group headings = %v", got)
	}
	for section, expected := range map[string]string{
		"Highlighting": "Colors", "Icons": "Artwork",
		"Sorting": "Sort priority", "Associations": "Applications", "Reset": "Restore standard values",
	} {
		if !slices.Contains(boxes[section], expected) {
			t.Errorf("%s boxes %v omit %s", section, boxes[section], expected)
		}
	}
	if child.page.X1 != child.X1+2 || child.page.X2 != child.X2-2 || child.fileGroupEditorSample.X1 != child.page.X1 {
		t.Fatal("child still reserves space for the sidebar")
	}
	if child.fileGroupEditorSample.Y2 >= child.page.Y1 {
		t.Fatal("preview overlaps the property page")
	}
	beforeX, beforeY := child.fileGroupEditorSample.X1, child.fileGroupEditorSample.Y1
	if !child.page.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(child.page), "action": "control.scroll", "value": 20,
	}) {
		t.Fatal("single page rejected scrolling")
	}
	if child.page.scroll == 0 || child.fileGroupEditorSample.X1 != beforeX || child.fileGroupEditorSample.Y1 != beforeY {
		t.Fatal("scrolling moved the pinned preview")
	}
}

func TestFileGroupSectionCollapsePreservesDraftAndReflows(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 149, 39)
	for _, section := range []string{"Conditions", "Highlighting", "Icons", "Sorting", "Associations", "Reset"} {
		category := "filegroup-" + section
		target := child.page.sectionID(category)
		toggle := map[string]any{"target": target, "action": "control.toggleSection"}
		before := child.page.total
		if !child.HandleSemanticAction(toggle) || child.page.total >= before {
			t.Fatalf("%s did not collapse", section)
		}
		for _, row := range child.page.rows {
			if row.field.Category == category && !row.sectionHeading {
				t.Fatalf("collapsed %s still has a body row", section)
			}
		}
		if !child.HandleSemanticAction(toggle) || child.page.total != before {
			t.Fatalf("%s did not restore its original layout", section)
		}
	}
	if got := child.sessions[0].draft.Values["group.NormalColor"]; got != "foreground:#112233" {
		t.Fatalf("color changed during collapse: %q", got)
	}
}

func TestFileGroupChildSinglePageKeyboardNeverFocusesHiddenNavigation(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 99, 29)
	child.SetFocusedItem(child.page)
	for _, key := range []uint16{vtinput.VK_LEFT, vtinput.VK_UP, vtinput.VK_PRIOR} {
		child.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: key})
		if child.GetFocusedItem() != child.page {
			t.Fatalf("key %d escaped to hidden navigation", key)
		}
	}
	child.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_F, ControlKeyState: vtinput.LeftCtrlPressed})
	if child.GetFocusedItem() != child.page {
		t.Fatal("Ctrl+F focused hidden search")
	}
	for _, event := range []vtinput.InputEvent{
		{KeyDown: true, VirtualKeyCode: vtinput.VK_TAB},
		{KeyDown: true, VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.ShiftPressed},
	} {
		for range 30 {
			child.ProcessKey(&event)
			if focused := child.GetFocusedItem(); focused != child.page && focused != child.ok && focused != child.cancel {
				t.Fatalf("Tab focused hidden chrome: %T", focused)
			}
		}
	}
}

func TestFileGroupConditionEditorsEnableIndependently(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	record := view.session.draft.Records["filegroups"][0]
	record.Values["group.ExcludeAttributes"] = "Directory"
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 149, 39)

	rowFor := func(id string) *settingsRow {
		for _, row := range child.page.rows {
			if row.field.ID == id {
				return row
			}
		}
		return nil
	}
	toggle := func(id string) {
		t.Helper()
		row := rowFor(id)
		if row == nil || !child.HandleSemanticAction(map[string]any{
			"target": vtui.SemanticID(row.control), "action": "control.toggle",
		}) {
			t.Fatalf("cannot toggle %s", id)
		}
	}
	assertEnabled := func(id string, enabled bool) {
		t.Helper()
		row := rowFor(id)
		if row == nil || row.control.IsDisabled() == enabled {
			t.Fatalf("%s should remain visible with enabled=%t", id, enabled)
		}
	}
	assertEnabled("group.ExcludeAttributes", true)
	assertEnabled("group.SizeAbove", false)
	assertEnabled("group.DateType", false)
	attr := rowFor("group.ExcludeAttributes")
	header := rowFor("filegroup.conditions.attributes")
	required := rowFor("group.IncludeAttributes")
	if header == nil || required == nil || attr.y != header.y+1 || required.y != attr.y ||
		attr.controlX <= required.controlX {
		t.Fatal("the two attribute editors should share the row below their checkbox heading")
	}
	projection := child.page.SemanticNode(&vtui.SemanticContext{})
	conditionGroups := 0
	for _, group := range semantic.AppMapSlice(projection["children"]) {
		if group["layoutRole"] == "condition-group" && group["headerControl"] != nil {
			conditionGroups++
		}
	}
	if conditionGroups != 4 {
		t.Fatal("expected four checkbox headings")
	}
	matchingFields := []string{"group.Mask", "group.MaskSyntax", "group.Case"}
	for _, id := range matchingFields {
		assertEnabled(id, true)
	}
	maskLabelColor := func() uint64 {
		mask := rowFor("group.Mask")
		scr := vtui.NewSilentScreenBuf()
		scr.AllocBuf(150, 40)
		child.page.Show(scr)
		x := child.page.X1 + 2 + mask.labelX
		y := child.page.Y1 + mask.y - child.page.scroll
		return scr.GetCell(x, y).Attributes
	}
	beforeY, beforeTotal := rowFor("group.Mask").y, child.page.total
	if got := maskLabelColor(); got != vtui.Palette[vtui.ColDialogText] {
		t.Fatalf("enabled label color = %#x", got)
	}
	toggle("filegroup.conditions.matching")
	if got := maskLabelColor(); got != vtui.DimColor(vtui.Palette[vtui.ColDialogText]) {
		t.Fatalf("disabled label color = %#x", got)
	}
	for _, id := range matchingFields {
		assertEnabled(id, false)
	}
	if rowFor("group.Mask").y != beforeY || child.page.total != beforeTotal {
		t.Fatal("disabling File name matching moved the layout")
	}
	toggle("filegroup.conditions.matching")
	for _, id := range matchingFields {
		assertEnabled(id, true)
	}
	toggle("filegroup.conditions.size")
	assertEnabled("group.SizeAbove", true)
	assertEnabled("group.DateType", false)
	toggle("filegroup.conditions.attributes")
	assertEnabled("group.ExcludeAttributes", false)
	assertEnabled("group.SizeAbove", true)
	child.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(rowFor("group.ExcludeAttributes").control), "action": "control.reset",
	})
	toggle("filegroup.conditions.timestamps")
	assertEnabled("group.DateAfter", true)
	assertEnabled("group.ExcludeAttributes", false)

	scratch := child.sessions[0].draft
	if scratch.Values["group.ExcludeAttributes"] != "Directory" || len(scratch.Changed()) != 0 {
		t.Fatalf("checkboxes modified the rule draft: %v", scratch.Changed())
	}
	child.SetPosition(0, 0, 49, 29)
	size := rowFor("group.SizeAbove")
	sizeHeader := rowFor("filegroup.conditions.size")
	if size.y <= sizeHeader.y {
		t.Fatal("the narrow layout should stack the fields below their header")
	}
}

func TestFileGroupNameAndMatchingUseCompactRows(t *testing.T) {
	_, view := setupFileGroupColorChild(t)
	child := view.childEditor(0)
	defer child.sessions[0].draft.Close()
	child.SetPosition(0, 0, 54, 28)
	if !child.HandleSemanticAction(map[string]any{
		"target": vtui.SemanticID(child), "action": "dialog.geometry",
		"x": 3, "y": 2, "w": 150, "h": 40,
	}) {
		t.Fatal("initial Qt geometry was not accepted")
	}
	if child.page.X1 != child.X1+2 {
		t.Fatal("initial Qt geometry did not position the page")
	}

	rows := map[string]*settingsRow{}
	for _, row := range child.page.rows {
		rows[row.field.ID] = row
	}
	enabled, name := rows["group.Enabled"], rows["group.Name"]
	if name.y+name.controlY != enabled.y+1 || name.controlX != enabled.controlX || len(name.label) != 0 {
		t.Fatal("the name editor should sit below its checkbox heading without a duplicate label")
	}
	mask, syntax, matchCase := rows["group.Mask"], rows["group.MaskSyntax"], rows["group.Case"]
	for _, field := range []*settingsRow{syntax, matchCase} {
		buttons, ok := field.control.(*settingsRadios)
		if !ok || buttons.SemanticNode(&vtui.SemanticContext{})["choicePresentation"] != "buttons" {
			t.Fatalf("%s does not export a choice button group", field.field.ID)
		}
	}
	attributes := rows["filegroup.conditions.attributes"]
	size := rows["filegroup.conditions.size"]
	timestamps := rows["filegroup.conditions.timestamps"]
	if syntax.y < mask.y+mask.controlY+mask.controlHeight || mask.controlHeight < 3 ||
		syntax.y != matchCase.y || syntax.labelX >= matchCase.labelX {
		t.Fatal("matching options should share a row below the masks")
	}
	if attributes.y != rows["filegroup.conditions.matching"].y ||
		attributes.controlX <= matchCase.labelX || size.controlX != attributes.controlX ||
		timestamps.controlX != attributes.controlX || attributes.y >= size.y || size.y >= timestamps.y {
		t.Fatal("attributes, size and date/time should stack in the right column")
	}
	dateType, relative := rows["group.DateType"], rows["group.DateRelative"]
	after, before := rows["group.DateAfter"], rows["group.DateBefore"]
	dateRow := dateType.y + dateType.controlY
	for _, field := range []*settingsRow{relative, after, before} {
		if field.y+field.controlY != dateRow {
			t.Fatalf("%s does not share the date input row", field.field.ID)
		}
	}
	if dateType.controlX >= relative.controlX || relative.controlX >= after.controlX || after.controlX >= before.controlX {
		t.Fatal("date and time controls should be ordered left to right")
	}
	const raw = "*.bash, /alpha,beta/,\n *.go, "
	target := vtui.SemanticID(mask.control)
	if !child.HandleSemanticAction(map[string]any{
		"target": target, "action": "control.focus",
	}) || child.page.GetFocusedItem() != mask.control || !mask.control.IsFocused() {
		t.Fatal("Qt focus did not select the mask editor")
	}
	if !child.HandleSemanticAction(map[string]any{
		"target": target, "action": "control.setText", "text": raw,
	}) {
		t.Fatal("mask editor did not accept multiline input")
	}
	var maskNode map[string]any
	for _, group := range semantic.AppMapSlice(child.page.SemanticNode(&vtui.SemanticContext{})["children"]) {
		for _, widget := range semantic.AppMapSlice(group["children"]) {
			if widget["id"] == target {
				maskNode = widget
			}
		}
	}
	if maskNode["kind"] != "multiLineEdit" || maskNode["focused"] != true || maskNode["text"] != raw ||
		!slices.Equal(semantic.AppStringSlice(maskNode["maskChips"]), []string{"*.bash", "/alpha,beta/", "*.go"}) {
		t.Fatalf("unexpected multiline mask projection: %+v", maskNode)
	}
	child.SetPosition(0, 0, 54, 28)
	if syntax.y <= mask.y+mask.controlY || syntax.labelX != 0 {
		t.Fatal("matching options should stack below the masks on a narrow layout")
	}
	if !child.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_BACK,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}) || child.sessions[0].draft.Values["group.Mask"] != "*.txt" {
		t.Fatal("Ctrl+Backspace did not restore the standard mask")
	}
}
