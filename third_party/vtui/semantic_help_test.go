package vtui

import "testing"

func TestHelpViewSemanticContentAndNavigation(t *testing.T) {
	engine := NewHelpEngine(nil)
	engine.AddTopic(&HelpTopic{Name: "Contents", StickyRows: 1,
		Lines: []string{"^#Heading#", "Read ~Next~Next@ and <text>"}})
	engine.AddTopic(&HelpTopic{Name: "Next", Lines: []string{"Destination"}})
	view := NewHelpView(engine, "Contents")
	node := semanticFrame(&SemanticContext{}, view)
	if node["kind"] != "dialog" || node["layout"] != "help" {
		t.Fatalf("help is not a native dialog: %v", node)
	}
	rows := node["helpLines"].([]map[string]any)
	if len(rows) != 2 || rows[0]["centered"] != true {
		t.Fatalf("missing formatted help rows: %v", rows)
	}
	spans := rows[1]["spans"].([]map[string]any)
	if len(spans) != 3 || spans[1]["text"] != "Next" || spans[1]["link"] != 0 {
		t.Fatalf("missing help link: %v", spans)
	}
	handler, ok := any(view).(SemanticActionHandler)
	if !ok {
		t.Fatal("help has no semantic action handler")
	}
	activate := map[string]any{"target": SemanticID(view), "action": "help.activate", "index": 0}
	if !handler.HandleSemanticAction(activate) || view.current.Name != "Next" {
		t.Fatal("help link did not open its topic")
	}
	if !handler.HandleSemanticAction(map[string]any{"target": SemanticID(view), "action": "help.back"}) {
		t.Fatal("help history action failed")
	}
	if view.current.Name != "Contents" || view.GetTitle() != " Help: Contents " {
		t.Fatalf("history did not restore topic/title: %q / %q", view.current.Name, view.GetTitle())
	}
	activate["index"] = 99
	if handler.HandleSemanticAction(activate) {
		t.Fatal("invalid link was accepted")
	}
	activate["target"] = "another-window"
	if handler.HandleSemanticAction(activate) {
		t.Fatal("wrong target was accepted")
	}
}

func TestHelpViewSemanticViewportIsBounded(t *testing.T) {
	engine := NewHelpEngine(nil)
	lines := make([]string, 1000)
	for i := range lines {
		lines[i] = "A line"
	}
	engine.AddTopic(&HelpTopic{Name: "Long", StickyRows: 1, Lines: lines})
	view := NewHelpView(engine, "Long")
	view.ResizeConsole(80, 25)
	node := semanticFrame(&SemanticContext{}, view)
	if node["layout"] != "help" {
		t.Fatal("help semantic content missing")
	}
	handler := any(view).(SemanticActionHandler)
	target := SemanticID(view)
	handler.HandleSemanticAction(map[string]any{"target": target, "action": "help.viewport", "rows": 8})
	handler.HandleSemanticAction(map[string]any{"target": target, "action": "help.scroll", "position": 100000})
	node = semanticFrame(&SemanticContext{}, view)
	rows := node["helpLines"].([]map[string]any)
	if len(rows) != 8 || rows[0]["index"] != 0 || rows[7]["index"] != 999 {
		t.Fatalf("viewport/sticky rows were not bounded: %v", rows)
	}
	handler.HandleSemanticAction(map[string]any{"target": target, "action": "help.scroll", "position": -100})
	if view.scrollTop != 0 {
		t.Fatalf("negative scroll not clamped: %d", view.scrollTop)
	}
	view.ScrollToHelpLine(500)
	node = semanticFrame(&SemanticContext{}, view)
	rows = node["helpLines"].([]map[string]any)
	if rows[4]["index"] != 500 {
		t.Fatalf("search result not centered in native viewport: %v", rows)
	}
}
