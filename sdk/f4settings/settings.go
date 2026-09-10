// Package f4settings describes editable preferences without depending on a UI
// toolkit. Frontends and plugins share descriptors, drafts and search semantics.
package f4settings

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
)

// Error retains a provider's English diagnostic while allowing the frontend to
// localize its format without translating record names or other arguments.
func Error(format string, args ...any) error {
	return &localizedError{format: format, args: args, original: fmt.Errorf(format, args...)}
}

type localizedError struct {
	format   string
	args     []any
	original error
}

func (e *localizedError) Error() string { return e.original.Error() }
func (e *localizedError) Unwrap() error { return e.original }
func (e *localizedError) Localized(language string, lookup func(string) string) string {
	format := (Text{English: e.format}).Resolve(language, lookup)
	return fmt.Errorf(format, e.args...).Error()
}

type Kind string

const (
	Chord      Kind = "chord"
	Boolean    Kind = "boolean"
	Integer    Kind = "integer"
	String     Kind = "string"
	Secret     Kind = "secret"
	Path       Kind = "path"
	ChoiceKind Kind = "choice"
	Color      Kind = "color"
	Multiline  Kind = "multiline"
)

type Text struct {
	English, Key string
	Translations map[string]string
	Args         []any
	// Literal marks user data and external identifiers that must not be translated.
	Literal bool
}

// ResourceKey gives unkeyed provider text a stable optional host-resource key.
// Changing the English text invalidates the old translation automatically.
func ResourceKey(english string) string {
	return fmt.Sprintf("SettingsCenter.Text.%x", sha256.Sum256([]byte(english)))
}

func (t Text) Resolve(language string, lookup func(string) string) string {
	if t.Literal {
		return t.English
	}
	format := func(s string) string {
		if len(t.Args) > 0 {
			return fmt.Sprintf(s, t.Args...)
		}
		return s
	}
	if s := t.Translations[language]; s != "" {
		return format(s)
	}
	if t.Key != "" && lookup != nil {
		if s := lookup(t.Key); s != "" && !strings.HasPrefix(s, "{") {
			return format(s)
		}
	}
	if t.English != "" && lookup != nil {
		key := ResourceKey(t.English)
		if s := lookup(key); s != "" && s != key && !strings.HasPrefix(s, "{") {
			return format(s)
		}
	}
	return format(t.English)
}

type Category struct {
	ID    string
	Label Text
}
type Group struct {
	ID, Category       string
	Label, Description Text
}
type Choice struct {
	Value string
	Label Text
	// Description explains the choice; empty uses the field's explanation.
	Description Text
}
type Field struct {
	ID, Category, Group string
	Label, Description  Text
	Kind                Kind
	// InputWidth suggests a compact single-line editor for short values.
	// It is a presentation hint in characters, not a validation or length limit.
	InputWidth int
	Choices    []Choice
	// ChoicePresentation optionally requests "radio" or "dropdown". Empty lets
	// the frontend expose small fixed lists as radios and longer lists as dropdowns.
	ChoicePresentation string
	// AllowCustom permits a choice field to accept a value outside its list.
	AllowCustom bool
	Aliases     []string
	Timing      string
	Unavailable string
	Default     string
	// Enabled returns an empty string when editable, otherwise a reason.
	Enabled func(map[string]string) string
}
type Record struct {
	ID, Revision string
	Values       map[string]string
}
type Collection struct {
	ID, Category, Group string
	Label, Description  Text
	Fields              []Field
	NameField           string
	Ordered             bool
	Fixed               bool
	Actions             []RecordCommand
}
type RecordCommand struct {
	RequiresApplied    bool
	ID                 string
	Label, Description Text
	Run                func(context.Context, Record) (map[string]string, error)
}

type Command struct {
	Requires            []string
	ID, Category, Group string
	Label, Description  Text
	Run                 func(context.Context) error
	Background          bool
}
type Catalog struct {
	ID          string
	Categories  []Category
	Groups      []Group
	Fields      []Field
	Collections []Collection
	Commands    []Command
	Background  bool
}
type Provider interface {
	Catalog() Catalog
	Begin(context.Context) (*Draft, error)
}

// Result acknowledges only successfully committed identities. A collection ID
// acknowledges its complete list; individual record IDs acknowledge partial saves.
type Result struct {
	Values    map[string]string
	Applied   []string
	Deleted   []string
	Records   map[string]Record
	Revisions map[string]string
	Errors    map[string]error
}
type Draft struct {
	Values          map[string]string
	Records         map[string][]Record
	Baseline        map[string]string
	BaselineRecords map[string][]Record
	ValidateFunc    func(*Draft) map[string]error
	CommitFunc      func(context.Context, *Draft) Result
	PreviewFunc     func(*Draft) error
	CloseFunc       func()
}

