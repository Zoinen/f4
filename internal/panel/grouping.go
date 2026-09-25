package panel

import (
	"cmp"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"golang.org/x/text/unicode/norm"
)

// GroupMode is independent of the sort mode used inside each group.
type GroupMode int

const (
	GroupNone GroupMode = iota
	GroupName
	GroupExtension
	GroupSize
	GroupPhysicalSize
	GroupModified
	GroupAccessed
	GroupChanged
	GroupOwner
	GroupOwnerGroup
	GroupPermissions
	GroupAttributes
	GroupModeText
	GroupKind
	GroupHidden
	GroupExecutable
	GroupFileField
)

// GroupModeInfo is the common catalog for menus, actions and persistence.
type GroupModeInfo struct {
	Mode  GroupMode
	ID    string
	Label string
}

var GroupModes = []GroupModeInfo{
	{GroupNone, "None", "Off"}, {GroupName, "Name", "Name"},
	{GroupExtension, "Extension", "Extension"}, {GroupSize, "Size", "Size"},
	{GroupPhysicalSize, "PhysicalSize", "Size on disk"},
	{GroupModified, "Modified", "Modification time"}, {GroupAccessed, "Accessed", "Access time"},
	{GroupChanged, "Changed", "Creation / metadata change time"},
	{GroupOwner, "Owner", "Owner (UID)"}, {GroupOwnerGroup, "OwnerGroup", "Owner group (GID)"},
	{GroupPermissions, "Permissions", "Unix permissions"}, {GroupAttributes, "Attributes", "Windows attributes"},
	{GroupModeText, "ModeText", "VFS type / mode"}, {GroupKind, "Kind", "Object kind"},
	{GroupHidden, "Hidden", "Hidden"}, {GroupExecutable, "Executable", "Executable"},
	{GroupFileField, "FileField", "File field"},
}

func ValidGroupMode(mode GroupMode) GroupMode {
	if mode < GroupNone || mode > GroupFileField {
		return GroupNone
	}
	return mode
}

// ParseGroupModeID accepts the stable semantic identifier used by the Qt
// bridge. Labels are localized and must not become part of the transport
// contract.
func ParseGroupModeID(id string) (GroupMode, bool) {
	for _, info := range GroupModes {
		if info.ID == id {
			return info.Mode, true
		}
	}
	return GroupNone, false
}

// PanelGroup describes a contiguous range in the file-only visible list.
type PanelGroup struct {
	Key        string
	Title      string
	StartIndex int
	Count      int
}

type groupKey struct {
	id, title  string
	pin        int // -1 folders, 0 ordinary, 1 unknown
	order      int64
	numeric    float64
	numeric2   float64
	numericSet bool
}

type displayRow struct {
	entry   int // -1 for a heading
	title   string
	heading int // preceding heading, or -1 for the parent entry
}

func namedGroup(id string, order int64) groupKey {
	return groupKey{id: id, title: i18n.Msg("Group." + id), order: order}
}

func unknownGroup() groupKey {
	k := namedGroup("Unknown", 0)
	k.pin = 1
	return k
}

func sizeGroup(size int64, known bool, limits [3]int64) groupKey {
	if !known || size < 0 {
		return unknownGroup()
	}
	names := [...]string{"Small", "Medium", "Large", "ExtraLarge"}
	for i, limit := range limits {
		if size <= limit {
			return namedGroup(names[i], int64(i))
		}
	}
	return namedGroup(names[3], 3)
}

func dateGroup(value, now time.Time, known bool) groupKey {
	if !known || value.IsZero() {
		return unknownGroup()
	}
	value = value.In(now.Location())
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	if !value.Before(today.AddDate(0, 0, 1)) {
		return namedGroup("Future", -4)
	}
	if !value.Before(today) {
		return namedGroup("Today", -3)
	}
	if !value.Before(today.AddDate(0, 0, -1)) {
		return namedGroup("Yesterday", -2)
	}
	if !value.Before(today.AddDate(0, 0, -2)) {
		return namedGroup("TwoDaysAgo", -1)
	}
	y, m, _ = value.Date()
	// Days have negative ranks. Older months follow, newest first.
	age := int64(now.Year()-y)*12 + int64(now.Month()-m)
	return groupKey{id: fmt.Sprintf("month:%04d-%02d", y, m), title: fmt.Sprintf("%s %d", i18n.Msg(fmt.Sprintf("Group.Month%d", m)), y), order: age}
}

