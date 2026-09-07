package vtui

import (
	"fmt"

	"github.com/unxed/kiwi-go"
)

// ElementVars holds Cassowary layout variables for a single UIElement.
type ElementVars struct {
	Element UIElement
	Left    *kiwi.Variable
	Top     *kiwi.Variable
	Width   *kiwi.Variable
	Height  *kiwi.Variable
	Right   *kiwi.Variable
	Bottom  *kiwi.Variable
}

// AutoLayout is a Cassowary constraint layout engine powered by Discrete Cassowary (kiwi-go).
// It allows declaring complex, proportional, aligned, and grid-hinted TUI layouts
// without manual coordinate math.
type AutoLayout struct {
	ScreenObject
	ds     *kiwi.DiscreteSolver
	solver *kiwi.Solver
	vars   map[UIElement]*ElementVars
	items  []UIElement

	BoundsLeft   *kiwi.Variable
	BoundsTop    *kiwi.Variable
	BoundsWidth  *kiwi.Variable
	BoundsHeight *kiwi.Variable
	BoundsRight  *kiwi.Variable
	BoundsBottom *kiwi.Variable
}

// NewAutoLayout creates a new AutoLayout container anchored at (x, y) with size (w, h).
func NewAutoLayout(x, y, w, h int) *AutoLayout {
	ds := kiwi.NewDiscreteSolver()
	solver := ds.Solver()

	al := &AutoLayout{
		ds:           ds,
		solver:       solver,
		vars:         make(map[UIElement]*ElementVars),
		BoundsLeft:   kiwi.NewVariable("Bounds.Left"),
		BoundsTop:    kiwi.NewVariable("Bounds.Top"),
		BoundsWidth:  kiwi.NewVariable("Bounds.Width"),
		BoundsHeight: kiwi.NewVariable("Bounds.Height"),
		BoundsRight:  kiwi.NewVariable("Bounds.Right"),
		BoundsBottom: kiwi.NewVariable("Bounds.Bottom"),
	}

	_ = solver.AddEditVariable(al.BoundsLeft, kiwi.StrengthStrong)
	_ = solver.AddEditVariable(al.BoundsTop, kiwi.StrengthStrong)
	_ = solver.AddEditVariable(al.BoundsWidth, kiwi.StrengthStrong)
	_ = solver.AddEditVariable(al.BoundsHeight, kiwi.StrengthStrong)

	_ = solver.AddConstraint(kiwi.NewConstraint(al.BoundsRight, kiwi.OpEq, al.BoundsLeft.Plus(al.BoundsWidth).Minus(1)))
	_ = solver.AddConstraint(kiwi.NewConstraint(al.BoundsBottom, kiwi.OpEq, al.BoundsTop.Plus(al.BoundsHeight).Minus(1)))

	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}

	ds.TrackVariable(al.BoundsLeft, al.BoundsTop, al.BoundsWidth, al.BoundsHeight, al.BoundsRight, al.BoundsBottom)

	_ = solver.SuggestValue(al.BoundsLeft, float64(x))
	_ = solver.SuggestValue(al.BoundsTop, float64(y))
	_ = solver.SuggestValue(al.BoundsWidth, float64(w))
	_ = solver.SuggestValue(al.BoundsHeight, float64(h))

	al.SetPosition(x, y, x+w-1, y+h-1)
	return al
}

// Solver returns the underlying Cassowary Solver.
func (al *AutoLayout) Solver() *kiwi.Solver {
	return al.solver
}

// DiscreteSolver returns the underlying DiscreteSolver.
func (al *AutoLayout) DiscreteSolver() *kiwi.DiscreteSolver {
	return al.ds
}

// Var returns or creates the ElementVars structure for the given UIElement.
func (al *AutoLayout) Var(el UIElement) *ElementVars {
	if ev, ok := al.vars[el]; ok {
		return ev
	}
	id := elementID(el)
	ev := &ElementVars{
		Element: el,
		Left:    kiwi.NewVariable(id + ".Left"),
		Top:     kiwi.NewVariable(id + ".Top"),
		Width:   kiwi.NewVariable(id + ".Width"),
		Height:  kiwi.NewVariable(id + ".Height"),
		Right:   kiwi.NewVariable(id + ".Right"),
		Bottom:  kiwi.NewVariable(id + ".Bottom"),
	}

	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Right, kiwi.OpEq, ev.Left.Plus(ev.Width).Minus(1)))
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Bottom, kiwi.OpEq, ev.Top.Plus(ev.Height).Minus(1)))

	x1, y1, x2, y2 := el.GetPosition()
	initW := x2 - x1 + 1
	if initW < 1 {
		initW = 1
	}
	initH := y2 - y1 + 1
	if initH < 1 {
		initH = 1
	}

	al.ds.TrackVariable(ev.Left, ev.Top, ev.Width, ev.Height, ev.Right, ev.Bottom)
	al.ds.SetMinSize(ev.Width, initW)
	al.ds.SetMinSize(ev.Height, initH)

	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Width, kiwi.OpGe, float64(initW), kiwi.StrengthWeak))
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Height, kiwi.OpGe, float64(initH), kiwi.StrengthWeak))

	al.vars[el] = ev
	al.items = append(al.items, el)
	return ev
}

