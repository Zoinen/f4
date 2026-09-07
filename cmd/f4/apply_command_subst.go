package main

import (
	"fmt"
	"strings"
	"unicode"
)

// ApplyCommandDialect controls quoting for aggregate command arguments. Scalar
// substitutions are deliberately left untouched, matching Far's template
// contract and allowing callers to choose where quoting belongs.
type ApplyCommandDialect uint8

const (
	ApplyCommandDialectRaw ApplyCommandDialect = iota
	ApplyCommandDialectPOSIX
	ApplyCommandDialectCMD
	ApplyCommandDialectPowerShell
)

type ApplyCommandPanelSide uint8

const (
	ApplyCommandLeftSide ApplyCommandPanelSide = iota
	ApplyCommandRightSide
)

type ApplyCommandPanelSelector uint8

const (
	ApplyCommandPanelActive ApplyCommandPanelSelector = iota
	ApplyCommandPanelPassive
	ApplyCommandPanelLeft
	ApplyCommandPanelRight
)

// ApplyCommandPathStyle describes panel path syntax independently from the
// command shell used to execute a template. PowerShell can target POSIX paths,
// and a POSIX-compatible shell can operate on Windows-shaped VFS paths.
type ApplyCommandPathStyle uint8

const (
	ApplyCommandPathStyleUnknown ApplyCommandPathStyle = iota
	ApplyCommandPathStylePOSIX
	ApplyCommandPathStyleWindows
)

type ApplyCommandFile struct {
	Name        string
	ShortName   string
	Description string
}

// ApplyCommandPanel is an immutable snapshot from the substitution engine's
// point of view. Empty short and real forms fall back to their long/logical
// counterparts. An empty Selected slice falls back to Current for aggregates.
type ApplyCommandPanel struct {
	PathStyle          ApplyCommandPathStyle
	Directory          string
	ShortDirectory     string
	RealDirectory      string
	RealShortDirectory string
	Current            ApplyCommandFile
	Selected           []ApplyCommandFile
}

type ApplyCommandContext struct {
	Dialect    ApplyCommandDialect
	ActiveSide ApplyCommandPanelSide
	Active     ApplyCommandPanel
	Passive    ApplyCommandPanel
}

type ApplyCommandPromptSpec struct {
	Index           int
	History         string
	TitleTemplate   string
	InitialTemplate string
}

type ApplyCommandResolvedPrompt struct {
	Index   int
	History string
	Title   string
	Initial string
}

// ApplyCommandPromptValues are collected by UI code before a batch is queued.
// An explicitly supplied empty value is different from a missing map entry.
type ApplyCommandPromptValues map[int]string

type ApplyCommandCardinality uint8

const (
	ApplyCommandPerTarget ApplyCommandCardinality = iota
	ApplyCommandOnce
)

type ApplyCommandTemplateMetadata struct {
	HasAggregate      bool
	RequiresListFiles bool
	RequiresDialect   bool
	AggregatePanels   []ApplyCommandPanelSelector
}

type ApplyCommandListEncoding uint8

const (
	ApplyCommandListUTF8 ApplyCommandListEncoding = iota
	ApplyCommandListANSI
	ApplyCommandListUTF8BOM
	ApplyCommandListUTF16LE
)

// ApplyCommandListFileSpec is a resource description, not a filesystem
// operation. Entries have already had short-name fallback, F and S applied.
// Lines applies Q, ready for the eventual resource provider to encode.
type ApplyCommandListFileSpec struct {
	Entries        []string
	ShortNames     bool
	FullPaths      bool
	QuoteEntries   bool
	ForwardSlashes bool
	Encoding       ApplyCommandListEncoding
	BOM            bool
}

func (s ApplyCommandListFileSpec) Lines() []string {
	lines := append([]string(nil), s.Entries...)
	if !s.QuoteEntries {
		return lines
	}
	for i, line := range lines {
		lines[i] = `"` + strings.ReplaceAll(line, `"`, `""`) + `"`
	}
	return lines
}

type ApplyCommandResourceKind uint8

const (
	ApplyCommandListFileResource ApplyCommandResourceKind = iota
)

type ApplyCommandResourceRequest struct {
	ID          int
	Placeholder string
	Kind        ApplyCommandResourceKind
	Panel       ApplyCommandPanelSelector
	ListFile    ApplyCommandListFileSpec
}

type ApplyCommandExpansion struct {
	Command         string
	Resources       []ApplyCommandResourceRequest
	Cardinality     ApplyCommandCardinality
	HasAggregate    bool
	AggregatePanels []ApplyCommandPanelSelector

	dialect ApplyCommandDialect
}

