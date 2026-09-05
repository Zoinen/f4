package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/vtui"
	"gopkg.in/yaml.v3"
)

// PlugRingCatalogURL is the URL where f4 fetches the compiled YAML catalog.
var PlugRingCatalogURL = "https://raw.githubusercontent.com/unxed/f4/main/plugring/index.yaml"

// PlugRingItem represents a single plugin available in the store.
type PlugRingItem struct {
	ID           string   `json:"id" yaml:"id"`
	Name         string   `json:"name" yaml:"name"`
	Version      string   `json:"version" yaml:"version"`
	Author       string   `json:"author" yaml:"author"`
	Description  string   `json:"description" yaml:"description"`
	URL          string   `json:"url" yaml:"url"`
	Entrypoint   string   `json:"entrypoint" yaml:"entrypoint"`
	SetupCmd     string   `json:"setup_cmd" yaml:"setup_cmd"`
	Dependencies []string `json:"dependencies" yaml:"dependencies"`
	// Category groups the plugin in the catalog, the way plugring.farmanager.com
	// does. Empty means PlugRingCategoryOther.
	Category string `json:"category" yaml:"category"`
	// Permissions maps a permission the plugin needs onto the author's own
	// explanation of why, which is what the user is shown when asked.
	Permissions map[string]string `json:"permissions" yaml:"permissions"`
	// Runtimes names the interpreters or runtimes the plugin works with, so
	// that f4 can tell whether it can run the thing before installing it
	// rather than after. Empty is inferred from the entrypoint.
	Runtimes []string `json:"runtimes" yaml:"runtimes"`
}

// FetchCatalog downloads and parses the plugin catalog.
func FetchCatalog(ctx context.Context) ([]PlugRingItem, error) {
	// Developer convenience: load local index if available, but only if we are using the default URL
	if PlugRingCatalogURL == "https://raw.githubusercontent.com/unxed/f4/main/plugring/index.yaml" {
		if data, err := os.ReadFile(filepath.Join("plugring", "index.yaml")); err == nil {
			var items []PlugRingItem
			if yaml.Unmarshal(data, &items) == nil {
				vtui.DebugLog("PLUGRING: Loaded catalog from local file")
				return NormalizePlugRingCatalog(items), nil
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", PlugRingCatalogURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Disable caching for the catalog fetch to ensure fresh results
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "f4-plugring")

	client := netproxy.HTTPClient(10 * time.Second)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error while fetching catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var items []PlugRingItem
	if err := yaml.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("failed to parse catalog YAML: %w", err)
	}

	return NormalizePlugRingCatalog(items), nil
}

// ResolveAssetURL replaces platform-specific placeholders in the download URL.
func ResolveAssetURL(urlTpl string) string {
	res := strings.ReplaceAll(urlTpl, "{os}", runtime.GOOS)
	res = strings.ReplaceAll(res, "{arch}", runtime.GOARCH)
	return res
}

// GetInstalledPlugRingItems scans the local plugins directory for manifests.
func GetInstalledPlugRingItems() map[string]PlugRingItem {
	dir := filepath.Join(GetF4ConfigDir(), "plugring")
	res := make(map[string]PlugRingItem)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res
	}
	for _, e := range entries {
		if e.IsDir() {
			manifestPath := filepath.Join(dir, e.Name(), "manifest.json")
			data, err := os.ReadFile(manifestPath)
			if err == nil {
				var item PlugRingItem
				if json.Unmarshal(data, &item) == nil {
					res[item.ID] = item
				}
			}
		}
	}
	return res
}

// CheckForPluginUpdates checks for available updates of installed plugins in background.
func CheckForPluginUpdates() {
	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	go func() {
		// Let the application UI initialize completely before checking
		time.Sleep(5 * time.Second)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		items, err := FetchCatalog(ctx)
		if err != nil {
			return
		}

		installed := GetInstalledPlugRingItems()
		updateCount := 0
		for _, itm := range items {
			if inst, ok := installed[itm.ID]; ok {
				if inst.Version != itm.Version {
					updateCount++
				}
			}
		}

		if updateCount > 0 {
			frames.PostTask(func() {
				showToast(fmt.Sprintf("PlugRing: %d plugin update(s) available!", updateCount), 5*time.Second)
			})
		}
	}()
}
