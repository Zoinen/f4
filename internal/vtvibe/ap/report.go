package ap

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// failedItem is one modification that failed to apply, kept for the
// afailed.md report.
type failedItem struct {
	ModIdx int
	Mod    *Modification
	Err    *AppError
}

// fileReport is the per-file section of afailed.md: what was asked for
// (TotalMods), what is now on disk (Current) vs. before the run
// (Original), and which modifications failed.
type fileReport struct {
	FilePath  string
	Original  string
	Current   string
	TotalMods int
	Failed    []failedItem
}

// fatalInfo describes a patch-level (not per-modification) failure.
type fatalInfo struct {
	HasFilePath bool
	FilePath    string
	Err         *AppError
}

func fence(text, lang string) string {
	body := text
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	label := ""
	if lang != "" {
		label = " " + strings.ToUpper(lang)
	}
	return fmt.Sprintf("--- BEGIN%s ---\n%s--- END%s ---\n", label, body, label)
}

func numbered(text string, limit int) string {
	lines := strings.Split(text, "\n")
	truncated := len(lines) > limit
	shown := lines
	if truncated {
		shown = lines[:limit]
	}
	width := len(strconv.Itoa(len(shown)))
	var b strings.Builder
	for i, l := range shown {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%*d | %s", width, i+1, l)
	}
	if truncated {
		fmt.Fprintf(&b, "\n... (%d more lines omitted)", len(lines)-limit)
	}
	return b.String()
}

type diffOp struct {
	tag  byte
	line string
}

// diffLines aligns two line slices with an LCS-based edit script. Guarded by
// the caller against pathologically large inputs.
func diffLines(a, b []string) []diffOp {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}

// unifiedDiff renders a diff-like text between a and b. It is not a
// byte-for-byte port of Python's difflib.unified_diff (no context trimming
// into multiple hunks), but uses the same +/-/space line prefixes and
// "--- a\n+++ b\n" header shape, which is what afailed.md's readers (models
// and humans) actually need.
func unifiedDiff(a, b, fromFile, toFile string) string {
	aLines := splitLinesKeepEnds(a)
	bLines := splitLinesKeepEnds(b)
	var buf strings.Builder
	fmt.Fprintf(&buf, "--- %s\n", fromFile)
	fmt.Fprintf(&buf, "+++ %s\n", toFile)
	fmt.Fprintf(&buf, "@@ -1,%d +1,%d @@\n", len(aLines), len(bLines))
	if len(aLines)*len(bLines) > 2000000 {
		buf.WriteString("(diff omitted: file too large)\n")
		return buf.String()
	}
	for _, op := range diffLines(aLines, bLines) {
		line := op.line
		if !strings.HasSuffix(line, "\n") {
			line += "\n"
		}
		buf.WriteByte(op.tag)
		buf.WriteString(line)
	}
	return buf.String()
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// writeLLMReport writes afailed.md: everything a model needs to repair the
// patch in one round trip.
func writeLLMReport(path, patchContent string, fileReports []fileReport, fatal *fatalInfo, strict bool) error {
	var out []string

	total := 0
	for _, fr := range fileReports {
		total += len(fr.Failed)
	}
	if fatal != nil {
		total++
	}
	out = append(out, fmt.Sprintf("# ap patch report: %d problem(s)\n", total))
	out = append(out, "This file was written by the `ap` patcher for the model that generated the patch.\n"+
		"**Read it, then emit a NEW `ap` patch containing only the fixes below.**\n"+
		"Do not resend modifications that already applied - the files on disk already contain them,\n"+
		"and the 'Current content' sections below show their state *after* the partial application.\n")

	if fatal != nil {
		out = append(out, "## Fatal error\n")
		out = append(out, fmt.Sprintf("- **Code:** `%s`", fatal.Err.Code))
		if fatal.HasFilePath {
			out = append(out, fmt.Sprintf("- **File:** `%s`", fatal.FilePath))
		}
		out = append(out, fmt.Sprintf("- **Message:** %s\n", fatal.Err.Message))
		if hint, ok := fixHints[fatal.Err.Code]; ok {
			out = append(out, fmt.Sprintf("**How to fix:** %s\n", hint))
		}
		if strict {
			out = append(out, "The patcher ran in strict mode, so **nothing was written to disk**: "+
				"the whole patch must be resent, corrected.\n")
		}
	}

	for _, fr := range fileReports {
		ok := fr.TotalMods - len(fr.Failed)
		out = append(out, fmt.Sprintf("## File `%s`\n", fr.FilePath))
		out = append(out, fmt.Sprintf("%d of %d modification(s) applied or skipped as already present; %d failed.\n",
			ok, fr.TotalMods, len(fr.Failed)))

		for _, item := range fr.Failed {
			mod := item.Mod
			aerr := item.Err
			where := ""
			if mod.Line != 0 {
				where = fmt.Sprintf(" (patch line %d)", mod.Line)
			}
			action := mod.Action
			if action == "" {
				action = "UNKNOWN"
			}
			out = append(out, fmt.Sprintf("### Modification #%d - `%s`%s\n", item.ModIdx+1, action, where))
			out = append(out, fmt.Sprintf("- **Code:** `%s`", aerr.Code))
			out = append(out, fmt.Sprintf("- **Message:** %s\n", aerr.Message))
			if hint, ok := fixHints[aerr.Code]; ok {
				out = append(out, fmt.Sprintf("**How to fix:** %s\n", hint))
			}

			ctx := aerr.Context
			if ctx != nil {
				if ml, ok := ctx["match_lines"].([]int); ok && len(ml) > 0 {
					strs := make([]string, len(ml))
					for i, v := range ml {
						strs[i] = strconv.Itoa(v)
					}
					out = append(out, "Matched at lines: "+strings.Join(strs, ", ")+"\n")
				}
				if h, ok := ctx["hint"].(string); ok && h != "" {
					out = append(out, h+"\n")
				}
				if pm, ok := ctx["partial_line_matches"].([]partialLineMatch); ok && len(pm) > 0 {
					out = append(out, "Lines that *contain* the snippet as a fragment:\n")
					var lines []string
					for _, m := range pm {
						lines = append(lines, fmt.Sprintf("%d | %s", m.LineNumber, m.Text))
					}
					out = append(out, fence(strings.Join(lines, "\n"), ""))
				}
				if fm, ok := ctx["fuzzy_matches"].([]fuzzyMatch); ok && len(fm) > 0 {
					out = append(out, "Closest existing text (did you mean one of these?):\n")
					for _, m := range fm {
						out = append(out, fmt.Sprintf("- line %d, similarity %s:", m.LineNumber, formatFloat(m.Score)))
						out = append(out, fence(m.Text, ""))
					}
				}
			}

			out = append(out, "What the failed modification asked for:\n")
			var sent []string
			if mod.Anchor != nil {
				sent = append(sent, "[anchor]\n"+*mod.Anchor)
			}
			if mod.Snippet != nil {
				sent = append(sent, "[snippet]\n"+*mod.Snippet)
			}
			if mod.SnippetTail != nil {
				sent = append(sent, "[snippet_tail]\n"+*mod.SnippetTail)
			}
			if mod.Content != nil {
				sent = append(sent, "[content]\n"+*mod.Content)
			}
			body := "(no locators)"
			if len(sent) > 0 {
				body = strings.Join(sent, "\n\n")
			}
			out = append(out, fence(body, ""))
		}

		if fr.Original != fr.Current {
			diffText := unifiedDiff(fr.Original, fr.Current, "a/"+fr.FilePath, "b/"+fr.FilePath)
			out = append(out, "### What the patcher already changed in this file\n")
			out = append(out, fence(diffText, "diff"))
		} else {
			out = append(out, "### This file was not modified at all\n")
		}

		out = append(out, "### Current content of the file, as it is on disk right now\n")
		out = append(out, "Use these exact lines when building new locators.\n")
		out = append(out, fence(numbered(fr.Current, 400), ""))
	}

	if patchContent != "" {
		out = append(out, "## The patch that failed\n")
		out = append(out, fence(patchContent, ""))
	}

	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o600)
}

