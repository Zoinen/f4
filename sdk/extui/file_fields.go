package extui

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
)

// FileFieldKind describes the comparison and wire representation of a
// value that augments a file entry without changing the VFS contract.
type FileFieldKind string

const (
	FileFieldNumber  FileFieldKind = "number"
	FileFieldInteger FileFieldKind = "integer"
	FileFieldText    FileFieldKind = "text"
	FileFieldRange   FileFieldKind = "range"
)

// FileFieldState keeps an unread value distinct from a completed read with no
// value. That distinction is needed by asynchronous filters and sort order.
type FileFieldState string

const (
	FileFieldUnread  FileFieldState = "unread"
	FileFieldMissing FileFieldState = "missing"
	FileFieldKnown   FileFieldState = "known"
)

// FileFieldDescriptor is the shared field schema sent to native GUI clients.
// Operations are stable protocol identifiers such as "gt" and "contains".
type FileFieldDescriptor struct {
	ID         string
	Title      string
	Kind       FileFieldKind
	Unit       string
	Format     string
	Precision  int
	Operations []string
}

// FileFieldValue is a typed value for one descriptor. Exactly one of Number,
// Integer, Text, or Min/Max is meaningful according to Kind.
type FileFieldValue struct {
	State     FileFieldState
	Kind      FileFieldKind
	Unit      string
	Format    string
	Precision int
	Number    float64
	Integer   int64
	Text      string
	Min       float64
	Max       float64
}

var fileFieldRegistry = struct {
	sync.RWMutex
	byID  map[string]FileFieldDescriptor
	order []string
}{byID: make(map[string]FileFieldDescriptor)}

// RegisterFileFieldDescriptor adds one stable typed field to the shared
// registry. Re-registering the identical descriptor is harmless; changing an
// existing field definition is rejected because saved panel state and caches
// use these IDs as protocol keys.
func RegisterFileFieldDescriptor(descriptor FileFieldDescriptor) error {
	descriptor.ID = strings.TrimSpace(descriptor.ID)
	descriptor.Title = strings.TrimSpace(descriptor.Title)
	if descriptor.ID == "" || strings.ContainsAny(descriptor.ID, " \t\r\n") ||
		descriptor.Title == "" || descriptor.Precision < 0 ||
		descriptor.Precision > 12 {
		return errors.New("invalid file-field descriptor")
	}
	switch descriptor.Kind {
	case FileFieldNumber, FileFieldInteger, FileFieldText, FileFieldRange:
	default:
		return errors.New("unsupported file-field kind")
	}
	if descriptor.Format == "" {
		switch descriptor.Kind {
		case FileFieldNumber:
			descriptor.Format = "number"
		case FileFieldInteger:
			descriptor.Format = "integer"
		case FileFieldText:
			descriptor.Format = "text"
		case FileFieldRange:
			descriptor.Format = "range"
		}
	}
	descriptor.Operations = append([]string(nil), descriptor.Operations...)
	if len(descriptor.Operations) == 0 {
		return errors.New("file-field descriptor must declare operations")
	}
	allowed := map[string]bool{"has": true, "missing": true}
	switch descriptor.Kind {
	case FileFieldNumber, FileFieldInteger:
		for _, operation := range []string{"eq", "ne", "lt", "le", "gt", "ge"} {
			allowed[operation] = true
		}
	case FileFieldText:
		allowed["eq"] = true
		allowed["contains"] = true
	case FileFieldRange:
		allowed["eq"] = true
		allowed["contains"] = true
	}
	seenOperations := make(map[string]bool, len(descriptor.Operations))
	for _, operation := range descriptor.Operations {
		if !allowed[operation] || seenOperations[operation] {
			return errors.New("invalid file-field operation")
		}
		seenOperations[operation] = true
	}
	fileFieldRegistry.Lock()
	defer fileFieldRegistry.Unlock()
	if current, ok := fileFieldRegistry.byID[descriptor.ID]; ok {
		if current.Kind == descriptor.Kind && current.Title == descriptor.Title &&
			current.Unit == descriptor.Unit && current.Format == descriptor.Format &&
			current.Precision == descriptor.Precision &&
			strings.Join(current.Operations, "\x00") == strings.Join(descriptor.Operations, "\x00") {
			return nil
		}
		return errors.New("file-field descriptor ID already registered")
	}
	fileFieldRegistry.byID[descriptor.ID] = descriptor
	fileFieldRegistry.order = append(fileFieldRegistry.order, descriptor.ID)
	return nil
}

