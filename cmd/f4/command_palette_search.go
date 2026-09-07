package main

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// commandPaletteMatchKind is ordered from the strongest to the weakest
// supported match.  Keeping the order explicit makes ranking independent of
// arbitrary score constants and prevents a collection of weak matches from
// overtaking a uniformly stronger result.
type commandPaletteMatchKind uint8

const (
	commandPaletteMatchExact commandPaletteMatchKind = iota
	commandPaletteMatchPrefix
	commandPaletteMatchSubstring
	commandPaletteMatchSubsequence
	commandPaletteMatchTypo
	commandPaletteMatchNone
)

// commandPaletteFieldTier expresses the relative importance of searchable
// metadata. Labels are what users see, IDs and categories are the next most
// useful stable vocabulary, and supporting metadata comes last.
type commandPaletteFieldTier uint8

const (
	commandPaletteFieldLabel commandPaletteFieldTier = iota
	commandPaletteFieldIdentity
	commandPaletteFieldSupporting
)

type commandPaletteSearchField struct {
	text string
	tier commandPaletteFieldTier
}

type commandPaletteTextMatch struct {
	kind   commandPaletteMatchKind
	tier   commandPaletteFieldTier
	detail int
}

type commandPaletteRank struct {
	entry commandPaletteEntry
	index int

	worstKind   commandPaletteMatchKind
	phrase      commandPaletteTextMatch
	kindTotal   int
	fieldTotal  int
	detailTotal int

	recentRank int
	recent     bool

	category string
	label    string
	key      string
}

// rankCommandPaletteEntries filters and ranks command-palette entries without
// consulting global UI state. Every whitespace-delimited query token must
// match at least one field. Recent commands affect ordering only after all
// textual ranking dimensions compare equal.
func rankCommandPaletteEntries(entries []commandPaletteEntry, query string, recent []string) []commandPaletteEntry {
	normalizedQuery := normalizeCommandPaletteText(query)
	recentRanks := commandPaletteRecentRanks(recent)

	ranked := make([]commandPaletteRank, 0, len(entries))
	if normalizedQuery == "" {
		for index, entry := range entries {
			rank := newCommandPaletteRank(entry, index, recentRanks)
			ranked = append(ranked, rank)
		}
		sort.SliceStable(ranked, func(i, j int) bool {
			return commandPaletteRankLess(ranked[i], ranked[j], false)
		})
		return commandPaletteRankEntries(ranked)
	}

	tokens := strings.Fields(normalizedQuery)
	for index, entry := range entries {
		fields := commandPaletteEntrySearchFields(entry)
		if len(fields) == 0 {
			continue
		}

		rank := newCommandPaletteRank(entry, index, recentRanks)
		rank.worstKind = commandPaletteMatchExact
		rank.phrase = commandPaletteBestPhraseMatch(normalizedQuery, fields)

		matched := true
		for _, token := range tokens {
			best := commandPaletteBestTokenMatch(token, fields)
			if best.kind == commandPaletteMatchNone {
				matched = false
				break
			}
			if best.kind > rank.worstKind {
				rank.worstKind = best.kind
			}
			rank.kindTotal += int(best.kind)
			rank.fieldTotal += int(best.tier)
			rank.detailTotal += best.detail
		}
		if matched {
			ranked = append(ranked, rank)
		}
	}

	sort.SliceStable(ranked, func(i, j int) bool {
		return commandPaletteRankLess(ranked[i], ranked[j], true)
	})
	return commandPaletteRankEntries(ranked)
}

func commandPaletteRankEntries(ranked []commandPaletteRank) []commandPaletteEntry {
	result := make([]commandPaletteEntry, len(ranked))
	for index := range ranked {
		result[index] = ranked[index].entry
	}
	return result
}

func newCommandPaletteRank(entry commandPaletteEntry, index int, recentRanks map[string]int) commandPaletteRank {
	label := entry.Label
	if strings.TrimSpace(label) == "" {
		label = entry.EnglishLabel
	}
	rank := commandPaletteRank{
		entry:      entry,
		index:      index,
		phrase:     commandPaletteTextMatch{kind: commandPaletteMatchNone, tier: commandPaletteFieldSupporting},
		recentRank: len(recentRanks),
		category:   normalizeCommandPaletteText(entry.Category),
		label:      normalizeCommandPaletteText(label),
		key:        normalizeCommandPaletteText(entry.Key),
	}
	if recentRank, ok := recentRanks[rank.key]; ok && rank.key != "" {
		rank.recent = true
		rank.recentRank = recentRank
	}
	return rank
}

func commandPaletteRecentRanks(recent []string) map[string]int {
	ranks := make(map[string]int, len(recent))
	for index, key := range recent {
		normalized := normalizeCommandPaletteText(key)
		if normalized == "" {
			continue
		}
		if _, exists := ranks[normalized]; !exists {
			ranks[normalized] = index
		}
	}
	return ranks
}

