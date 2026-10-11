package ap

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func pathExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func isDirPath(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func osLineSep() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

// detectLineEndings sniffs the first 1KB of an existing file for its
// dominant line ending, defaulting to the OS convention when none is found.
func detectLineEndings(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return osLineSep()
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, 1024)
	n, _ := f.Read(buf)
	chunk := buf[:n]
	switch {
	case bytes.Contains(chunk, []byte("\r\n")):
		return "\r\n"
	case bytes.Contains(chunk, []byte("\n")):
		return "\n"
	case bytes.Contains(chunk, []byte("\r")):
		return "\r"
	}
	return osLineSep()
}

func lastPathComponent(p string) string {
	p = strings.TrimRight(p, "/")
	idx := strings.LastIndex(p, "/")
	if idx == -1 {
		return p
	}
	return p[idx+1:]
}

// resolvePathPrefix implements the reference's "path search heuristic":
// when the declared relative path does not exist verbatim under projectDir,
// try stripping a redundant leading project-name-like prefix a model often
// prepends (e.g. "my_project/src/x.txt" when the project root already IS
// "my_project"). Returns the (possibly adjusted) relative path and the
// prefix that was stripped, if any.
func resolvePathPrefix(projectDir, relativePath string) (resolved string, strippedPrefix string) {
	if pathExists(filepath.Join(projectDir, relativePath)) {
		return relativePath, ""
	}
	parts := strings.Split(strings.ReplaceAll(relativePath, "\\", "/"), "/")
	if len(parts) <= 1 {
		return relativePath, ""
	}
	topPart := parts[0]
	if pathExists(filepath.Join(projectDir, topPart)) {
		return relativePath, ""
	}

	for i := 1; i < len(parts); i++ {
		testPath := strings.Join(parts[i:], "/")
		if pathExists(filepath.Join(projectDir, testPath)) {
			return testPath, strings.Join(parts[:i], "/")
		}
	}

	for i := 1; i < len(parts); i++ {
		prefixPath := filepath.Join(projectDir, filepath.Join(parts[:i]...))
		targetDirPath := filepath.Join(projectDir, parts[i])
		if !pathExists(prefixPath) && isDirPath(targetDirPath) {
			return strings.Join(parts[i:], "/"), strings.Join(parts[:i], "/")
		}
	}

	projectDirAbs, err := filepath.Abs(projectDir)
	if err == nil {
		projectDirAbs = strings.ReplaceAll(projectDirAbs, "\\", "/")
		projectDirName := lastPathComponent(projectDirAbs)
		for i := len(parts) - 1; i >= 1; i-- {
			prefix := strings.Join(parts[:i], "/")
			if strings.HasSuffix(projectDirAbs, "/"+prefix) || projectDirAbs == prefix ||
				(len(prefix) >= 2 && (strings.HasPrefix(projectDirName, prefix) || strings.HasPrefix(prefix, projectDirName))) {
				return strings.Join(parts[i:], "/"), prefix
			}
		}
	}

	return relativePath, ""
}

// securePath resolves relativePath against projectDir and rejects any path
// that would (after lexical cleaning of ".."/"." segments) escape it. Unlike
// a realpath-based check it does not require the target to already exist,
// which CREATE needs.
func securePath(projectDir, relativePath string) (string, error) {
	if filepath.IsAbs(relativePath) {
		return "", fmt.Errorf("path traversal detected")
	}
	cleanRel := filepath.Clean(relativePath)
	full := filepath.Join(projectDir, cleanRel)

	absProject, err := filepath.Abs(projectDir)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absProject, absFull)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal detected")
	}
	return full, nil
}
