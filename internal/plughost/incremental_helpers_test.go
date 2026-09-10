package plughost

func incrementalTestPanel(side int, entries []map[string]any) map[string]any {
	return map[string]any{
		"id": "panel-" + string(rune('0'+side)), "kind": "filePanel",
		"side": side, "active": side == 0, "path": "C:/large",
		"catalogRevision": int64(11), "selectionRevision": int64(5),
		"metadataDeferred": true, "metadataRevision": int64(7),
		"cursor": 0, "cursorEntryId": "entry-0", "selectedCount": 0,
		"totalCount": len(entries), "entries": entries,
	}
}