func (fp *FileSystemPanel) groupFor(e *FileEntry, now time.Time, limits [3]int64) groupKey {
	if e.IsDir && fp.GroupFoldersSeparately {
		k := namedGroup("Folders", 0)
		k.pin = -1
		return k
	}
	item := e.VFSItem
	switch fp.GroupBy {
	case GroupName:
		r, _ := utf8.DecodeRuneInString(norm.NFC.String(e.Name))
		if unicode.IsDigit(r) {
			return namedGroup("Digits", -1)
		}
		if !unicode.IsLetter(r) {
			return namedGroup("Other", 1)
		}
		s := string(unicode.ToUpper(r))
		return groupKey{id: "name:" + s, title: s}
	case GroupExtension:
		_, ext := splitFileExtension(e.Name)
		if ext == "" || e.NoExtension {
			return namedGroup("NoExtension", 1)
		}
		ext = strings.ToLower(norm.NFC.String(ext))
		return groupKey{id: "ext:" + ext, title: ext}
	case GroupSize:
		known := item.SizeKnown || item.Size != 0
		if e.IsDir {
			known = e.SizeCalculated
		}
		return sizeGroup(e.Size, known, limits)
	case GroupPhysicalSize:
		return sizeGroup(e.PhysicalSize, !e.IsDir && item.HasMetadata(vfs.MetadataPhysicalSize), limits)
	case GroupModified:
		return dateGroup(e.MTime, now, item.HasMetadata(vfs.MetadataMTime))
	case GroupAccessed:
		return dateGroup(e.ATime, now, item.HasMetadata(vfs.MetadataATime))
	case GroupChanged:
		return dateGroup(e.CTime, now, item.HasMetadata(vfs.MetadataCTime))
	case GroupOwner, GroupOwnerGroup:
		n, field := e.Uid, vfs.MetadataUID
		if fp.GroupBy == GroupOwnerGroup {
			n, field = e.Gid, vfs.MetadataGID
		}
		if n < 0 || !item.HasMetadata(field) {
			return unknownGroup()
		}
		return groupKey{id: fmt.Sprintf("id:%d", n), title: fmt.Sprintf("%d", n), order: int64(n)}
	case GroupPermissions:
		if !item.HasMetadata(vfs.MetadataPermissions) {
			return unknownGroup()
		}
		n := e.UnixMode & 07777
		return groupKey{id: fmt.Sprintf("permissions:%04o", n), title: fmt.Sprintf("%04o", n), order: int64(n)}
	case GroupAttributes:
		if !item.HasMetadata(vfs.MetadataWinAttrs) {
			return unknownGroup()
		}
		return groupKey{id: fmt.Sprintf("attrs:%08x", e.WinAttrs), title: windowsGroupAttributes(e.WinAttrs), order: int64(e.WinAttrs)}
	case GroupModeText:
		if e.Mode == "" {
			return unknownGroup()
		}
		return groupKey{id: "mode:" + e.Mode, title: e.Mode}
	case GroupKind:
		if e.ReparseTag == 0xa0000003 {
			return namedGroup("Junctions", 3)
		}
		if e.IsSymlink {
			return namedGroup("Links", 2)
		}
		if e.IsDir {
			return namedGroup("Folders", 0)
		}
		return namedGroup("Files", 1)
	case GroupFileField:
		descriptor, ok := fileFieldDescriptor(fp.FileFieldGroup)
		if !ok {
			return unknownGroup()
		}
		value := fieldValueForSort(e, descriptor)
		switch value.State {
		case extui.FileFieldUnread, "":
			key := groupKey{id: "field:unread", title: "Not read", pin: 1, order: 1}
			return key
		case extui.FileFieldMissing:
			return groupKey{id: "field:missing", title: "No data", pin: 1, order: 0}
		case extui.FileFieldKnown:
			key := groupKey{pin: 0, numericSet: descriptor.Kind != extui.FileFieldText}
			switch descriptor.Kind {
			case extui.FileFieldText:
				key.id = "field:" + fp.FileFieldGroup + ":text:" + value.Text
				key.title = value.Text
			case extui.FileFieldRange:
				key.id = fmt.Sprintf("field:%s:range:%g:%g", fp.FileFieldGroup, value.Min, value.Max)
				key.title = formatFileField(fp.FileFieldGroup, value)
				key.numeric, key.numeric2 = value.Min, value.Max
			case extui.FileFieldNumber:
				key.id = fmt.Sprintf("field:%s:number:%g", fp.FileFieldGroup, value.Number)
				key.title = formatFileField(fp.FileFieldGroup, value)
				key.numeric = value.Number
			case extui.FileFieldInteger:
				key.id = fmt.Sprintf("field:%s:integer:%d", fp.FileFieldGroup, value.Integer)
				key.title = formatFileField(fp.FileFieldGroup, value)
				key.numeric = float64(value.Integer)
			}
			return key
		}
	case GroupHidden, GroupExecutable:
		value, field := e.IsHidden, vfs.MetadataHidden
		if fp.GroupBy == GroupExecutable {
			value, field = e.IsExecutable, vfs.MetadataExecutable
		}
		if !item.HasMetadata(field) {
			return unknownGroup()
		}
		if value {
			return namedGroup("Yes", 1)
		}
		return namedGroup("No", 0)
	}
	return unknownGroup()
}

