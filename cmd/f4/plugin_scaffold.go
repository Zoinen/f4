package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// The scaffolder writes a Lua plugin and nothing else, on purpose.
//
// A Go template would need a toolchain and a go.mod pointing at the SDK; a
// wasm one needs a target as well. Neither is a five minute hello world. A Lua
// plugin needs nothing at all: the file exists, f4 reads it with its own
// interpreter, the drive is in the menu. That distance is what this command is
// meant to shorten.

// scaffoldPluginLua is the plugin a newcomer gets. It is deliberately a real
// working drive rather than an empty stub: the first thing to do with it is
// change a line and see what happens.
const scaffoldPluginLua = `-- {{name}}: an f4 plugin.
--
-- It mounts a virtual drive whose files are made up on the spot. Nothing
-- needs installing: f4 runs this on its own Lua interpreter.
--
-- Change something here, restart f4, and look again.

local f4rpc = require('f4rpc')

local DRIVE = "{{name}}"

-- The contents of the drive, in the order the panel shows them.
local files = {
  { name = "hello.txt", body = "Hello from {{name}}.\n" },
  { name = "notes.txt", body = "Edit plugin.lua and restart f4 to see this change.\n" },
}

local byName = {}
for _, file in ipairs(files) do
  byName[file.name] = file.body
end

-- Open file handles, so that a read knows which file it belongs to.
local handles, nextHandle = {}, 1

local function bodyOf(path)
  local name = tostring(path or ""):match("([^/\\]+)$") or ""
  return byName[name]
end

-- Plugin.Init is the first thing f4 asks. The drives named here appear in the
-- drive menu, on Alt+F1 and Alt+F2.
f4rpc.register("Plugin.Init", function()
  f4rpc.call("Host.Log", DRIVE .. " loaded")
  return { Drives = { DRIVE } }
end)

f4rpc.register("VFS.ReadDir", function(req)
  local items = {}
  for _, file in ipairs(files) do
    table.insert(items, { Name = file.name, Size = #file.body, IsDir = false })
  end
  return items
end)

f4rpc.register("VFS.Open", function(req)
  local body = bodyOf(req.Path)
  if not body then
    error("no such file: " .. tostring(req.Path))
  end
  local id = nextHandle
  nextHandle = nextHandle + 1
  handles[id] = body
  return { ID = id, Size = #body }
end)

f4rpc.register("VFS.ReadAt", function(req)
  local body = handles[req.ID] or ""
  return body:sub(req.Off + 1, req.Off + req.Len)
end)

f4rpc.register("VFS.CloseFile", function(req)
  handles[req.ID] = nil
end)

-- Out of process this enters the read loop; embedded it does nothing. Leave
-- it in, and the same file works either way.
f4rpc.serve()
`

const scaffoldReadme = `# {{name}}

An f4 plugin. It mounts a virtual drive with a couple of invented files in it.

## Trying it

1. Start f4 and open the plugin manager from the Options menu.
2. Press Insert and point it at ` + "`plugin.lua`" + ` in this directory.
3. Restart f4, press Alt+F1 and pick the **{{name}}** drive.

There is nothing to build. f4 carries its own Lua interpreter, so the file you
edit is the plugin that runs.

## What is in here

- ` + "`plugin.lua`" + ` — the plugin. Start reading at ` + "`Plugin.Init`" + `.
- ` + "`manifest.json`" + ` — what PlugRing needs in order to distribute it.

## Where to go next

- ` + "`PLUGINS.md`" + ` in the f4 repository: the protocol, the host methods and
  the other two transports.
- ` + "`FFI.md`" + `: calling native libraries from a plugin.
- ` + "`PLUGRING.md`" + `: publishing this so that other people can install it.
`

// ScaffoldPlugin writes a new plugin into dir and reports the files created.
func ScaffoldPlugin(dir, name string) ([]string, error) {
	if err := validatePluginName(name); err != nil {
		return nil, err
	}
	if err := ensureEmptyDir(dir); err != nil {
		return nil, err
	}
	// #nosec G703 -- dir is the complete destination explicitly entered by the user, not an untrusted child component.
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	manifest, err := scaffoldManifest(name)
	if err != nil {
		return nil, err
	}

	contents := []struct {
		file string
		body string
	}{
		{"plugin.lua", renderScaffold(scaffoldPluginLua, name)},
		{"manifest.json", manifest},
		{"README.md", renderScaffold(scaffoldReadme, name)},
	}

	created := make([]string, 0, len(contents))
	for _, entry := range contents {
		path := filepath.Join(dir, entry.file)
		// #nosec G703 -- entry.file comes only from the fixed three-name table above; dir is the user-selected destination root.
		if err := os.WriteFile(path, []byte(entry.body), 0o600); err != nil {
			return created, err
		}
		created = append(created, path)
	}
	return created, nil
}

func renderScaffold(template, name string) string {
	return strings.ReplaceAll(template, "{{name}}", name)
}

func scaffoldManifest(name string) (string, error) {
	item := PlugRingItem{
		ID:           name,
		Name:         name,
		Version:      "0.1.0",
		Description:  "An f4 plugin.",
		Entrypoint:   "plugin.lua",
		Category:     PlugRingCategoryFilesystem,
		Runtimes:     []string{PlugRingRuntimeEmbedded},
		Dependencies: []string{},
	}
	encoded, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return "", err
	}
	return string(encoded) + "\n", nil
}

// validatePluginName keeps a name usable as a directory, a drive name and a
// PlugRing id all at once, which is what saves the newcomer from finding out
// later that one of the three disagreed.
func validatePluginName(name string) error {
	if name == "" {
		return fmt.Errorf("a plugin needs a name")
	}
	if len(name) > 64 {
		return fmt.Errorf("the name %q is too long", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_':
		default:
			return fmt.Errorf("the name %q may only contain letters, digits, dashes and underscores", name)
		}
	}
	return nil
}

// ensureEmptyDir refuses to scaffold over someone's work.
func ensureEmptyDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return fmt.Errorf("%s already exists and is not empty", dir)
	}
	return nil
}

// RunNewPlugin implements the --new-plugin command and returns an exit code.
func RunNewPlugin(name string, out, errOut io.Writer) int {
	name = strings.TrimSpace(name)
	if name == "" {
		fmt.Fprintln(errOut, "usage: f4 --new-plugin <name>")
		return 2
	}

	created, err := ScaffoldPlugin(name, filepath.Base(name))
	if err != nil {
		fmt.Fprintf(errOut, "f4: %v\n", err)
		return 1
	}

	for _, path := range created {
		fmt.Fprintf(out, "created %s\n", path)
	}
	fmt.Fprintf(out, "%s", scaffoldNextSteps(name, filepath.Base(name)))
	return 0
}

func scaffoldNextSteps(dir, name string) string {
	return fmt.Sprintf(`
Next:
  1. Start f4 and open the plugin manager from the Options menu.
  2. Press Insert and select %s.
  3. Restart f4, press Alt+F1 and pick the "%s" drive.

Then edit %s. There is nothing to build.
`, filepath.Join(dir, "plugin.lua"), name, filepath.Join(dir, "plugin.lua"))
}
