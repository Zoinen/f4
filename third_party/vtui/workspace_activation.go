package vtui

import "sort"

// beginActivationTraversal takes a unique recency snapshot. Unvisited tabs
// follow the numbered predecessors of the current tab, including wraparound.
func (fm *frameManager) beginActivationTraversal() {
	active := fm.Screens[fm.ActiveIdx]
	order := make([]*AppScreen, 0, len(fm.Screens))
	seen := make(map[*AppScreen]bool, len(fm.Screens))
	appendScreen := func(screen *AppScreen) {
		if !seen[screen] && fm.screenIndex(screen) >= 0 {
			seen[screen] = true
			order = append(order, screen)
		}
	}
	appendScreen(active)
	for i := len(fm.activationHistory) - 1; i >= 0; i-- {
		appendScreen(fm.activationHistory[i])
	}
	numbered := append([]*AppScreen{}, fm.Screens...)
	sort.Slice(numbered, func(i, j int) bool {
		return numbered[i].Number < numbered[j].Number
	})
	activeIndex := 0
	for i, screen := range numbered {
		if screen == active {
			activeIndex = i
			break
		}
	}
	for offset := 1; offset < len(numbered); offset++ {
		index := (activeIndex - offset + len(numbered)) % len(numbered)
		appendScreen(numbered[index])
	}
	fm.activationTraversal = order
	fm.activationTraversalIndex = 0
}

func (fm *frameManager) switchPreviousScreen() {
	if fm.ActiveIdx < 0 || fm.ActiveIdx >= len(fm.Screens) {
		return
	}
	if len(fm.activationTraversal) == 0 ||
		fm.activationTraversal[fm.activationTraversalIndex] != fm.Screens[fm.ActiveIdx] {
		fm.beginActivationTraversal()
	}
	for offset := 1; offset < len(fm.activationTraversal); offset++ {
		cursor := (fm.activationTraversalIndex + offset) % len(fm.activationTraversal)
		if idx := fm.screenIndex(fm.activationTraversal[cursor]); idx >= 0 {
			fm.activationTraversalIndex = cursor
			fm.switchScreen(idx, false)
			return
		}
	}
}
