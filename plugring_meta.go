package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/unxed/vtui"
)

// What the catalog is allowed to carry, and why.
//
// The three transports differ in what has to reach the user. A Lua plugin is
// text: portable, readable, nothing to build. A wasm plugin is one artifact
// for every platform, sandboxed and not bound to a libc. A native plugin is a
// platform binary, which is the only one that is a problem: a distribution
// maintainer will not ship prebuilt binaries from a third party catalog, it
// cannot be reviewed, and it needs a build per operating system and
// architecture.
//
// So the catalog distributes source and wasm. Native plugins remain a local
// affair: the user registers a path, or the system package manager installs
// one. Almost nothing has to give up speed for this, because the case that
// needs it is served by wasm, and the case that needs native APIs is served by
// the FFI bridge, which lets a plugin stay portable text and call the target
// system's own libraries.

// Categories, following plugring.farmanager.com closely enough that somebody
// arriving from there recognises the shelves.
const (
	PlugRingCategoryArchive    = "archive"
	PlugRingCategoryEditor     = "editor"
	PlugRingCategoryFilesystem = "filesystem"
	PlugRingCategoryNetwork    = "network"
	PlugRingCategoryPanel      = "panel"
	PlugRingCategoryService    = "service"
	PlugRingCategoryTools      = "tools"
	PlugRingCategoryViewer     = "viewer"
	PlugRingCategoryOther      = "other"
)

// PlugRingCategories lists the categories in the order a catalog should show
// them, with the catch-all last.
var PlugRingCategories = []string{
	PlugRingCategoryArchive,
	PlugRingCategoryEditor,
	PlugRingCategoryFilesystem,
	PlugRingCategoryNetwork,
	PlugRingCategoryPanel,
	PlugRingCategoryService,
	PlugRingCategoryTools,
	PlugRingCategoryViewer,
	PlugRingCategoryOther,
}

// plugRingCategoryAliases accepts the spellings an author is likely to reach
// for. A manifest is written by hand, so refusing "file system" over
// "filesystem" would only teach people that the field is finicky.
var plugRingCategoryAliases = map[string]string{
	"file system": PlugRingCategoryFilesystem,
	"filesystem":  PlugRingCategoryFilesystem,
	"fs":          PlugRingCategoryFilesystem,
	"vfs":         PlugRingCategoryFilesystem,
	"drive":       PlugRingCategoryFilesystem,
	"archives":    PlugRingCategoryArchive,
	"editors":     PlugRingCategoryEditor,
	"net":         PlugRingCategoryNetwork,
	"panels":      PlugRingCategoryPanel,
	"services":    PlugRingCategoryService,
	"tool":        PlugRingCategoryTools,
	"utils":       PlugRingCategoryTools,
	"utilities":   PlugRingCategoryTools,
	"viewers":     PlugRingCategoryViewer,
	"misc":        PlugRingCategoryOther,
}

// Runtimes a plugin can declare.
const (
	// PlugRingRuntimeEmbedded is f4's own Lua interpreter. A plugin that
	// names it needs nothing installed on the target machine.
	PlugRingRuntimeEmbedded = "embedded"
	// PlugRingRuntimeWasm is f4's own WebAssembly runtime.
	PlugRingRuntimeWasm = "wasm"
	// The rest name an interpreter the user has to already have, which is
	// exactly the dependency f4 exists to avoid. They are accepted, because a
	// plugin that truly needs LuaJIT's cdef has nowhere else to go, and
	// declaring it is better than failing at load.
	PlugRingRuntimeLua51  = "lua51"
	PlugRingRuntimeLua54  = "lua54"
	PlugRingRuntimeLuaJIT = "luajit"
	// PlugRingRuntimeNative is a platform binary. Declaring it is allowed;
	// distributing one through the catalog is not.
	PlugRingRuntimeNative = "native"
)

var plugRingRuntimes = map[string]bool{
	PlugRingRuntimeEmbedded: true,
	PlugRingRuntimeWasm:     true,
	PlugRingRuntimeLua51:    true,
	PlugRingRuntimeLua54:    true,
	PlugRingRuntimeLuaJIT:   true,
	PlugRingRuntimeNative:   true,
}

var plugRingRuntimeAliases = map[string]string{
	"gopher-lua":  PlugRingRuntimeEmbedded,
	"gopherlua":   PlugRingRuntimeEmbedded,
	"builtin":     PlugRingRuntimeEmbedded,
	"internal":    PlugRingRuntimeEmbedded,
	"lua":         PlugRingRuntimeLua51,
	"lua5.1":      PlugRingRuntimeLua51,
	"lua5.4":      PlugRingRuntimeLua54,
	"lua-jit":     PlugRingRuntimeLuaJIT,
	"webassembly": PlugRingRuntimeWasm,
	"binary":      PlugRingRuntimeNative,
}

// NormalizePlugRingCategory maps whatever an author wrote onto a known
// category.
func NormalizePlugRingCategory(category string) string {
	name := strings.ToLower(strings.TrimSpace(category))
	if name == "" {
		return PlugRingCategoryOther
	}
	if canonical, ok := plugRingCategoryAliases[name]; ok {
		return canonical
	}
	for _, known := range PlugRingCategories {
		if name == known {
			return name
		}
	}
	return PlugRingCategoryOther
}

