package app

import (
	"testing"

	"github.com/unxed/f4/internal/panel"
)

func TestOpenPlayerPanelFindsPlayerAfterSwitchingSides(t *testing.T) {
	player := &panel.PlayerPanel{}

	for _, test := range []struct {
		name   string
		active int
		slot   int
	}{
		{name: "active side", active: 0, slot: 0},
		{name: "passive side", active: 0, slot: 1},
		{name: "after switching", active: 1, slot: 0},
		{name: "after switching back", active: 1, slot: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			pf := &panel.PanelsFrame{ShowPanels: true, ActiveIdx: test.active}
			pf.AltPanels[test.slot] = player

			if got := openPlayerPanel(pf); got != player {
				t.Fatalf("openPlayerPanel() = %p, want %p", got, player)
			}
		})
	}
}
