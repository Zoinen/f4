// Package mdmath turns the LaTeX formulas of a Markdown document into plain
// Unicode text before the document reaches the terminal Markdown viewer
// (f4#1664). A terminal cannot typeset, so the goal is the readable subset:
// Greek letters, operators, super- and subscripts, fractions, roots. A formula
// that uses anything outside that subset is not guessed at: it stays as its
// source, in a code span or block, which is what the reader would see without
// this package.
package mdmath

import (
	"strings"
	"unicode"
)

var symbols = map[string]string{
	// Greek.
	"alpha": "α", "beta": "β", "gamma": "γ", "delta": "δ", "epsilon": "ϵ", "varepsilon": "ε",
	"zeta": "ζ", "eta": "η", "theta": "θ", "vartheta": "ϑ", "iota": "ι", "kappa": "κ",
	"lambda": "λ", "mu": "μ", "nu": "ν", "xi": "ξ", "pi": "π", "varpi": "ϖ", "rho": "ρ",
	"varrho": "ϱ", "sigma": "σ", "varsigma": "ς", "tau": "τ", "upsilon": "υ", "phi": "ϕ",
	"varphi": "φ", "chi": "χ", "psi": "ψ", "omega": "ω",
	"Gamma": "Γ", "Delta": "Δ", "Theta": "Θ", "Lambda": "Λ", "Xi": "Ξ", "Pi": "Π",
	"Sigma": "Σ", "Upsilon": "Υ", "Phi": "Φ", "Psi": "Ψ", "Omega": "Ω",
	// Operators and relations.
	"pm": "±", "mp": "∓", "times": "×", "div": "÷", "cdot": "·", "ast": "∗", "star": "⋆",
	"circ": "∘", "bullet": "•", "oplus": "⊕", "otimes": "⊗",
	"le": "≤", "leq": "≤", "ge": "≥", "geq": "≥", "ne": "≠", "neq": "≠", "approx": "≈",
	"equiv": "≡", "sim": "∼", "simeq": "≃", "cong": "≅", "propto": "∝", "ll": "≪", "gg": "≫",
	"in": "∈", "notin": "∉", "ni": "∋", "subset": "⊂", "subseteq": "⊆", "supset": "⊃",
	"supseteq": "⊇", "cup": "∪", "cap": "∩", "setminus": "∖", "emptyset": "∅", "varnothing": "∅",
	"forall": "∀", "exists": "∃", "nexists": "∄", "neg": "¬", "lnot": "¬", "land": "∧",
	"wedge": "∧", "lor": "∨", "vee": "∨", "perp": "⊥", "parallel": "∥", "angle": "∠",
	// Big operators, calculus.
	"sum": "∑", "prod": "∏", "int": "∫", "iint": "∬", "oint": "∮", "partial": "∂",
	"nabla": "∇", "infty": "∞", "sqrt": "√",
	// Arrows.
	"to": "→", "rightarrow": "→", "leftarrow": "←", "gets": "←", "leftrightarrow": "↔",
	"Rightarrow": "⇒", "Leftarrow": "⇐", "Leftrightarrow": "⇔", "implies": "⟹", "iff": "⟺",
	"mapsto": "↦", "uparrow": "↑", "downarrow": "↓",
	// Miscellany.
	"ldots": "…", "dots": "…", "cdots": "⋯", "vdots": "⋮", "prime": "′", "degree": "°",
	"hbar": "ℏ", "ell": "ℓ", "Re": "ℜ", "Im": "ℑ", "aleph": "ℵ",
	"langle": "⟨", "rangle": "⟩", "lfloor": "⌊", "rfloor": "⌋", "lceil": "⌈", "rceil": "⌉",
	"lbrace": "{", "rbrace": "}", "vert": "|", "Vert": "‖", "backslash": "\\",
	// Spacing.
	"quad": " ", "qquad": "  ", "space": " ",
	// Function names.
	"sin": "sin", "cos": "cos", "tan": "tan", "cot": "cot", "sec": "sec", "csc": "csc",
	"arcsin": "arcsin", "arccos": "arccos", "arctan": "arctan", "sinh": "sinh", "cosh": "cosh",
	"tanh": "tanh", "log": "log", "ln": "ln", "lg": "lg", "exp": "exp", "lim": "lim",
	"max": "max", "min": "min", "sup": "sup", "inf": "inf", "det": "det", "dim": "dim",
	"gcd": "gcd", "deg": "deg", "arg": "arg", "ker": "ker", "mod": "mod",
}

