package vtui

import (
	"fmt"
	"sync"
)

var (
	stringsMu  sync.RWMutex
	stringsMap = map[string]string{
		"vtui.Ok":      "&Ok",
		"vtui.Cancel":  "Cancel",
		"vtui.Save":    "&Save",
		"vtui.Delete":  "&Delete",
		"vtui.Path":    "Path:",
		"vtui.File":    "&File:",
		"vtui.History": "History",
	}
)

// Msg retrieves a localized string by key.
// It looks into the global vtui strings map.
func Msg(key string) string {
	stringsMu.RLock()
	defer stringsMu.RUnlock()
	if val, ok := stringsMap[key]; ok {
		return val
	}
	return fmt.Sprintf("{%s}", key)
}

// ReverseLookup attempts to find the translation key for a given localized string.
// This is used exclusively by the developer/translator tools.
func ReverseLookup(val string) string {
	stringsMu.RLock()
	defer stringsMu.RUnlock()
	for k, v := range stringsMap {
		if v == val {
			return k
		}
	}
	return ""
}

// SnapshotStrings returns a copy of the currently loaded localization table.
// It is mainly used by tooling that needs to temporarily switch languages
// (for example the layout validator) and restore the original state after.
func SnapshotStrings() map[string]string {
	stringsMu.RLock()
	defer stringsMu.RUnlock()
	out := make(map[string]string, len(stringsMap))
	for k, v := range stringsMap {
		out[k] = v
	}
	return out
}

// ReplaceStrings atomically replaces the whole localization table with a copy
// of m. Unlike AddStrings it does not merge: keys missing from m are dropped.
func ReplaceStrings(m map[string]string) {
	next := make(map[string]string, len(m))
	for k, v := range m {
		next[k] = v
	}
	stringsMu.Lock()
	defer stringsMu.Unlock()
	stringsMap = next
}

// AddStrings allows an application to add or override strings in the UI.
func AddStrings(m map[string]string) {
	stringsMu.Lock()
	defer stringsMu.Unlock()
	for k, v := range m {
		stringsMap[k] = v
	}
}