// Render binds materialized list-file paths into an expansion. Until this is
// called, an expansion with Resources contains NUL-delimited sentinels and is
// intentionally not executable.
func (e ApplyCommandExpansion) Render(listPaths map[int]string) (string, error) {
	command := e.Command
	for _, request := range e.Resources {
		path, ok := listPaths[request.ID]
		if !ok || path == "" {
			return "", fmt.Errorf("apply command: missing path for resource %d", request.ID)
		}
		if strings.IndexByte(path, 0) >= 0 {
			return "", fmt.Errorf("apply command: resource %d path contains NUL", request.ID)
		}
		if e.dialect == ApplyCommandDialectCMD && strings.ContainsAny(path, "\"\r\n") {
			return "", fmt.Errorf("apply command: resource %d path contains a quote or line break unsupported by cmd", request.ID)
		}
		// Resource paths are generated values, never user-authored shell text.
		// Always quote them: characters that are harmless in one expression
		// context (notably comma in PowerShell) can still be operators in another.
		command = strings.ReplaceAll(command, request.Placeholder, quoteApplyCommandArg(path, e.dialect, true))
	}
	return command, nil
}

type ApplyCommandSyntaxError struct {
	Offset int
	Reason string
}

func (e *ApplyCommandSyntaxError) Error() string {
	return fmt.Sprintf("apply command syntax at byte %d: %s", e.Offset, e.Reason)
}

type applyCommandNodeKind uint8

const (
	applyNodeText applyCommandNodeKind = iota
	applyNodeName
	applyNodeNameExt
	applyNodeShortName
	applyNodeShortNameExt
	applyNodeExtension
	applyNodeShortExtension
	applyNodeDescription
	applyNodeDirectory
	applyNodeShortDirectory
	applyNodeRealDirectory
	applyNodeRealShortDirectory
	applyNodeLegacyDirectory
	applyNodeLegacyTrimmedDirectory
	applyNodeVolume
	applyNodePrompt
	applyNodeInlineList
	applyNodeListFile
)

type applyCommandListModifiers struct {
	short          bool
	full           bool
	quote          bool
	forwardSlashes bool
	encoding       ApplyCommandListEncoding
	bom            bool
	inlineQuote    byte // 0 = force, Q = force, q = quote only when needed
}

type applyCommandNode struct {
	kind        applyCommandNodeKind
	text        string
	panel       ApplyCommandPanelSelector
	promptIndex int
	modifiers   applyCommandListModifiers
}

type compiledApplyPrompt struct {
	spec         ApplyCommandPromptSpec
	titleNodes   []applyCommandNode
	initialNodes []applyCommandNode
}

// CompiledApplyCommand is safe to reuse for every target in a batch.
type CompiledApplyCommand struct {
	source   string
	nodes    []applyCommandNode
	prompts  []compiledApplyPrompt
	metadata ApplyCommandTemplateMetadata
}

func CompileApplyCommand(source string) (*CompiledApplyCommand, error) {
	if offset := strings.IndexByte(source, 0); offset >= 0 {
		return nil, &ApplyCommandSyntaxError{Offset: offset, Reason: "NUL is not allowed"}
	}
	parser := applyCommandParser{source: source, panel: ApplyCommandPanelActive, allowPrompts: true}
	nodes, err := parser.parse()
	if err != nil {
		return nil, err
	}
	return &CompiledApplyCommand{
		source:  source,
		nodes:   nodes,
		prompts: parser.prompts,
		metadata: ApplyCommandTemplateMetadata{
			HasAggregate:      len(parser.aggregatePanels) != 0,
			RequiresListFiles: parser.requiresListFiles,
			RequiresDialect:   parser.requiresDialect,
			AggregatePanels:   append([]ApplyCommandPanelSelector(nil), parser.aggregatePanels...),
		},
	}, nil
}

func (c *CompiledApplyCommand) Source() string {
	if c == nil {
		return ""
	}
	return c.source
}

func (c *CompiledApplyCommand) Metadata() ApplyCommandTemplateMetadata {
	if c == nil {
		return ApplyCommandTemplateMetadata{}
	}
	result := c.metadata
	result.AggregatePanels = append([]ApplyCommandPanelSelector(nil), result.AggregatePanels...)
	return result
}

func (c *CompiledApplyCommand) Prompts() []ApplyCommandPromptSpec {
	if c == nil {
		return nil
	}
	result := make([]ApplyCommandPromptSpec, len(c.prompts))
	for i := range c.prompts {
		result[i] = c.prompts[i].spec
	}
	return result
}