// FileFieldDescriptors returns all registered descriptions in registration
// order. Callers own the returned slice and each descriptor's operation list.
func FileFieldDescriptors() []FileFieldDescriptor {
	fileFieldRegistry.RLock()
	defer fileFieldRegistry.RUnlock()
	fields := make([]FileFieldDescriptor, 0, len(fileFieldRegistry.order))
	for _, id := range fileFieldRegistry.order {
		descriptor := fileFieldRegistry.byID[id]
		descriptor.Operations = append([]string(nil), descriptor.Operations...)
		fields = append(fields, descriptor)
	}
	return fields
}

// FileFieldDescriptorByID returns a defensive copy of a registered schema.
func FileFieldDescriptorByID(id string) (FileFieldDescriptor, bool) {
	fileFieldRegistry.RLock()
	defer fileFieldRegistry.RUnlock()
	descriptor, ok := fileFieldRegistry.byID[id]
	if ok {
		descriptor.Operations = append([]string(nil), descriptor.Operations...)
	}
	return descriptor, ok
}

// ExifFileFieldDescriptors returns the EXIF subset in registry order.
func ExifFileFieldDescriptors() []FileFieldDescriptor {
	all := FileFieldDescriptors()
	fields := make([]FileFieldDescriptor, 0, len(all))
	for _, descriptor := range all {
		if strings.HasPrefix(descriptor.ID, "exif.") {
			fields = append(fields, descriptor)
		}
	}
	return fields
}

func init() {
	const numericOps = "eq ne lt le gt ge has missing"
	const textOps = "eq contains has missing"
	const rangeOps = "eq contains has missing"
	for _, descriptor := range []FileFieldDescriptor{
		{ID: "exif.exposure_time", Title: "Shutter speed", Kind: FileFieldNumber, Unit: "s", Format: "exposure", Precision: 3, Operations: strings.Fields(numericOps)},
		{ID: "exif.iso", Title: "ISO", Kind: FileFieldInteger, Format: "integer", Operations: strings.Fields(numericOps)},
		{ID: "exif.f_number", Title: "Aperture", Kind: FileFieldNumber, Unit: "f", Format: "aperture", Precision: 1, Operations: strings.Fields(numericOps)},
		{ID: "exif.focal_length_35mm", Title: "Focal length (35 mm)", Kind: FileFieldNumber, Unit: "mm", Format: "quantity", Precision: 0, Operations: strings.Fields(numericOps)},
		{ID: "exif.lens_focal_range", Title: "Lens focal range", Kind: FileFieldRange, Unit: "mm", Format: "range", Precision: 0, Operations: strings.Fields(rangeOps)},
		{ID: "exif.camera_model", Title: "Camera model", Kind: FileFieldText, Format: "text", Operations: strings.Fields(textOps)},
		{ID: "exif.lens_model", Title: "Lens model", Kind: FileFieldText, Format: "text", Operations: strings.Fields(textOps)},
	} {
		if err := RegisterFileFieldDescriptor(descriptor); err != nil {
			panic(err)
		}
	}
}

// ToMap serializes a descriptor with stable lower-camel-case field names.
func (f FileFieldDescriptor) ToMap() M {
	operations := make([]string, len(f.Operations))
	copy(operations, f.Operations)
	return M{"id": f.ID, "title": f.Title, "kind": string(f.Kind), "unit": f.Unit, "format": f.Format, "precision": f.Precision, "operations": operations}
}

