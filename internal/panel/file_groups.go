package panel

import (
	"strconv"
	"time"

	"github.com/unxed/f4/internal/filegroups"
	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// AssociationGroups is wired once by the composition root. Preview snapshots
// must not be returned here: command dispatch sees committed settings only.
var AssociationGroups = func() *filegroups.Snapshot { return nil }

func (s *SortGroupSet) LoadFileGroups(snapshot *filegroups.Snapshot) error {
	groups := []SortGroupRule{}
	for _, g := range snapshot.Groups() {
		if !g.Enabled("Sort") {
			continue
		}
		predicate := snapshot.Predicate(g.ID)
		file := ini.New()
		file.Data["Highlight_0"] = g.Fields
		parsed := theme.ParseHighlightRules(file)
		rule := parsed[0]
		rule.Condition = predicate
		rule.Masks = nil
		rule.RuleID = g.ID
		order := DefaultSortGroupOrder
		if raw := g.Fields["Sort.Number"]; raw != "" {
			order, _ = strconv.Atoi(raw)
		}
		groups = append(groups, SortGroupRule{Name: g.Label(), Order: order, Filter: rule})
	}
	s.Groups = groups
	return nil
}

// RefreshFileGroupAppearance reorders existing entries, not filesystem contents.
// Existing entry objects retain their marks; cursor and viewport anchors survive.
func RefreshFileGroupAppearance(resort bool) {
	if vtui.FrameManager == nil {
		return
	}
	for _, screen := range vtui.FrameManager.Screens {
		for _, frame := range screen.Frames {
			pf, ok := frame.(*PanelsFrame)
			if !ok || pf.Closed {
				continue
			}
			if !resort {
				continue
			}
			for _, p := range pf.Panels {
				if fp, ok := p.(*FileSystemPanel); ok {
					if fp.UseSortGroups {
						fp.sortEntriesKeepingCursor()
					}
				}
			}
		}
	}
	vtui.FrameManager.HardRefresh()
}

func matchingGroupAssociations(snapshot *filegroups.Snapshot, item *vfs.VFSItem, kind AssocKind) []FileAssoc {
	result := []FileAssoc{}
	if item == nil || item.IsDir || item.Name == ".." || kind < 0 || int(kind) >= AssocKindCount {
		return result
	}
	for _, group := range snapshot.Matching(item, true, "Associations", time.Now()) {
		for _, command := range group.Commands {
			if !command.Enabled[kind] || command.Commands[kind] == "" {
				continue
			}
			name := command.Name
			if name == "" {
				name = group.Label()
			}
			result = append(result, FileAssoc{Mask: group.Fields["Mask"], Description: name, Commands: command.Commands, Enabled: command.Enabled})
		}
	}
	return result
}
