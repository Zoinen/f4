package panel

import (
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/unxed/f4/internal/media"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// FileFieldFilter is one flat field/operation/value predicate. An empty value
// is valid for the presence operations "has" and "missing".
type FileFieldFilter struct {
	FieldID   string `json:"fieldId"`
	Operation string `json:"operation"`
	Value     string `json:"value"`
}

// FileFieldColumn persists one visible Details column and its width in the
// panel's file-field schema order.
type FileFieldColumn struct {
	FieldID string `json:"fieldId"`
	Width   int    `json:"width"`
}

// GalleryColumnWidth stores the relative width used by the semantic Details
// header and row renderer. IDs cover built-in columns and registered fields.
type GalleryColumnWidth struct {
	ID    string
	Width int
}

const maxGalleryColumnWidth = 1_000_000

func validGalleryColumnID(id string) bool {
	if id == "name" || id == "size" {
		return true
	}
	_, ok := fileFieldDescriptor(id)
	return ok
}

func CloneGalleryColumnWidths(source map[string]int) map[string]int {
	clone := make(map[string]int, len(source))
	for id, width := range source {
		if validGalleryColumnID(id) && width > 0 &&
			width <= maxGalleryColumnWidth {
			clone[id] = width
		}
	}
	return clone
}

// SetGalleryColumnWidths updates only presentation state. It does not change
// the directory generation or request another thumbnail/metadata read.
func (fp *FileSystemPanel) SetGalleryColumnWidths(widths []GalleryColumnWidth) bool {
	if fp == nil || len(widths) == 0 {
		return false
	}
	updates := make(map[string]int, len(widths))
	for _, column := range widths {
		if !validGalleryColumnID(column.ID) || column.Width < 1 ||
			column.Width > maxGalleryColumnWidth {
			return false
		}
		if _, exists := updates[column.ID]; exists {
			return false
		}
		updates[column.ID] = column.Width
	}
	next := CloneGalleryColumnWidths(fp.GalleryColumnWidths)
	for id, width := range updates {
		next[id] = width
	}
	if reflect.DeepEqual(fp.GalleryColumnWidths, next) {
		return true
	}
	fp.GalleryColumnWidths = next
	fp.Refresh()
	return true
}

type fileFieldMatchState uint8

const (
	fileFieldMatchFalse fileFieldMatchState = iota
	fileFieldMatchTrue
	fileFieldMatchUnknown
)

func fileFieldDescriptor(id string) (extui.FileFieldDescriptor, bool) {
	return extui.FileFieldDescriptorByID(id)
}

func fileFieldValuesToMap(values map[string]extui.FileFieldValue) map[string]any {
	out := make(map[string]any, len(values))
	for id, value := range values {
		out[id] = value.ToMap()
	}
	return out
}

func numericFieldValue(value any) (float64, bool) {
	var number float64
	switch n := value.(type) {
	case float64:
		number = n
	case float32:
		number = float64(n)
	case int:
		number = float64(n)
	case int8:
		number = float64(n)
	case int16:
		number = float64(n)
	case int32:
		number = float64(n)
	case int64:
		number = float64(n)
	case uint:
		number = float64(n)
	case uint8:
		number = float64(n)
	case uint16:
		number = float64(n)
	case uint32:
		number = float64(n)
	case uint64:
		number = float64(n)
	default:
		return 0, false
	}
	return number, !math.IsNaN(number) && !math.IsInf(number, 0)
}

func parseTypedFileField(descriptor extui.FileFieldDescriptor, raw any) (extui.FileFieldValue, bool) {
	value := extui.FileFieldValue{
		State:     extui.FileFieldKnown,
		Kind:      descriptor.Kind,
		Unit:      descriptor.Unit,
		Format:    descriptor.Format,
		Precision: descriptor.Precision,
	}
	switch descriptor.Kind {
	case extui.FileFieldNumber:
		number, ok := numericFieldValue(raw)
		if !ok || number <= 0 {
			return extui.FileFieldValue{}, false
		}
		value.Number = number
	case extui.FileFieldInteger:
		number, ok := numericFieldValue(raw)
		if !ok || number <= 0 || number != math.Trunc(number) {
			return extui.FileFieldValue{}, false
		}
		value.Integer = int64(number)
	case extui.FileFieldText:
		text, ok := raw.(string)
		text = strings.TrimSpace(text)
		if !ok || text == "" {
			return extui.FileFieldValue{}, false
		}
		value.Text = text
	case extui.FileFieldRange:
		fields, ok := raw.(map[string]any)
		if !ok {
			return extui.FileFieldValue{}, false
		}
		minimum, minOK := numericFieldValue(fields["min"])
		maximum, maxOK := numericFieldValue(fields["max"])
		if !minOK || !maxOK || minimum <= 0 || maximum < minimum {
			return extui.FileFieldValue{}, false
		}
		value.Min, value.Max = minimum, maximum
	default:
		return extui.FileFieldValue{}, false
	}
	return value, true
}

func (fp *FileSystemPanel) fileFieldIdentity(entry *FileEntry) (sourceKey, version string) {
	if fp == nil || fp.Vfs == nil || entry == nil || entry.IsDir || entry.Name == ".." {
		return "", ""
	}
	logicalPath := fp.Vfs.Join(fp.Vfs.GetPath(), entry.Name)
	storage := plughost.MediaStorageClass(fp.Vfs.GetCapabilities(), "")
	sourceKey = plughost.MediaSourceKey(fp.Vfs, logicalPath)
	version, _ = plughost.MediaSourceVersion(
		fp.Vfs, entry.VFSItem, storage == vfs.StorageClassLocal,
		fp.catalogRevision, fp.mediaSourceEpoch)
	return sourceKey, version
}

// ApplyFileFieldsUpdate validates a result against the current panel,
// directory-read generation, source identity, and content version. Sorting and
// filtering can change catalog order without invalidating an otherwise current
// result because none of those operations advance mediaSourceEpoch.
func (fp *FileSystemPanel) ApplyFileFieldsUpdate(action map[string]any) bool {
	return fp.ApplyFileFieldsUpdates([]map[string]any{action})
}

// ApplyFileFieldsUpdates applies one thumbnail metadata batch before it sorts,
// filters, groups, or refreshes the presentation. This preserves the cursor
// and viewport with one coherent catalog reconciliation per decoder batch.
func (fp *FileSystemPanel) ApplyFileFieldsUpdates(actions []map[string]any) bool {
	if fp == nil || fp.Vfs == nil {
		return false
	}
	entries := fp.AllEntries()
	updated := false
	for _, action := range actions {
		if semantic.String(action["panelId"]) != vtui.SemanticID(fp) ||
			semantic.Int64(action["generation"]) != fp.mediaSourceEpoch {
			continue
		}
		sourceKey := semantic.String(action["sourceKey"])
		sourceVersion := semantic.String(action["sourceVersion"])
		if sourceKey == "" || sourceVersion == "" {
			continue
		}
		rawFields, _ := action["values"].(map[string]any)
		complete := semantic.Bool(action["complete"])
		descriptors := extui.FileFieldDescriptors()
		values := make(map[string]extui.FileFieldValue, len(rawFields)+len(descriptors))
		for _, descriptor := range descriptors {
			raw, present := rawFields[descriptor.ID]
			if !present {
				if complete {
					values[descriptor.ID] = extui.FileFieldValue{
						State: extui.FileFieldMissing, Kind: descriptor.Kind, Unit: descriptor.Unit,
					}
				}
				continue
			}
			if parsed, ok := parseTypedFileField(descriptor, raw); ok {
				values[descriptor.ID] = parsed
			} else if complete {
				values[descriptor.ID] = extui.FileFieldValue{
					State: extui.FileFieldMissing, Kind: descriptor.Kind, Unit: descriptor.Unit,
				}
			}
		}
		for _, entry := range entries {
			currentKey, currentVersion := fp.fileFieldIdentity(entry)
			if currentKey != sourceKey || currentVersion != sourceVersion {
				continue
			}
			identityChanged := entry.FileFieldsSourceKey != sourceKey ||
				entry.FileFieldsSourceVersion != sourceVersion ||
				entry.FileFieldsGeneration != fp.mediaSourceEpoch
			previous := entry.FileFields
			previousComplete := entry.FileFieldsComplete
			if identityChanged {
				previous = nil
				previousComplete = false
			}
			next := make(map[string]extui.FileFieldValue, len(previous)+len(values))
			for id, value := range previous {
				next[id] = value
			}
			for id, value := range values {
				next[id] = value
			}
			nextComplete := previousComplete || complete
			if identityChanged || nextComplete != entry.FileFieldsComplete ||
				!reflect.DeepEqual(next, entry.FileFields) {
				entry.FileFields = next
				entry.FileFieldsComplete = nextComplete
				entry.FileFieldsSourceKey = sourceKey
				entry.FileFieldsSourceVersion = sourceVersion
				entry.FileFieldsGeneration = fp.mediaSourceEpoch
				updated = true
			}
		}
	}
	if !updated {
		return false
	}
	return fp.reconcileFileFieldResult()
}

func (fp *FileSystemPanel) reconcileFileFieldResult() bool {
	focused := fp.GetRawSelectedName()
	oldDisplay := fp.displayOfEntry(fp.GetCursorIndex())
	topOffset := oldDisplay - fp.Table.TopPos
	oldIndex := fp.GetCursorIndex()
	if fp.FileFieldSort != "" || fp.GroupBy != GroupNone {
		fp.SortEntries()
	} else {
		fp.refilterEntries()
	}
	if focused != "" && fp.isEntryVisible(focused) {
		fp.focusEntryByName(focused)
	} else if len(fp.Entries) > 0 {
		fp.SetCursorIndex(min(oldIndex, len(fp.Entries)-1))
	}
	fp.Table.TopPos = max(0, fp.displayOfEntry(fp.GetCursorIndex())-topOffset)
	fp.Refresh()
	return true
}

func (fp *FileSystemPanel) isEntryVisible(name string) bool {
	for _, entry := range fp.Entries {
		if entry.Name == name {
			return true
		}
	}
	return false
}

func (fp *FileSystemPanel) reconcileStaleFileFields(entries []*FileEntry) {
	if fp == nil || (fp.FileFieldSort == "" && len(fp.FileFieldFilters) == 0 &&
		fp.FileFieldGroup == "") {
		return
	}
	for _, entry := range entries {
		if len(entry.FileFields) == 0 && !entry.FileFieldsComplete {
			continue
		}
		key, version := fp.fileFieldIdentity(entry)
		if key != entry.FileFieldsSourceKey ||
			version != entry.FileFieldsSourceVersion ||
			fp.mediaSourceEpoch != entry.FileFieldsGeneration {
			entry.FileFields = nil
			entry.FileFieldsComplete = false
			entry.FileFieldsSourceKey = ""
			entry.FileFieldsSourceVersion = ""
			entry.FileFieldsGeneration = 0
		}
	}
}

func (fp *FileSystemPanel) SetFileFieldSort(fieldID string) bool {
	if fieldID != "" {
		if _, ok := fileFieldDescriptor(fieldID); !ok {
			return false
		}
	}
	if fp.FileFieldSort == fieldID {
		fp.SortReverse = !fp.SortReverse
	} else {
		fp.FileFieldSort = fieldID
		fp.SortReverse = false
	}
	fp.sortEntriesKeepingCursor()
	return true
}

func (fp *FileSystemPanel) SetFileFieldFilters(filters []FileFieldFilter, any bool) bool {
	for _, filter := range filters {
		if !validFileFieldFilter(filter) {
			return false
		}
	}
	fp.FileFieldFilters = append([]FileFieldFilter(nil), filters...)
	fp.FileFieldFilterAny = any
	fp.reconcileStaleFileFields(fp.AllEntries())
	focused := fp.GetRawSelectedName()
	fp.refilterEntries()
	fp.focusEntryByName(focused)
	fp.markSemanticCatalogMutation()
	fp.Refresh()
	return true
}

// SetFileFieldColumns replaces the visible Details field columns. A zero
// width uses the schema's default width; duplicate and unknown fields are
// rejected so column order is stable across model refreshes.
func (fp *FileSystemPanel) SetFileFieldColumns(columns []FileFieldColumn) bool {
	if fp == nil {
		return false
	}
	seen := make(map[string]bool, len(columns))
	normalized := make([]FileFieldColumn, 0, len(columns))
	for _, column := range columns {
		if _, ok := fileFieldDescriptor(column.FieldID); !ok || seen[column.FieldID] ||
			column.Width < 0 || column.Width > maxGalleryColumnWidth {
			return false
		}
		seen[column.FieldID] = true
		if column.Width == 0 {
			column.Width = 12
		}
		normalized = append(normalized, column)
	}
	if reflect.DeepEqual(fp.FileFieldColumns, normalized) {
		return true
	}
	fp.FileFieldColumns = normalized
	if fp.GalleryColumnWidths == nil {
		fp.GalleryColumnWidths = make(map[string]int)
	}
	for _, column := range normalized {
		fp.GalleryColumnWidths[column.FieldID] = column.Width
	}
	fp.Refresh()
	return true
}

func (fp *FileSystemPanel) sortFileFieldEntries() bool {
	entries := fp.AllEntries()
	if fp.FileFieldSort == "" || len(entries) < 2 {
		return false
	}
	fp.reconcileStaleFileFields(entries)
	descriptor, ok := fileFieldDescriptor(fp.FileFieldSort)
	if !ok {
		return false
	}
	before := make([]*FileEntry, len(entries))
	copy(before, entries)
	sortFieldEntries(entries, descriptor, fp.SortReverse)
	for index := range entries {
		if entries[index] != before[index] {
			return true
		}
	}
	return false
}

func sortFieldEntries(entries []*FileEntry, descriptor extui.FileFieldDescriptor, reverse bool) {
	compareName := comparePanelNames
	sort.SliceStable(entries, func(i, j int) bool {
		left, right := entries[i], entries[j]
		if left.Name == ".." || right.Name == ".." {
			return left.Name == ".." && right.Name != ".."
		}
		if left.IsDir != right.IsDir {
			return left.IsDir
		}
		a := fieldValueForSort(left, descriptor)
		b := fieldValueForSort(right, descriptor)
		stateOrderA, stateOrderB := fieldValueStateOrder(a), fieldValueStateOrder(b)
		if stateOrderA != stateOrderB {
			return stateOrderA < stateOrderB
		}
		if a.State == extui.FileFieldKnown && b.State == extui.FileFieldKnown {
			comparison := compareFileFieldValues(a, b, descriptor)
			if reverse {
				comparison = -comparison
			}
			if comparison != 0 {
				return comparison < 0
			}
		}
		return compareName(left.Name, right.Name) < 0
	})
}

func fieldValueForSort(entry *FileEntry, descriptor extui.FileFieldDescriptor) extui.FileFieldValue {
	if entry == nil {
		return extui.FileFieldValue{State: extui.FileFieldUnread, Kind: descriptor.Kind}
	}
	// Non-image files never enter ZoinGallery's image metadata pass. Their
	// EXIF state is therefore known-missing from the catalog alone, rather
	// than perpetually unread and counted as pending by an EXIF filter.
	if entry.Name != ".." && !entry.IsDir && !media.IsImageFile(entry.Name) {
		return extui.FileFieldValue{State: extui.FileFieldMissing, Kind: descriptor.Kind}
	}
	value, ok := entry.FileFields[descriptor.ID]
	if !ok || entry.FileFieldsSourceKey == "" ||
		entry.FileFieldsSourceVersion == "" {
		return extui.FileFieldValue{State: extui.FileFieldUnread, Kind: descriptor.Kind}
	}
	if value.Kind == "" {
		value.Kind = descriptor.Kind
	}
	value.Unit, value.Format, value.Precision = descriptor.Unit, descriptor.Format, descriptor.Precision
	return value
}

func fieldValueStateOrder(value extui.FileFieldValue) int {
	switch value.State {
	case extui.FileFieldKnown:
		return 0
	case extui.FileFieldMissing:
		return 1
	default:
		return 2
	}
}

func compareFileFieldValues(left, right extui.FileFieldValue, descriptor extui.FileFieldDescriptor) int {
	switch descriptor.Kind {
	case extui.FileFieldNumber:
		if left.Number < right.Number {
			return -1
		}
		if left.Number > right.Number {
			return 1
		}
	case extui.FileFieldInteger:
		if left.Integer < right.Integer {
			return -1
		}
		if left.Integer > right.Integer {
			return 1
		}
	case extui.FileFieldRange:
		if left.Min < right.Min {
			return -1
		}
		if left.Min > right.Min {
			return 1
		}
		if left.Max < right.Max {
			return -1
		}
		if left.Max > right.Max {
			return 1
		}
	case extui.FileFieldText:
		return comparePanelNames(left.Text, right.Text)
	}
	return 0
}

func (fp *FileSystemPanel) fieldFilterResult(entry *FileEntry) fileFieldMatchState {
	if len(fp.FileFieldFilters) == 0 {
		return fileFieldMatchTrue
	}
	unknown := false
	for _, filter := range fp.FileFieldFilters {
		descriptor, ok := fileFieldDescriptor(filter.FieldID)
		if !ok {
			continue
		}
		value := fieldValueForSort(entry, descriptor)
		state := matchFileFieldFilter(value, descriptor, filter)
		switch state {
		case fileFieldMatchUnknown:
			unknown = true
		case fileFieldMatchTrue:
			if fp.FileFieldFilterAny {
				return fileFieldMatchTrue
			}
		case fileFieldMatchFalse:
			if !fp.FileFieldFilterAny {
				return fileFieldMatchFalse
			}
		}
	}
	if unknown {
		return fileFieldMatchUnknown
	}
	if fp.FileFieldFilterAny {
		return fileFieldMatchFalse
	}
	return fileFieldMatchTrue
}

func (fp *FileSystemPanel) FileFieldFilterPendingCount() int {
	if fp == nil || len(fp.FileFieldFilters) == 0 {
		return 0
	}
	count := 0
	for _, entry := range fp.AllEntries() {
		if entry.Name == ".." || entry.IsDir {
			continue
		}
		if fp.fieldFilterResult(entry) == fileFieldMatchUnknown {
			count++
		}
	}
	return count
}

func matchFileFieldFilter(value extui.FileFieldValue, descriptor extui.FileFieldDescriptor, filter FileFieldFilter) fileFieldMatchState {
	if value.State == extui.FileFieldUnread || value.State == "" {
		return fileFieldMatchUnknown
	}
	if filter.Operation == "has" {
		return fileFieldBool(value.State == extui.FileFieldKnown)
	}
	if filter.Operation == "missing" {
		return fileFieldBool(value.State == extui.FileFieldMissing)
	}
	if value.State == extui.FileFieldMissing {
		return fileFieldMatchFalse
	}
	switch descriptor.Kind {
	case extui.FileFieldText:
		expected := strings.TrimSpace(filter.Value)
		if filter.Operation == "eq" {
			return fileFieldBool(strings.EqualFold(value.Text, expected))
		}
		return fileFieldBool(strings.Contains(strings.ToLower(value.Text), strings.ToLower(expected)))
	case extui.FileFieldNumber:
		expected, ok := parseFieldFilterNumber(descriptor, filter.Value)
		if !ok {
			return fileFieldMatchFalse
		}
		return fileFieldBool(compareNumber(value.Number, expected, filter.Operation))
	case extui.FileFieldInteger:
		expected, ok := parseFieldFilterNumber(descriptor, filter.Value)
		if !ok || expected != math.Trunc(expected) {
			return fileFieldMatchFalse
		}
		return fileFieldBool(compareNumber(float64(value.Integer), expected, filter.Operation))
	case extui.FileFieldRange:
		if filter.Operation == "contains" {
			expected, ok := parseFieldFilterNumber(descriptor, filter.Value)
			return fileFieldBool(ok && value.Min <= expected && expected <= value.Max)
		}
		minimum, maximum, ok := parseFieldFilterRange(filter.Value)
		return fileFieldBool(ok && value.Min == minimum && value.Max == maximum)
	}
	return fileFieldMatchFalse
}

func fileFieldBool(value bool) fileFieldMatchState {
	if value {
		return fileFieldMatchTrue
	}
	return fileFieldMatchFalse
}

func compareNumber(left, right float64, operation string) bool {
	switch operation {
	case "eq":
		return left == right
	case "ne":
		return left != right
	case "lt":
		return left < right
	case "le":
		return left <= right
	case "gt":
		return left > right
	case "ge":
		return left >= right
	default:
		return false
	}
}

func parseFieldFilterNumber(descriptor extui.FileFieldDescriptor, input string) (float64, bool) {
	value := strings.TrimSpace(strings.ToLower(input))
	value = strings.TrimSuffix(value, "seconds")
	value = strings.TrimSuffix(value, "second")
	value = strings.TrimSuffix(value, "sec")
	value = strings.TrimSuffix(value, "с")
	value = strings.TrimSuffix(value, "s")
	value = strings.TrimSuffix(value, "мм")
	value = strings.TrimSuffix(value, "mm")
	value = strings.TrimPrefix(value, "f/")
	value = strings.TrimSpace(value)
	if descriptor.ID == "exif.exposure_time" {
		if numerator, denominator, ok := strings.Cut(value, "/"); ok {
			top, topErr := strconv.ParseFloat(strings.TrimSpace(numerator), 64)
			bottom, bottomErr := strconv.ParseFloat(strings.TrimSpace(denominator), 64)
			if topErr != nil || bottomErr != nil || bottom == 0 {
				return 0, false
			}
			number := top / bottom
			return number, number > 0 && !math.IsNaN(number) && !math.IsInf(number, 0)
		}
	}
	number, err := strconv.ParseFloat(value, 64)
	return number, err == nil && number > 0 && !math.IsNaN(number) && !math.IsInf(number, 0)
}

func parseFieldFilterRange(input string) (float64, float64, bool) {
	value := strings.ReplaceAll(strings.TrimSpace(input), "–", "-")
	minimumText, maximumText, ok := strings.Cut(value, "-")
	if !ok {
		return 0, 0, false
	}
	minimum, minErr := strconv.ParseFloat(strings.TrimSpace(trimLensUnit(minimumText)), 64)
	maximum, maxErr := strconv.ParseFloat(strings.TrimSpace(trimLensUnit(maximumText)), 64)
	return minimum, maximum, minErr == nil && maxErr == nil && minimum > 0 && maximum >= minimum
}

func trimLensUnit(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimSuffix(value, "мм")
	return strings.TrimSpace(strings.TrimSuffix(value, "mm"))
}

func validFileFieldFilter(filter FileFieldFilter) bool {
	descriptor, ok := fileFieldDescriptor(filter.FieldID)
	if !ok {
		return false
	}
	operationAllowed := false
	for _, operation := range descriptor.Operations {
		if operation == filter.Operation {
			operationAllowed = true
			break
		}
	}
	if !operationAllowed {
		return false
	}
	if filter.Operation == "has" || filter.Operation == "missing" {
		return true
	}
	if descriptor.Kind == extui.FileFieldText {
		return strings.TrimSpace(filter.Value) != ""
	}
	if descriptor.Kind == extui.FileFieldRange && filter.Operation == "eq" {
		_, _, ok := parseFieldFilterRange(filter.Value)
		return ok
	}
	number, ok := parseFieldFilterNumber(descriptor, filter.Value)
	if !ok {
		return false
	}
	return descriptor.Kind != extui.FileFieldInteger || number == math.Trunc(number)
}

func formatFileField(id string, value extui.FileFieldValue) string {
	descriptor, ok := fileFieldDescriptor(id)
	if !ok {
		return ""
	}
	value.Kind, value.Unit = descriptor.Kind, descriptor.Unit
	value.Format, value.Precision = descriptor.Format, descriptor.Precision
	return value.Display()
}
