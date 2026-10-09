package mdmath

import (
	"strings"
	"testing"
)

func TestToUnicode(t *testing.T) {
	cases := map[string]string{
		`\alpha + \beta`:                "α+β",
		`x^2 + y_1`:                     "x²+y₁",
		`e^{i\pi} = -1`:                 "e^(iπ)=-1",
		`a \le b \ne c`:                 "a≤b≠c",
		`\frac{1}{2}`:                   "1/2",
		`\frac{a+b}{c}`:                 "(a+b)/c",
		`\sqrt{x}`:                      "√x",
		`\sqrt{x+1}`:                    "√(x+1)",
		`\sqrt[3]{x}`:                   "³√x",
		`\sum_{i=0}^{n} i`:              "∑ᵢ₌₀ⁿi",
		`\mathbb{R}^n`:                  "ℝⁿ",
		`\text{area} = \pi r^2`:         "area=πr²",
		`\hat{x}`:                       "x\u0302",
		`x^{a+b}`:                       "xᵃ⁺ᵇ",
		`x^{\alpha}`:                    "x^α",
		`f'(x)`:                         "f′(x)",
		`\left( \frac{a}{b} \right)`:    "(a/b)",
		`\int_0^1 f(x)\,dx`:             "∫₀¹f(x) dx",
		`\{ x \}`:                       "{x}",
		`\lim_{x \to \infty} f`:         "lim_(x→∞)f",
		`\vec{v}`:                       "v\u20d7",
		`\hat{xy}`:                      "hat(xy)",
		`a ~ b`:                         "a b",
		`x_{ab}`:                        "x_(ab)",
		`\mathbb{Z}`:                    "ℤ",
		`a\!b`:                          "ab",
		`\left. x \right|`:              "x|",
		`\operatorname{tr} A`:           "trA",
		`\Gamma(n) = (n-1)!`:            "Γ(n)=(n-1)!",
		`\forall x \in A`:               "∀x∈A",
		`a \Rightarrow b`:               "a⇒b",
		`\dot{x}`:                       "x\u0307",
		`\frac{\partial f}{\partial x}`: "(∂f)/(∂x)",
		`\begin{matrix} a \end{matrix}`: "",
		`\unknown`:                      "",
		`x^`:                            "",
		`}`:                             "",
		`\text{a \alpha}`:               "",
		`{a`:                            "",
		`\frac{1}`:                      "",
		`\sqrt[3`:                       "",
		`x & y`:                         "",
		`\`:                             "",
		`\sqrt`:                         "",
	}
	for src, want := range cases {
		got, ok := ToUnicode(src)
		if want == "" {
			if ok {
				t.Errorf("ToUnicode(%q) = %q, want a refusal", src, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("ToUnicode(%q) = %q (%v), want %q", src, got, ok, want)
		}
	}
}

func TestPrepareInline(t *testing.T) {
	cases := map[string]string{
		"no math here":                    "no math here",
		"area is $\\pi r^2$ exactly":      "area is πr² exactly",
		"it costs $5 and $6":              "it costs $5 and $6",
		"a lone $ sign":                   "a lone $ sign",
		"kept `$x^2$` in code":            "kept `$x^2$` in code",
		"kept ``a ` $x$`` too":            "kept ``a ` $x$`` too",
		"escaped \\$x\\$ stays":           "escaped \\$x\\$ stays",
		"odd $\\unknown$ formula":         "odd `$\\unknown$` formula",
		"snake $a_{bc}$ case":             "snake `a_(bc)` case",
		"two $x^2$ and $y^3$":             "two x² and y³",
		"open $x^2 never closed":          "open $x^2 never closed",
		"trailing space $x $ is not math": "trailing space $x $ is not math",
		"unbalanced `tick $x$":            "unbalanced `tick x",
	}
	for in, want := range cases {
		if got := Prepare(in); got != want {
			t.Errorf("Prepare(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrepareBlocksAndFences(t *testing.T) {
	in := strings.Join([]string{
		"# Title",
		"",
		"$$",
		`\alpha^2 + \beta^2 = \gamma^2`,
		"$$",
		"",
		"$$ E = mc^2 $$",
		"",
		"$$",
		`\unknown`,
		"$$",
		"",
		"```",
		"$x^2$",
		"$$",
		"```",
		"",
		"~~~go",
		"$y$",
		"~~~",
		"after $z$",
		"$$",
		"never closed",
	}, "\n")
	got := Prepare(in)
	for _, want := range []string{"\nα²+β²=γ²\n", "\nE=mc²\n", "```\n\\unknown\n```", "```\n$x^2$\n$$\n```", "~~~go\n$y$\n~~~", "after z", "$$\nnever closed"} {
		if !strings.Contains(got, want) {
			t.Errorf("result lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "$$\n\\alpha") {
		t.Error("a converted block kept its source")
	}
}

func TestPrepareMermaid(t *testing.T) {
	in := strings.Join([]string{
		"before",
		"```mermaid",
		"graph TD",
		"A[Start] --> B",
		"```",
		"",
		"```mermaid",
		"gantt",
		"title X",
		"```",
		"~~~ Mermaid",
		"flowchart LR",
		"X --> Y",
		"~~~",
		"```mermaid",
		"graph TD",
		"never closed --> B",
	}, "\n")
	got := Prepare(in)
	for _, want := range []string{"```\n[Start] ──▶ [B]\n```", "```mermaid\ngantt\ntitle X\n```", "```\n[X] ──▶ [Y]\n```", "```mermaid\ngraph TD\nnever closed --> B"} {
		if !strings.Contains(got, want) {
			t.Errorf("result lacks %q:\n%s", want, got)
		}
	}
	if isMermaid("```go", "```") || !isMermaid("```mermaid", "```") || isMermaid("```", "```") {
		t.Error("isMermaid misreads the info string")
	}
}

func TestPrepareMappedOrigins(t *testing.T) {
	md := "intro\n\n$$\nx^2\n$$\n\nlast $a$ line"
	out, origin := PrepareMapped(md)
	if strings.Count(out, "\n")+1 != len(origin) {
		t.Fatalf("%d lines but %d origins", strings.Count(out, "\n")+1, len(origin))
	}
	for i := 1; i < len(origin); i++ {
		if origin[i] < origin[i-1] {
			t.Fatalf("origins go back: %v", origin)
		}
	}
	if origin[0] != 0 || origin[len(origin)-1] != 6 {
		t.Errorf("origins = %v, want them to run from line 0 to line 6", origin)
	}
	plain, po := PrepareMapped("a\nb")
	if plain != "a\nb" || len(po) != 2 || po[1] != 1 {
		t.Errorf("no formulas: %q %v", plain, po)
	}
	if got := Prepare(md); got != out {
		t.Errorf("Prepare and PrepareMapped disagree: %q vs %q", got, out)
	}
}
