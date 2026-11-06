// Command file-color-audit extracts projected colours without changing a profile.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"maps"
	"os"

	"github.com/unxed/f4/internal/filegroups"
)

func main() {
	profile := flag.String("profile", "", "highlight.ini to inspect read-only")
	themes := flag.String("themes", "", "optional colour theme data to compare against current")
	flag.Parse()
	if err := run(*profile, *themes); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path, themesPath string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	document, err := filegroups.Parse(data)
	if err != nil {
		return err
	}
	groups, err := document.Groups()
	if err != nil {
		return err
	}
	colors := map[string]map[string]string{}
	for _, group := range groups {
		projected := group.Highlighting("Modern")
		if !projected.Enabled("Highlight") {
			continue
		}
		fields := map[string]string{}
		for _, key := range []string{"NormalColor", "SelectedColor", "CursorColor", "SelectedCursorColor"} {
			if value, present := projected.Fields[key]; present {
				fields[key] = value
			}
		}
		name := filegroups.IconName(group.Label())
		if _, exists := colors[name]; exists {
			return fmt.Errorf("duplicate normalized group %q", name)
		}
		colors[name] = fields
	}
	if themesPath != "" {
		data, err := os.ReadFile(themesPath)
		if err != nil {
			return err
		}
		document, err := filegroups.Parse(data)
		if err != nil {
			return err
		}
		actual := document.Section("FileColorTheme.current")
		expected := map[string]string{"Name": "Current"}
		for name, fields := range colors {
			for key, value := range fields {
				expected["Color."+name+"."+key] = value
			}
		}
		if !maps.Equal(actual, expected) {
			return fmt.Errorf("current theme differs from projected profile colours")
		}
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(colors)
}
