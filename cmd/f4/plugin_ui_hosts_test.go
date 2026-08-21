package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type promptSegmentProviderFunc func(vfs.VFS, string) (vfs.PromptSegment, bool)

func (f promptSegmentProviderFunc) PromptSegment(filesystem vfs.VFS, path string) (vfs.PromptSegment, bool) {
	return f(filesystem, path)
}

type fileDecorationProviderFunc func(vfs.VFS, string, vfs.VFSItem) (vfs.FileDecoration, bool)

func (f fileDecorationProviderFunc) DecorateFile(filesystem vfs.VFS, directory string, item vfs.VFSItem) (vfs.FileDecoration, bool) {
	return f(filesystem, directory, item)
}

func promptTextAndAttribute(cells []vtui.CharInfo, text string) (uint64, bool) {
	want := []rune(text)
	if len(want) == 0 {
		return 0, false
	}
	for start := range cells {
		if cells[start].Char != uint64(want[0]) {
			continue
		}
		matched := true
		for offset, char := range want {
			if start+offset >= len(cells) || cells[start+offset].Char != uint64(char) {
				matched = false
				break
			}
		}
		if matched {
			return cells[start].Attributes, true
		}
	}
	return 0, false
}

func promptString(cells []vtui.CharInfo) string {
	var text strings.Builder
	for _, cell := range cells {
		if cell.Char != vtui.WideCharFiller {
			text.WriteRune(rune(cell.Char))
		}
	}
	return text.String()
}

func TestPluginPanelHostSnapshotsObserverAndPassiveOpen(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer func() {
		pf.Close()
		panelObserverStates.Delete(pf)
	}()
	pf.ResizeConsole(100, 30)

	sourceRoot := t.TempDir()
	sourceChild := filepath.Join(sourceRoot, "child")
	if err := os.MkdirAll(sourceChild, 0o755); err != nil {
		t.Fatal(err)
	}
	source := vfs.NewOSVFS(sourceRoot)
	active := pf.Active().(*FileSystemPanel)
	active.vfs = source

	initial := pf.PanelSnapshot(vfs.PanelActive)
	if initial.Side != vfs.PanelActive || !sameVFSInstance(initial.VFS, source) || initial.Path != sourceRoot {
		t.Fatalf("active snapshot = %#v, want source VFS at %q", initial, sourceRoot)
	}

	var changes []vfs.PanelSnapshot
	registration := pf.ObservePanelChanges(func(snapshot vfs.PanelSnapshot) {
		changes = append(changes, snapshot)
	})
	pf.publishPanelSnapshots()
	if len(changes) != 2 {
		t.Fatalf("initial observer calls = %d, want active and passive snapshots", len(changes))
	}
	pf.publishPanelSnapshots()
	if len(changes) != 2 {
		t.Fatalf("unchanged snapshots called observer again: %#v", changes)
	}

	if err := source.SetPath(sourceChild); err != nil {
		t.Fatal(err)
	}
	pf.publishPanelSnapshots()
	if len(changes) != 3 || changes[2].Side != vfs.PanelActive || changes[2].Path != sourceChild {
		t.Fatalf("navigation notification = %#v, want active %q", changes, sourceChild)
	}

	destination := vfs.NewOSVFS(t.TempDir())
	if err := pf.OpenPassiveVFS(destination); err != nil {
		t.Fatalf("OpenPassiveVFS: %v", err)
	}
	passive := pf.PanelSnapshot(vfs.PanelPassive)
	if !sameVFSInstance(passive.VFS, destination) {
		t.Fatalf("passive snapshot VFS = %T %v, want destination", passive.VFS, passive.VFS)
	}
	if len(changes) != 4 || changes[3].Side != vfs.PanelPassive || !sameVFSInstance(changes[3].VFS, destination) {
		t.Fatalf("passive-open notification = %#v", changes)
	}

	registration.Unregister()
	if err := source.SetPath(sourceRoot); err != nil {
		t.Fatal(err)
	}
	pf.publishPanelSnapshots()
	if len(changes) != 4 {
		t.Fatalf("unregistered observer received %d notifications", len(changes))
	}
}