func commandPaletteRankLess(left, right commandPaletteRank, searching bool) bool {
	if searching {
		if left.worstKind != right.worstKind {
			return left.worstKind < right.worstKind
		}
		if comparison := compareCommandPaletteMatches(left.phrase, right.phrase); comparison != 0 {
			return comparison < 0
		}
		if left.kindTotal != right.kindTotal {
			return left.kindTotal < right.kindTotal
		}
		if left.fieldTotal != right.fieldTotal {
			return left.fieldTotal < right.fieldTotal
		}
		if left.detailTotal != right.detailTotal {
			return left.detailTotal < right.detailTotal
		}
	}

	if left.recent != right.recent {
		return left.recent
	}
	if left.recent && left.recentRank != right.recentRank {
		return left.recentRank < right.recentRank
	}
	if left.category != right.category {
		return left.category < right.category
	}
	if left.label != right.label {
		return left.label < right.label
	}
	if left.key != right.key {
		return left.key < right.key
	}
	return left.index < right.index
}

func compareCommandPaletteMatches(left, right commandPaletteTextMatch) int {
	if left.kind != right.kind {
		if left.kind < right.kind {
			return -1
		}
		return 1
	}
	// The field carrying no match is irrelevant. In particular, an entry must
	// not gain a phrase-ranking advantage merely because it has a label field.
	if left.kind == commandPaletteMatchNone {
		return 0
	}
	if left.tier != right.tier {
		if left.tier < right.tier {
			return -1
		}
		return 1
	}
	if left.detail < right.detail {
		return -1
	}
	if left.detail > right.detail {
		return 1
	}
	return 0
}

func commandPaletteEntrySearchFields(entry commandPaletteEntry) []commandPaletteSearchField {
	fields := make([]commandPaletteSearchField, 0, 9+len(entry.SearchFields))
	add := func(text string, tier commandPaletteFieldTier) {
		text = normalizeCommandPaletteText(text)
		if text != "" {
			fields = append(fields, commandPaletteSearchField{text: text, tier: tier})
		}
	}

	add(entry.Label, commandPaletteFieldLabel)
	add(entry.EnglishLabel, commandPaletteFieldLabel)
	add(entry.ID, commandPaletteFieldIdentity)
	add(entry.Category, commandPaletteFieldIdentity)
	add(entry.Key, commandPaletteFieldIdentity)
	add(entry.Description, commandPaletteFieldSupporting)
	add(entry.EnglishDescription, commandPaletteFieldSupporting)
	for _, field := range entry.SearchFields {
		add(field, commandPaletteFieldSupporting)
	}
	add(entry.Shortcut, commandPaletteFieldSupporting)
	return fields
}

func commandPaletteBestPhraseMatch(query string, fields []commandPaletteSearchField) commandPaletteTextMatch {
	best := commandPaletteTextMatch{kind: commandPaletteMatchNone, tier: commandPaletteFieldSupporting}
	for _, field := range fields {
		match := commandPalettePhraseMatch(query, field)
		if compareCommandPaletteMatches(match, best) < 0 {
			best = match
		}
	}
	return best
}

func commandPalettePhraseMatch(query string, field commandPaletteSearchField) commandPaletteTextMatch {
	if field.text == query {
		return commandPaletteTextMatch{kind: commandPaletteMatchExact, tier: field.tier}
	}
	if strings.HasPrefix(field.text, query) {
		return commandPaletteTextMatch{
			kind:   commandPaletteMatchPrefix,
			tier:   field.tier,
			detail: utf8.RuneCountInString(field.text) - utf8.RuneCountInString(query),
		}
	}
	if index := strings.Index(field.text, query); index >= 0 {
		return commandPaletteTextMatch{
			kind:   commandPaletteMatchSubstring,
			tier:   field.tier,
			detail: utf8.RuneCountInString(field.text[:index]),
		}
	}
	return commandPaletteTextMatch{kind: commandPaletteMatchNone, tier: field.tier}
}

func commandPaletteBestTokenMatch(token string, fields []commandPaletteSearchField) commandPaletteTextMatch {
	best := commandPaletteTextMatch{kind: commandPaletteMatchNone, tier: commandPaletteFieldSupporting}
	for _, field := range fields {
		match := commandPaletteTokenMatch(token, field)
		if compareCommandPaletteMatches(match, best) < 0 {
			best = match
		}
	}
	return best
}

