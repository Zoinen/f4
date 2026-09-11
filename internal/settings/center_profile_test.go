package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

// These diagnostic benchmarks exercise the settings owner without a live Qt
// process. The test harness uses its isolated profile directory and never saves
// a draft. GUI replay fixtures are opt-in through F4_SETTINGS_PROFILE_DIR.
func settingsProfileSetup(t testing.TB) {
	t.Helper()
	oldConfig, oldCategory, oldOffsets := config.App, lastSettingsCategory, lastSettingsOffsets
	config.App.Language, config.App.HelpLanguage = "en", "en"
	config.App.UseLocalLanguageFiles = false
	lastSettingsCategory, lastSettingsOffsets = "", nil
	InitLang()
	t.Cleanup(func() {
		config.App, lastSettingsCategory, lastSettingsOffsets = oldConfig, oldCategory, oldOffsets
		InitLang()
	})
}

func settingsProfileBegin(t testing.TB) []*settingsSession {
	t.Helper()
	sessions, err := beginSettingsSessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return sessions
}

func settingsProfileClose(sessions []*settingsSession) {
	for _, session := range sessions {
		session.draft.Close()
	}
}

func settingsProfileCenter(t testing.TB) (*settingsCenter, *vtui.ScreenBuf, *vtui.SemanticContext) {
	t.Helper()
	sessions := settingsProfileBegin(t)
	t.Cleanup(func() { settingsProfileClose(sessions) })
	center := newSettingsCenter(sessions)
	center.SetPosition(0, 0, 99, 49)
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(160, 60)
	center.Show(screen)
	return center, screen, &vtui.SemanticContext{Width: 160, Height: 60}
}

