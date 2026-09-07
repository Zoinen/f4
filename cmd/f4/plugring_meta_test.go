package main

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizePlugRingCategory(t *testing.T) {
	cases := map[string]string{
		"":            PlugRingCategoryOther,
		"archive":     PlugRingCategoryArchive,
		"Archive":     PlugRingCategoryArchive,
		"  Viewer  ":  PlugRingCategoryViewer,
		"file system": PlugRingCategoryFilesystem,
		"vfs":         PlugRingCategoryFilesystem,
		"utils":       PlugRingCategoryTools,
		"nonsense":    PlugRingCategoryOther,
	}
	for input, want := range cases {
		if got := NormalizePlugRingCategory(input); got != want {
			t.Errorf("NormalizePlugRingCategory(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizePlugRingRuntimes(t *testing.T) {
	got := NormalizePlugRingRuntimes([]string{"GopherLua", "luajit", "embedded", "nonsense", ""})
	want := []string{PlugRingRuntimeEmbedded, PlugRingRuntimeLuaJIT}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("NormalizePlugRingRuntimes = %v, want %v", got, want)
	}
	if got := NormalizePlugRingRuntimes(nil); len(got) != 0 {
		t.Errorf("NormalizePlugRingRuntimes(nil) = %v, want empty", got)
	}
}

func TestInferPlugRingRuntimes(t *testing.T) {
	cases := map[string]string{
		"plugin.lua":      PlugRingRuntimeEmbedded,
		"plugin.wasm":     PlugRingRuntimeWasm,
		"luajit main.lua": PlugRingRuntimeLua51,
		"helper":          PlugRingRuntimeNative,
	}
	for entrypoint, want := range cases {
		got := InferPlugRingRuntimes(entrypoint)
		if len(got) != 1 {
			t.Fatalf("InferPlugRingRuntimes(%q) = %v", entrypoint, got)
		}
		// An entrypoint with arguments runs as a process, so it is native as
		// far as f4 is concerned, whatever it runs inside.
		if entrypoint == "luajit main.lua" {
			want = PlugRingRuntimeNative
		}
		if got[0] != want {
			t.Errorf("InferPlugRingRuntimes(%q) = %v, want %q", entrypoint, got, want)
		}
	}
}

func TestPlugRingItemRunsHere(t *testing.T) {
	embedded := PlugRingItem{ID: "a", Entrypoint: "plugin.lua"}
	if ok, reason := PlugRingItemRunsHere(embedded); !ok {
		t.Errorf("a Lua plugin cannot run here: %s", reason)
	}

	wasm := PlugRingItem{ID: "b", Entrypoint: "plugin.wasm"}
	if ok, reason := PlugRingItemRunsHere(wasm); !ok {
		t.Errorf("a wasm plugin cannot run here: %s", reason)
	}

	// A plugin that only works on LuaJIT needs something f4 does not carry,
	// and saying so before installing is the whole point of the field.
	external := PlugRingItem{ID: "c", Entrypoint: "plugin.lua", Runtimes: []string{"luajit"}}
	ok, reason := PlugRingItemRunsHere(external)
	if ok {
		t.Error("a LuaJIT-only plugin was reported as runnable")
	}
	if !strings.Contains(reason, "luajit") {
		t.Errorf("reason = %q, want it to name what is missing", reason)
	}

	// Declaring both means it works either way.
	both := PlugRingItem{ID: "d", Entrypoint: "plugin.lua", Runtimes: []string{"luajit", "embedded"}}
	if ok, _ := PlugRingItemRunsHere(both); !ok {
		t.Error("a plugin that also runs embedded was reported as unrunnable")
	}
}

func TestPlugRingItemProblem(t *testing.T) {
	clean := PlugRingItem{ID: "a", Entrypoint: "plugin.lua"}
	if problem := PlugRingItemProblem(clean); problem != "" {
		t.Errorf("a plain Lua entry was rejected: %s", problem)
	}
	if problem := PlugRingItemProblem(PlugRingItem{ID: "b", Entrypoint: "plugin.wasm"}); problem != "" {
		t.Errorf("a wasm entry was rejected: %s", problem)
	}

	cases := []struct {
		name string
		item PlugRingItem
		want string
	}{
		{"no id", PlugRingItem{Entrypoint: "plugin.lua"}, "id"},
		{"no entrypoint", PlugRingItem{ID: "a"}, "entrypoint"},
		{"setup command", PlugRingItem{ID: "a", Entrypoint: "plugin.lua", SetupCmd: "make"}, "setup_cmd"},
		{"binary", PlugRingItem{ID: "a", Entrypoint: "plugin-helper"}, "entrypoint"},
		{"per platform url", PlugRingItem{ID: "a", Entrypoint: "plugin.lua", URL: "https://x/{os}-{arch}.zip"}, "binaries"},
	}
	for _, tc := range cases {
		problem := PlugRingItemProblem(tc.item)
		if problem == "" {
			t.Errorf("%s: the entry was accepted into the catalog", tc.name)
			continue
		}
		if !strings.Contains(problem, tc.want) {
			t.Errorf("%s: problem = %q, want it to mention %q", tc.name, problem, tc.want)
		}
	}
}

func TestNormalizePlugRingCatalogFillsDefaults(t *testing.T) {
	items := NormalizePlugRingCatalog([]PlugRingItem{
		{ID: "a", Entrypoint: "plugin.lua"},
		{ID: "b", Entrypoint: "plugin.wasm", Category: "File System"},
	})

	if items[0].Category != PlugRingCategoryOther {
		t.Errorf("category = %q, want the catch-all", items[0].Category)
	}
	if !reflect.DeepEqual(items[0].Runtimes, []string{PlugRingRuntimeEmbedded}) {
		t.Errorf("runtimes = %v, want them inferred from the entrypoint", items[0].Runtimes)
	}
	if items[1].Category != PlugRingCategoryFilesystem {
		t.Errorf("category = %q, want it normalised", items[1].Category)
	}
	if !reflect.DeepEqual(items[1].Runtimes, []string{PlugRingRuntimeWasm}) {
		t.Errorf("runtimes = %v, want wasm", items[1].Runtimes)
	}

	// An entry that breaks the policy is reported, not hidden: the catalog in
	// the wild predates the rule.
	if len(NormalizePlugRingCatalog([]PlugRingItem{{ID: "c", Entrypoint: "helper"}})) != 1 {
		t.Error("a policy violation was silently dropped from the catalog")
	}
}

func TestGroupPlugRingByCategory(t *testing.T) {
	order, grouped := GroupPlugRingByCategory([]PlugRingItem{
		{ID: "z", Name: "Zebra", Category: "tools"},
		{ID: "a", Name: "apple", Category: "tools"},
		{ID: "n", Name: "Net", Category: "network"},
		{ID: "u", Name: "Unfiled"},
	})

	want := []string{PlugRingCategoryNetwork, PlugRingCategoryTools, PlugRingCategoryOther}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	tools := grouped[PlugRingCategoryTools]
	if len(tools) != 2 || tools[0].Name != "apple" {
		t.Errorf("tools = %v, want them sorted by name regardless of case", tools)
	}
}
