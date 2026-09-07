package vtui

// Distribute1D implements Section 7.4 deterministic 1D integer distribution.
func Distribute1D(length int, items []SizeSpec, spacing int, marginBefore, marginAfter int) (sizes []int, positions []int) {
	n := len(items)
	sizes = make([]int, n)
	positions = make([]int, n)
	if n == 0 {
		return sizes, positions
	}

	// 1. Usable space
	usable := length - marginBefore - marginAfter - spacing*(n-1)
	if usable < 0 {
		usable = 0
	}

	// 2. Initial size clamp(Hint, Min, Max)
	sumSize := 0
	for i, it := range items {
		s := it.Hint
		if s < it.Min {
			s = it.Min
		}
		if it.Max > 0 && s > it.Max {
			s = it.Max
		}
		sizes[i] = s
		sumSize += s
	}

	surplus := usable - sumSize

	// 4a. Surplus > 0 (distribute extra space)
	if surplus > 0 {
		for iter := 0; iter < 8 && surplus > 0; iter++ {
			var candidates []int
			for i, it := range items {
				if it.Policy == PolicyExpanding && (it.Max <= 0 || sizes[i] < it.Max) {
					candidates = append(candidates, i)
				}
			}
			if len(candidates) == 0 {
				for i, it := range items {
					if (it.Policy == PolicyPreferred || it.Policy == PolicyMinimum) && (it.Max <= 0 || sizes[i] < it.Max) {
						candidates = append(candidates, i)
					}
				}
			}
			if len(candidates) == 0 {
				break
			}

			totalWeight := 0
			for _, c := range candidates {
				w := items[c].Stretch
				if w < 1 {
					w = 1
				}
				totalWeight += w
			}

			addSum := 0
			adds := make([]int, len(candidates))
			for j, c := range candidates {
				w := items[c].Stretch
				if w < 1 {
					w = 1
				}
				add := (surplus * w) / totalWeight
				adds[j] = add
				addSum += add
			}
			remainder := surplus - addSum
			for j := 0; j < remainder && j < len(candidates); j++ {
				adds[j]++
				addSum++
			}

			reclaimed := 0
			for j, c := range candidates {
				sizes[c] += adds[j]
				if items[c].Max > 0 && sizes[c] > items[c].Max {
					reclaimed += sizes[c] - items[c].Max
					sizes[c] = items[c].Max
				}
			}
			surplus = reclaimed
		}
	} else if surplus < 0 {
		// 4b. Surplus < 0 (reclaim deficit)
		deficit := -surplus
		for iter := 0; iter < 8 && deficit > 0; iter++ {
			var candidates []int
			totalWeight := 0
			for i, it := range items {
				if sizes[i] > it.Min {
					candidates = append(candidates, i)
					totalWeight += sizes[i] - it.Min
				}
			}
			if len(candidates) == 0 || totalWeight == 0 {
				break
			}

			subSum := 0
			subs := make([]int, len(candidates))
			for j, c := range candidates {
				weight := sizes[c] - items[c].Min
				sub := (deficit * weight) / totalWeight
				subs[j] = sub
				subSum += sub
			}
			remainder := deficit - subSum
			for j := 0; j < remainder && j < len(candidates); j++ {
				subs[j]++
				subSum++
			}

			remDeficit := 0
			for j, c := range candidates {
				sizes[c] -= subs[j]
				if sizes[c] < items[c].Min {
					remDeficit += items[c].Min - sizes[c]
					sizes[c] = items[c].Min
				}
			}
			deficit = remDeficit
		}
	}

	// 5. Positions
	pos := marginBefore
	for i := 0; i < n; i++ {
		positions[i] = pos
		pos += sizes[i] + spacing
	}

	return sizes, positions
}