// NormalizePlugRingRuntimes canonicalises a declared runtime list, dropping
// what it cannot make sense of.
func NormalizePlugRingRuntimes(runtimes []string) []string {
	seen := make(map[string]bool, len(runtimes))
	out := make([]string, 0, len(runtimes))
	for _, runtime := range runtimes {
		name := strings.ToLower(strings.TrimSpace(runtime))
		if canonical, ok := plugRingRuntimeAliases[name]; ok {
			name = canonical
		}
		if !plugRingRuntimes[name] || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// InferPlugRingRuntimes fills in what an entrypoint implies when the author
// said nothing, which is the common case and should not be an error.
func InferPlugRingRuntimes(entrypoint string) []string {
	switch {
	case IsLuaEntrypoint(entrypoint):
		return []string{PlugRingRuntimeEmbedded}
	case IsWasmEntrypoint(entrypoint):
		return []string{PlugRingRuntimeWasm}
	}
	return []string{PlugRingRuntimeNative}
}

// PlugRingItemRunsHere reports whether this build of f4 can run the plugin,
// and why not when it cannot. The point is to say so before installing rather
// than after.
func PlugRingItemRunsHere(item PlugRingItem) (bool, string) {
	runtimes := NormalizePlugRingRuntimes(item.Runtimes)
	if len(runtimes) == 0 {
		runtimes = InferPlugRingRuntimes(item.Entrypoint)
	}

	for _, runtime := range runtimes {
		switch runtime {
		case PlugRingRuntimeEmbedded, PlugRingRuntimeWasm, PlugRingRuntimeNative:
			// f4 carries the first two and can launch the third.
			return true, ""
		}
	}
	return false, fmt.Sprintf("needs %s, which has to be installed separately", strings.Join(runtimes, " or "))
}

// PlugRingItemProblem reports what stops an entry from belonging in the
// catalog, or an empty string when nothing does.
//
// A bare .lua or .wasm entrypoint is source or an architecture independent
// artifact: one file, no build, the same on every platform. Anything else is a
// platform binary or a shell command, and a catalog that carries those is a
// catalog no distribution will mirror and no reviewer can audit.
func PlugRingItemProblem(item PlugRingItem) string {
	switch {
	case strings.TrimSpace(item.ID) == "":
		return "the entry has no id"
	case strings.TrimSpace(item.Entrypoint) == "":
		return "the entry has no entrypoint"
	}

	if strings.TrimSpace(item.SetupCmd) != "" {
		// Worse than shipping a binary: an arbitrary command, run with the
		// user's privileges, at install time.
		return "setup_cmd runs an arbitrary command at install time"
	}

	if !IsLuaEntrypoint(item.Entrypoint) && !IsWasmEntrypoint(item.Entrypoint) {
		return fmt.Sprintf("entrypoint %q is neither a .lua source file nor a .wasm module", item.Entrypoint)
	}

	if strings.Contains(item.URL, "{os}") || strings.Contains(item.URL, "{arch}") {
		return "the download URL is per platform, so the entry ships built binaries"
	}
	return ""
}

// NormalizePlugRingCatalog fills in the fields authors leave out and reports
// the entries that do not belong in a catalog.
//
// Nothing is dropped yet. The catalog in the wild predates this rule, and
// silently hiding half of it would be a worse first impression than a warning
// in the log while the entries are brought into line.
func NormalizePlugRingCatalog(items []PlugRingItem) []PlugRingItem {
	out := make([]PlugRingItem, 0, len(items))
	for _, item := range items {
		item.Category = NormalizePlugRingCategory(item.Category)

		item.Runtimes = NormalizePlugRingRuntimes(item.Runtimes)
		if len(item.Runtimes) == 0 {
			item.Runtimes = InferPlugRingRuntimes(item.Entrypoint)
		}

		if problem := PlugRingItemProblem(item); problem != "" {
			vtui.DebugLog("PLUGRING: %q does not meet the distribution policy: %s", item.ID, problem)
		}
		if ok, reason := PlugRingItemRunsHere(item); !ok {
			vtui.DebugLog("PLUGRING: %q cannot run here: %s", item.ID, reason)
		}
		out = append(out, item)
	}
	return out
}

// plugRingCategoryTitles are the headings a user sees. The stored value stays
// lowercase and machine friendly; this is only for the screen.
var plugRingCategoryTitles = map[string]string{
	PlugRingCategoryArchive:    "Archives",
	PlugRingCategoryEditor:     "Editor",
	PlugRingCategoryFilesystem: "File systems",
	PlugRingCategoryNetwork:    "Network",
	PlugRingCategoryPanel:      "Panels",
	PlugRingCategoryService:    "Services",
	PlugRingCategoryTools:      "Tools",
	PlugRingCategoryViewer:     "Viewer",
	PlugRingCategoryOther:      "Other",
}

// PlugRingCategoryTitle is the heading for a category.
func PlugRingCategoryTitle(category string) string {
	if title, ok := plugRingCategoryTitles[NormalizePlugRingCategory(category)]; ok {
		return title
	}
	return plugRingCategoryTitles[PlugRingCategoryOther]
}

// GroupPlugRingByCategory arranges a catalog for display, keeping the category
// order and sorting the entries inside each one by name.
func GroupPlugRingByCategory(items []PlugRingItem) ([]string, map[string][]PlugRingItem) {
	grouped := make(map[string][]PlugRingItem)
	for _, item := range items {
		category := NormalizePlugRingCategory(item.Category)
		grouped[category] = append(grouped[category], item)
	}

	order := make([]string, 0, len(grouped))
	for _, category := range PlugRingCategories {
		if len(grouped[category]) > 0 {
			order = append(order, category)
		}
	}
	for _, list := range grouped {
		sort.Slice(list, func(a, b int) bool {
			return strings.ToLower(list[a].Name) < strings.ToLower(list[b].Name)
		})
	}
	return order, grouped
}