func windowsGroupAttributes(bits uint32) string {
	var names []string
	for _, attr := range []struct {
		bit  uint32
		name string
	}{
		{1, "R"}, {2, "H"}, {4, "S"}, {16, "D"}, {32, "A"}, {128, "N"}, {256, "T"},
		{512, "Sparse"}, {1024, "Reparse"}, {2048, "Compressed"}, {4096, "Offline"}, {8192, "NotIndexed"}, {16384, "Encrypted"},
	} {
		if bits&attr.bit != 0 {
			names = append(names, attr.name)
			bits &^= attr.bit
		}
	}
	if bits != 0 {
		names = append(names, fmt.Sprintf("0x%08X", bits))
	}
	if len(names) == 0 {
		return "0"
	}
	return strings.Join(names, " | ")
}

func (fp *FileSystemPanel) prepareGrouping(entries []*FileEntry, now time.Time) {
	fp.groupKeys = nil
	if fp.GroupBy == GroupNone {
		return
	}
	fp.groupDate = now.Format("2006-01-02")
	fp.groupLimits = config.PanelGroupLimits()
	fp.groupKeys = make(map[*FileEntry]groupKey, len(entries))
	for _, e := range entries {
		fp.groupKeys[e] = fp.groupFor(e, now, fp.groupLimits)
	}
}

func (fp *FileSystemPanel) compareGroups(a, b *FileEntry, compareText func(string, string) int) int {
	ka, kb := fp.groupKeys[a], fp.groupKeys[b]
	if n := cmp.Compare(ka.pin, kb.pin); n != 0 {
		return n
	}
	n := 0
	if ka.numericSet && kb.numericSet {
		n = cmp.Compare(ka.numeric, kb.numeric)
		if n == 0 {
			n = cmp.Compare(ka.numeric2, kb.numeric2)
		}
	} else {
		n = cmp.Compare(ka.order, kb.order)
	}
	if n == 0 {
		n = compareText(ka.title, kb.title)
	}
	if n == 0 {
		// Provider Mode values are exact strings. Collation can consider two
		// different strings equivalent; keep their groups contiguous anyway.
		n = cmp.Compare(ka.id, kb.id)
	}
	if fp.GroupReverse && ka.pin == 0 {
		n = -n
	}
	return n
}

func (fp *FileSystemPanel) rebuildDisplayRows() {
	fp.displayRows, fp.entryRows, fp.visibleGroups = nil, nil, nil
	if fp.GroupBy == GroupNone {
		return
	}
	fp.entryRows = make([]int, len(fp.Entries))
	lastKey := ""
	heading := -1
	for idx, entry := range fp.Entries {
		if entry.Name != ".." {
			key, ok := fp.groupKeys[entry]
			if !ok {
				key = fp.groupFor(entry, time.Now(), config.PanelGroupLimits())
			}
			if key.id != lastKey {
				heading = len(fp.displayRows)
				fp.displayRows = append(fp.displayRows, displayRow{entry: -1, title: key.title})
				fp.visibleGroups = append(fp.visibleGroups, PanelGroup{Key: key.id, Title: key.title, StartIndex: idx})
				lastKey = key.id
			}
			fp.visibleGroups[len(fp.visibleGroups)-1].Count++
		}
		fp.entryRows[idx] = len(fp.displayRows)
		fp.displayRows = append(fp.displayRows, displayRow{entry: idx, heading: heading})
	}
}

// rebuildGroupingRows refreshes the group-key cache and the presentation-only
// rows after a catalog replacement. Directory loading can replace Entries
// without going through SortEntries, so leaving this to the next user action
// makes a stale group range describe the new catalog (most visibly, it can
// absorb the leading ".." row).
func (fp *FileSystemPanel) rebuildGroupingRows(now time.Time) {
	if fp == nil {
		return
	}
	fp.prepareGrouping(fp.AllEntries(), now)
	fp.rebuildDisplayRows()
}

