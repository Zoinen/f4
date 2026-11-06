package theme

import (
	"encoding/xml"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/filegroups"
)

func readFileColorThemeData(t *testing.T) *filegroups.Document {
	t.Helper()
	data, err := os.ReadFile("../filegroups/colors.default.ini")
	if err != nil {
		t.Fatal(err)
	}
	document, err := filegroups.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func TestCurrentFileColorThemeExtractedSnapshot(t *testing.T) {
	// Fixed extraction from the saved 46-group snapshot, with the former Qt
	// folder blue moved into Current's normal state. Tests never open a profile.
	colors := map[string]string{
		"archives": "#CB78EB", "audio": "#68A318", "books": "#88C7E1",
		"debug-symbols": "#ECE9D8", "executables": "#72EE48", "files": "#AAAAAA",
		"folders": "#5AB2F1", "hidden-directories": "#6E6E6E", "hidden-files": "#5A5A5A",
		"images": "#FF9238", "libraries": "#BEB9FF", "parent-folder": "#5AB2F1",
		"projects": "#BEB9FF", "scripts": "#72EE48", "source-code": "#68A318",
		"symbolic-links": "#66D9EF", "system-files": "#ECE9D8", "temporary-files": "#A39276",
		"text-files": "#ECE9D8", "video": "#AAF046", "web-pages": "#0082FF",
	}
	want := map[string]string{"Name": "Current"}
	for name, color := range colors {
		prefix := "Color." + name + "."
		want[prefix+"NormalColor"] = "foreground:" + color
		want[prefix+"CursorColor"] = "foreground:" + color
		want[prefix+"SelectedColor"] = "foreground:#000000"
		want[prefix+"SelectedCursorColor"] = "foreground:#000000"
	}
	want["Color.folders.CursorColor"] = "foreground:#FFFFFF"
	want["Color.parent-folder.CursorColor"] = "foreground:#B4B4B4"
	for _, key := range []string{"Color.libraries.SelectedColor", "Color.libraries.SelectedCursorColor",
		"Color.symbolic-links.SelectedColor", "Color.symbolic-links.SelectedCursorColor", "Color.symbolic-links.CursorColor"} {
		want[key] = ""
	}
	got := readFileColorThemeData(t).Section("FileColorTheme.current")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("current colour extraction differs\ngot: %#v\nwant: %#v", got, want)
	}
}

func TestZoinFileColorThemeEverySVG(t *testing.T) {
	paths, err := filepath.Glob("../../qt/host/icons/zoin/*.svg")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no Zoin SVGs found; artwork coverage must not silently pass")
	}
	want := map[string]string{"Name": "Zoin"}
	hexColor := regexp.MustCompile(`#[[:xdigit:]]{3,8}\b`)
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			decoder := xml.NewDecoder(strings.NewReader(string(data)))
			for {
				_, err := decoder.Token()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			colors := map[string]bool{}
			for _, color := range hexColor.FindAllString(string(data), -1) {
				colors[opaqueSVGColor(t, color)] = true
			}
			if len(colors) != 1 {
				t.Fatalf("multiple or missing hues require an extraction decision: %v", colors)
			}
			name := strings.TrimSuffix(filepath.Base(path), ".svg")
			if name != filegroups.IconName(name) {
				t.Fatalf("SVG basename is not a normalized group name: %s", name)
			}
			for color := range colors {
				want["Color."+name+".NormalColor"] = "foreground:" + color
			}
		})
	}
	got := readFileColorThemeData(t).Section("FileColorTheme.zoin")
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Zoin data differs from every SVG's base RGB\ngot: %#v\nwant: %#v", got, want)
	}
}

func opaqueSVGColor(t *testing.T, raw string) string {
	t.Helper()
	color := strings.ToUpper(strings.TrimPrefix(raw, "#"))
	switch len(color) {
	case 3, 4:
		color = string([]byte{color[0], color[0], color[1], color[1], color[2], color[2]})
	case 6, 8:
		color = color[:6]
	default:
		t.Fatalf("unsupported SVG colour %q", raw)
	}
	return "#" + color
}

func TestFileColorThemesContainOnlyGroupColors(t *testing.T) {
	document := readFileColorThemeData(t)
	if got := document.Section("FileGroups"); !reflect.DeepEqual(got, map[string]string{"Version": "3"}) {
		t.Fatal(got)
	}
	allowed := map[string]bool{"NormalColor": true, "SelectedColor": true, "CursorColor": true, "SelectedCursorColor": true}
	sections := document.SectionNames()
	if len(sections) != 3 {
		t.Fatal("unexpected sections", sections)
	}
	for _, id := range []string{"current", "zoin"} {
		for key := range document.Section("FileColorTheme." + id) {
			if key == "Name" {
				continue
			}
			parts := strings.Split(key, ".")
			if len(parts) != 3 || parts[0] != "Color" || parts[1] != filegroups.IconName(parts[1]) || !allowed[parts[2]] {
				t.Fatalf("non-colour theme field: %s", key)
			}
		}
	}
}