func NewDraft(values map[string]string, records map[string][]Record) *Draft {
	return &Draft{Values: cloneValues(values), Baseline: cloneValues(values), Records: cloneRecords(records), BaselineRecords: cloneRecords(records)}
}
func cloneValues(m map[string]string) map[string]string {
	n := make(map[string]string, len(m))
	for k, v := range m {
		n[k] = v
	}
	return n
}
func cloneRecords(m map[string][]Record) map[string][]Record {
	n := make(map[string][]Record, len(m))
	for k, rows := range m {
		n[k] = nil
		if rows != nil {
			n[k] = make([]Record, 0, len(rows))
		}
		for _, r := range rows {
			r.Values = cloneValues(r.Values)
			n[k] = append(n[k], r)
		}
	}
	return n
}
func (d *Draft) Dirty(id string) bool {
	if rows, ok := d.Records[id]; ok {
		return !reflect.DeepEqual(rows, d.BaselineRecords[id])
	}
	return d.Values[id] != d.Baseline[id]
}
func (d *Draft) Changed() []string {
	var ids []string
	for id := range d.Values {
		if d.Dirty(id) {
			ids = append(ids, id)
		}
	}
	for id := range d.Records {
		if d.Dirty(id) {
			ids = append(ids, id)
		}
	}
	return ids
}
func (d *Draft) Validate() map[string]error {
	if d.ValidateFunc != nil {
		return d.ValidateFunc(d)
	}
	return nil
}
func (d *Draft) Commit(ctx context.Context) Result {
	if d.CommitFunc == nil {
		return Result{Errors: map[string]error{"provider": fmt.Errorf("provider is read-only")}}
	}
	r := d.CommitFunc(ctx, d)
	d.Accept(r)
	return r
}

// Accept runs on the frontend thread after a background commit completes.
func (d *Draft) Accept(r Result) {
	for id, value := range r.Values {
		d.Values[id] = value
	}
	for cid, rows := range d.Records {
		for i, row := range rows {
			if updated, ok := r.Records[row.ID]; ok {
				d.Records[cid][i] = updated
			}
		}
	}
	for _, id := range r.Deleted {
		for cid, rows := range d.BaselineRecords {
			for i, row := range rows {
				if row.ID == id {
					d.BaselineRecords[cid] = append(rows[:i], rows[i+1:]...)
					break
				}
			}
		}
	}
	for _, id := range r.Applied {
		if rows, ok := d.Records[id]; ok {
			d.BaselineRecords[id] = cloneRecords(map[string][]Record{id: rows})[id]
			continue
		}
		if v, ok := d.Values[id]; ok {
			d.Baseline[id] = v
			continue
		}
		for cid, rows := range d.Records {
			for i, row := range rows {
				if row.ID != id {
					continue
				}
				if rev := r.Revisions[id]; rev != "" {
					row.Revision = rev
					d.Records[cid][i] = row
				}
				row.Values = cloneValues(row.Values)
				found := false
				for j, old := range d.BaselineRecords[cid] {
					if old.ID == id {
						d.BaselineRecords[cid][j] = row
						found = true
						break
					}
				}
				if !found {
					d.BaselineRecords[cid] = append(d.BaselineRecords[cid], row)
				}
			}
		}
	}
}
func (d *Draft) Close() {
	if d.CloseFunc != nil {
		d.CloseFunc()
	}
	for k := range d.Values {
		delete(d.Values, k)
	}
	for k := range d.Records {
		delete(d.Records, k)
	}
	clear(d.Baseline)
	clear(d.BaselineRecords)
	d.CloseFunc = nil
	d.CommitFunc = nil
	d.ValidateFunc = nil
	d.PreviewFunc = nil
}

// Matches deliberately does not inspect values: secrets and command contents
// must never become a searchable side channel. Every query token must match.
func Matches(query string, f Field, category string, language string, lookup func(string) string) bool {
	parts := []string{f.Label.Resolve(language, lookup), f.Description.Resolve(language, lookup), f.Label.English, f.Description.English, category, f.Group}
	parts = append(parts, f.Aliases...)
	for _, c := range f.Choices {
		parts = append(parts, c.Label.Resolve(language, lookup), c.Label.English, c.Description.Resolve(language, lookup), c.Description.English)
	}
	haystack := strings.ToLower(strings.Join(parts, " "))
	for _, token := range strings.Fields(strings.ToLower(query)) {
		if !strings.Contains(haystack, token) {
			return false
		}
	}
	return true
}
func ValidateCatalog(c Catalog) error {
	if c.ID == "" {
		return fmt.Errorf("settings provider has no ID")
	}
	seen := map[string]bool{}
	categories := map[string]bool{}
	for _, cat := range c.Categories {
		if cat.ID == "" || cat.Label.English == "" {
			return fmt.Errorf("invalid category")
		}
		categories[cat.ID] = true
	}
	check := func(f Field) error {
		if f.ID == "" || seen[f.ID] {
			return fmt.Errorf("duplicate or empty setting ID %q", f.ID)
		}
		seen[f.ID] = true
		if f.Label.English == "" || f.Description.English == "" {
			return fmt.Errorf("setting %s needs label and description", f.ID)
		}
		return nil
	}
	for _, f := range c.Fields {
		if !categories[f.Category] {
			return fmt.Errorf("unknown category %s", f.Category)
		}
		if err := check(f); err != nil {
			return err
		}
	}
	for _, col := range c.Collections {
		if !categories[col.Category] {
			return fmt.Errorf("unknown category %s", col.Category)
		}
		if err := check(Field{ID: col.ID, Label: col.Label, Description: col.Description}); err != nil {
			return err
		}
		for _, f := range col.Fields {
			if err := check(f); err != nil {
				return err
			}
		}
	}
	return nil
}