func (fp *FileSystemPanel) displayCount() int {
	if fp.GroupBy == GroupNone {
		return len(fp.Entries)
	}
	return len(fp.displayRows)
}

func (fp *FileSystemPanel) entryAtDisplay(row int) int {
	if row < 0 || row >= fp.displayCount() {
		return -1
	}
	if fp.GroupBy == GroupNone {
		return row
	}
	return fp.displayRows[row].entry
}

func (fp *FileSystemPanel) displayOfEntry(idx int) int {
	if fp.GroupBy != GroupNone && idx >= 0 && idx < len(fp.entryRows) {
		return fp.entryRows[idx]
	}
	return idx
}

func (fp *FileSystemPanel) nearestDisplayEntry(row, direction int) int {
	n := fp.displayCount()
	if n == 0 {
		return 0
	}
	row = max(0, min(row, n-1))
	for i := row; i >= 0 && i < n; i += direction {
		if idx := fp.entryAtDisplay(i); idx >= 0 {
			return idx
		}
	}
	for i := row; i >= 0 && i < n; i -= direction {
		if idx := fp.entryAtDisplay(i); idx >= 0 {
			return idx
		}
	}
	return 0
}

func (fp *FileSystemPanel) SetGrouping(mode GroupMode, reverse, foldersSeparately bool) {
	fp.setGroupingAt(mode, reverse, foldersSeparately, time.Now())
}

// SetFileFieldGroup groups by exact typed values from the current file-field
// schema. It only reorders the loaded listing and never starts a directory read.
func (fp *FileSystemPanel) SetFileFieldGroup(fieldID string, reverse bool) bool {
	if fieldID != "" {
		if _, ok := fileFieldDescriptor(fieldID); !ok {
			return false
		}
	}
	if fieldID == "" {
		fp.FileFieldGroup = ""
		fp.SetGrouping(GroupNone, false, fp.GroupFoldersSeparately)
		return true
	}
	fp.FileFieldGroup = fieldID
	fp.setGroupingAt(GroupFileField, reverse, fp.GroupFoldersSeparately, time.Now())
	return true
}

func (fp *FileSystemPanel) setGroupingAt(mode GroupMode, reverse, foldersSeparately bool, now time.Time) {
	if fp == nil {
		return
	}
	mode = ValidGroupMode(mode)
	if fp.GroupBy == mode && fp.GroupReverse == reverse &&
		fp.GroupFoldersSeparately == foldersSeparately &&
		!fp.groupingNeedsRefresh(now) {
		return
	}
	focused := fp.GetRawSelectedName()
	offset := fp.displayOfEntry(fp.GetCursorIndex()) - fp.Table.TopPos
	wasGrouped := fp.GroupBy != GroupNone
	fp.GroupBy, fp.GroupReverse, fp.GroupFoldersSeparately = ValidGroupMode(mode), reverse, foldersSeparately
	if fp.GroupBy != GroupFileField {
		fp.FileFieldGroup = ""
	}
	if wasGrouped && fp.GroupBy == GroupNone && fp.SortMode == SortUnsorted {
		entries := fp.AllEntries()
		sort.SliceStable(entries, func(i, j int) bool { return entries[i].sourceOrder < entries[j].sourceOrder })
	}
	fp.sortEntriesAt(now)
	fp.focusEntryByName(focused)
	fp.Table.TopPos = max(0, fp.displayOfEntry(fp.GetCursorIndex())-offset)
	fp.SetCursorIndex(fp.GetCursorIndex())
	fp.Refresh()
	vtui.DebugLog("PANEL: grouping mode=%d reverse=%t folders=%t groups=%d", fp.GroupBy, reverse, foldersSeparately, len(fp.visibleGroups))
}

func (fp *FileSystemPanel) Groups() []PanelGroup { return fp.visibleGroups }

func (fp *FileSystemPanel) FileTop() int { return fp.nearestDisplayEntry(fp.Table.TopPos, 1) }

func (fp *FileSystemPanel) groupingNeedsRefresh(now time.Time) bool {
	return fp.GroupBy != GroupNone && (fp.groupDate != now.Format("2006-01-02") || fp.groupLimits != config.PanelGroupLimits())
}

// RefreshGrouping rebuilds on the UI task queue, outside painting.
func (fp *FileSystemPanel) RefreshGrouping(now time.Time) {
	if fp.groupingNeedsRefresh(now) {
		fp.setGroupingAt(fp.GroupBy, fp.GroupReverse, fp.GroupFoldersSeparately, now)
	}
}