func TestPromptSegmentRegistryKeepsRegistrationsIndependent(t *testing.T) {
	api := &coreAPI{}
	first, err := api.RegisterPromptSegmentProvider(promptSegmentProviderFunc(func(vfs.VFS, string) (vfs.PromptSegment, bool) {
		return vfs.PromptSegment{Text: "[first]"}, true
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Unregister()
	second, err := api.RegisterPromptSegmentProvider(promptSegmentProviderFunc(func(vfs.VFS, string) (vfs.PromptSegment, bool) {
		return vfs.PromptSegment{Text: "[second]"}, true
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Unregister()

	segments := promptSegmentsSnapshot(nil, "")
	seen := make(map[string]bool, len(segments))
	for _, segment := range segments {
		seen[segment.Text] = true
	}
	if !seen["[first]"] || !seen["[second]"] {
		t.Fatalf("registered prompt segments = %#v, want both independent registrations", segments)
	}

	first.Unregister()
	segments = promptSegmentsSnapshot(nil, "")
	seen = make(map[string]bool, len(segments))
	for _, segment := range segments {
		seen[segment.Text] = true
	}
	if seen["[first]"] || !seen["[second]"] {
		t.Fatalf("unregistering first provider affected wrong registrations: %#v", segments)
	}
}

func TestPromptSegmentsRenderCachedValueWithCurrentPalette(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	oldBranchColor := vtui.Palette[ColGitPromptBranch]
	defer func() { vtui.Palette[ColGitPromptBranch] = oldBranchColor }()

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(160, 30)
	source := vfs.NewOSVFS(t.TempDir())
	pf.Active().(*FileSystemPanel).vfs = source

	const marker = "[git-cache-segment]"
	calls := 0
	var gotVFS vfs.VFS
	var gotPath string
	registration, err := (&coreAPI{}).RegisterPromptSegmentProvider(promptSegmentProviderFunc(func(filesystem vfs.VFS, path string) (vfs.PromptSegment, bool) {
		calls++
		gotVFS, gotPath = filesystem, path
		return vfs.PromptSegment{Text: marker, Color: vfs.PromptSegmentGitBranch}, true
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()

	firstPrompt := pf.buildPrompt()
	if calls != 1 || !sameVFSInstance(gotVFS, source) || gotPath != source.GetPath() {
		t.Fatalf("prompt provider was not queried with cached panel identity: calls=%d vfs=%T path=%q", calls, gotVFS, gotPath)
	}
	if attr, ok := promptTextAndAttribute(firstPrompt, marker); !ok || attr != vtui.Palette[ColGitPromptBranch] {
		t.Fatalf("first prompt marker attr = %#x, present=%v, want %#x", attr, ok, vtui.Palette[ColGitPromptBranch])
	}

	newBranchColor := vtui.SetRGBBoth(0, 0x112233, 0x445566)
	vtui.Palette[ColGitPromptBranch] = newBranchColor
	secondPrompt := pf.buildPrompt()
	if calls != 2 {
		t.Fatalf("prompt redraw did not query cache provider exactly once: calls=%d", calls)
	}
	if attr, ok := promptTextAndAttribute(secondPrompt, marker); !ok || attr != newBranchColor {
		t.Fatalf("prompt kept stale semantic colour: attr=%#x, present=%v, want %#x", attr, ok, newBranchColor)
	}

	registration.Unregister()
	if got := promptString(pf.buildPrompt()); strings.Contains(got, marker) {
		t.Fatalf("unregistered prompt segment still rendered: %q", got)
	}
}

func TestFileDecorationRegistryMapsCachedPresentationAndKeepsRegistrationsIndependent(t *testing.T) {
	vtui.SetDefaultPalette()
	SetDefaultF4Palette()
	oldUnstagedColor := vtui.Palette[ColGitUnstaged]
	defer func() { vtui.Palette[ColGitUnstaged] = oldUnstagedColor }()

	api := &coreAPI{}
	source := vfs.NewOSVFS(t.TempDir())
	calls := 0
	first, err := api.RegisterFileDecorationProvider(fileDecorationProviderFunc(func(filesystem vfs.VFS, directory string, item vfs.VFSItem) (vfs.FileDecoration, bool) {
		calls++
		if !sameVFSInstance(filesystem, source) || directory != "work" || item.Name != "tracked.go" {
			t.Errorf("decoration input = (%T, %q, %q), want source/work/tracked.go", filesystem, directory, item.Name)
		}
		return vfs.FileDecoration{
			Prefix:     "~",
			Color:      vfs.FileDecorationGitUnstaged,
			Attributes: map[string]string{"git.worktree": "modified"},
		}, true
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer first.Unregister()
	second, err := api.RegisterFileDecorationProvider(fileDecorationProviderFunc(func(vfs.VFS, string, vfs.VFSItem) (vfs.FileDecoration, bool) {
		return vfs.FileDecoration{Attributes: map[string]string{"provider.two": "present"}}, true
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer second.Unregister()

	original := vfs.VFSItem{Name: "tracked.go", ExtendedAttributes: map[string]string{"existing": "value"}}
	decorated := decorateVFSItem(source, "work", original)
	if calls != 1 {
		t.Fatalf("cache provider calls = %d, want 1", calls)
	}
	if decorated.Name != original.Name || decorated.DisplayName != "~ tracked.go" || !decorated.NoExtension {
		t.Fatalf("decoration changed operation identity or display mapping: %#v", decorated)
	}
	for key, want := range map[string]string{
		"existing":                   "value",
		"git.worktree":               "modified",
		"provider.two":               "present",
		fileDecorationColorAttribute: "2",
	} {
		if got := decorated.ExtendedAttributes[key]; got != want {
			t.Errorf("extended attribute %q = %q, want %q", key, got, want)
		}
	}
	if original.DisplayName != "" || original.NoExtension || len(original.ExtendedAttributes) != 1 {
		t.Fatalf("source VFS item was mutated: %#v", original)
	}

	base := vtui.SetRGBBoth(0, 0xAABBCC, 0x101112)
	if got, want := fileDecorationAttr(&decorated, base), vtui.SetRGBFore(base, vtui.GetRGBFore(vtui.Palette[ColGitUnstaged])); got != want {
		t.Fatalf("unstaged decoration attr = %#x, want %#x", got, want)
	}
	if got := (&fileEntry{VFSItem: decorated}).GetCellAttr(0, base); got != fileDecorationAttr(&decorated, base) {
		t.Fatalf("rendering did not use the cached decoration attr: got %#x", got)
	}
	if calls != 1 {
		t.Fatalf("file row render synchronously re-queried provider: calls=%d", calls)
	}

	newUnstagedColor := vtui.SetRGBBoth(0, 0x223344, 0x556677)
	vtui.Palette[ColGitUnstaged] = newUnstagedColor
	if got, want := fileDecorationAttr(&decorated, base), vtui.SetRGBFore(base, vtui.GetRGBFore(newUnstagedColor)); got != want {
		t.Fatalf("decoration kept stale semantic colour: got %#x, want %#x", got, want)
	}

	first.Unregister()
	withoutFirst := decorateVFSItem(source, "work", original)
	if withoutFirst.DisplayName != "" || withoutFirst.ExtendedAttributes["provider.two"] != "present" || withoutFirst.ExtendedAttributes["git.worktree"] != "" {
		t.Fatalf("unregistering first decoration affected independent provider: %#v", withoutFirst)
	}
}