func (c *CompiledApplyCommand) ResolvePrompts(ctx ApplyCommandContext) ([]ApplyCommandResolvedPrompt, error) {
	if c == nil {
		return nil, fmt.Errorf("apply command: nil compiled template")
	}
	result := make([]ApplyCommandResolvedPrompt, 0, len(c.prompts))
	for _, prompt := range c.prompts {
		title, err := expandApplyCommandNodes(prompt.titleNodes, ctx, nil, false)
		if err != nil {
			return nil, fmt.Errorf("apply command prompt %d title: %w", prompt.spec.Index, err)
		}
		initial, err := expandApplyCommandNodes(prompt.initialNodes, ctx, nil, false)
		if err != nil {
			return nil, fmt.Errorf("apply command prompt %d initial value: %w", prompt.spec.Index, err)
		}
		result = append(result, ApplyCommandResolvedPrompt{
			Index: prompt.spec.Index, History: prompt.spec.History,
			Title: title.Command, Initial: initial.Command,
		})
	}
	return result, nil
}

func (c *CompiledApplyCommand) Expand(ctx ApplyCommandContext, values ApplyCommandPromptValues) (ApplyCommandExpansion, error) {
	if c == nil {
		return ApplyCommandExpansion{}, fmt.Errorf("apply command: nil compiled template")
	}
	return expandApplyCommandNodes(c.nodes, ctx, values, true)
}

type applyCommandParser struct {
	source            string
	panel             ApplyCommandPanelSelector
	allowPrompts      bool
	sawMetasymbol     bool
	prompts           []compiledApplyPrompt
	aggregatePanels   []ApplyCommandPanelSelector
	requiresListFiles bool
	requiresDialect   bool
}

func (p *applyCommandParser) parse() ([]applyCommandNode, error) {
	var nodes []applyCommandNode
	textStart := 0
	flushText := func(end int) {
		if end > textStart {
			nodes = append(nodes, applyCommandNode{kind: applyNodeText, text: p.source[textStart:end]})
		}
	}

	for i := 0; i < len(p.source); {
		if p.source[i] != '!' {
			i++
			continue
		}
		flushText(i)
		rest := p.source[i:]
		appendToken := func(kind applyCommandNodeKind, consumed int) {
			p.sawMetasymbol = true
			nodes = append(nodes, applyCommandNode{kind: kind, panel: p.panel})
			i += consumed
			textStart = i
		}

		switch {
		case strings.HasPrefix(rest, "!!"):
			p.sawMetasymbol = true
			nodes = append(nodes, applyCommandNode{kind: applyNodeText, text: "!"})
			i += 2
			textStart = i
		case strings.HasPrefix(rest, "!#"):
			p.sawMetasymbol = true
			p.panel = ApplyCommandPanelPassive
			i += 2
			textStart = i
		case strings.HasPrefix(rest, "!^"):
			p.sawMetasymbol = true
			p.panel = ApplyCommandPanelActive
			i += 2
			textStart = i
		case strings.HasPrefix(rest, "!["):
			p.sawMetasymbol = true
			p.panel = ApplyCommandPanelLeft
			i += 2
			textStart = i
		case strings.HasPrefix(rest, "!]"):
			p.sawMetasymbol = true
			p.panel = ApplyCommandPanelRight
			i += 2
			textStart = i
		case strings.HasPrefix(rest, "!?!"):
			appendToken(applyNodeDescription, 3)
		case strings.HasPrefix(rest, "!?"):
			if !p.allowPrompts {
				return nil, &ApplyCommandSyntaxError{Offset: i, Reason: "nested prompt is not allowed"}
			}
			prompt, consumed, err := p.parsePrompt(i)
			if err != nil {
				return nil, err
			}
			p.sawMetasymbol = true
			nodes = append(nodes, applyCommandNode{kind: applyNodePrompt, promptIndex: prompt.spec.Index})
			p.prompts = append(p.prompts, prompt)
			if applyCommandNodesRequireDialect(prompt.titleNodes) || applyCommandNodesRequireDialect(prompt.initialNodes) {
				p.requiresDialect = true
			}
			i += consumed
			textStart = i
		case strings.HasPrefix(rest, "!.!"):
			appendToken(applyNodeNameExt, 3)
		case strings.HasPrefix(rest, "!-!") || strings.HasPrefix(rest, "!+!"):
			appendToken(applyNodeShortNameExt, 3)
		case strings.HasPrefix(rest, "!`!"): // Existing F4 alias: extension including dot.
			appendToken(applyNodeExtension, 3)
			nodes[len(nodes)-1].text = "dot"
		case strings.HasPrefix(rest, "!~!"): // Existing F4 alias: long name without extension.
			appendToken(applyNodeName, 3)
		case strings.HasPrefix(rest, "!\\!"): // Existing F4 alias: directory as captured.
			appendToken(applyNodeLegacyDirectory, 3)
		case strings.HasPrefix(rest, "!/!"): // Existing F4 alias: trim one trailing separator.
			appendToken(applyNodeLegacyTrimmedDirectory, 3)
		case strings.HasPrefix(rest, "!@") || strings.HasPrefix(rest, "!$"):
			node, consumed, err := p.parseListFile(i)
			if err != nil {
				return nil, err
			}
			p.sawMetasymbol = true
			nodes = append(nodes, node)
			p.noteAggregate(p.panel)
			p.requiresListFiles = true
			p.requiresDialect = true
			i += consumed
			textStart = i
		case strings.HasPrefix(rest, "!&"):
			node, consumed := p.parseInlineList(i)
			p.sawMetasymbol = true
			nodes = append(nodes, node)
			p.noteAggregate(p.panel)
			p.requiresDialect = true
			i += consumed
			textStart = i
		case strings.HasPrefix(rest, "!=\\"):
			appendToken(applyNodeRealDirectory, 3)
		case strings.HasPrefix(rest, "!=/"):
			appendToken(applyNodeRealShortDirectory, 3)
		case strings.HasPrefix(rest, "!="):
			return nil, &ApplyCommandSyntaxError{Offset: i, Reason: "expected \\ or / after !="}
		case strings.HasPrefix(rest, "!:"):
			appendToken(applyNodeVolume, 2)
		case strings.HasPrefix(rest, "!`~"):
			appendToken(applyNodeShortExtension, 3)
		case strings.HasPrefix(rest, "!`"): // Canonical form excludes the dot.
			appendToken(applyNodeExtension, 2)
		case strings.HasPrefix(rest, "!~"):
			appendToken(applyNodeShortName, 2)
		case strings.HasPrefix(rest, "!\\"):
			appendToken(applyNodeDirectory, 2)
		case strings.HasPrefix(rest, "!/"):
			appendToken(applyNodeShortDirectory, 2)
		default:
			appendToken(applyNodeName, 1)
		}
	}
	flushText(len(p.source))
	return nodes, nil
}

