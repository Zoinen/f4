package main

import (
	"bytes"
	"sort"
)

type lineDiffStats struct {
	LCS          int
	Insertions   int
	Deletions    int
	Replacements int
}

// lcsLineStats computes a sparse LCS over normalized line bytes. It avoids a
// quadratic matrix while still separating sequence insertions/deletions from
// substitutions; renderer padding is normalized only for this diagnostic.
func lcsLineStats(expected, observed [][]byte) lineDiffStats {
	positions := make(map[string][]int, len(observed))
	for index, line := range observed {
		key := string(bytes.TrimRight(line, " "))
		positions[key] = append(positions[key], index)
	}
	tails := make([]int, 0, len(expected))
	for _, line := range expected {
		places := positions[string(bytes.TrimRight(line, " "))]
		for index := len(places) - 1; index >= 0; index-- {
			place := places[index]
			position := sort.SearchInts(tails, place)
			if position == len(tails) {
				tails = append(tails, place)
			} else {
				tails[position] = place
			}
		}
	}
	lcs := len(tails)
	insertions, deletions := len(observed)-lcs, len(expected)-lcs
	replacements := insertions
	if deletions < replacements {
		replacements = deletions
	}
	return lineDiffStats{LCS: lcs, Insertions: insertions - replacements, Deletions: deletions - replacements, Replacements: replacements}
}
