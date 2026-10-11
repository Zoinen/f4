package ap

import (
	"fmt"
	"strconv"
	"strings"
)

// Parse parses an ap-format patch (§2 of the spec) into a PatchData. warn,
// when non-nil, receives one message per tolerant-mode correction (mirrors
// the reference's "[TOLERANT] ..." prints); it is never called in strict
// mode, since any condition that would trigger it is a hard error there.
func Parse(patchText string, strict bool, warn func(string)) (*PatchData, error) {
	text := stripBOM(patchText)
	lines := stripMarkdownFences(splitLinesUniversal(text))

	patchID, version, startIdx, err := locateHeader(lines, strict, warn)
	if err != nil {
		return nil, err
	}

	p := &apParser{
		strict:  strict,
		warn:    warn,
		data:    &PatchData{Version: version, PatchID: patchID},
		patchID: patchID,
	}
	p.knownIDs = []string{patchID}

	if err := p.run(lines, startIdx); err != nil {
		return nil, err
	}

	for _, change := range p.data.Changes {
		pathValue := strings.TrimSpace(change.FilePath)
		if pathValue == "" {
			return nil, fmt.Errorf(
				"a FILE block has an empty path. The path MUST be given either as " +
					"the value block of the FILE directive or as its inline argument")
		}
		if len(splitLinesUniversal(pathValue)) > 1 {
			return nil, fmt.Errorf("a FILE block declares a multi-line path: %q", pathValue)
		}
	}

	return p.data, nil
}

func locateHeader(lines []string, strict bool, warn func(string)) (patchID, version string, startIdx int, err error) {
	for i, line := range lines {
		stripped := strings.TrimSpace(line)
		if stripped == "" {
			continue
		}
		if m := headerRE.FindStringSubmatch(stripped); m != nil {
			id, ver := m[1], m[2]
			if !hexIDRE.MatchString(id) {
				if strict {
					return "", "", 0, fmt.Errorf(
						"invalid patch ID '%s' on line %d. ID MUST be exactly 8 hexadecimal characters", id, i+1)
				}
				callWarn(warn, "Tolerating invalid non-hex or semantic patch ID: '%s'.", id)
			}
			if !supportedVersions[ver] {
				if strict {
					return "", "", 0, fmt.Errorf(
						"unsupported AP version '%s' on line %d. This patcher implements %s", ver, i+1, FormatVersion)
				}
				callWarn(warn, "Patch declares AP %s; parsing it as AP %s.", ver, FormatVersion)
			}
			return id, ver, i + 1, nil
		}
		if !strict {
			if m := headerlessActionRE.FindStringSubmatch(stripped); m != nil {
				id := m[1]
				callWarn(warn, "Missing AP header. Auto-detected patch ID: '%s' from line %d", id, i+1)
				return id, FormatVersion, i, nil
			}
		}
	}
	return "", "", 0, fmt.Errorf("valid AP %s header not found in the file", FormatVersion)
}

func callWarn(warn func(string), format string, args ...any) {
	if warn != nil {
		warn(fmt.Sprintf(format, args...))
	}
}

// apParser holds the mutable state of one parsing pass; it plays the role of
// the closures over local variables in the Python reference.
type apParser struct {
	strict bool
	warn   func(string)

	data    *PatchData
	patchID string

	knownIDs []string

	currentFileChange   *FileChange
	currentModification *Modification
	readingKey          string
	valueLines          []string
	pendingArgs         *string
}

func (p *apParser) warnf(format string, args ...any) {
	callWarn(p.warn, format, args...)
}

// matchDirective reports whether line is a directive line (starts with one
// of the known patch IDs followed by whitespace) and, if so, returns
// everything after that whitespace run.
func (p *apParser) matchDirective(line string) (rest string, ok bool) {
	for _, id := range p.knownIDs {
		if !strings.HasPrefix(line, id) {
			continue
		}
		after := line[len(id):]
		i := 0
		for i < len(after) && isSpaceByte(after[i]) {
			i++
		}
		if i == 0 {
			continue
		}
		return after[i:], true
	}
	return "", false
}