func settingsProfileExport(t testing.TB, center *settingsCenter, ctx *vtui.SemanticContext) []byte {
	t.Helper()
	data, err := msgpack.Marshal(center.SemanticNode(ctx))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func settingsProfileHover(t testing.TB, center *settingsCenter, index int) {
	t.Helper()
	id := []string{"Language", "GuiFont"}[index%2]
	if !center.HandleSemanticAction(map[string]any{
		"target": "id:setting:" + id, "action": "control.explain",
	}) {
		t.Fatalf("hover of %s was not handled", id)
	}
}

func BenchmarkSettingsSemanticProfile(b *testing.B) {
	settingsProfileSetup(b)
	for _, operation := range []string{"CoreCatalog", "BeginSessions", "Open", "Hover", "Category", "Resize", "Show", "Semantic", "Msgpack", "JSON"} {
		b.Run(operation, func(b *testing.B) {
			center, screen, ctx := settingsProfileCenter(b)
			node := center.SemanticNode(ctx)
			b.ReportAllocs()
			b.ResetTimer()
			pprof.Do(context.Background(), pprof.Labels("settings-operation", operation), func(context.Context) {
				for i := 0; i < b.N; i++ {
					switch operation {
					case "CoreCatalog":
						_ = (coreSettingsProvider{}).Catalog()
					case "BeginSessions":
						sessions := settingsProfileBegin(b)
						settingsProfileClose(sessions)
					case "Open":
						sessions := settingsProfileBegin(b)
						opened := newSettingsCenter(sessions)
						opened.ResizeConsole(160, 60)
						opened.Show(screen)
						_ = settingsProfileExport(b, opened, ctx)
						settingsProfileClose(sessions)
					case "Hover":
						settingsProfileHover(b, center, i)
						center.Show(screen)
						_ = settingsProfileExport(b, center, ctx)
					case "Category":
						center.selectCategory([]string{"startup", "operations", "editor", "appearance"}[i%4])
						center.Show(screen)
						_ = settingsProfileExport(b, center, ctx)
					case "Resize":
						if i%2 == 0 {
							center.SetPosition(0, 0, 129, 44)
						} else {
							center.SetPosition(0, 0, 99, 49)
						}
						center.Show(screen)
						_ = settingsProfileExport(b, center, ctx)
					case "Show":
						center.Show(screen)
					case "Semantic":
						_ = center.SemanticNode(ctx)
					case "Msgpack":
						if _, err := msgpack.Marshal(node); err != nil {
							b.Fatal(err)
						}
					case "JSON":
						if _, err := json.Marshal(node); err != nil {
							b.Fatal(err)
						}
					}
				}
			})
		})
	}
}

type settingsProfileFixture struct {
	Name         string             `json:"name"`
	Category     string             `json:"category"`
	JSONBytes    int                `json:"jsonBytes"`
	MsgpackBytes int                `json:"msgpackBytes"`
	WidgetCount  int                `json:"widgetCount"`
	FieldRows    int                `json:"fieldRows"`
	KindCounts   map[string]int     `json:"kindCounts"`
	StageMillis  map[string]float64 `json:"stageMillis"`
	HelpText     string             `json:"helpText"`
}

func TestSettingsProfileFixtures(t *testing.T) {
	directory := os.Getenv("F4_SETTINGS_PROFILE_DIR")
	if directory == "" {
		t.Skip("set F4_SETTINGS_PROFILE_DIR to export real dialog replay fixtures")
	}
	settingsProfileSetup(t)
	if err := os.MkdirAll(directory, 0700); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	sessions := settingsProfileBegin(t)
	t.Cleanup(func() { settingsProfileClose(sessions) })
	beginMillis := float64(time.Since(started)) / float64(time.Millisecond)
	started = time.Now()
	center := newSettingsCenter(sessions)
	center.SetPosition(0, 0, 99, 49)
	createMillis := float64(time.Since(started)) / float64(time.Millisecond)
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(160, 60)
	ctx := &vtui.SemanticContext{Width: 160, Height: 60}
	fixtures := []settingsProfileFixture{}
	for _, name := range []string{"appearance", "hover-a", "hover-b", "startup", "operations", "editor", "resized"} {
		measurement := settingsProfileFixture{Name: name, StageMillis: map[string]float64{}, KindCounts: map[string]int{}}
		if name == "appearance" {
			measurement.StageMillis["beginSessions"] = beginMillis
			measurement.StageMillis["constructAndLayout"] = createMillis
		}
		started = time.Now()
		switch name {
		case "hover-a":
			settingsProfileHover(t, center, 0)
		case "hover-b":
			settingsProfileHover(t, center, 1)
		case "startup", "operations", "editor":
			center.selectCategory(name)
		case "resized":
			center.SetPosition(0, 0, 129, 44)
		}
		measurement.StageMillis["action"] = float64(time.Since(started)) / float64(time.Millisecond)
		started = time.Now()
		center.Show(screen)
		measurement.StageMillis["show"] = float64(time.Since(started)) / float64(time.Millisecond)
		started = time.Now()
		node := center.SemanticNode(ctx)
		measurement.StageMillis["semantic"] = float64(time.Since(started)) / float64(time.Millisecond)
		started = time.Now()
		compact, err := json.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		measurement.JSONBytes = len(compact)
		measurement.StageMillis["json"] = float64(time.Since(started)) / float64(time.Millisecond)
		started = time.Now()
		binary, err := msgpack.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		measurement.MsgpackBytes = len(binary)
		measurement.StageMillis["msgpack"] = float64(time.Since(started)) / float64(time.Millisecond)
		for _, widget := range settingsSemanticNodes(node) {
			measurement.WidgetCount++
			kind, _ := widget["kind"].(string)
			measurement.KindCounts[kind]++
		}
		measurement.Category, measurement.HelpText = center.category, center.help.text
		measurement.FieldRows = len(center.page.rows)
		// Pointer identities are transport details. Replace only those in the
		// exported fixture with deterministic references; live controls are untouched.
		stable := settingsProfileStableValue(node, "dialog", map[string]string{})
		pretty, err := json.MarshalIndent(stable, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, name+".json"), pretty, 0600); err != nil {
			t.Fatal(err)
		}
		fixtures = append(fixtures, measurement)
		t.Logf("%s category=%s widgets=%d json=%d msgpack=%d stages_ms=%v", name, center.category, measurement.WidgetCount, measurement.JSONBytes, measurement.MsgpackBytes, measurement.StageMillis)
	}
	data, err := json.MarshalIndent(fixtures, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "manifest.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func settingsProfileStableValue(value any, path string, identities map[string]string) any {
	switch current := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(current))
		for key := range current {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		result := make(map[string]any, len(current))
		for _, key := range keys {
			result[key] = settingsProfileStableValue(current[key], path+"-"+key, identities)
		}
		return result
	case []map[string]any:
		result := make([]any, len(current))
		for index, child := range current {
			result[index] = settingsProfileStableValue(child, fmt.Sprintf("%s-%d", path, index), identities)
		}
		return result
	case string:
		if strings.Contains(current, ":0x") && strings.HasPrefix(current, "*") {
			if stable, exists := identities[current]; exists {
				return stable
			}
			identities[current] = "fixture-" + path
			return identities[current]
		}
	}
	return value
}
