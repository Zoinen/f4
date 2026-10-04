package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
)

func TestFolderHistoryActionsWithOverflowingNames(t *testing.T) {
	for _, backend := range []string{"qt", "win32"} {
		t.Run(backend, func(t *testing.T) {
			setupPortableIni(t, "0")
			t.Cleanup(paneltest.SwapFrameManager(t))
			previousBackend, previousDetails := vtui.ActiveBackend(), vtui.BackendDetails()
			t.Cleanup(func() { vtui.SetActiveBackend(previousBackend, previousDetails...) })
			vtui.SetActiveBackend(backend)

			root := t.TempDir()
			newest := filepath.Join(root, "newest")
			older := filepath.Join(root, "older")
			for _, path := range []string{newest, older} {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
				name := strings.Repeat("long-name-", 12) + ".txt"
				if err := os.WriteFile(filepath.Join(path, name), nil, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			pf := seedPanelForCopyName(t, newest)
			fsp := pf.GetActivePanel()
			fsp.ReadDirectory()
			paneltest.WaitForLoad(t, fsp)
			if !fsp.NamesOverflow() {
				t.Fatal("fixture must have a filename wider than its terminal column")
			}
			provider := history.NewProviderAtPath(filepath.Join(root, "history.json"))
			provider.SaveHistory("folders", []string{newest, older})
			previousProvider := vtui.GlobalHistoryProvider
			t.Cleanup(func() { vtui.GlobalHistoryProvider = previousProvider })
			vtui.GlobalHistoryProvider = provider
			pf.FolderHistoryPos = [2]int{-1, -1}
			fsp.SetNameLeftPos(1)

			for _, step := range []struct {
				action string
				want   string
			}{
				{action: "Panel.HistoryBack", want: older},
				{action: "Panel.HistoryForward", want: newest},
			} {
				if !fsp.NamesOverflow() {
					t.Fatalf("%s fixture no longer has overflowing names", step.action)
				}
				if !RunAction(step.action) {
					t.Fatalf("%s was not handled", step.action)
				}
				paneltest.WaitForLoad(t, fsp)
				want := newest
				if backend == "qt" {
					want = step.want
				}
				if !fileops.SameFolderHistoryPath(fsp.Vfs.GetPath(), want) {
					t.Fatalf("%s path = %q, want %q", step.action, fsp.Vfs.GetPath(), want)
				}
			}
			if backend == "win32" && !fsp.SetNameLeftPos(0) {
				t.Fatal("terminal Alt+Right must retain filename scrolling")
			}
		})
	}
}