// ComputeContainerSizeHint computes bottom-up SizeSpec for container and layout.
func ComputeContainerSizeHint(children []UIElement, layoutType string, spacing int, margins Margins) (hSpec SizeSpec, vSpec SizeSpec) {
	if len(children) == 0 {
		hSpec = SizeSpec{Hint: margins.Left + margins.Right, Min: margins.Left + margins.Right, Policy: PolicyPreferred, Stretch: 1}
		vSpec = SizeSpec{Hint: margins.Top + margins.Bottom, Min: margins.Top + margins.Bottom, Policy: PolicyPreferred, Stretch: 1}
		return
	}

	switch layoutType {
	case "HBox":
		sumW, sumMinW := 0, 0
		maxH, maxMinH := 0, 0
		hExpanding, vExpanding := false, false
		for _, ch := range children {
			hs := ch.SizeSpecH()
			vs := ch.SizeSpecV()
			sumW += hs.Hint
			sumMinW += hs.Min
			if hs.Policy == PolicyExpanding {
				hExpanding = true
			}
			if vs.Hint > maxH {
				maxH = vs.Hint
			}
			if vs.Min > maxMinH {
				maxMinH = vs.Min
			}
			if vs.Policy == PolicyExpanding {
				vExpanding = true
			}
		}
		spacingW := (len(children) - 1) * spacing
		hPolicy := PolicyPreferred
		if hExpanding {
			hPolicy = PolicyExpanding
		}
		vPolicy := PolicyPreferred
		if vExpanding {
			vPolicy = PolicyExpanding
		}
		hSpec = SizeSpec{
			Hint:    sumW + spacingW + margins.Left + margins.Right,
			Min:     sumMinW + spacingW + margins.Left + margins.Right,
			Policy:  hPolicy,
			Stretch: 1,
		}
		vSpec = SizeSpec{
			Hint:    maxH + margins.Top + margins.Bottom,
			Min:     maxMinH + margins.Top + margins.Bottom,
			Policy:  vPolicy,
			Stretch: 1,
		}
	case "Form":
		// Form is 2-column: col0 is label, col1 is field
		maxLabelW, maxLabelMinW := 0, 0
		maxFieldW, maxFieldMinW := 0, 0
		sumH, sumMinH := 0, 0
		rowCount := (len(children) + 1) / 2
		for r := 0; r < rowCount; r++ {
			lbl := children[r*2]
			lhs := lbl.SizeSpecH()
			lvs := lbl.SizeSpecV()
			if lhs.Hint > maxLabelW {
				maxLabelW = lhs.Hint
			}
			if lhs.Min > maxLabelMinW {
				maxLabelMinW = lhs.Min
			}
			rowH := lvs.Hint
			rowMinH := lvs.Min

			if r*2+1 < len(children) {
				fld := children[r*2+1]
				fhs := fld.SizeSpecH()
				fvs := fld.SizeSpecV()
				if fhs.Hint > maxFieldW {
					maxFieldW = fhs.Hint
				}
				if fhs.Min > maxFieldMinW {
					maxFieldMinW = fhs.Min
				}
				if fvs.Hint > rowH {
					rowH = fvs.Hint
				}
				if fvs.Min > rowMinH {
					rowMinH = fvs.Min
				}
			}
			sumH += rowH
			sumMinH += rowMinH
		}
		spacingH := (rowCount - 1) * spacing
		hSpec = SizeSpec{
			Hint:    maxLabelW + 1 + maxFieldW + margins.Left + margins.Right,
			Min:     maxLabelMinW + 1 + maxFieldMinW + margins.Left + margins.Right,
			Policy:  PolicyExpanding,
			Stretch: 1,
		}
		vSpec = SizeSpec{
			Hint:    sumH + spacingH + margins.Top + margins.Bottom,
			Min:     sumMinH + spacingH + margins.Top + margins.Bottom,
			Policy:  PolicyPreferred,
			Stretch: 1,
		}
	default: // VBox
		maxW, maxMinW := 0, 0
		sumH, sumMinH := 0, 0
		hExpanding, vExpanding := false, false
		for _, ch := range children {
			hs := ch.SizeSpecH()
			vs := ch.SizeSpecV()
			if hs.Hint > maxW {
				maxW = hs.Hint
			}
			if hs.Min > maxMinW {
				maxMinW = hs.Min
			}
			if hs.Policy == PolicyExpanding {
				hExpanding = true
			}
			sumH += vs.Hint
			sumMinH += vs.Min
			if vs.Policy == PolicyExpanding {
				vExpanding = true
			}
		}
		spacingH := (len(children) - 1) * spacing
		hPolicy := PolicyPreferred
		if hExpanding {
			hPolicy = PolicyExpanding
		}
		vPolicy := PolicyPreferred
		if vExpanding {
			vPolicy = PolicyExpanding
		}
		hSpec = SizeSpec{
			Hint:    maxW + margins.Left + margins.Right,
			Min:     maxMinW + margins.Left + margins.Right,
			Policy:  hPolicy,
			Stretch: 1,
		}
		vSpec = SizeSpec{
			Hint:    sumH + spacingH + margins.Top + margins.Bottom,
			Min:     sumMinH + spacingH + margins.Top + margins.Bottom,
			Policy:  vPolicy,
			Stretch: 1,
		}
	}
	return
}

