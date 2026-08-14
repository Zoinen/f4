package vtvibe

import (
	"regexp"
	"strings"
)

// Recognition of the ap patch format (github.com/unxed/ap) inside a model
// answer. We only detect and slice out the patch here; applying it is the
// host's business, see vtvibe_ap.go.
//
// The rules are the ones vtvibe.md, section 8.3 asks for: look at fenced
// blocks first, take a block that carries the "[8 hex] AP 3.1" header or a
// consistent pair of "[8 hex] FILE" plus an action directive, and fall back
// to the bare text of the answer when the model forgot the fences.

// PatchPath is where a detected patch is filed inside the session, so F3, F4
// and F5 work on it like on any other artifact.
const PatchPath = outDir + "/afix.ap"

// Patch is one ap patch found in an answer.
type Patch struct {
	// ID is the 8 hex character prefix every directive of the patch carries.
	ID string
	// Text is the patch itself, ready to be handed to a patcher.
	Text string
	// Files lists the targets of the FILE directives, in order, for the
	// confirmation dialog. It is a courtesy for the human, not a contract:
	// the patcher reads the paths from the patch, not from this slice.
	Files []string
	// Ignored counts patch blocks that used a different ID and were left
	// out, because merging patches with different IDs is not possible.
	Ignored int
}

var (
	apHeaderRe = regexp.MustCompile(`(?m)^([0-9a-f]{8})[ \t]+AP[ \t]+[0-9]+(?:\.[0-9]+)?[ \t]*\r?$`)
	apFileRe   = regexp.MustCompile(`(?m)^([0-9a-f]{8})[ \t]+FILE(?:[ \t]+(?:LF|CRLF|CR))?[ \t]*\r?$`)
	apActionRe = regexp.MustCompile(`(?m)^([0-9a-f]{8})[ \t]+(?:REPLACE|INSERT_AFTER|INSERT_BEFORE|DELETE|CREATE|RECREATE|RENAME)\b`)
)

// detectPatchID returns the patch ID of text, or "" when text is not a patch.
//
// A header is enough on its own. Without one we require both a FILE directive
// and an action directive sharing the same ID: that is what tells a patch
// apart from a code sample that merely happens to start with a hex word.
func detectPatchID(text string) string {
	if m := apHeaderRe.FindStringSubmatch(text); m != nil {
		return m[1]
	}
	fm := apFileRe.FindStringSubmatch(text)
	am := apActionRe.FindStringSubmatch(text)
	if fm != nil && am != nil && fm[1] == am[1] {
		return fm[1]
	}
	return ""
}

// patchFiles collects the value of every FILE directive: the first non empty
// line after it that is not a directive itself.
func patchFiles(text, id string) []string {
	lines := strings.Split(text, "\n")
	prefix := id + " "
	var out []string
	seen := map[string]bool{}
	for i, line := range lines {
		m := apFileRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil || m[1] != id {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			v := strings.TrimSpace(strings.TrimRight(lines[j], "\r"))
			if v == "" {
				continue
			}
			if strings.HasPrefix(v, prefix) {
				break // the block has no path, let the patcher complain
			}
			if !seen[v] {
				seen[v] = true
				out = append(out, v)
			}
			break
		}
	}
	return out
}

// fencedBlocks returns the contents of every triple backtick block. An
// unterminated block counts too: a truncated answer still carries a usable
// patch more often than not.
func fencedBlocks(reply string) []string {
	var blocks []string
	var buf []string
	inBlock := false
	for _, line := range strings.Split(reply, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inBlock {
				blocks = append(blocks, strings.Join(buf, "\n"))
			}
			inBlock, buf = !inBlock, nil
			continue
		}
		if inBlock {
			buf = append(buf, line)
		}
	}
	if inBlock && len(buf) > 0 {
		blocks = append(blocks, strings.Join(buf, "\n"))
	}
	return blocks
}

// unfencedPatch slices the patch out of an answer that has no fences around
// it: everything from the header (or from the first directive) to the end of
// the text, or to the next fence if the model started one afterwards.
func unfencedPatch(reply, id string) string {
	lines := strings.Split(reply, "\n")
	start := -1
	for i, line := range lines {
		l := strings.TrimRight(line, "\r")
		if start == -1 {
			if apHeaderRe.MatchString(l) || strings.HasPrefix(l, id+" ") {
				start = i
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			return strings.Join(lines[start:i], "\n")
		}
	}
	if start == -1 {
		return ""
	}
	return strings.Join(lines[start:], "\n")
}

// ExtractPatch returns the ap patch carried by a model answer, or nil.
//
// Several blocks with the same ID are concatenated: models like to split a
// long patch. Blocks with a different ID cannot be merged, so the first one
// wins and the rest are only counted, for the panel to mention them.
func ExtractPatch(reply string) *Patch {
	var texts, ids []string
	for _, b := range fencedBlocks(reply) {
		if id := detectPatchID(b); id != "" {
			texts = append(texts, strings.Trim(b, "\n"))
			ids = append(ids, id)
		}
	}
	if len(texts) == 0 {
		if id := detectPatchID(reply); id != "" {
			if body := strings.Trim(unfencedPatch(reply, id), "\n"); body != "" {
				texts = append(texts, body)
				ids = append(ids, id)
			}
		}
	}
	if len(texts) == 0 {
		return nil
	}

	p := &Patch{ID: ids[0], Text: texts[0]}
	for i := 1; i < len(texts); i++ {
		if ids[i] == p.ID {
			p.Text += "\n\n" + texts[i]
		} else {
			p.Ignored++
		}
	}
	if !strings.HasSuffix(p.Text, "\n") {
		p.Text += "\n"
	}
	p.Files = patchFiles(p.Text, p.ID)
	return p
}