func splitMax1(s string) (first, rest string) {
	i := 0
	for i < len(s) && !isSpaceByte(s[i]) {
		i++
	}
	first = s[:i]
	j := i
	for j < len(s) && isSpaceByte(s[j]) {
		j++
	}
	rest = s[j:]
	return
}

func isSnippetlessAction(action string) bool {
	switch action {
	case "REPLACE", "DELETE", "INSERT_AFTER", "INSERT_BEFORE":
		return true
	}
	return false
}

func modHasKey(m *Modification, key string) bool {
	switch key {
	case "snippet":
		return m.Snippet != nil
	case "anchor":
		return m.Anchor != nil
	case "content":
		return m.Content != nil
	case "snippet_tail":
		return m.SnippetTail != nil
	}
	return false
}

func setModField(m *Modification, key, value string) {
	v := value
	switch key {
	case "snippet":
		m.Snippet = &v
	case "anchor":
		m.Anchor = &v
	case "content":
		m.Content = &v
	case "snippet_tail":
		m.SnippetTail = &v
	}
}

func stringInSlice(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// run is the main directive-scanning loop (mirrors the "for i in
// range(start_idx, len(lines))" loop of the reference).
func (p *apParser) run(lines []string, startIdx int) error {
	insideFencedBlock := false

loop:
	for i := startIdx; i < len(lines); i++ {
		line := lines[i]
		lineNum := i + 1
		stripped := strings.TrimSpace(line)

		if fenceRE.MatchString(line) {
			insideFencedBlock = !insideFencedBlock
		}

		if !insideFencedBlock {
			if m := driftPattern.FindStringSubmatch(stripped); m != nil {
				newID := m[1]
				fields := strings.Fields(m[2])
				keywordPart := ""
				if len(fields) > 0 {
					keywordPart = fields[0]
				}
				if !stringInSlice(p.knownIDs, newID) && canonicalKeys[keywordPart] && isIDLike(newID, p.patchID) {
					if p.strict {
						return fmt.Errorf(
							"patch ID mismatch on line %d: expected '%s', found '%s'. Run without --strict to allow ID correction",
							lineNum, p.patchID, newID)
					}
					p.warnf("ID drift detected on line %d: '%s' -> '%s'. Accepting both.", lineNum, p.patchID, newID)
					p.knownIDs = append(p.knownIDs, newID)
				}
			}
		}

		if rest, matched := p.matchDirective(line); matched {
			if err := p.flushValue(false); err != nil {
				return err
			}

			trimmedRest := strings.TrimSpace(rest)
			rawKey, args := splitMax1(trimmedRest)
			if rawKey == "" {
				continue
			}
			key, ok := resolveDirectiveKey(rawKey, p.strict, p.warn)
			if !ok {
				return fmt.Errorf("unknown directive '%s' on line %d", rawKey, lineNum)
			}

			switch {
			case key == "END":
				if args != "" {
					return fmt.Errorf("directive '%s' on line %d takes no arguments", key, lineNum)
				}
				break loop

			case key == "FILE":
				fc := &FileChange{}
				p.data.Changes = append(p.data.Changes, fc)
				p.currentFileChange = fc
				inlinePath := ""
				if args != "" {
					tok, restTok := splitMax1(args)
					if newlineVals[tok] {
						fc.Newline = tok
						if restTok != "" {
							inlinePath = strings.TrimSpace(restTok)
						}
					} else {
						inlinePath = strings.TrimSpace(args)
					}
				}
				p.currentModification = nil
				if inlinePath != "" {
					fc.FilePath = inlinePath
					fc.HasFilePath = true
					p.readingKey = ""
				} else {
					p.readingKey = "path"
				}

			case actionKeys[key]:
				if p.currentFileChange == nil {
					return fmt.Errorf("action '%s' on line %d before FILE", key, lineNum)
				}
				if args != "" {
					return fmt.Errorf("action '%s' on line %d takes no arguments", key, lineNum)
				}
				m := &Modification{Action: key, Line: lineNum}
				p.currentFileChange.Modifications = append(p.currentFileChange.Modifications, m)
				p.currentModification = m

			case key == "CREATE":
				if err := p.handleCreateDirective(args, lineNum); err != nil {
					return err
				}

			case valueKeys[key]:
				if err := p.handleValueDirective(key, args, lineNum); err != nil {
					return err
				}

			case key == "RENAME":
				if p.currentFileChange == nil {
					return fmt.Errorf("'%s' on line %d outside file block", key, lineNum)
				}
				if len(p.currentFileChange.Modifications) > 0 {
					return fmt.Errorf("'%s' on line %d cannot be combined with other actions in the same file block", key, lineNum)
				}
				if args != "" {
					v := strings.TrimSpace(args)
					p.currentFileChange.RenameTo = &v
					p.readingKey = ""
				} else {
					p.readingKey = "RENAME"
				}

			case argKeys[key]:
				if p.currentModification == nil {
					return fmt.Errorf("'%s' on line %d outside modification", key, lineNum)
				}
				if args == "" {
					if key == "scope_end" {
						p.currentModification.ScopeEnd = 1
						continue
					}
					return fmt.Errorf("directive '%s' on line %d requires an argument", key, lineNum)
				}
				n, convErr := strconv.Atoi(strings.TrimSpace(args))
				if convErr != nil {
					return fmt.Errorf("argument for '%s' on line %d must be an integer", key, lineNum)
				}
				switch key {
				case "include_leading_blank_lines":
					p.currentModification.IncludeLeadingBlankLines = n
				case "include_trailing_blank_lines":
					p.currentModification.IncludeTrailingBlankLines = n
				case "scope_end":
					p.currentModification.ScopeEnd = n
				}

			default:
				return fmt.Errorf("unknown directive '%s' on line %d", key, lineNum)
			}
			continue
		}

		switch {
		case p.readingKey != "":
			if !p.strict && (p.readingKey == "path" || p.readingKey == "RENAME" || p.readingKey == "CREATE_PATH") &&
				strings.HasPrefix(stripped, "#") {
				// Ignore comments inside path values.
			} else {
				p.valueLines = append(p.valueLines, line)
			}
		case stripped == "":
			// Blank line between directives, ignored.
		case !p.strict && strings.HasPrefix(stripped, "#"):
			// Comment between directives, tolerated.
		case !p.strict && fenceRE.MatchString(line):
			// Stray markdown fence between directives.
		case !p.strict && p.currentModification != nil &&
			isSnippetlessAction(p.currentModification.Action) &&
			p.currentModification.Snippet == nil && p.currentModification.Content == nil:
			p.warnf("Text on line %d follows an Action Directive with no "+
				"'snippet' keyword. Treating it as the snippet.", lineNum)
			p.readingKey = "snippet"
			p.valueLines = []string{line}
		default:
			return fmt.Errorf("unexpected content on line %d: '%s'", lineNum, line)
		}
	}

	return p.flushValue(true)
}

func (p *apParser) handleCreateDirective(args string, lineNum int) error {
	if p.currentFileChange != nil && p.currentFileChange.HasFilePath {
		// Contextual Creation: CREATE used after FILE.
		m := &Modification{Action: "CREATE", Line: lineNum}
		p.currentFileChange.Modifications = append(p.currentFileChange.Modifications, m)
		p.currentModification = m
		p.readingKey = "CREATE_CONTENT"
		return nil
	}
	if args != "" && !newlineVals[args] {
		// Path provided as argument: CREATE path/to/file
		fc := &FileChange{FilePath: args, HasFilePath: true}
		p.data.Changes = append(p.data.Changes, fc)
		p.currentFileChange = fc
		m := &Modification{Action: "CREATE", Line: lineNum}
		fc.Modifications = append(fc.Modifications, m)
		p.currentModification = m
		p.readingKey = "content"
		return nil
	}
	// Hybrid: acts as key-value (for path) AND action.
	p.readingKey = "CREATE_PATH"
	if args != "" {
		a := args
		p.pendingArgs = &a
	} else {
		p.pendingArgs = nil
	}
	return nil
}

func (p *apParser) handleValueDirective(key, args string, lineNum int) error {
	if args != "" {
		return fmt.Errorf("directive '%s' on line %d takes no arguments", key, lineNum)
	}
	if p.currentModification == nil && p.currentFileChange != nil && key == "content" {
		// Heuristic: a 'content' block directly after 'FILE' implies 'CREATE'.
		m := &Modification{Action: "CREATE", Line: lineNum}
		p.currentFileChange.Modifications = append(p.currentFileChange.Modifications, m)
		p.currentModification = m
	}
	if p.currentModification == nil {
		return fmt.Errorf("'%s' on line %d outside modification", key, lineNum)
	}
	mod := p.currentModification
	if (key == "snippet" || key == "anchor") && mod.Content != nil {
		// A locator after a finished modification means the model forgot to
		// repeat the Action Directive.
		previousAction := mod.Action
		if p.strict {
			return fmt.Errorf(
				"directive '%s' on line %d starts a new modification but no Action Directive precedes it",
				key, lineNum)
		}
		p.warnf("Missing Action Directive before '%s' on line %d. "+
			"Starting a new '%s' modification.", key, lineNum, previousAction)
		newMod := &Modification{Action: previousAction, Line: lineNum}
		p.currentFileChange.Modifications = append(p.currentFileChange.Modifications, newMod)
		p.currentModification = newMod
	} else if modHasKey(mod, key) {
		if p.strict || key == "content" {
			return fmt.Errorf("duplicate '%s' directive on line %d within one modification", key, lineNum)
		}
		p.warnf("Duplicate '%s' on line %d; the last one wins.", key, lineNum)
	}
	p.readingKey = key
	return nil
}

func (p *apParser) flushValue(atEOF bool) error {
	if p.readingKey == "" {
		return nil
	}
	// A value block closed by the END of the file is the one place where an
	// empty block is ambiguous: it may be deliberate, or the answer may have
	// been cut off mid-generation.
	if atEOF && p.currentModification != nil && valueKeys[p.readingKey] {
		p.currentModification.eofValue = p.readingKey
	}

	start := 0
	for start < len(p.valueLines) && strings.TrimSpace(p.valueLines[start]) == "" {
		start++
	}
	end := len(p.valueLines)
	for end > start && strings.TrimSpace(p.valueLines[end-1]) == "" {
		end--
	}
	value := strings.Join(p.valueLines[start:end], "\n")

	switch {
	case p.readingKey == "CREATE_CONTENT":
		if value != "" && p.currentModification != nil {
			v := value
			p.currentModification.Content = &v
		}
	case p.readingKey == "path" && p.currentFileChange != nil:
		p.currentFileChange.FilePath = value
		p.currentFileChange.HasFilePath = true
	case p.readingKey == "CREATE_PATH":
		if value != "" {
			fc := &FileChange{FilePath: value, HasFilePath: true}
			p.data.Changes = append(p.data.Changes, fc)
			p.currentFileChange = fc
			if p.pendingArgs != nil && newlineVals[*p.pendingArgs] {
				fc.Newline = *p.pendingArgs
			}
		}
		if p.currentFileChange == nil {
			return fmt.Errorf("action 'CREATE' used before any FILE directive")
		}
		m := &Modification{Action: "CREATE"}
		p.currentFileChange.Modifications = append(p.currentFileChange.Modifications, m)
		p.currentModification = m
	case p.readingKey == "RENAME" && p.currentFileChange != nil && p.currentModification == nil:
		v := value
		p.currentFileChange.RenameTo = &v
	case p.currentModification != nil:
		setModField(p.currentModification, p.readingKey, value)
	case p.readingKey == "RENAME" && p.currentFileChange != nil:
		v := value
		p.currentFileChange.RenameTo = &v
	}

	p.readingKey = ""
	p.valueLines = nil
	p.pendingArgs = nil
	return nil
}