// AddConstraint adds a Cassowary constraint directly to the layout solver.
func (al *AutoLayout) AddConstraint(cn *kiwi.Constraint) *AutoLayout {
	if cn != nil {
		_ = al.solver.AddConstraint(cn)
	}
	return al
}

// Constraint creates and adds a constraint using Cassowary syntax.
func (al *AutoLayout) Constraint(lhs any, op kiwi.Operator, rhsAndStrength ...any) *AutoLayout {
	_ = al.solver.AddConstraint(kiwi.NewConstraint(lhs, op, rhsAndStrength...))
	return al
}

// PinLeft constrains el.Left == BoundsLeft + margin.
func (al *AutoLayout) PinLeft(el UIElement, margin int) *AutoLayout {
	ev := al.Var(el)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Left, kiwi.OpEq, al.BoundsLeft.Plus(float64(margin))))
	return al
}

// PinRight constrains el.Right == BoundsRight - margin.
func (al *AutoLayout) PinRight(el UIElement, margin int) *AutoLayout {
	ev := al.Var(el)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Right, kiwi.OpEq, al.BoundsRight.Minus(float64(margin))))
	return al
}

// PinTop constrains el.Top == BoundsTop + margin.
func (al *AutoLayout) PinTop(el UIElement, margin int) *AutoLayout {
	ev := al.Var(el)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Top, kiwi.OpEq, al.BoundsTop.Plus(float64(margin))))
	return al
}

// PinBottom constrains el.Bottom == BoundsBottom - margin.
func (al *AutoLayout) PinBottom(el UIElement, margin int) *AutoLayout {
	ev := al.Var(el)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Bottom, kiwi.OpEq, al.BoundsBottom.Minus(float64(margin))))
	return al
}

// PinEdges pins all four edges of el to the layout bounds with margins.
func (al *AutoLayout) PinEdges(el UIElement, m Margins) *AutoLayout {
	al.PinLeft(el, m.Left)
	al.PinTop(el, m.Top)
	al.PinRight(el, m.Right)
	al.PinBottom(el, m.Bottom)
	return al
}

// FillWidth pins el.Left and el.Right to the layout bounds with margins.
func (al *AutoLayout) FillWidth(el UIElement, marginLeft, marginRight int) *AutoLayout {
	al.PinLeft(el, marginLeft)
	al.PinRight(el, marginRight)
	return al
}

// FillHeight pins el.Top and el.Bottom to the layout bounds with margins.
func (al *AutoLayout) FillHeight(el UIElement, marginTop, marginBottom int) *AutoLayout {
	al.PinTop(el, marginTop)
	al.PinBottom(el, marginBottom)
	return al
}

// StackVertical positions elements top-to-bottom: el[i+1].Top == el[i].Bottom + 1 + spacing.
func (al *AutoLayout) StackVertical(spacing int, elements ...UIElement) *AutoLayout {
	for i := 0; i < len(elements)-1; i++ {
		curr := al.Var(elements[i])
		next := al.Var(elements[i+1])
		_ = al.solver.AddConstraint(kiwi.NewConstraint(next.Top, kiwi.OpEq, curr.Bottom.Plus(float64(1+spacing))))
	}
	return al
}

// StackHorizontal positions elements left-to-right: el[i+1].Left == el[i].Right + 1 + spacing.
func (al *AutoLayout) StackHorizontal(spacing int, elements ...UIElement) *AutoLayout {
	for i := 0; i < len(elements)-1; i++ {
		curr := al.Var(elements[i])
		next := al.Var(elements[i+1])
		_ = al.solver.AddConstraint(kiwi.NewConstraint(next.Left, kiwi.OpEq, curr.Right.Plus(float64(1+spacing))))
	}
	return al
}

// AlignLeft aligns the left edges of all specified elements.
func (al *AutoLayout) AlignLeft(elements ...UIElement) *AutoLayout {
	if len(elements) < 2 {
		return al
	}
	first := al.Var(elements[0])
	for i := 1; i < len(elements); i++ {
		ev := al.Var(elements[i])
		_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Left, kiwi.OpEq, first.Left))
	}
	return al
}

// AlignRight aligns the right edges of all specified elements.
func (al *AutoLayout) AlignRight(elements ...UIElement) *AutoLayout {
	if len(elements) < 2 {
		return al
	}
	first := al.Var(elements[0])
	for i := 1; i < len(elements); i++ {
		ev := al.Var(elements[i])
		_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Right, kiwi.OpEq, first.Right))
	}
	return al
}