// ApplyLayoutTree recursively calculates positions for an element tree given its bounding box.
func ApplyLayoutTree(container UIElement, layoutType string, spacing int, margins Margins, align string, children []UIElement) {
	if len(children) == 0 {
		return
	}
	cx1, cy1, cx2, cy2 := container.GetPosition()
	cw := cx2 - cx1 + 1
	ch := cy2 - cy1 + 1

	switch layoutType {
	case "HBox":
		specs := make([]SizeSpec, len(children))
		for i, ch := range children {
			specs[i] = ch.SizeSpecH()
		}
		widths, xOffsets := Distribute1D(cw, specs, spacing, margins.Left, margins.Right)
		if align == "center" || align == "right" {
			totalW := 0
			for i, w := range widths {
				totalW += w
				if i < len(widths)-1 {
					totalW += spacing
				}
			}
			rem := cw - margins.Left - margins.Right - totalW
			if rem > 0 {
				shift := 0
				if align == "center" {
					shift = (rem + 1) / 2
				} else if align == "right" {
					shift = rem
				}
				for i := range xOffsets {
					xOffsets[i] += shift
				}
			}
		}

		usableH := ch - margins.Top - margins.Bottom
		if usableH < 1 {
			usableH = 1
		}
		for i, child := range children {
			x := cx1 + xOffsets[i]
			w := widths[i]
			if w < 1 {
				w = 1
			}
			y := cy1 + margins.Top
			h := usableH
			childAlign := "fill"
			if so, ok := child.(interface {
				GetProperty(string) (PropValue, bool)
			}); ok {
				if v, ok := so.GetProperty("align"); ok && v.S != "" {
					childAlign = v.S
				}
			}
			vs := child.SizeSpecV()
			if childAlign != "fill" && vs.Hint > 0 && vs.Hint < usableH {
				h = vs.Hint
				switch childAlign {
				case "center":
					y += (usableH - h + 1) / 2
				case "end", "bottom":
					y += usableH - h
				}
			}
			child.SetPosition(x, y, x+w-1, y+h-1)
			if sub, ok := child.(interface{ ApplyLayout() }); ok {
				sub.ApplyLayout()
			}
		}

	case "Form":
		rowCount := (len(children) + 1) / 2
		labelSpecs := make([]SizeSpec, rowCount)
		fieldSpecs := make([]SizeSpec, rowCount)
		rowSpecs := make([]SizeSpec, rowCount)

		for r := 0; r < rowCount; r++ {
			lbl := children[r*2]
			labelSpecs[r] = lbl.SizeSpecH()
			rowSpecs[r] = lbl.SizeSpecV()
			if r*2+1 < len(children) {
				fld := children[r*2+1]
				fieldSpecs[r] = fld.SizeSpecH()
				fvs := fld.SizeSpecV()
				if fvs.Hint > rowSpecs[r].Hint {
					rowSpecs[r] = fvs
				}
			}
		}

		maxLabelW := 0
		for _, s := range labelSpecs {
			if s.Hint > maxLabelW {
				maxLabelW = s.Hint
			}
		}

		heights, yOffsets := Distribute1D(ch, rowSpecs, spacing, margins.Top, margins.Bottom)
		usableW := cw - margins.Left - margins.Right
		fieldW := usableW - maxLabelW - 1
		if fieldW < 1 {
			fieldW = 1
		}

		for r := 0; r < rowCount; r++ {
			y := cy1 + yOffsets[r]
			h := heights[r]
			if h < 1 {
				h = 1
			}

			lbl := children[r*2]
			lblX := cx1 + margins.Left
			lbl.SetPosition(lblX, y, lblX+maxLabelW-1, y+h-1)

			if r*2+1 < len(children) {
				fld := children[r*2+1]
				fldX := lblX + maxLabelW + 1
				fld.SetPosition(fldX, y, fldX+fieldW-1, y+h-1)
				if sub, ok := fld.(interface{ ApplyLayout() }); ok {
					sub.ApplyLayout()
				}
			}
		}

	default: // VBox
		specs := make([]SizeSpec, len(children))
		for i, ch := range children {
			specs[i] = ch.SizeSpecV()
		}
		heights, yOffsets := Distribute1D(ch, specs, spacing, margins.Top, margins.Bottom)
		usableW := cw - margins.Left - margins.Right
		if usableW < 1 {
			usableW = 1
		}

		for i, child := range children {
			y := cy1 + yOffsets[i]
			h := heights[i]
			if h < 1 {
				h = 1
			}
			x := cx1 + margins.Left
			w := usableW
			childAlign := "fill"
			if so, ok := child.(interface {
				GetProperty(string) (PropValue, bool)
			}); ok {
				if v, ok := so.GetProperty("align"); ok && v.S != "" {
					childAlign = v.S
				}
			}
			hs := child.SizeSpecH()
			if childAlign != "fill" && hs.Hint > 0 && hs.Hint < usableW {
				w = hs.Hint
				switch childAlign {
				case "center":
					x += (usableW - w + 1) / 2
				case "end", "right":
					x += usableW - w
				}
			}
			child.SetPosition(x, y, x+w-1, y+h-1)
			if sub, ok := child.(interface{ ApplyLayout() }); ok {
				sub.ApplyLayout()
			}
		}
	}
}
