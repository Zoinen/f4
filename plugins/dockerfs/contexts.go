package dockerfs

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"sort"
)

// maxContexts bounds how many docker contexts get a drive of their own.
const maxContexts = 64

// listContexts names the docker contexts of the CLI's store that have a docker
// endpoint, sorted. The built-in "default" context is the plain Docker drive
// and is not listed. An absent or unreadable store is an empty list.
func listContexts(cfgDir string) []string {
	if cfgDir == "" {
		return nil
	}
	dirs, err := os.ReadDir(filepath.Join(cfgDir, "contexts", "meta"))
	if err != nil {
		return nil
	}
	var names []string
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		data, err := readSmall(filepath.Join(cfgDir, "contexts", "meta", d.Name(), "meta.json"))
		if err != nil {
			continue
		}
		var meta struct {
			Name      string `json:"Name"`
			Endpoints map[string]struct {
				Host string `json:"Host"`
			} `json:"Endpoints"`
		}
		if json.Unmarshal(data, &meta) != nil || meta.Name == "" || meta.Name == "default" {
			continue
		}
		// The store names the directory by the hash of the name; a directory
		// that disagrees is not a context this plugin can open by name.
		if contextID(meta.Name) != d.Name() || meta.Endpoints["docker"].Host == "" {
			continue
		}
		names = append(names, meta.Name)
	}
	sort.Strings(names)
	if len(names) > maxContexts {
		names = names[:maxContexts]
	}
	return names
}

// contextDriveName is what the drive menu calls the panel of one context.
func contextDriveName(name string) string { return driveName + " (" + name + ")" }

// contextPrefix is the URI head of the panel of one docker context.
func contextPrefix(name string) string { return uriPrefix + url.PathEscape(name) }

// contextOpener connects to the daemon of one docker context.
func contextOpener(name string) func() (*client, error) {
	return func() (*client, error) {
		ep, err := contextEndpoint(dockerConfigDir(os.Getenv), name)
		if err != nil {
			return nil, err
		}
		return newClientTLS(ep.host, ep.tls)
	}
}

// newContextVFS is the panel of one docker context.
func newContextVFS(name string) *dockerVFS {
	v := newDockerVFS(contextOpener(name))
	v.prefix = contextPrefix(name)
	return v
}