// ToMap serializes a file field value without inventing default values.
func (v FileFieldValue) ToMap() M {
	out := M{"state": string(v.State), "kind": string(v.Kind)}
	if v.State != FileFieldKnown {
		return out
	}
	if v.Unit != "" {
		out["unit"] = v.Unit
	}
	if v.Format != "" {
		out["format"] = v.Format
	}
	if v.Precision > 0 {
		out["precision"] = v.Precision
	}
	switch v.Kind {
	case FileFieldNumber:
		out["number"] = v.Number
	case FileFieldInteger:
		out["integer"] = v.Integer
	case FileFieldText:
		out["text"] = v.Text
	case FileFieldRange:
		out["min"] = v.Min
		out["max"] = v.Max
	}
	return out
}

// Display formats known values for columns and future icon captions.
func (v FileFieldValue) Display() string {
	if v.State != FileFieldKnown {
		return ""
	}
	switch v.Kind {
	case FileFieldNumber:
		if v.Number <= 0 || math.IsNaN(v.Number) || math.IsInf(v.Number, 0) {
			return ""
		}
		switch {
		case v.Format == "exposure" || v.Unit == "s":
			return formatExposure(v.Number)
		case v.Format == "aperture" || v.Unit == "f":
			return "f/" + formatDecimal(v.Number, v.PrecisionOr(1))
		case v.Format == "quantity" || v.Unit == "mm":
			return withUnit(formatDecimal(v.Number, v.Precision), v.Unit)
		default:
			return strconv.FormatFloat(v.Number, 'f', v.Precision, 64)
		}
	case FileFieldInteger:
		if v.Integer <= 0 {
			return ""
		}
		return strconv.FormatInt(v.Integer, 10)
	case FileFieldText:
		return strings.TrimSpace(v.Text)
	case FileFieldRange:
		if v.Min <= 0 || v.Max < v.Min || math.IsNaN(v.Min) ||
			math.IsNaN(v.Max) || math.IsInf(v.Min, 0) || math.IsInf(v.Max, 0) {
			return ""
		}
		unit := v.Unit
		if unit == "" {
			unit = "mm"
		}
		if v.Min == v.Max {
			return withUnit(formatDecimal(v.Min, v.Precision), unit)
		}
		return withUnit(formatDecimal(v.Min, v.Precision)+"–"+formatDecimal(v.Max, v.Precision), unit)
	default:
		return ""
	}
}

func (v FileFieldValue) PrecisionOr(fallback int) int {
	if v.Precision > 0 {
		return v.Precision
	}
	return fallback
}

func withUnit(value, unit string) string {
	if value == "" {
		return ""
	}
	if unit == "mm" {
		return value + " мм"
	}
	if unit == "s" {
		return value + " с"
	}
	if unit == "" {
		return value
	}
	return value + " " + unit
}

func formatDecimal(value float64, precision int) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	return strconv.FormatFloat(value, 'f', precision, 64)
}

func formatExposure(seconds float64) string {
	if seconds <= 0 || math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return ""
	}
	if seconds < 1 {
		denominator := math.Round(1 / seconds)
		if denominator >= 1 && denominator <= 1_000_000 && math.Abs((1/seconds-denominator)/denominator) < 0.02 {
			return fmt.Sprintf("1/%d с", int64(denominator))
		}
	}
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(seconds, 'f', 3, 64), "0"), ".") + " с"
}

// FileFieldUpdateModel is one asynchronous result from the existing thumbnail
// metadata pipeline. Generation is the observed directory content generation,
// so a later sort or filter does not invalidate a result for the same file.
type FileFieldUpdateModel struct {
	PanelID       string
	Generation    int64
	SourceKey     string
	SourceVersion string
	Complete      bool
	Values        map[string]FileFieldValue
}

func (u FileFieldUpdateModel) ToMap() M {
	return M{
		"type":          "panel_file_fields_update",
		"panelId":       u.PanelID,
		"generation":    u.Generation,
		"sourceKey":     u.SourceKey,
		"sourceVersion": u.SourceVersion,
		"complete":      u.Complete,
		"values":        fileFieldValuesToMap(u.Values),
	}
}

func fileFieldValuesToMap(values map[string]FileFieldValue) M {
	out := make(M, len(values))
	for id, value := range values {
		out[id] = value.ToMap()
	}
	return out
}
