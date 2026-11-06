// Package filegroups owns frontend-neutral file classification and its configuration.
// Renderers consume compiled snapshots; they do not interpret configuration files.
package filegroups

import (
	"crypto/rand"
	"fmt"
	"maps"
	"slices"
)

// Group is an editable record. Legacy sparse documents may contain field-level
// patches; runtime groups and saved working sets are materialized. Unknown fields
// and explicit empty values are retained.
type Group struct {
	ID       string
	Fields   map[string]string
	Commands []CommandSet
}

// CommandSet is a named, ordered choice for the six Far association actions.
// Slot order is Execute, AltExec, View, AltView, Edit, AltEdit.
type CommandSet struct {
	ID, Name string
	Commands [6]string
	Enabled  [6]bool
	Extra    map[string]string
	// Overrides is non-nil only for a layered command patch read from INI.
	// Missing keys inherit, including individual commands and enable bits.
	Overrides map[string]string
}

var CommandNames = [6]string{"Execute", "AltExec", "View", "AltView", "Edit", "AltEdit"}

func NewID() string { return "user-" + rand.Text() }

func (g Group) Clone() Group {
	g.Fields = maps.Clone(g.Fields)
	g.Commands = slices.Clone(g.Commands)
	for i := range g.Commands {
		g.Commands[i].Extra = maps.Clone(g.Commands[i].Extra)
		g.Commands[i].Overrides = maps.Clone(g.Commands[i].Overrides)
	}
	return g
}

func (g Group) Enabled(facet string) bool {
	return g.Fields["Enabled"] != "false" && g.Fields["Enabled"] != "0" &&
		(g.Fields[facet+".Enabled"] == "true" || g.Fields[facet+".Enabled"] == "1")
}

func (g Group) Label() string {
	if name := g.Fields["Name"]; name != "" {
		return name
	}
	return g.ID
}

// IconTheme is the scope of the entire group, not an icon resource selector.
// Empty means universal; unknown theme IDs are intentionally preserved.
func (g Group) IconTheme() string { return g.Fields["IconTheme"] }

func validID(id string) error {
	if id == "" {
		return fmt.Errorf("empty group identity")
	}
	for _, r := range id {
		if r < 33 || r == ']' || r == '[' || r == ',' || r == '=' || r == '/' {
			return fmt.Errorf("invalid group identity %q", id)
		}
	}
	return nil
}

// IconReference is presentation-neutral. Theme assets are resolved by the
// contributing frontend, never interpreted as Qt resources by the Go core.
type IconReference struct {
	Kind  string // asset, file, native, none
	Value string
}

type IconTheme struct {
	ID, Name  string
	Assets    []string
	Fallbacks map[string]IconReference
}

type IconChoice struct {
	Order          int
	GroupID        string
	Reference      IconReference
	OverrideNative bool
}
