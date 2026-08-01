package main

import (
	"context"
	_ "embed"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/unxed/vtui"
)

//go:embed help/en.hlf
var defaultHelpData string

//go:embed README.md
var readmeData string

type memoryHelpVFS struct {
	files map[string]string
}

func (m *memoryHelpVFS) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	content, ok := m.files[path]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

var mdLinkRegex = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)

func convertMarkdownLinks(line string) string {
	return mdLinkRegex.ReplaceAllString(line, "~$1~$2@")
}

func parseMarkdownToHelpTopic(name string, mdContent string) *vtui.HelpTopic {
	topic := &vtui.HelpTopic{
		Name: name,
	}
	lines := strings.Split(mdContent, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			level := 0
			for level < len(trimmed) && trimmed[level] == '#' {
				level++
			}
			headerText := strings.TrimSpace(trimmed[level:])
			line = "#" + headerText + "#"
			if level == 1 && topic.StickyRows == 0 {
				topic.StickyRows = 1
				topic.Lines = append([]string{headerText}, topic.Lines...)
				continue
			}
			line = convertMarkdownLinks(line)
			topic.Lines = append(topic.Lines, line)
			continue
		}

		wrapped := vtui.WrapText(line, 70)
		for _, wLine := range wrapped {
			wLine = convertMarkdownLinks(wLine)
			topic.Lines = append(topic.Lines, wLine)
		}
	}
	return topic
}

func InitHelpSystem() {
	files := map[string]string{}

	versionedEnglishHelp := strings.ReplaceAll(defaultHelpData, "%Ver", getLongVersionInfo())
	files["help.hlf"] = versionedEnglishHelp

	lang := AppConfig.Language
	if lang == "" {
		lang = "en"
	}

	hasLocalHelp := false
	if lang != "en" && lang != "eng" {
		exeDir := filepath.Dir(os.Args[0])
		userDir := filepath.Join(GetF4ConfigDir(), "help")

		candidates := []string{
			filepath.Join(userDir, lang+".hlf"),
			filepath.Join(exeDir, "help", lang+".hlf"),
			filepath.Join("help", lang+".hlf"), // Fallback for "go run ." development
		}

		var helpContent string
		for _, cand := range candidates {
			if data, err := os.ReadFile(cand); err == nil {
				helpContent = string(data)
				vtui.DebugLog("HELP: Loaded language help file from disk: %s", cand)
				break
			}
		}

		if helpContent != "" {
			versionedHelp := strings.ReplaceAll(helpContent, "%Ver", getLongVersionInfo())
			files["help_local.hlf"] = versionedHelp
			hasLocalHelp = true
		}
	}

	v := &memoryHelpVFS{
		files: files,
	}
	vtui.GlobalHelpEngine = vtui.NewHelpEngine(v)

	_ = vtui.GlobalHelpEngine.LoadFile("help.hlf")

	if hasLocalHelp {
		_ = vtui.GlobalHelpEngine.LoadFile("help_local.hlf")
	}

	readmeTopic := parseMarkdownToHelpTopic("README", readmeData)
	vtui.GlobalHelpEngine.AddTopic(readmeTopic)
}
