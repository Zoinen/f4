package vtvibe

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Dialogs on disk (unxed/f4#1842, docs/VTVIBE.md § 19a, stage H3, first
// step): the dialog used to live only in memory and was gone with f4. With a
// store path set, the session writes itself to disk after every change that
// matters (a new message, a name, a reset, the patch mode) and reads itself
// back when f4 starts again. Detaching and picking up a dialog from another
// f4 is the next step.

const storeVersion = 1

type savedDialog struct {
	Version   int               `json:"version"`
	Title     string            `json:"title,omitempty"`
	PatchMode bool              `json:"patch_mode,omitempty"`
	Turns     []Turn            `json:"turns"`
	Context   map[string][]byte `json:"context,omitempty"` // ctx/ files by path
	Draft     string            `json:"draft,omitempty"`
}

// SetStorePath makes path the dialog's file: a dialog saved there earlier is
// restored now, and every later change is written back. An empty path stops
// saving.
func (s *Session) SetStorePath(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storePath = ""
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path) // #nosec G304 -- the host's own file in the f4 config directory
	switch {
	case err == nil:
		var d savedDialog
		if err := json.Unmarshal(data, &d); err != nil {
			s.storePath = path
			return fmt.Errorf("vtvibe: %s is damaged, starting a new dialog: %w", path, err)
		}
		s.restoreLocked(d)
	case !os.IsNotExist(err):
		return err
	}
	s.storePath = path
	return nil
}

// restoreLocked replaces the dialog with d. Caller holds s.mu.
func (s *Session) restoreLocked(d savedDialog) {
	s.treeMu.Lock()
	s.reset()
	s.turns = nil
	for _, t := range d.Turns {
		s.appendTurn(t)
	}
	for p, data := range d.Context {
		clean := "/" + strings.TrimLeft(filepathToSlash(p), "/")
		if strings.HasPrefix(clean, ctxDir+"/") && !strings.Contains(clean, "/../") {
			_ = s.tree.writeFile(clean, data)
		}
	}
	if d.Draft != "" {
		_ = s.tree.writeFile(draftFile, []byte(d.Draft))
	}
	s.treeMu.Unlock()
	s.title = d.Title
	s.apMode = d.PatchMode
	s.writeSessionFile()
}

func filepathToSlash(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// saveLocked writes the dialog to its store path, if it has one. A failed
// write is kept for StoreError and retried with the next change. Caller holds
// s.mu.
func (s *Session) saveLocked() {
	if s.storePath == "" {
		return
	}
	d := savedDialog{Version: storeVersion, Title: s.title, PatchMode: s.apMode, Turns: s.turns}
	for _, p := range s.tree.walkFiles(ctxDir) {
		if data, ok := s.tree.readFile(p); ok {
			if d.Context == nil {
				d.Context = map[string][]byte{}
			}
			d.Context[p] = data
		}
	}
	if draft, ok := s.tree.readFile(draftFile); ok && string(draft) != draftTemplate {
		d.Draft = string(draft)
	}
	s.storeErr = writeJSONAtomically(s.storePath, d)
}

// StoreError is the last failure to save the dialog, nil when it is saved.
func (s *Session) StoreError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.storeErr
}

// Archive copies the saved dialog into dir under a name made of the time and
// the dialog's title, and returns that path; "" when there is nothing saved.
func (s *Session) Archive(dir string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storePath == "" || len(s.turns) <= 1 {
		return "", nil
	}
	s.saveLocked()
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return "", err
	}
	name := time.Now().Format("2006-01-02_150405")
	if slug := archiveSlug(s.title); slug != "" {
		name += "_" + slug
	}
	target := filepath.Join(dir, name+".json")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return target, os.WriteFile(target, data, 0o600)
}

func archiveSlug(title string) string {
	var sb strings.Builder
	for _, r := range title {
		switch {
		case r == ' ' || r == '-' || r == '_':
			sb.WriteRune('-')
		case strings.ContainsRune(`/\:*?"<>|`, r) || r < ' ':
		default:
			sb.WriteRune(r)
		}
		if sb.Len() >= 40 {
			break
		}
	}
	return strings.Trim(sb.String(), "-")
}

func writeJSONAtomically(path string, v any) error {
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".dialog-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}
