package mermaid

import (
	"strings"
	"testing"
)

func TestFlowchart(t *testing.T) {
	got, ok := Flowchart("graph TD\n  A[Start] --> B{Ok?}\n  B -->|yes| C(Do it)\n  B -- no --> D((Stop))\n  E")
	want := strings.Join([]string{
		"[Start] ──▶ {Ok?}",
		"{Ok?} ── yes ─▶ (Do it)",
		"{Ok?} ── no ─▶ ((Stop))",
		"[E]",
	}, "\n")
	if !ok || got != want {
		t.Errorf("basic:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	got, ok = Flowchart("flowchart LR\nA-->B-->C")
	if !ok || got != "[A] ──▶ [B]\n[B] ──▶ [C]" {
		t.Errorf("chain: %q %v", got, ok)
	}
	got, ok = Flowchart("flowchart TD\nA[\"Quoted [text]\"] -.-> B([Round])\nB ==> C[[Sub]]\nC --- D\nD <--> A\nA -. why .-> C")
	want = strings.Join([]string{
		"[Quoted [text]] ┄┄▶ ([Round])",
		"([Round]) ━━▶ [[Sub]]",
		"[[Sub]] ─── [D]",
		"[D] ◀──▶ [Quoted [text]]",
		"[Quoted [text]] ┄┄ why ┄▶ [[Sub]]",
	}, "\n")
	if !ok || got != want {
		t.Errorf("styles:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	got, ok = Flowchart("graph TD\nA[One<br/>two] --> B")
	if !ok || got != "[One two] ──▶ [B]" {
		t.Errorf("br: %q %v", got, ok)
	}
}

func TestFlowchartRefusals(t *testing.T) {
	for name, src := range map[string]string{
		"sequence":          "sequenceDiagram\nA->>B: hi",
		"empty":             "",
		"header":            "graph TD",
		"unclosed subgraph": "graph TD\nsubgraph X\nA-->B",
		"stray end":         "graph TD\nA-->B\nend",
		"edge id":           "graph TD\nA@{ shape: rect } --> B",
		"unclosed":          "graph TD\nA[oops --> B",
		"garbage":           "graph TD\nA --> ",
		"nodeless":          "graph TD\n--> B",
		"too long":          "graph TD\n" + strings.Repeat("A-->B\n", MaxLines),
	} {
		if got, ok := Flowchart(src); ok {
			t.Errorf("%s: converted to %q", name, got)
		}
	}
}

func TestSequence(t *testing.T) {
	src := strings.Join([]string{
		"%% a comment",
		"sequenceDiagram",
		"  autonumber",
		"  participant A as Alice",
		"  actor B",
		"  A->>B: Hello",
		"  activate B",
		"  B-->>A: Hi back",
		"  A-)B: async",
		"  B--xA: lost;",
		"  A->+B: plain",
		"  Note over A,B: shared note",
		"  Note right of B: side",
		"  deactivate B",
	}, "\n")
	got, ok := Convert(src)
	want := strings.Join([]string{
		"1. Alice ──▶ B: Hello",
		"2. B ┄┄▶ Alice: Hi back",
		"3. Alice ──▷ B: async",
		"4. B ┄┄✕ Alice: lost",
		"5. Alice ─── B: plain",
		"✎ Alice, B: shared note",
		"✎ B: side",
	}, "\n")
	if !ok || got != want {
		t.Errorf("sequence:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	for name, bad := range map[string]string{
		"loop":     "sequenceDiagram\nloop every minute\nA->>B: ping\nend",
		"header":   "sequenceDiagram",
		"garbage":  "sequenceDiagram\nA ~~ B",
		"gantt":    "gantt\ntitle X",
		"nothing":  "%% only a comment",
		"too long": "sequenceDiagram\n" + strings.Repeat("A->>B: x\n", MaxLines),
	} {
		if got, ok := Convert(bad); ok {
			t.Errorf("%s: converted to %q", name, got)
		}
	}
	if got, ok := Convert("graph TD\nA-->B"); !ok || got != "[A] ──▶ [B]" {
		t.Errorf("Convert must hand flowcharts to Flowchart: %q %v", got, ok)
	}
}

func TestClass(t *testing.T) {
	src := strings.Join([]string{
		"classDiagram",
		"  direction LR",
		"  class Animal {",
		"    <<abstract>>",
		"    +String name",
		"    +eat() void",
		"  }",
		"  class Duck",
		"  Animal <|-- Duck",
		"  Duck : +swim()",
		"  Zoo \"1\" *-- \"many\" Animal : houses",
		"  Duck ..> Pond",
		"  Keeper --> Zoo",
		"  Pond <.. Fish",
		"  Fish -- Pond",
	}, "\n")
	got, ok := Convert(src)
	want := strings.Join([]string{
		"class Animal",
		"  <<abstract>>",
		"  +String name",
		"  +eat() void",
		"class Duck",
		"  +swim()",
		"Duck ──▷ Animal",
		"Zoo \"1\" ◆── \"many\" Animal: houses",
		"Duck ┄┄▶ Pond",
		"Keeper ──▶ Zoo",
		"Pond ◀┄┄ Fish",
		"Fish ─── Pond",
	}, "\n")
	if !ok || got != want {
		t.Errorf("class:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	for name, bad := range map[string]string{
		"generic":   "classDiagram\nclass Box~T~",
		"note":      "classDiagram\nnote for A \"x\"",
		"unclosed":  "classDiagram\nclass A {\n+x",
		"namespace": "classDiagram\nnamespace N {\nclass A\n}",
		"header":    "classDiagram",
		"too long":  "classDiagram\n" + strings.Repeat("A --> B\n", MaxLines),
	} {
		if got, ok := Class(bad); ok {
			t.Errorf("%s: converted to %q", name, got)
		}
	}
}

func TestFlowchartSubgraphs(t *testing.T) {
	src := strings.Join([]string{
		"graph TD",
		"  A --> B",
		"  subgraph one [First group]",
		"    B --> C",
		"    D",
		"    subgraph inner",
		"      C --> E",
		"    end",
		"  end",
		"  F",
	}, "\n")
	got, ok := Flowchart(src)
	want := strings.Join([]string{
		"[A] ──▶ [B]",
		"┌ First group",
		"│ [B] ──▶ [C]",
		"│ [D]",
		"│ ┌ inner",
		"│ │ [C] ──▶ [E]",
		"│ └",
		"└",
		"[F]",
	}, "\n")
	if !ok || got != want {
		t.Errorf("subgraphs:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	deep := "graph TD\n" + strings.Repeat("subgraph s\n", maxSubgraphDepth+1) + "A-->B\n" + strings.Repeat("end\n", maxSubgraphDepth+1)
	if got, ok := Flowchart(deep); ok {
		t.Errorf("nesting past the limit converted to %q", got)
	}
}

func TestPie(t *testing.T) {
	got, ok := Convert("pie showData title Pets\n  \"Dogs\" : 60\n  \"Cats\" : 30\n  \"Fish\" : 10\n")
	row := func(label string, blocks int, pct, raw string) string {
		return label + "  " + strings.Repeat("█", blocks) + strings.Repeat(" ", pieBar-blocks) + "  " + pct + "%  (" + raw + ")"
	}
	want := strings.Join([]string{"Pets", row("Dogs", 12, "60.0", "60"), row("Cats", 6, "30.0", "30"), row("Fish", 2, "10.0", "10")}, "\n")
	if !ok || got != want {
		t.Errorf("pie:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	if got, ok := Pie("pie\ntitle Later\n\"A\" : 1.5"); !ok || !strings.HasPrefix(got, "Later\n") {
		t.Errorf("a title line: %q %v", got, ok)
	}
	for name, bad := range map[string]string{
		"empty":    "pie",
		"zero":     "pie\n\"A\" : 0",
		"garbage":  "pie\nA = 1",
		"negative": "pie\n\"A\" : -1",
		"header":   "piechart\n\"A\" : 1",
		"too long": "pie\n" + strings.Repeat("\"A\" : 1\n", MaxLines),
	} {
		if got, ok := Pie(bad); ok {
			t.Errorf("%s: converted to %q", name, got)
		}
	}
}

func TestGantt(t *testing.T) {
	src := strings.Join([]string{
		"gantt",
		"  title Plan",
		"  dateFormat YYYY-MM-DD",
		"  section Build",
		"  Design :done, d1, 2024-01-01, 10d",
		"  Code :active, after d1, 20d",
		"  section Ship",
		"  Release :milestone, 2024-03-01, 0d",
	}, "\n")
	got, ok := Convert(src)
	want := strings.Join([]string{
		"Plan",
		"▸ Build",
		"  Design — done, d1, 2024-01-01, 10d",
		"  Code — active, after d1, 20d",
		"▸ Ship",
		"  Release — milestone, 2024-03-01, 0d",
	}, "\n")
	if !ok || got != want {
		t.Errorf("gantt:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	for name, bad := range map[string]string{"no tasks": "gantt\ntitle x", "header": "gantt chart", "garbage": "gantt\nnot a task"} {
		if got, ok := Gantt(bad); ok {
			t.Errorf("%s: converted to %q", name, got)
		}
	}
}

func TestState(t *testing.T) {
	src := strings.Join([]string{
		"stateDiagram-v2",
		"  direction LR",
		"  state \"Waiting for input\" as Wait",
		"  [*] --> Wait",
		"  Wait --> Run : go",
		"  Run --> [*]",
		"  Run : does the work",
	}, "\n")
	got, ok := Convert(src)
	want := strings.Join([]string{
		"(●) ──▶ (Waiting for input)",
		"(Waiting for input) ──▶ (Run): go",
		"(Run) ──▶ (◎)",
		"Run: does the work",
	}, "\n")
	if !ok || got != want {
		t.Errorf("state:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	for name, bad := range map[string]string{
		"composite": "stateDiagram\nstate Big {\nA --> B\n}",
		"note":      "stateDiagram\nnote right of A : x",
		"header":    "stateDiagram\n",
		"too long":  "stateDiagram\n" + strings.Repeat("A --> B\n", MaxLines),
	} {
		if got, ok := State(bad); ok {
			t.Errorf("%s: converted to %q", name, got)
		}
	}
}

func TestER(t *testing.T) {
	src := strings.Join([]string{
		"erDiagram",
		"  CUSTOMER ||--o{ ORDER : places",
		"  ORDER |o..|{ LINE-ITEM : \"contains items\"",
		"  CUSTOMER {",
		"    string name",
		"    int id PK",
		"  }",
	}, "\n")
	got, ok := Convert(src)
	want := strings.Join([]string{
		"entity CUSTOMER",
		"  string name",
		"  int id PK",
		"CUSTOMER 1 ─── 0..* ORDER: places",
		"ORDER 0..1 ┄┄┄ 1..* LINE-ITEM: contains items",
	}, "\n")
	if !ok || got != want {
		t.Errorf("er:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	for name, bad := range map[string]string{
		"unclosed": "erDiagram\nA {\nint x",
		"garbage":  "erDiagram\nA -- B",
		"header":   "erDiagram",
		"too long": "erDiagram\n" + strings.Repeat("A ||--|| B : x\n", MaxLines),
	} {
		if got, ok := ER(bad); ok {
			t.Errorf("%s: converted to %q", name, got)
		}
	}
}

func TestFlowchartGroupsAndClassShorthand(t *testing.T) {
	src := strings.Join([]string{
		"graph TD",
		"  A:::hot --> B[Two]:::cold",
		"  B & C --> D & E",
		"  F & G",
	}, "\n")
	got, ok := Flowchart(src)
	want := strings.Join([]string{
		"[A] ──▶ [Two]",
		"[Two] ──▶ [D]",
		"[Two] ──▶ [E]",
		"[C] ──▶ [D]",
		"[C] ──▶ [E]",
		"[F]",
		"[G]",
	}, "\n")
	if !ok || got != want {
		t.Errorf("groups:\n%s\nwant:\n%s (ok=%v)", got, want, ok)
	}
	if got, ok := Flowchart("graph TD\nA & --> B"); ok {
		t.Errorf("a dangling & converted to %q", got)
	}
}