// AlignTop aligns the top edges of all specified elements.
func (al *AutoLayout) AlignTop(elements ...UIElement) *AutoLayout {
	if len(elements) < 2 {
		return al
	}
	first := al.Var(elements[0])
	for i := 1; i < len(elements); i++ {
		ev := al.Var(elements[i])
		_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Top, kiwi.OpEq, first.Top))
	}
	return al
}

// AlignBottom aligns the bottom edges of all specified elements.
func (al *AutoLayout) AlignBottom(elements ...UIElement) *AutoLayout {
	if len(elements) < 2 {
		return al
	}
	first := al.Var(elements[0])
	for i := 1; i < len(elements); i++ {
		ev := al.Var(elements[i])
		_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Bottom, kiwi.OpEq, first.Bottom))
	}
	return al
}

// SameWidth forces all specified elements to have equal width.
func (al *AutoLayout) SameWidth(elements ...UIElement) *AutoLayout {
	if len(elements) < 2 {
		return al
	}
	first := al.Var(elements[0])
	for i := 1; i < len(elements); i++ {
		ev := al.Var(elements[i])
		_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Width, kiwi.OpEq, first.Width))
	}
	return al
}

// SameHeight forces all specified elements to have equal height.
func (al *AutoLayout) SameHeight(elements ...UIElement) *AutoLayout {
	if len(elements) < 2 {
		return al
	}
	first := al.Var(elements[0])
	for i := 1; i < len(elements); i++ {
		ev := al.Var(elements[i])
		_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Height, kiwi.OpEq, first.Height))
	}
	return al
}

// CenterHorizontal centers el horizontally within the layout bounds.
func (al *AutoLayout) CenterHorizontal(el UIElement) *AutoLayout {
	ev := al.Var(el)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Left.Plus(ev.Right), kiwi.OpEq, al.BoundsLeft.Plus(al.BoundsRight)))
	return al
}

// CenterVertical centers el vertically within the layout bounds.
func (al *AutoLayout) CenterVertical(el UIElement) *AutoLayout {
	ev := al.Var(el)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Top.Plus(ev.Bottom), kiwi.OpEq, al.BoundsTop.Plus(al.BoundsBottom)))
	return al
}

// CenterHorizontalGroup centers a block of elements (from first's Left to last's Right) horizontally.
func (al *AutoLayout) CenterHorizontalGroup(first, last UIElement) *AutoLayout {
	evFirst := al.Var(first)
	evLast := al.Var(last)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(evFirst.Left.Plus(evLast.Right), kiwi.OpEq, al.BoundsLeft.Plus(al.BoundsRight)))
	return al
}

// SetMinWidth sets the minimum character width for el.
func (al *AutoLayout) SetMinWidth(el UIElement, minW int) *AutoLayout {
	ev := al.Var(el)
	al.ds.SetMinSize(ev.Width, minW)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Width, kiwi.OpGe, float64(minW), kiwi.StrengthStrong))
	return al
}

// SetMinHeight sets the minimum character height for el.
func (al *AutoLayout) SetMinHeight(el UIElement, minH int) *AutoLayout {
	ev := al.Var(el)
	al.ds.SetMinSize(ev.Height, minH)
	_ = al.solver.AddConstraint(kiwi.NewConstraint(ev.Height, kiwi.OpGe, float64(minH), kiwi.StrengthStrong))
	return al
}

// ApportionWidths registers an ApportionGroup in DiscreteSolver to distribute rounding remainders
// across elements so that sum(Widths) == targetWidth with zero gaps or overflows (FreeType autohinting).
func (al *AutoLayout) ApportionWidths(targetWidthVarOrConst any, elements ...UIElement) *AutoLayout {
	vars := make([]*kiwi.Variable, len(elements))
	var sumExpr *kiwi.Expression
	for i, el := range elements {
		wVar := al.Var(el).Width
		vars[i] = wVar
		if sumExpr == nil {
			sumExpr = kiwi.NewExpression(wVar)
		} else {
			sumExpr = sumExpr.Plus(wVar)
		}
	}
	group := kiwi.ApportionGroup{Vars: vars}
	switch t := targetWidthVarOrConst.(type) {
	case *kiwi.Variable:
		group.TargetVar = t
		if sumExpr != nil {
			_ = al.solver.AddConstraint(kiwi.NewConstraint(sumExpr, kiwi.OpEq, t))
		}
	case int:
		group.TargetConst = t
		if sumExpr != nil {
			_ = al.solver.AddConstraint(kiwi.NewConstraint(sumExpr, kiwi.OpEq, float64(t)))
		}
	case float64:
		group.TargetConst = int(t)
		if sumExpr != nil {
			_ = al.solver.AddConstraint(kiwi.NewConstraint(sumExpr, kiwi.OpEq, t))
		}
	case UIElement:
		targetVar := al.Var(t).Width
		group.TargetVar = targetVar
		if sumExpr != nil {
			_ = al.solver.AddConstraint(kiwi.NewConstraint(sumExpr, kiwi.OpEq, targetVar))
		}
	default:
		panic(fmt.Errorf("invalid target for ApportionWidths: %v", targetWidthVarOrConst))
	}
	al.ds.AddApportionGroup(group)
	return al
}

