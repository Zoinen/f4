package plughost

import (
	"bytes"
	"testing"

	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/vtui"
)

type retainedHistoryPresentation struct {
	PresentationAdapter
	state map[string]any
}

func (p retainedHistoryPresentation) RetainedMenuState(_ *vtui.SemanticContext, _ map[string]any) (map[string]any, bool) {
	return p.state, true
}

func TestHistoryNavigationWireRetainsCompleteAcceptedRows(t *testing.T) {
	rows := make([]map[string]any, 2500)
	for index := range rows {
		rows[index] = map[string]any{"index": index, "text": "command --long-argument"}
	}
	menu := map[string]any{"id": "history", "itemsRevision": uint64(1), "items": rows, "selected": 2499}
	previous := map[string]any{"schema": "app", "menus": []map[string]any{menu}}
	nextMenu := map[string]any{"id": "history", "itemsRevision": uint64(1), "items": rows, "selected": 2459}
	state := map[string]any{"menus": []map[string]any{nextMenu}}
	oldPresentation := Presentation
	Presentation = retainedHistoryPresentation{PresentationAdapter: oldPresentation, state: state}
	defer func() { Presentation = oldPresentation }()
	var wire bytes.Buffer
	renderer := &ExtUiRenderer{send: &extUiMessageSender{w: &wire}, lastScene: previous,
		lastCompactScene: previous, sceneRevision: 1}
	renderer.BeginSemanticSceneUpdate()
	if !renderer.SetSemanticMenuState(nil) {
		t.Fatal("navigation rejected")
	}
	renderer.EndSemanticSceneUpdate()
	if wire.Len() > 4096 {
		t.Fatalf("navigation packet: %d bytes", wire.Len())
	}
	message, err := extUiReadMessage(&wire)
	if err != nil {
		t.Fatal(err)
	}
	root := message["root"].(map[string]any)["set"].(map[string]any)
	encodedMenu := semantic.AppMapSlice(root["menus"])[0]
	if !semantic.Bool(encodedMenu["itemsUnchanged"]) || encodedMenu["items"] != nil {
		t.Fatal(encodedMenu)
	}
	for _, snapshot := range []map[string]any{renderer.lastScene, renderer.lastCompactScene} {
		retained := semantic.AppMapSlice(snapshot["menus"])[0]
		if len(semantic.AppMapSlice(retained["items"])) != len(rows) {
			t.Fatal("accepted rows lost")
		}
		if semantic.Int(retained["selected"]) != 2459 {
			t.Fatal("selection not committed")
		}
	}
}
