package main

import (
	"embed"
	"sort"
	"strings"

	"github.com/unxed/vtui"
)

//go:embed lang/*.lng
var langPackFS embed.FS

// LoadAllLanguagePacks returns every translation shipped with f4, ready to be
// handed to the vtui layout validator. Captions have different lengths in
// different languages, so a dialog layout must be checked against all of them,
// not only against the language that happens to be loaded.
func LoadAllLanguagePacks() []vtui.LanguagePack {
	entries, err := langPackFS.ReadDir("lang")
	if err != nil {
		return nil
	}

	packs := make([]vtui.LanguagePack, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".lng") {
			continue
		}
		data, err := langPackFS.ReadFile("lang/" + entry.Name())
		if err != nil {
			continue
		}

		ini := ParseIni(strings.NewReader(string(data)))
		name := strings.TrimSuffix(entry.Name(), ".lng")
		if sec, ok := ini.data["Language"]; ok {
			if code := strings.TrimSpace(sec["Code"]); code != "" {
				name = code
			}
		}

		packs = append(packs, vtui.LanguagePack{
			Name:    name,
			Strings: loadLangMapFromINI(ini),
		})
	}

	sort.Slice(packs, func(i, j int) bool { return packs[i].Name < packs[j].Name })
	return packs
}