// failedFileBlock is one FILE block's worth of modifications that failed in
// tolerant mode, replayed into afailed.ap so a following patch run can retry
// just them.
type failedFileBlock struct {
	FilePath      string
	Newline       string
	Modifications []*Modification
}

func writeAfailedAP(path, patchID string, blocks []*failedFileBlock) error {
	var b strings.Builder
	b.WriteString("# Summary: Failed changes from a tolerant patch application.\n\n")
	fmt.Fprintf(&b, "%s AP %s\n\n", patchID, FormatVersion)
	for _, block := range blocks {
		fmt.Fprintf(&b, "%s FILE", patchID)
		if block.Newline != "" {
			fmt.Fprintf(&b, " %s", block.Newline)
		}
		fmt.Fprintf(&b, "\n%s\n\n", block.FilePath)
		for _, mod := range block.Modifications {
			fmt.Fprintf(&b, "%s %s\n", patchID, mod.Action)
			if mod.Anchor != nil {
				fmt.Fprintf(&b, "%s anchor\n%s\n", patchID, *mod.Anchor)
			}
			if mod.Snippet != nil {
				fmt.Fprintf(&b, "%s snippet\n%s\n", patchID, *mod.Snippet)
			}
			if mod.SnippetTail != nil {
				fmt.Fprintf(&b, "%s snippet_tail\n%s\n", patchID, *mod.SnippetTail)
			}
			if mod.Content != nil {
				fmt.Fprintf(&b, "%s content\n%s\n", patchID, *mod.Content)
			}
			if mod.IncludeLeadingBlankLines != 0 {
				fmt.Fprintf(&b, "%s include_leading_blank_lines %d\n", patchID, mod.IncludeLeadingBlankLines)
			}
			if mod.IncludeTrailingBlankLines != 0 {
				fmt.Fprintf(&b, "%s include_trailing_blank_lines %d\n", patchID, mod.IncludeTrailingBlankLines)
			}
			if mod.ScopeEnd != 0 {
				fmt.Fprintf(&b, "%s scope_end %d\n", patchID, mod.ScopeEnd)
			}
			b.WriteString("\n")
		}
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}