func applyCommandNodesRequireDialect(nodes []applyCommandNode) bool {
	for _, node := range nodes {
		if node.kind == applyNodeListFile || node.kind == applyNodeInlineList {
			return true
		}
	}
	return false
}

func (p *applyCommandParser) parsePrompt(offset int) (compiledApplyPrompt, int, error) {
	separator, end, depth := -1, -1, 0
	for i := offset + 2; i < len(p.source); i++ {
		switch p.source[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '?':
			if depth == 0 && separator < 0 {
				separator = i
			}
		case '!':
			if depth == 0 {
				end = i
				i = len(p.source)
			}
		}
	}
	if end < 0 {
		reason := "unterminated prompt"
		if depth != 0 {
			reason = "unbalanced prompt parentheses"
		}
		return compiledApplyPrompt{}, 0, &ApplyCommandSyntaxError{Offset: offset, Reason: reason}
	}

	titleStart, titleEnd := offset+2, end
	initial := ""
	if separator >= 0 {
		titleEnd = separator
		initial = p.source[separator+1 : end]
	}
	title := p.source[titleStart:titleEnd]
	titleOffset := titleStart
	history := ""
	if strings.HasPrefix(title, "$") {
		if second := strings.IndexByte(title[1:], '$'); second >= 0 {
			second++
			history = title[1:second]
			title = title[second+1:]
			titleOffset += second + 1
		} else {
			return compiledApplyPrompt{}, 0, &ApplyCommandSyntaxError{Offset: titleStart, Reason: "unterminated prompt history name"}
		}
	}

	titleNodes, err := compileApplyPromptFragment(title, titleOffset)
	if err != nil {
		return compiledApplyPrompt{}, 0, err
	}
	initialOffset := offset + 2
	if separator >= 0 {
		initialOffset = separator + 1
	}
	initialNodes, err := compileApplyPromptFragment(initial, initialOffset)
	if err != nil {
		return compiledApplyPrompt{}, 0, err
	}
	index := len(p.prompts)
	return compiledApplyPrompt{
		spec: ApplyCommandPromptSpec{
			Index: index, History: history, TitleTemplate: title, InitialTemplate: initial,
		},
		titleNodes: titleNodes, initialNodes: initialNodes,
	}, end - offset + 1, nil
}