func commandPaletteTokenMatch(token string, field commandPaletteSearchField) commandPaletteTextMatch {
	if field.text == token {
		return commandPaletteTextMatch{kind: commandPaletteMatchExact, tier: field.tier}
	}

	fieldRunes := utf8.RuneCountInString(field.text)
	tokenRunes := utf8.RuneCountInString(token)
	bestPrefixDetail := -1
	if strings.HasPrefix(field.text, token) {
		bestPrefixDetail = fieldRunes - tokenRunes
	}
	wordOffset := 0
	for _, word := range strings.Fields(field.text) {
		if strings.HasPrefix(word, token) {
			detail := wordOffset + utf8.RuneCountInString(word) - tokenRunes
			if bestPrefixDetail < 0 || detail < bestPrefixDetail {
				bestPrefixDetail = detail
			}
		}
		wordOffset += utf8.RuneCountInString(word) + 1
	}
	if bestPrefixDetail >= 0 {
		return commandPaletteTextMatch{kind: commandPaletteMatchPrefix, tier: field.tier, detail: bestPrefixDetail}
	}

	if index := strings.Index(field.text, token); index >= 0 {
		return commandPaletteTextMatch{
			kind:   commandPaletteMatchSubstring,
			tier:   field.tier,
			detail: utf8.RuneCountInString(field.text[:index]) + fieldRunes - tokenRunes,
		}
	}

	if gaps, start, ok := commandPaletteSubsequence(token, field.text); ok {
		return commandPaletteTextMatch{
			kind:   commandPaletteMatchSubsequence,
			tier:   field.tier,
			detail: gaps*2 + start,
		}
	}

	maxDistance := tokenRunes / 3
	if maxDistance < 1 {
		maxDistance = 1
	}
	bestDistance := maxDistance + 1
	bestLengthDifference := maxDistance + 1
	for _, word := range strings.Fields(field.text) {
		wordRunes := utf8.RuneCountInString(word)
		lengthDifference := wordRunes - tokenRunes
		if lengthDifference < 0 {
			lengthDifference = -lengthDifference
		}
		if lengthDifference > maxDistance {
			continue
		}
		if distance, ok := boundedCommandPaletteDamerauLevenshtein(token, word, maxDistance); ok &&
			(distance < bestDistance || distance == bestDistance && lengthDifference < bestLengthDifference) {
			bestDistance = distance
			bestLengthDifference = lengthDifference
		}
	}
	if bestDistance <= maxDistance {
		return commandPaletteTextMatch{
			kind:   commandPaletteMatchTypo,
			tier:   field.tier,
			detail: bestDistance*4 + bestLengthDifference,
		}
	}

	return commandPaletteTextMatch{kind: commandPaletteMatchNone, tier: field.tier}
}

// commandPaletteSubsequence checks an ordered Unicode-rune subsequence and
// returns its internal gap count and starting offset for deterministic ranking.
func commandPaletteSubsequence(needle, haystack string) (gaps int, start int, ok bool) {
	needleRunes := []rune(needle)
	if len(needleRunes) == 0 {
		return 0, 0, true
	}

	matched := 0
	last := -1
	start = -1
	for index, current := range []rune(haystack) {
		if current != needleRunes[matched] {
			continue
		}
		if start < 0 {
			start = index
		} else {
			gaps += index - last - 1
		}
		last = index
		matched++
		if matched == len(needleRunes) {
			return gaps, start, true
		}
	}
	return 0, 0, false
}

// boundedCommandPaletteDamerauLevenshtein computes optimal-string-alignment
// Damerau-Levenshtein distance over runes. The boolean reports whether the
// distance stays within maxDistance.
func boundedCommandPaletteDamerauLevenshtein(left, right string, maxDistance int) (int, bool) {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	lengthDifference := len(leftRunes) - len(rightRunes)
	if lengthDifference < 0 {
		lengthDifference = -lengthDifference
	}
	if lengthDifference > maxDistance {
		return maxDistance + 1, false
	}
	if len(leftRunes) == 0 {
		return len(rightRunes), len(rightRunes) <= maxDistance
	}
	if len(rightRunes) == 0 {
		return len(leftRunes), len(leftRunes) <= maxDistance
	}

	previousPrevious := make([]int, len(rightRunes)+1)
	previous := make([]int, len(rightRunes)+1)
	current := make([]int, len(rightRunes)+1)
	for column := range previous {
		previous[column] = column
	}

	for row := 1; row <= len(leftRunes); row++ {
		current[0] = row
		for column := 1; column <= len(rightRunes); column++ {
			cost := 1
			if leftRunes[row-1] == rightRunes[column-1] {
				cost = 0
			}
			current[column] = minCommandPaletteInt(
				previous[column]+1,
				current[column-1]+1,
				previous[column-1]+cost,
			)
			if row > 1 && column > 1 &&
				leftRunes[row-1] == rightRunes[column-2] &&
				leftRunes[row-2] == rightRunes[column-1] {
				transposed := previousPrevious[column-2] + 1
				if transposed < current[column] {
					current[column] = transposed
				}
			}
		}
		previousPrevious, previous, current = previous, current, previousPrevious
	}

	distance := previous[len(rightRunes)]
	return distance, distance <= maxDistance
}

func minCommandPaletteInt(values ...int) int {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

// normalizeCommandPaletteText applies the same lightweight normalization to
// queries and metadata. It deliberately retains punctuation (useful in stable
// IDs and shortcuts), removes menu accelerator markers, lowercases Unicode,
// and collapses every Unicode whitespace run to one ASCII space.
func normalizeCommandPaletteText(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	pendingSpace := false
	wroteText := false
	for _, current := range strings.ToLower(value) {
		if current == '&' {
			continue
		}
		if unicode.IsSpace(current) {
			if wroteText {
				pendingSpace = true
			}
			continue
		}
		if pendingSpace {
			result.WriteByte(' ')
			pendingSpace = false
		}
		result.WriteRune(current)
		wroteText = true
	}
	return result.String()
}
