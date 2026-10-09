package settings

import (
	"fmt"
	"sort"
	"strings"

	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

// Diagnostics for pending edits nobody meant to make (#1154 follow-up):
// Apply reports "EditorTabSize: enter an integer of at least 1" and
// "envman: entry 1: profile name is empty" after the caret setting alone
// was changed. Validation only runs for drafts that differ from their
// baseline, so something wrote those drafts. These lines, written with
// --debug, record every draft write with the control that made it and the
// dirty state at Apply. Values of secrets and multi-line fields are replaced
// by their length: profile variables can hold tokens.

func settingsTraceValue(f f4settings.Field, v string) string {
	if f.Kind == f4settings.Secret || f.Kind == f4settings.Multiline {
		return fmt.Sprintf("<%d bytes>", len(v))
	}
	return fmt.Sprintf("%q", v)
}

func settingsTraceWrite(s *settingsSession, record string, f f4settings.Field, old, value string, control vtui.UIElement) {
	where := s.catalog.ID + "/" + f.ID
	if record != "" {
		where = s.catalog.ID + "/" + record + "/" + f.ID
	}
	vtui.DebugLog("SETTINGS_TRACE: write %s %s -> %s via %T", where, settingsTraceValue(f, old), settingsTraceValue(f, value), control)
}

func settingsTraceRecords(s *settingsSession, collection, action, record string) {
	vtui.DebugLog("SETTINGS_TRACE: records %s/%s %s %s (now %d, baseline %d)", s.catalog.ID, collection, action, record, len(s.draft.Records[collection]), len(s.draft.BaselineRecords[collection]))
}

// settingsTraceDirty logs what Apply is about to validate for one session.
func settingsTraceDirty(s *settingsSession) {
	changed := s.draft.Changed()
	sort.Strings(changed)
	fields := map[string]f4settings.Field{}
	for _, f := range s.catalog.Fields {
		fields[f.ID] = f
	}
	for _, id := range changed {
		if rows, ok := s.draft.Records[id]; ok {
			vtui.DebugLog("SETTINGS_TRACE: dirty %s/%s records: %s", s.catalog.ID, id, settingsTraceRecordDiff(rows, s.draft.BaselineRecords[id]))
			continue
		}
		vtui.DebugLog("SETTINGS_TRACE: dirty %s/%s %s -> %s", s.catalog.ID, id, settingsTraceValue(fields[id], s.draft.Baseline[id]), settingsTraceValue(fields[id], s.draft.Values[id]))
	}
}

// settingsTraceRecordDiff names added and removed record IDs and, for kept
// records, the keys whose values differ -- keys only, never values.
func settingsTraceRecordDiff(rows, baseline []f4settings.Record) string {
	base := map[string]f4settings.Record{}
	var order []string
	for _, r := range baseline {
		base[r.ID] = r
		order = append(order, r.ID)
	}
	var parts []string
	var now []string
	for _, r := range rows {
		now = append(now, r.ID)
		old, ok := base[r.ID]
		if !ok {
			parts = append(parts, "added "+r.ID)
			continue
		}
		delete(base, r.ID)
		var keys []string
		for k, v := range r.Values {
			if ov, had := old.Values[k]; !had || ov != v {
				keys = append(keys, k)
			}
		}
		for k := range old.Values {
			if _, has := r.Values[k]; !has {
				keys = append(keys, k)
			}
		}
		if len(keys) > 0 {
			sort.Strings(keys)
			parts = append(parts, r.ID+" changed "+strings.Join(keys, ","))
		}
	}
	for _, id := range order {
		if _, gone := base[id]; gone {
			parts = append(parts, "removed "+id)
		}
	}
	if strings.Join(now, ",") != strings.Join(order, ",") {
		parts = append(parts, "order "+strings.Join(now, ","))
	}
	if len(parts) == 0 {
		return "no key differs (DeepEqual still false)"
	}
	return strings.Join(parts, "; ")
}