func compileApplyPromptFragment(source string, baseOffset int) ([]applyCommandNode, error) {
	open, close, depth := -1, -1, 0
	for i, r := range source {
		switch r {
		case '(':
			if depth == 0 && open < 0 {
				open = i
			}
			depth++
		case ')':
			if depth == 0 {
				return nil, &ApplyCommandSyntaxError{Offset: baseOffset + i, Reason: "unmatched ')' in prompt"}
			}
			depth--
			if depth == 0 && close < 0 {
				close = i
			}
		}
	}
	if depth != 0 {
		return nil, &ApplyCommandSyntaxError{Offset: baseOffset + open, Reason: "unclosed '(' in prompt"}
	}
	if open < 0 {
		return []applyCommandNode{{kind: applyNodeText, text: source}}, nil
	}

	inner := source[open+1 : close]
	parser := applyCommandParser{source: inner, panel: ApplyCommandPanelActive, allowPrompts: false}
	innerNodes, err := parser.parse()
	if syntax, ok := err.(*ApplyCommandSyntaxError); ok {
		syntax.Offset += baseOffset + open + 1
	}
	if err != nil {
		return nil, err
	}
	if parser.requiresListFiles {
		return nil, &ApplyCommandSyntaxError{
			Offset: baseOffset + open + 1,
			Reason: "list-file token is not allowed inside a prompt",
		}
	}
	if !parser.sawMetasymbol {
		return []applyCommandNode{{kind: applyNodeText, text: source}}, nil
	}

	nodes := make([]applyCommandNode, 0, len(innerNodes)+2)
	if open != 0 {
		nodes = append(nodes, applyCommandNode{kind: applyNodeText, text: source[:open]})
	}
	nodes = append(nodes, innerNodes...)
	if close+1 != len(source) {
		nodes = append(nodes, applyCommandNode{kind: applyNodeText, text: source[close+1:]})
	}
	return nodes, nil
}

func (p *applyCommandParser) parseInlineList(offset int) (applyCommandNode, int) {
	i := offset + 2
	modifiers := applyCommandListModifiers{encoding: ApplyCommandListUTF8}
	if i < len(p.source) && p.source[i] == '~' {
		modifiers.short = true
		i++
	}
	if i < len(p.source) && (p.source[i] == 'Q' || p.source[i] == 'q') {
		modifiers.inlineQuote = p.source[i]
		i++
	}
	return applyCommandNode{kind: applyNodeInlineList, panel: p.panel, modifiers: modifiers}, i - offset
}

func (p *applyCommandParser) parseListFile(offset int) (applyCommandNode, int, error) {
	endRel := strings.IndexByte(p.source[offset+2:], '!')
	if endRel < 0 {
		return applyCommandNode{}, 0, &ApplyCommandSyntaxError{Offset: offset, Reason: "unterminated list-file token"}
	}
	end := offset + 2 + endRel
	modifiers := applyCommandListModifiers{
		short: p.source[offset+1] == '$', encoding: ApplyCommandListUTF8,
	}
	for i := offset + 2; i < end; i++ {
		switch p.source[i] {
		case 'F':
			modifiers.full = true
		case 'Q':
			modifiers.quote = true
		case 'S':
			modifiers.forwardSlashes = true
		case 'A':
			modifiers.encoding = ApplyCommandListANSI
			modifiers.bom = false
		case 'U':
			modifiers.encoding = ApplyCommandListUTF8BOM
			modifiers.bom = true
		case 'W':
			modifiers.encoding = ApplyCommandListUTF16LE
			modifiers.bom = true
		default:
			return applyCommandNode{}, 0, &ApplyCommandSyntaxError{
				Offset: i, Reason: fmt.Sprintf("unknown list-file modifier %q", p.source[i]),
			}
		}
	}
	return applyCommandNode{kind: applyNodeListFile, panel: p.panel, modifiers: modifiers}, end - offset + 1, nil
}

func (p *applyCommandParser) noteAggregate(panel ApplyCommandPanelSelector) {
	for _, existing := range p.aggregatePanels {
		if existing == panel {
			return
		}
	}
	p.aggregatePanels = append(p.aggregatePanels, panel)
}

