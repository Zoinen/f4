// Package settingstest checks settings localization across bundled providers.
package settingstest

import (
	"github.com/unxed/f4/sdk/f4settings"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func RussianCatalog(t *testing.T, c f4settings.Catalog) {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "../i18n/lang/ru.lng"))
	if err != nil {
		t.Fatal(err)
	}
	ru := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSuffix(line, "\r"), "="); ok {
			ru[k] = v
		}
	}
	check := func(x f4settings.Text) {
		if x.English == "" || x.Literal {
			return
		}
		if x.Translations["ru"] != "" {
			return
		}
		key := x.Key
		if key == "" {
			key = f4settings.ResourceKey(x.English)
		}
		if ru[key] == "" {
			t.Errorf("%s: missing Russian text %q (%s)", c.ID, x.English, key)
		}
	}
	var walk func(reflect.Value)
	walk = func(v reflect.Value) {
		if v.Type() == reflect.TypeOf(f4settings.Text{}) {
			check(v.Interface().(f4settings.Text))
			return
		}
		switch v.Kind() {
		case reflect.Struct:
			for i := 0; i < v.NumField(); i++ {
				name := v.Type().Field(i).Name
				if (name == "Group" || name == "Timing" || name == "Unavailable") && v.Field(i).Kind() == reflect.String {
					check(f4settings.Text{English: v.Field(i).String()})
				}
				walk(v.Field(i))
			}
		case reflect.Slice:
			for i := 0; i < v.Len(); i++ {
				walk(v.Index(i))
			}
		}
	}
	walk(reflect.ValueOf(c))
}