// escaped are the characters a backslash makes literal.
const escaped = `{}%$&#_ |`

// ignored are commands that only affect typesetting.
var ignored = map[string]bool{
	"left": true, "right": true, "big": true, "Big": true, "bigg": true, "Bigg": true,
	"displaystyle": true, "textstyle": true, "limits": true, "nolimits": true,
}

// styled commands take a group and keep its text.
var styled = map[string]bool{
	"text": true, "mathrm": true, "mathbf": true, "mathit": true, "mathsf": true,
	"mathtt": true, "mathcal": true, "mathscr": true, "mathfrak": true, "operatorname": true,
	"textbf": true, "textit": true, "boldsymbol": true, "bm": true,
}

var blackboard = map[rune]string{
	'R': "ℝ", 'N': "ℕ", 'Z': "ℤ", 'Q': "ℚ", 'C': "ℂ", 'P': "ℙ", 'H': "ℍ",
}

var accents = map[string]rune{
	"hat": '̂', "bar": '̄', "overline": '̄', "vec": '⃗',
	"tilde": '̃', "dot": '̇', "ddot": '̈', "widehat": '̂', "widetilde": '̃',
}

const (
	superFrom = "0123456789+-=()niabcdefghjklmoprstuvwxyzT"
	superTo   = "⁰¹²³⁴⁵⁶⁷⁸⁹⁺⁻⁼⁽⁾ⁿⁱᵃᵇᶜᵈᵉᶠᵍʰʲᵏˡᵐᵒᵖʳˢᵗᵘᵛʷˣʸᶻᵀ"
	subFrom   = "0123456789+-=()aehijklmnoprstuvx"
	subTo     = "₀₁₂₃₄₅₆₇₈₉₊₋₌₍₎ₐₑₕᵢⱼₖₗₘₙₒₚᵣₛₜᵤᵥₓ"
)

func mapRunes(s, from, to string) (string, bool) {
	fr, tr := []rune(from), []rune(to)
	if len(fr) != len(tr) {
		return "", false
	}
	var b strings.Builder
	for _, r := range s {
		found := false
		for i, f := range fr {
			if f == r {
				b.WriteRune(tr[i])
				found = true
				break
			}
		}
		if !found {
			return "", false
		}
	}
	return b.String(), b.Len() > 0
}

type converter struct {
	s   []rune
	i   int
	bad bool
}

// ToUnicode converts a LaTeX formula, without its dollar signs, to Unicode
// text. ok is false when the formula uses something outside the supported
// subset, in which case text is empty.
func ToUnicode(latex string) (text string, ok bool) {
	c := &converter{s: []rune(latex)}
	out := c.sequence(false)
	if c.bad || c.i < len(c.s) {
		return "", false
	}
	return strings.TrimSpace(out), true
}

func (c *converter) peek() rune {
	if c.i < len(c.s) {
		return c.s[c.i]
	}
	return 0
}

func (c *converter) skipSpaces() {
	for c.i < len(c.s) && unicode.IsSpace(c.s[c.i]) {
		c.i++
	}
}

// sequence reads atoms, each with its scripts, until the end or, inside a
// group, its closing brace (which it leaves for the caller).
func (c *converter) sequence(inGroup bool) string {
	var b strings.Builder
	for c.i < len(c.s) && !c.bad {
		r := c.s[c.i]
		if r == '}' {
			if inGroup {
				return b.String()
			}
			c.bad = true
			return ""
		}
		if unicode.IsSpace(r) {
			c.i++
			continue
		}
		b.WriteString(c.withScripts(c.atom()))
	}
	return b.String()
}

func (c *converter) withScripts(base string) string {
	for !c.bad {
		c.skipSpaces()
		r := c.peek()
		if r != '^' && r != '_' {
			break
		}
		c.i++
		arg := c.scriptArg()
		if c.bad {
			return ""
		}
		base += script(r == '^', arg)
	}
	return base
}

func script(up bool, arg string) string {
	from, to, mark := subFrom, subTo, "_"
	if up {
		from, to, mark = superFrom, superTo, "^"
	}
	if arg == "'" {
		return "′"
	}
	if mapped, ok := mapRunes(arg, from, to); ok {
		return mapped
	}
	if len([]rune(arg)) > 1 {
		return mark + "(" + arg + ")"
	}
	return mark + arg
}

func (c *converter) scriptArg() string {
	c.skipSpaces()
	switch r := c.peek(); r {
	case 0:
		c.bad = true
		return ""
	case '{':
		return c.group()
	case '\\':
		return c.atom()
	default:
		c.i++
		return string(r)
	}
}