// ApportionHeights registers an ApportionGroup in DiscreteSolver to distribute rounding remainders
// across elements so that sum(Heights) == targetHeight with zero gaps or overflows (FreeType autohinting).
func (al *AutoLayout) ApportionHeights(targetHeightVarOrConst any, elements ...UIElement) *AutoLayout {
	vars := make([]*kiwi.Variable, len(elements))
	var sumExpr *kiwi.Expression
	for i, el := range elements {
		hVar := al.Var(el).Height
		vars[i] = hVar
		if sumExpr == nil {
			sumExpr = kiwi.NewExpression(hVar)
		} else {
			sumExpr = sumExpr.Plus(hVar)
		}
	}
	group := kiwi.ApportionGroup{Vars: vars}
	switch t := targetHeightVarOrConst.(type) {
	case *kiwi.Variable:
		group.TargetVar = t
		if sumExpr != nil {
			_ = al.solver.AddConstraint(kiwi.NewConstraint(sumExpr, kiwi.OpEq, t))
		}
	case int:
		group.TargetConst = t
		if sumExpr != nil {
			_ = al.solver.AddConstraint(kiwi.NewConstraint(sumExpr, kiwi.OpEq, float64(t)))
		}
	case float64:
		group.TargetConst = int(t)
		if sumExpr != nil {
			_ = al.solver.AddConstraint(kiwi.NewConstraint(sumExpr, kiwi.OpEq, t))
		}
	case UIElement:
		targetVar := al.Var(t).Height
		group.TargetVar = targetVar
		if sumExpr != nil {
			_ = al.solver.AddConstraint(kiwi.NewConstraint(sumExpr, kiwi.OpEq, targetVar))
		}
	default:
		panic(fmt.Errorf("invalid target for ApportionHeights: %v", targetHeightVarOrConst))
	}
	al.ds.AddApportionGroup(group)
	return al
}

// SnapWidthToGrid adds a TrueType-style SnapToGrid hinting directive for element's width.
func (al *AutoLayout) SnapWidthToGrid(el UIElement, step int) *AutoLayout {
	ev := al.Var(el)
	al.ds.AddDirective(kiwi.SnapToGrid(ev.Width, step))
	return al
}

// EqualizeWidthsGroup adds an EqualizeGroup hinting directive forcing elements to equal integer widths.
func (al *AutoLayout) EqualizeWidthsGroup(elements ...UIElement) *AutoLayout {
	vars := make([]*kiwi.Variable, len(elements))
	for i, el := range elements {
		vars[i] = al.Var(el).Width
	}
	al.ds.AddDirective(kiwi.EqualizeGroup(vars...))
	return al
}

// SetPosition updates container bounds and solves layout.
func (al *AutoLayout) SetPosition(x1, y1, x2, y2 int) {
	al.ScreenObject.SetPosition(x1, y1, x2, y2)
	w := x2 - x1 + 1
	h := y2 - y1 + 1
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	_ = al.solver.SuggestValue(al.BoundsLeft, float64(x1))
	_ = al.solver.SuggestValue(al.BoundsTop, float64(y1))
	_ = al.solver.SuggestValue(al.BoundsWidth, float64(w))
	_ = al.solver.SuggestValue(al.BoundsHeight, float64(h))
	al.Apply()
}

// MoveRelative shifts container and re-solves layout.
func (al *AutoLayout) MoveRelative(dx, dy int) {
	al.ScreenObject.MoveRelative(dx, dy)
	x1, y1, x2, y2 := al.GetPosition()
	al.SetPosition(x1, y1, x2, y2)
}

// Show is an invisible container.
func (al *AutoLayout) Show(scr *ScreenBuf) {}

// Apply solves constraints and updates positions of all registered UIElements.
func (al *AutoLayout) Apply() {
	res := al.ds.SolveDiscrete()
	for el, ev := range al.vars {
		left := res.Get(ev.Left)
		top := res.Get(ev.Top)
		width := res.Get(ev.Width)
		height := res.Get(ev.Height)
		if width < 1 {
			width = 1
		}
		if height < 1 {
			height = 1
		}
		el.SetPosition(left, top, left+width-1, top+height-1)
		if sub, ok := el.(interface{ Apply() }); ok {
			sub.Apply()
		}
	}
}
