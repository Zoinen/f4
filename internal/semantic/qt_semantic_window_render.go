package semantic

import (
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
)

func SemanticRowsWithContentKeys(rows []extui.TextRowModel) []extui.TextRowModel {
	for index := range rows {
		rows[index].ContentKey = extui.TextRowContentKey(rows[index])
	}
	return rows
}

func CloneInt64Slice(values []int64) []int64 {
	if values == nil {
		return nil
	}
	return append([]int64(nil), values...)
}

type semanticScrollBarSnapshot struct {
	present bool
	visible bool
	value   int
	min     int
	max     int
	pgStep  int
}

func SemanticCaptureScrollBar(scrollBar *vtui.ScrollBar) semanticScrollBarSnapshot {
	if scrollBar == nil {
		return semanticScrollBarSnapshot{}
	}
	return semanticScrollBarSnapshot{
		present: true,
		visible: scrollBar.IsVisible(),
		value:   scrollBar.Value,
		min:     scrollBar.Min,
		max:     scrollBar.Max,
		pgStep:  scrollBar.PgStep,
	}
}

func SemanticRestoreScrollBar(scrollBar *vtui.ScrollBar, state semanticScrollBarSnapshot) {
	if scrollBar == nil || !state.present {
		return
	}
	scrollBar.Value = state.value
	scrollBar.Min = state.min
	scrollBar.Max = state.max
	scrollBar.PgStep = state.pgStep
	scrollBar.SetVisible(state.visible)
}
