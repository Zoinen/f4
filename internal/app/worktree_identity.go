package app

import (
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"strings"
)

// A linked checkout has its own HEAD file. Read that file at runtime so a
// branch switch cannot leave a build-time name or another checkout's identity.
func worktreeBranchAt(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, ".git"))
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir: ") {
		return ""
	}
	gitdir := strings.TrimPrefix(line, "gitdir: ")
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(dir, gitdir)
	}
	root, err := os.OpenRoot(gitdir)
	if err != nil {
		return ""
	}
	defer func() { _ = root.Close() }()
	head, err := root.ReadFile("HEAD")
	if err != nil {
		return ""
	}
	ref := strings.TrimSpace(string(head))
	if !strings.HasPrefix(ref, "ref: refs/heads/") {
		return ""
	}
	return strings.TrimPrefix(ref, "ref: refs/heads/")
}
func drawWorktreeIdentity(scr *vtui.ScreenBuf) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	branch := worktreeBranchAt(filepath.Dir(exe))
	if branch == "" {
		return
	}
	fm := vtui.FrameManager
	if fm == nil {
		return
	}
	if top := fm.GetTopFrame(); top != nil && top.IsModal() {
		return
	}
	if fm.MenuBar != nil && fm.MenuBar.Active {
		return
	}
	width := fm.GetScreenSize()
	text := vtui.TruncateString(branch, max(1, width-4), "…")
	// The workspace strip is painted after OnRender and owns its reserved row.
	// Use the application's top header, below that strip when it is visible.
	scr.Write(max(0, (width-vtui.StringWidth(text))/2), fm.WorkspaceTopInset(), vtui.StringToCharInfo(text, vtui.Palette[vtui.ColMenuBarItem]))
}
