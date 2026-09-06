package vtui

import "testing"

type keyBarLabelFrame struct {
	mockFrame
	labels *KeySet
	shows  int
}

func (frame *keyBarLabelFrame) GetKeyLabels() *KeySet { return frame.labels }
func (frame *keyBarLabelFrame) Show(*ScreenBuf)       { frame.shows++ }

type keyBarOwnerFrame struct {
	keyBarLabelFrame
	bar *KeyBar
}

func (frame *keyBarOwnerFrame) GetKeyBar() *KeyBar { return frame.bar }

type keyBarTransitionObserver struct {
	semanticTransitionTestRenderer
	manager *frameManager
	labels  []string
}

func (renderer *keyBarTransitionObserver) SetSemanticSceneTransition(ctx *SemanticContext) bool {
	label := ""
	if renderer.manager.KeyBar != nil {
		label = renderer.manager.KeyBar.Normal[0]
	}
	renderer.labels = append(renderer.labels, label)
	return renderer.semanticTransitionTestRenderer.SetSemanticSceneTransition(ctx)
}

func TestDirectDocumentTransitionsRefreshKeyBarWithoutPainting(t *testing.T) {
	previous := FrameManager
	manager := &frameManager{}
	screen := NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	manager.Init(screen)
	FrameManager = manager
	t.Cleanup(func() { manager.Shutdown(); FrameManager = previous })
	renderer := &keyBarTransitionObserver{manager: manager}
	screen.Renderer = renderer
	panels := &keyBarOwnerFrame{bar: NewKeyBar()}
	panels.labels = &KeySet{Normal: KeyBarLabels{"Panels"}, NormalIcons: KeyBarIconNames{"folder"}}
	manager.Push(panels)
	manager.refreshKeyBarState()
	viewer := &keyBarLabelFrame{labels: &KeySet{Normal: KeyBarLabels{"Viewer"}, NormalIcons: KeyBarIconNames{"file"}}}
	before := manager.semanticMenuInputState()
	manager.AddScreen(viewer)
	if !manager.publishSemanticSceneTransition(before) {
		t.Fatal("viewer open was not a direct semantic transition")
	}
	before = manager.semanticMenuInputState()
	manager.CloseActiveScreen()
	if !manager.publishSemanticSceneTransition(before) {
		t.Fatal("viewer close was not a direct semantic transition")
	}
	if len(renderer.labels) != 2 || renderer.labels[0] != "Viewer" || renderer.labels[1] != "Panels" {
		t.Fatalf("transition labels=%v", renderer.labels)
	}
	if manager.KeyBar != panels.bar || manager.KeyBar.NormalIcons[0] != "folder" ||
		panels.shows != 0 || viewer.shows != 0 || renderer.renders != 0 {
		t.Fatal("keybar restoration changed owner/icons or painted hidden cells")
	}
	// A modal label override does not replace the workspace's keybar, and
	// HideBars still wins without needing Show/Hide's rendering side effects.
	modal := &keyBarLabelFrame{labels: &KeySet{Normal: KeyBarLabels{"Dialog"}}}
	manager.Push(modal)
	manager.HideBars = true
	manager.refreshKeyBarState()
	if manager.KeyBar != panels.bar || manager.KeyBar.Normal[0] != "Dialog" || manager.KeyBar.IsVisible() {
		t.Fatal("modal labels or HideBars were lost")
	}
	manager.HideBars = false
	manager.refreshKeyBarState()
	if !manager.KeyBar.IsVisible() {
		t.Fatal("bar did not return after HideBars was cleared")
	}
	panels.bar = nil
	manager.refreshKeyBarState()
	if manager.KeyBar != nil {
		t.Fatal("explicit owner suppression was ignored")
	}
}

type activationResizeFrame struct {
	mockFrame
	manager        *frameManager
	activeAtResize []bool
}

func (frame *activationResizeFrame) ResizeConsole(width, height int) {
	active := false
	for _, candidate := range frame.manager.GetActiveFrames(frame.manager.ActiveIdx) {
		if candidate == frame {
			active = true
		}
	}
	frame.activeAtResize = append(frame.activeAtResize, active)
	frame.mockFrame.ResizeConsole(width, height)
}

func TestDocumentTabInsetResizeSeesOutgoingWorkspaceAlreadyHidden(t *testing.T) {
	manager := &frameManager{}
	screen := NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	manager.Init(screen)
	defer manager.Shutdown()
	manager.ConfigureWorkspaceTabs(WorkspaceTabsMultiple, WorkspaceCtrlTabDirect)
	outgoing := &activationResizeFrame{manager: manager}
	manager.Push(outgoing)
	incoming := &activationResizeFrame{manager: manager}
	manager.AddScreen(incoming)
	if len(outgoing.activeAtResize) != 1 || outgoing.activeAtResize[0] {
		t.Fatalf("outgoing visibility during inset resize = %v", outgoing.activeAtResize)
	}
	if len(incoming.activeAtResize) != 1 || !incoming.activeAtResize[0] {
		t.Fatalf("incoming visibility during inset resize = %v", incoming.activeAtResize)
	}
}
