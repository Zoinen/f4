package ap

import "strings"

// PreviewContext is how many unchanged lines a Preview keeps on each side of
// the changed ones - the usual unified-diff amount.
const PreviewContext = 3

// Preview is one applied modification's edit as a fragment of its file: the
// changed lines with up to PreviewContext unchanged lines around them, as
// the file read just before the modification ran and just after. Earlier
// modifications of the same FILE block are already part of both sides - it
// is the diff of this one edit, not of the whole patch against the disk.
//
// Lines carry no line terminator and are always LF-split (the engine works
// on LF-normalized text whatever the file's own endings are). A difference
// in the file's final newline alone is not shown.
type Preview struct {
	// StartLine is the 1-based line number of Before[0] and After[0] (the
	// fragment's first line is the same on both sides: it is context, or
	// the edit starts at the very top of the file). It counts lines of the
	// file as it stood when the modification ran.
	StartLine int
	// Before and After are the fragment before and after the edit. Either
	// may be empty (a new file, or everything deleted).
	Before, After []string
}

// modPreview cuts the edit that turned before into after down to a Preview:
// the lines both share at the top and at the bottom are the context, what
// lies between is the change. A modification rewrites one contiguous span of
// the file, so this is exact for everything applyOneModification does; for
// two identical texts (nothing to show) it returns nil.
func modPreview(before, after string) *Preview {
	a, b := previewLines(before), previewLines(after)
	head := 0
	for head < len(a) && head < len(b) && a[head] == b[head] {
		head++
	}
	if head == len(a) && head == len(b) {
		return nil
	}
	tail := 0
	for tail < len(a)-head && tail < len(b)-head && a[len(a)-1-tail] == b[len(b)-1-tail] {
		tail++
	}
	start := max(head-PreviewContext, 0)
	keep := min(tail, PreviewContext)
	return &Preview{
		StartLine: start + 1,
		Before:    append([]string(nil), a[start:len(a)-tail+keep]...),
		After:     append([]string(nil), b[start:len(b)-tail+keep]...),
	}
}

// previewLines splits LF text into lines; the empty string after a final
// newline is not a line of its own.
func previewLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
