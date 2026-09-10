package editor

import (
	semantic "github.com/unxed/f4/internal/semantic"
	reflect "reflect"
	testing "testing"
)

func comparableSemanticRow(row map[string]any) map[string]any {
	result := make(map[string]any, len(row)-1)
	for key, value := range row {
		// Index is local to the bounded array. Absolute byte/visual extents are
		// the stable identity shared by two overlapping windows.
		if key != "index" {
			result[key] = value
		}
	}
	return result
}

func semanticRowsByExtent(t *testing.T, node map[string]any, unit string) map[int64]map[string]any {
	t.Helper()
	result := make(map[int64]map[string]any)
	for _, row := range semantic.AppMapSlice(node["windowRows"]) {
		var extent int64
		if unit == "rows" {
			extent = semantic.AppInt64(row["visualRow"])
		} else {
			extent = semantic.AppInt64(row["offset"])
		}
		if _, duplicate := result[extent]; duplicate {
			t.Fatalf("duplicate %s extent %d in %#v", unit, extent, node["windowRows"])
		}
		result[extent] = row
	}
	return result
}

func assertSemanticOverlapStable(t *testing.T, first, second map[string]any, unit string) int {
	t.Helper()
	firstRows := semanticRowsByExtent(t, first, unit)
	secondRows := semanticRowsByExtent(t, second, unit)
	overlap := 0
	for extent, oldRow := range firstRows {
		newRow, ok := secondRows[extent]
		if !ok {
			continue
		}
		overlap++
		if !reflect.DeepEqual(comparableSemanticRow(oldRow), comparableSemanticRow(newRow)) {
			t.Fatalf("overlap changed at %s extent %d:\nold %#v\nnew %#v",
				unit, extent, oldRow, newRow)
		}
	}
	return overlap
}