func expandApplyCommandNodes(nodes []applyCommandNode, ctx ApplyCommandContext, values ApplyCommandPromptValues, allowResources bool) (ApplyCommandExpansion, error) {
	var out strings.Builder
	var resources []ApplyCommandResourceRequest
	var aggregatePanels []ApplyCommandPanelSelector
	cardinality := ApplyCommandPerTarget

	for _, node := range nodes {
		panel := applyCommandSelectedPanel(ctx, node.panel)
		switch node.kind {
		case applyNodeText:
			out.WriteString(node.text)
		case applyNodeName:
			out.WriteString(applyCommandStem(panel.Current.Name))
		case applyNodeNameExt:
			out.WriteString(panel.Current.Name)
		case applyNodeShortName:
			out.WriteString(applyCommandStem(applyCommandShortName(panel.Current)))
		case applyNodeShortNameExt:
			out.WriteString(applyCommandShortName(panel.Current))
		case applyNodeExtension:
			ext := applyCommandExtension(panel.Current.Name)
			if node.text != "dot" {
				ext = strings.TrimPrefix(ext, ".")
			}
			out.WriteString(ext)
		case applyNodeShortExtension:
			out.WriteString(strings.TrimPrefix(applyCommandExtension(applyCommandShortName(panel.Current)), "."))
		case applyNodeDescription:
			out.WriteString(panel.Current.Description)
		case applyNodeDirectory:
			out.WriteString(applyCommandAddTrailingSeparator(applyCommandDirectory(panel, false, false), panel.PathStyle))
		case applyNodeShortDirectory:
			out.WriteString(applyCommandAddTrailingSeparator(applyCommandDirectory(panel, true, false), panel.PathStyle))
		case applyNodeRealDirectory:
			out.WriteString(applyCommandAddTrailingSeparator(applyCommandDirectory(panel, false, true), panel.PathStyle))
		case applyNodeRealShortDirectory:
			out.WriteString(applyCommandAddTrailingSeparator(applyCommandDirectory(panel, true, true), panel.PathStyle))
		case applyNodeLegacyDirectory:
			out.WriteString(panel.Directory)
		case applyNodeLegacyTrimmedDirectory:
			out.WriteString(applyCommandTrimOneSeparator(panel.Directory, panel.PathStyle))
		case applyNodeVolume:
			out.WriteString(applyCommandVolume(panel.Directory, panel.PathStyle))
		case applyNodePrompt:
			value, ok := values[node.promptIndex]
			if !ok {
				return ApplyCommandExpansion{}, fmt.Errorf("apply command: missing value for prompt %d", node.promptIndex)
			}
			out.WriteString(value)
		case applyNodeInlineList:
			entries := applyCommandPanelEntries(panel, node.modifiers.short, false, false)
			words := make([]string, len(entries))
			for i, entry := range entries {
				if ctx.Dialect == ApplyCommandDialectCMD && strings.ContainsAny(entry, "\"\r\n") {
					return ApplyCommandExpansion{}, fmt.Errorf("apply command: selected name contains a quote or line break unsupported by cmd")
				}
				// Far quotes every entry by default; Q is the explicit
				// spelling of the same rule. Lowercase q keeps shell-safe
				// names bare while still protecting names that need quoting.
				words[i] = quoteApplyCommandArg(entry, ctx.Dialect, node.modifiers.inlineQuote != 'q')
			}
			out.WriteString(strings.Join(words, " "))
			aggregatePanels = appendApplyCommandPanelOnce(aggregatePanels, node.panel)
			if applyCommandSelectorIsActive(node.panel, ctx.ActiveSide) {
				cardinality = ApplyCommandOnce
			}
		case applyNodeListFile:
			if !allowResources {
				return ApplyCommandExpansion{}, fmt.Errorf("list-file resource is not allowed here")
			}
			entries := applyCommandPanelEntries(panel, node.modifiers.short, node.modifiers.full, node.modifiers.forwardSlashes)
			id := len(resources)
			placeholder := fmt.Sprintf("\x00f4-apply-list:%d\x00", id)
			resources = append(resources, ApplyCommandResourceRequest{
				ID: id, Placeholder: placeholder, Kind: ApplyCommandListFileResource, Panel: node.panel,
				ListFile: ApplyCommandListFileSpec{
					Entries: entries, ShortNames: node.modifiers.short, FullPaths: node.modifiers.full,
					QuoteEntries: node.modifiers.quote, ForwardSlashes: node.modifiers.forwardSlashes,
					Encoding: node.modifiers.encoding, BOM: node.modifiers.bom,
				},
			})
			out.WriteString(placeholder)
			aggregatePanels = appendApplyCommandPanelOnce(aggregatePanels, node.panel)
			if applyCommandSelectorIsActive(node.panel, ctx.ActiveSide) {
				cardinality = ApplyCommandOnce
			}
		}
	}
	return ApplyCommandExpansion{
		Command: out.String(), Resources: resources, Cardinality: cardinality,
		HasAggregate: len(aggregatePanels) != 0, AggregatePanels: aggregatePanels, dialect: ctx.Dialect,
	}, nil
}

