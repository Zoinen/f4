package panel

import (
	"strings"

	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
)

// defaultSortGroupOrder is the group number every file that matches no rule
// falls into. far/far2l use the same idea (DEFAULT_SORT_GROUP): the number is
// deliberately large so unclassified files land after the configured groups,
// while a user who wants a group *below* them can still say Group = 20000.
const DefaultSortGroupOrder = 10000

// SortGroupRule is one entry of the sort-group list. A rule can come directly
// from a coloured [Highlight_N] section, which is how f4 follows Far's model:
// the same matcher paints the file and places it in a sort group.
type SortGroupRule struct {
	Name   string
	Order  int
	Filter theme.HighlightRule
}

// SortGroupSet holds the configured groups in the order they must appear on
// the panel.
type SortGroupSet struct {
	Groups []SortGroupRule
}

// GlobalSortGroups is populated from highlight.ini at startup. It stays empty
// when the user configured no groups, which turns the whole feature into a
// no-op even for panels that have grouping switched on.
var GlobalSortGroups *SortGroupSet

func init() {
	GlobalSortGroups = &SortGroupSet{}
}

func (s *SortGroupSet) LoadFromIni(file *ini.File, sharedRules ...[]theme.HighlightRule) {
	if s == nil {
		return
	}
	s.Groups = nil
	if len(sharedRules) > 0 {
		s.Groups = append(s.Groups, sortGroupsFromHighlightRules(sharedRules[0])...)
	}
	// Keep accepting the original [SortGroup_N] sections so existing profiles
	// continue to work while users migrate matching fields into Highlight_N.
	s.Groups = append(s.Groups, ParseSortGroups(file)...)
}

// Configured reports whether any group is defined. Grouping a panel by an
// empty rule list would put every file into the default group, which is just
// the ungrouped order with extra work.
func (s *SortGroupSet) Configured() bool {
	return s != nil && len(s.Groups) > 0
}

// GroupOf returns the sort-group number of an item: the Order of the first
// matching rule, or defaultSortGroupOrder when nothing matches. First match
// wins, so an earlier, narrower rule can carve items out of a later one.
func (s *SortGroupSet) GroupOf(item *vfs.VFSItem) int {
	if s == nil || item == nil {
		return DefaultSortGroupOrder
	}
	for i := range s.Groups {
		if s.Groups[i].Filter.Match(item, true) {
			return s.Groups[i].Order
		}
	}
	return DefaultSortGroupOrder
}

// sortGroupsFromHighlightRules takes the groups declared inside coloured
// [Highlight_N] sections, which is Far's model: one matcher both paints the
// file and places it in a group.
func sortGroupsFromHighlightRules(rules []theme.HighlightRule) []SortGroupRule {
	groups := make([]SortGroupRule, 0, len(rules))
	for _, rule := range rules {
		if !rule.HasSortGroup {
			continue
		}
		name := rule.Name
		if name == "" {
			name = strings.Join(rule.Masks, ", ")
		}
		groups = append(groups, SortGroupRule{
			Name:   name,
			Order:  rule.SortGroup,
			Filter: rule,
		})
	}
	return groups
}

// ParseSortGroups reads the legacy [SortGroup_N] sections. The section number
// decides the default order, so the plain case — SortGroup_1, SortGroup_2, … —
// needs no Group key.
func ParseSortGroups(file *ini.File) []SortGroupRule {
	sections := theme.ParseRuleSections(file, "sortgroup_")
	groups := make([]SortGroupRule, 0, len(sections))
	for i, section := range sections {
		group := SortGroupRule{
			Name:   section.Rule.Name,
			Order:  i,
			Filter: section.Rule,
		}
		if section.Rule.HasSortGroup {
			group.Order = section.Rule.SortGroup
		}
		if group.Name == "" {
			group.Name = strings.Join(group.Filter.Masks, ", ")
		}
		groups = append(groups, group)
	}
	return groups
}
