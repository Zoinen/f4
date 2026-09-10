package settings

import (
	"github.com/unxed/f4/internal/config"

	"context"
	"fmt"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/sdk/f4settings"
	"reflect"
	"strings"
)

type pluginSettingsProvider struct{}

func (pluginSettingsProvider) Catalog() f4settings.Catalog {
	paths := recordCollection("plugins.registered", "plugins", "Registered plugins", "Ordered external plugin paths loaded on startup. Apply saves the list; restart loads additions and fully unloads removals.", "plugin.Path", []f4settings.Field{recordField("plugin.Path", "Plugin path", "Path to the external Lua, WASM or executable plugin. This list preserves startup ordering.", f4settings.Path)})
	grant := recordField("grant.Keep", "Remember permission", "Clear to revoke this remembered answer on Apply. The next protected request asks again. This control cannot create grants.", f4settings.Boolean)
	grants := recordCollection("plugins.permissions", "plugins", "Remembered permissions", "Per-plugin remembered permission decisions. Unchecking an answer revokes it only after Apply.", "grant.Name", []f4settings.Field{grant})
	grants.Fixed = true
	grants.Ordered = false
	return f4settings.Catalog{ID: "plugins", Categories: Categories, Collections: []f4settings.Collection{paths, grants}}
}
func (pluginSettingsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	var paths, grants []f4settings.Record
	for i, path := range config.App.RegisteredPlugins {
		paths = append(paths, f4settings.Record{ID: fmt.Sprintf("plugin:%d", i), Values: map[string]string{"plugin.Path": path}})
	}
	for _, g := range plughost.PluginPermissions().Grants() {
		grants = append(grants, f4settings.Record{ID: "grant:" + g.Plugin + ":" + g.Permission, Values: map[string]string{"grant.Name": plughost.PermissionGrantLine(g), "grant.Plugin": g.Plugin, "grant.Permission": g.Permission, "grant.Decision": g.Decision, "grant.Keep": "true"}})
	}
	d := f4settings.NewDraft(nil, map[string][]f4settings.Record{"plugins.registered": paths, "plugins.permissions": grants})
	extract := func(rows []f4settings.Record) []string {
		var paths []string
		for _, r := range rows {
			paths = append(paths, r.Values["plugin.Path"])
		}
		return paths
	}
	d.ValidateFunc = func(d *f4settings.Draft) map[string]error {
		errs := map[string]error{}
		if d.Dirty("plugins.registered") {
			if !reflect.DeepEqual(config.App.RegisteredPlugins, extract(d.BaselineRecords["plugins.registered"])) {
				errs["plugins.registered"] = settingsError("plugin list changed outside Settings Center")
			}
			for _, r := range d.Records["plugins.registered"] {
				if strings.TrimSpace(r.Values["plugin.Path"]) == "" || settingsSingleLine(r.Values["plugin.Path"]) != nil {
					errs[r.ID] = settingsError("enter a nonempty plugin path on one line")
				}
			}
		}
		return errs
	}
	d.CommitFunc = func(ctx context.Context, d *f4settings.Draft) f4settings.Result {
		result := f4settings.Result{Errors: d.Validate()}
		if len(result.Errors) > 0 {
			return result
		}
		result.Errors = map[string]error{}
		if d.Dirty("plugins.registered") {
			before := config.App
			next := before
			next.RegisteredPlugins = extract(d.Records["plugins.registered"])
			if err := writeSettingsCandidate(before, next); err != nil {
				result.Errors["plugins.registered"] = err
			} else {
				config.App.RegisteredPlugins = next.RegisteredPlugins
				result.Applied = append(result.Applied, "plugins.registered")
			}
		}
		if d.Dirty("plugins.permissions") {
			for _, r := range d.Records["plugins.permissions"] {
				if r.Values["grant.Keep"] != "false" {
					continue
				}
				if err := ctx.Err(); err != nil {
					result.Errors[r.ID] = err
					continue
				}
				plugin, permission := r.Values["grant.Plugin"], r.Values["grant.Permission"]
				decision, exists := plughost.PluginPermissions().Decision(plugin, permission)
				if exists && decision != r.Values["grant.Decision"] {
					result.Errors[r.ID] = settingsError("permission answer changed concurrently")
					continue
				}
				if err := plughost.PluginPermissions().Revoke(plugin, permission); err != nil {
					result.Errors[r.ID] = err
				} else {
					result.Applied = append(result.Applied, r.ID)
				}
			}
			if len(result.Errors) == 0 {
				result.Applied = append(result.Applied, "plugins.permissions")
			}
		}
		return result
	}
	return d, nil
}
