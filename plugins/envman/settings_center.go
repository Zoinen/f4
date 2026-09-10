package envman

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/unxed/f4/sdk/f4settings"
)

type settingsProvider struct{ plugin *Plugin }

func (p *settingsProvider) Catalog() f4settings.Catalog {
	field := func(name, label, description string, kind f4settings.Kind) f4settings.Field {
		return f4settings.Scalar("envman."+name, "terminal", "Environment profiles", label, description, kind)
	}
	kind := field("Kind", "Entry kind", "A profile changes the environment; a separator only organizes the list.", f4settings.ChoiceKind)
	kind.Choices = f4settings.Choices("profile:Profile", "separator:Separator")
	return f4settings.Catalog{ID: "envman", Categories: []f4settings.Category{{ID: "terminal", Label: f4settings.Text{English: "Terminal & environment"}}}, Fields: []f4settings.Field{field("IgnoredVariables", "Ignored environment variables", "Exclude these names from environment reconciliation and preserve their process values. Commas, semicolons and whitespace separate names; case rules follow the platform.", f4settings.String), field("AlwaysUseEditor", "Edit profiles in text editor", "Prefer the f4 text editor when editing profiles from the environment-manager workflow.", f4settings.Boolean)}, Collections: []f4settings.Collection{{ID: "envman.profiles", Category: "terminal", Group: "Environment profiles", Label: f4settings.Text{English: "Environment profiles"}, Description: f4settings.Text{English: "Enabled profiles apply in list order over the startup environment. Changes are reconciled with the process and local shells after Apply."}, NameField: "envman.Name", Ordered: true, Fields: []f4settings.Field{kind, field("Name", "Profile name", "Name shown in the environment profile list.", f4settings.String), field("Enabled", "Enable profile", "Apply this profile's assignments in list order.", f4settings.Boolean), field("Variables", "Variable assignments", "One NAME=value per line. NAME= removes a variable. Later enabled profiles override earlier assignments.", f4settings.Multiline)}}}, Commands: []f4settings.Command{{ID: "envman.reconcile", Category: "terminal", Group: "Environment profiles", Label: f4settings.Text{English: "Apply saved environment profiles"}, Description: f4settings.Text{English: "Reconcile saved profiles with the current process and local shells."}, Run: func(context.Context) error { return p.plugin.applyStoredConfig() }}}}
}
func (p *settingsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	initial := p.plugin.snapshotConfig()
	records := []f4settings.Record{}
	for i, e := range initial.Entries {
		records = append(records, f4settings.Record{ID: fmt.Sprintf("envman:%d", i), Values: map[string]string{"envman.Name": e.Name, "envman.Kind": string(e.Kind), "envman.Enabled": strconv.FormatBool(e.Enabled), "envman.Variables": strings.Join(e.Variables, "\n")}})
	}
	d := f4settings.NewDraft(map[string]string{"envman.IgnoredVariables": strings.Join(initial.IgnoredVariables, ", "), "envman.AlwaysUseEditor": strconv.FormatBool(initial.AlwaysUseEditor)}, map[string][]f4settings.Record{"envman.profiles": records})
	build := func(current Config) (Config, error) {
		next := cloneConfig(current)
		if d.Dirty("envman.IgnoredVariables") {
			if !reflect.DeepEqual(current.IgnoredVariables, initial.IgnoredVariables) {
				return next, f4settings.Error("ignored variables changed outside Settings Center")
			}
			next.IgnoredVariables = splitIgnoredVariables(d.Values["envman.IgnoredVariables"], p.plugin.options)
		}
		if d.Dirty("envman.AlwaysUseEditor") {
			if current.AlwaysUseEditor != initial.AlwaysUseEditor {
				return next, f4settings.Error("editor preference changed outside Settings Center")
			}
			next.AlwaysUseEditor = d.Values["envman.AlwaysUseEditor"] == "true"
		}
		if d.Dirty("envman.profiles") {
			if !reflect.DeepEqual(current.Entries, initial.Entries) {
				return next, f4settings.Error("environment profiles changed outside Settings Center")
			}
			next.Entries = nil
			for _, r := range d.Records["envman.profiles"] {
				e := Entry{Kind: Kind(r.Values["envman.Kind"]), Name: r.Values["envman.Name"], Enabled: r.Values["envman.Enabled"] == "true"}
				if lines := r.Values["envman.Variables"]; lines != "" {
					e.Variables = strings.Split(lines, "\n")
				}
				next.Entries = append(next.Entries, e)
			}
		}
		return next, next.Validate(p.plugin.options)
	}
	d.ValidateFunc = func(*f4settings.Draft) map[string]error {
		_, err := build(p.plugin.snapshotConfig())
		if err != nil {
			return map[string]error{"envman": err}
		}
		return nil
	}
	d.CommitFunc = func(ctx context.Context, d *f4settings.Draft) f4settings.Result {
		if err := ctx.Err(); err != nil {
			return f4settings.Result{Errors: map[string]error{"envman": err}}
		}
		if err := p.plugin.mutateConfig(build, false); err != nil {
			return f4settings.Result{Errors: map[string]error{"envman": err}}
		}
		initial = p.plugin.snapshotConfig()
		result := f4settings.Result{Applied: d.Changed()}
		if err := p.plugin.applyStoredConfig(); err != nil {
			result.Errors = map[string]error{"envman.runtime": f4settings.Error("profiles saved, but applying the environment failed: %w", err)}
		}
		return result
	}
	return d, nil
}