func applyCommandSelectedPanel(ctx ApplyCommandContext, selector ApplyCommandPanelSelector) ApplyCommandPanel {
	switch selector {
	case ApplyCommandPanelPassive:
		return ctx.Passive
	case ApplyCommandPanelLeft:
		if ctx.ActiveSide == ApplyCommandLeftSide {
			return ctx.Active
		}
		return ctx.Passive
	case ApplyCommandPanelRight:
		if ctx.ActiveSide == ApplyCommandRightSide {
			return ctx.Active
		}
		return ctx.Passive
	default:
		return ctx.Active
	}
}

func applyCommandSelectorIsActive(selector ApplyCommandPanelSelector, activeSide ApplyCommandPanelSide) bool {
	switch selector {
	case ApplyCommandPanelActive:
		return true
	case ApplyCommandPanelPassive:
		return false
	case ApplyCommandPanelLeft:
		return activeSide == ApplyCommandLeftSide
	case ApplyCommandPanelRight:
		return activeSide == ApplyCommandRightSide
	default:
		return false
	}
}

func appendApplyCommandPanelOnce(panels []ApplyCommandPanelSelector, panel ApplyCommandPanelSelector) []ApplyCommandPanelSelector {
	for _, existing := range panels {
		if existing == panel {
			return panels
		}
	}
	return append(panels, panel)
}

func applyCommandShortName(file ApplyCommandFile) string {
	if file.ShortName != "" {
		return file.ShortName
	}
	return file.Name
}

func applyCommandStem(name string) string {
	ext := applyCommandExtension(name)
	return strings.TrimSuffix(name, ext)
}

func applyCommandExtension(name string) string {
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		return name[dot:]
	}
	return ""
}

func applyCommandDirectory(panel ApplyCommandPanel, short, real bool) string {
	if real {
		if short && panel.RealShortDirectory != "" {
			return panel.RealShortDirectory
		}
		if panel.RealDirectory != "" {
			return panel.RealDirectory
		}
	}
	if short && panel.ShortDirectory != "" {
		return panel.ShortDirectory
	}
	return panel.Directory
}

