package cmdline

import (
	"fmt"
	"strings"
)

// FilenameTemplate reuses Apply command parsing, scalar expansion and prompts.
// Sequence and prefix nodes are interpreted only in this compilation mode.
type FilenameTemplate struct {
	compiled *CompiledApplyCommand
}

func CompileFilenameTemplate(source string) (*FilenameTemplate, error) {
	if strings.IndexByte(source, 0) >= 0 {
		return nil, fmt.Errorf("NUL is not allowed in a filename template")
	}
	p := applyCommandParser{source: source, panel: ApplyCommandPanelActive, allowPrompts: true, filename: true}
	nodes, err := p.parse()
	if err != nil {
		return nil, err
	}
	sequence := 0
	check := func(nodes []applyCommandNode, prompt bool) error {
		for _, node := range nodes {
			switch node.kind {
			case applyNodeText:
				if !prompt && strings.ContainsAny(node.text, `/\:<>"|?*`+"\r\n") {
					return fmt.Errorf("paths and invalid filename characters are not allowed in filename templates")
				}
			case applyNodeInlineList, applyNodeListFile:
				return fmt.Errorf("lists are not allowed in filename templates")
			case applyNodeFilenameSequence:
				if prompt {
					return fmt.Errorf("sequence is not allowed inside prompts")
				}
				sequence++
			}
		}
		return nil
	}
	if err := check(nodes, false); err != nil {
		return nil, err
	}
	for _, prompt := range p.prompts {
		if err := check(prompt.titleNodes, true); err != nil {
			return nil, err
		}
		if err := check(prompt.initialNodes, true); err != nil {
			return nil, err
		}
	}
	if sequence != 1 {
		return nil, fmt.Errorf("filename template must contain exactly one !{seq}! token")
	}
	return &FilenameTemplate{compiled: &CompiledApplyCommand{source: source, nodes: nodes, prompts: p.prompts}}, nil
}

func (t *FilenameTemplate) ResolvePrompts(ctx ApplyCommandContext) ([]ApplyCommandResolvedPrompt, error) {
	return t.ResolvePromptsWithPrefix(ctx, "")
}

func (t *FilenameTemplate) ResolvePromptsWithPrefix(ctx ApplyCommandContext, prefix string) ([]ApplyCommandResolvedPrompt, error) {
	c := *t.compiled
	c.prompts = append([]compiledApplyPrompt(nil), c.prompts...)
	literalize := func(nodes []applyCommandNode) []applyCommandNode {
		result := append([]applyCommandNode(nil), nodes...)
		for i := range result {
			if result[i].kind == applyNodeFilenamePrefix {
				result[i] = applyCommandNode{kind: applyNodeText, text: prefix}
			}
		}
		return result
	}
	for i := range c.prompts {
		c.prompts[i].titleNodes = literalize(c.prompts[i].titleNodes)
		c.prompts[i].initialNodes = literalize(c.prompts[i].initialNodes)
	}
	return c.ResolvePrompts(ctx)
}

// ExpandParts preserves literal values: substituted text is never parsed again.
func (t *FilenameTemplate) ExpandParts(ctx ApplyCommandContext, values ApplyCommandPromptValues, prefix string) (before, after string, err error) {
	nodes := t.compiled.nodes
	sequence := -1
	copyNodes := append([]applyCommandNode(nil), nodes...)
	for i := range copyNodes {
		switch copyNodes[i].kind {
		case applyNodeFilenamePrefix:
			copyNodes[i] = applyCommandNode{kind: applyNodeText, text: prefix}
		case applyNodeFilenameSequence:
			sequence = i
		}
	}
	left, err := expandApplyCommandNodes(copyNodes[:sequence], ctx, values, false)
	if err != nil {
		return "", "", err
	}
	right, err := expandApplyCommandNodes(copyNodes[sequence+1:], ctx, values, false)
	if err != nil {
		return "", "", err
	}
	return left.Command, right.Command, nil
}