// group reads {...} and returns its converted text.
func (c *converter) group() string {
	if c.peek() != '{' {
		c.bad = true
		return ""
	}
	c.i++
	inner := c.sequence(true)
	if c.bad || c.peek() != '}' {
		c.bad = true
		return ""
	}
	c.i++
	return inner
}

// rawGroup reads {...} keeping its text as written, for \text and friends.
func (c *converter) rawGroup() string {
	c.skipSpaces()
	if c.peek() != '{' {
		c.bad = true
		return ""
	}
	depth := 0
	start := c.i + 1
	for ; c.i < len(c.s); c.i++ {
		switch c.s[c.i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				text := string(c.s[start:c.i])
				c.i++
				return text
			}
		case '\\':
			c.bad = true // a command inside \text is beyond the subset
			return ""
		}
	}
	c.bad = true
	return ""
}

func (c *converter) atom() string {
	r := c.peek()
	switch r {
	case '{':
		return c.group()
	case '\\':
		return c.command()
	case '~':
		c.i++
		return " "
	case '\'':
		c.i++
		return "′"
	case '^', '_':
		c.bad = true // a script with nothing to attach to
		return ""
	case '$', '&', '#', '%':
		// Alignment, comments and stray markup are not formula text.
		c.bad = true
		return ""
	}
	c.i++
	return string(r)
}

func (c *converter) command() string {
	c.i++ // the backslash
	if c.i >= len(c.s) {
		c.bad = true
		return ""
	}
	first := c.s[c.i]
	if !unicode.IsLetter(first) {
		c.i++
		switch {
		case strings.ContainsRune(escaped, first):
			return string(first)
		case first == ',' || first == ';' || first == ':' || first == '!':
			if first == '!' {
				return ""
			}
			return " "
		case first == '\\':
			return " "
		}
		c.bad = true
		return ""
	}
	start := c.i
	for c.i < len(c.s) && unicode.IsLetter(c.s[c.i]) {
		c.i++
	}
	name := string(c.s[start:c.i])
	switch {
	case name == "frac" || name == "dfrac" || name == "tfrac":
		return c.fraction()
	case name == "sqrt":
		return c.root()
	case styled[name]:
		return c.rawGroup()
	case name == "mathbb":
		text := c.rawGroup()
		var b strings.Builder
		for _, r := range text {
			if v, ok := blackboard[r]; ok {
				b.WriteString(v)
			} else {
				b.WriteRune(r)
			}
		}
		return b.String()
	case ignored[name]:
		// \left( and \right] keep their delimiter; \left. and \right. have none.
		c.skipSpaces()
		if name == "left" || name == "right" || strings.HasPrefix(name, "big") || strings.HasPrefix(name, "Big") {
			if c.peek() == '.' {
				c.i++
				return ""
			}
		}
		return ""
	}
	if mark, ok := accents[name]; ok {
		arg := c.accentArg()
		if c.bad {
			return ""
		}
		if r := []rune(arg); len(r) == 1 {
			return arg + string(mark)
		}
		return name + "(" + arg + ")"
	}
	if v, ok := symbols[name]; ok {
		return v
	}
	c.bad = true
	return ""
}

func (c *converter) accentArg() string {
	c.skipSpaces()
	if c.peek() == '{' {
		return c.group()
	}
	return c.atom()
}

func (c *converter) argument() string {
	c.skipSpaces()
	if c.peek() == '{' {
		return c.group()
	}
	if c.peek() == 0 {
		c.bad = true
		return ""
	}
	return c.atom()
}

func simple(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func (c *converter) fraction() string {
	num := c.argument()
	den := c.argument()
	if c.bad {
		return ""
	}
	if !simple(num) {
		num = "(" + num + ")"
	}
	if !simple(den) {
		den = "(" + den + ")"
	}
	return num + "/" + den
}

func (c *converter) root() string {
	c.skipSpaces()
	index := ""
	if c.peek() == '[' {
		end := -1
		for j := c.i + 1; j < len(c.s); j++ {
			if c.s[j] == ']' {
				end = j
				break
			}
		}
		if end < 0 {
			c.bad = true
			return ""
		}
		index = string(c.s[c.i+1 : end])
		c.i = end + 1
	}
	body := c.argument()
	if c.bad {
		return ""
	}
	if !simple(body) {
		body = "(" + body + ")"
	}
	if index != "" {
		if mapped, ok := mapRunes(index, superFrom, superTo); ok {
			index = mapped
		} else {
			index += " "
		}
	}
	return index + "√" + body
}