func applyCommandAddTrailingSeparator(path string, style ApplyCommandPathStyle) string {
	if path == "" {
		return path
	}
	separator := applyCommandPathSeparator(path, style)
	if strings.HasSuffix(path, separator) ||
		(separator == `\` && strings.HasSuffix(path, "/")) {
		return path
	}
	return path + separator
}

func applyCommandTrimOneSeparator(path string, style ApplyCommandPathStyle) string {
	separator := applyCommandPathSeparator(path, style)
	if len(path) > 1 && (strings.HasSuffix(path, separator) ||
		(separator == `\` && strings.HasSuffix(path, "/"))) {
		return path[:len(path)-1]
	}
	return path
}

func applyCommandVolume(path string, style ApplyCommandPathStyle) string {
	style = effectiveApplyCommandPathStyle(path, style)
	if style == ApplyCommandPathStylePOSIX {
		if strings.HasPrefix(path, "/") {
			return "/"
		}
		return ""
	}
	if len(path) >= 2 && path[1] == ':' && isApplyCommandASCIIAlpha(path[0]) {
		return path[:2]
	}
	if strings.HasPrefix(path, `\\`) || strings.HasPrefix(path, "//") {
		separator := byte('\\')
		if strings.HasPrefix(path, "//") {
			separator = '/'
		}
		trimmed := strings.TrimLeft(path, `/\`)
		parts := strings.FieldsFunc(trimmed, func(r rune) bool { return r == '/' || r == '\\' })
		if len(parts) >= 2 {
			prefix := string([]byte{separator, separator})
			return prefix + parts[0] + string(separator) + parts[1]
		}
	}
	if strings.HasPrefix(path, `\`) {
		return `\`
	}
	if strings.HasPrefix(path, "/") {
		return "/"
	}
	return ""
}

func isApplyCommandASCIIAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func applyCommandPanelEntries(panel ApplyCommandPanel, short, full, forwardSlashes bool) []string {
	files := panel.Selected
	if len(files) == 0 && panel.Current.Name != "" {
		files = []ApplyCommandFile{panel.Current}
	}
	entries := make([]string, 0, len(files))
	for _, file := range files {
		name := file.Name
		if short {
			name = applyCommandShortName(file)
		}
		if full && !applyCommandIsAbsolute(name, panel.PathStyle) {
			directory := applyCommandDirectory(panel, short, false)
			name = applyCommandJoinPath(directory, name, panel.PathStyle)
		}
		if forwardSlashes && effectiveApplyCommandPathStyle(panel.Directory, panel.PathStyle) == ApplyCommandPathStyleWindows {
			name = strings.ReplaceAll(name, `\`, "/")
		}
		entries = append(entries, name)
	}
	return entries
}

func applyCommandJoinPath(directory, name string, style ApplyCommandPathStyle) string {
	if directory == "" {
		return name
	}
	separator := applyCommandPathSeparator(directory, style)
	if strings.HasSuffix(directory, separator) ||
		(separator == `\` && strings.HasSuffix(directory, "/")) {
		return directory + name
	}
	return directory + separator + name
}

func applyCommandPathSeparator(path string, style ApplyCommandPathStyle) string {
	switch effectiveApplyCommandPathStyle(path, style) {
	case ApplyCommandPathStylePOSIX:
		return "/"
	case ApplyCommandPathStyleWindows:
		return `\`
	}
	return "/"
}

func effectiveApplyCommandPathStyle(path string, style ApplyCommandPathStyle) ApplyCommandPathStyle {
	if style != ApplyCommandPathStyleUnknown {
		return style
	}
	if strings.HasPrefix(path, "/") {
		return ApplyCommandPathStylePOSIX
	}
	if (len(path) >= 2 && path[1] == ':' && isApplyCommandASCIIAlpha(path[0])) || strings.HasPrefix(path, `\`) ||
		(strings.Contains(path, `\`) && !strings.Contains(path, "/")) {
		return ApplyCommandPathStyleWindows
	}
	return ApplyCommandPathStylePOSIX
}

func applyCommandIsAbsolute(path string, style ApplyCommandPathStyle) bool {
	if effectiveApplyCommandPathStyle(path, style) == ApplyCommandPathStylePOSIX {
		return strings.HasPrefix(path, "/")
	}
	windowsAbsolute := strings.HasPrefix(path, `\`) || strings.HasPrefix(path, "/") ||
		(len(path) >= 3 && path[1] == ':' && isApplyCommandASCIIAlpha(path[0]) && (path[2] == '\\' || path[2] == '/'))
	return windowsAbsolute
}

func quoteApplyCommandArg(value string, dialect ApplyCommandDialect, force bool) string {
	switch dialect {
	case ApplyCommandDialectPOSIX:
		if !force && isApplyCommandSafePOSIX(value) {
			return value
		}
		return `'` + strings.ReplaceAll(value, `'`, `'\''`) + `'`
	case ApplyCommandDialectCMD:
		if !force && isApplyCommandSafeCMD(value) {
			return value
		}
		return quoteApplyCommandWindowsArg(value)
	case ApplyCommandDialectPowerShell:
		if !force && isApplyCommandSafePowerShell(value) {
			return value
		}
		return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
	default:
		if !force {
			return value
		}
		return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"`
	}
}

func isApplyCommandSafePOSIX(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_@%+=:,./-", r) {
			continue
		}
		return false
	}
	return true
}

func isApplyCommandSafeCMD(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_@+=:,./\\-", r) {
			continue
		}
		return false
	}
	return true
}

func isApplyCommandSafePowerShell(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("_%+=:./\\-", r) {
			continue
		}
		return false
	}
	return true
}

func quoteApplyCommandWindowsArg(value string) string {
	value = strings.ReplaceAll(value, "%", "%"+applyCommandLiteralPercentEnv+"%")
	var out strings.Builder
	out.WriteByte('"')
	backslashes := 0
	for _, r := range value {
		if r == '\\' {
			backslashes++
			continue
		}
		if r == '"' {
			out.WriteString(strings.Repeat(`\`, backslashes*2+1))
			out.WriteRune(r)
			backslashes = 0
			continue
		}
		out.WriteString(strings.Repeat(`\`, backslashes))
		backslashes = 0
		out.WriteRune(r)
	}
	out.WriteString(strings.Repeat(`\`, backslashes*2))
	out.WriteByte('"')
	return out.String()
}
