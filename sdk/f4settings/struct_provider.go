package f4settings

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// StructProvider adapts scalar exported struct fields. Save must persist the
// candidate before publishing it and enforce any store-specific concurrency rule.
type StructProvider[T any] struct {
	Definition Catalog
	Read       func() (T, error)
	Save       func(before, after T) error
	Validate   func(T) error
}

func (p *StructProvider[T]) Catalog() Catalog { return p.Definition }
func (p *StructProvider[T]) Begin(context.Context) (*Draft, error) {
	initial, err := p.Read()
	if err != nil {
		return nil, err
	}
	values := map[string]string{}
	for _, f := range p.Definition.Fields {
		values[f.ID] = StructValue(initial, f.ID)
	}
	d := NewDraft(values, nil)
	candidate := func(d *Draft) (T, T, error) {
		before, err := p.Read()
		if err != nil {
			return before, before, err
		}
		after := before
		for _, f := range p.Definition.Fields {
			if !d.Dirty(f.ID) {
				continue
			}
			if StructValue(before, f.ID) != d.Baseline[f.ID] && StructValue(before, f.ID) != d.Values[f.ID] {
				return before, after, fmt.Errorf("%s changed outside Settings Center", f.Label.English)
			}
			if err := SetStructValue(&after, f.ID, d.Values[f.ID]); err != nil {
				return before, after, err
			}
		}
		if p.Validate != nil {
			err = p.Validate(after)
		}
		return before, after, err
	}
	d.ValidateFunc = func(d *Draft) map[string]error {
		_, _, err := candidate(d)
		if err != nil {
			return map[string]error{p.Definition.ID: err}
		}
		return nil
	}
	d.CommitFunc = func(ctx context.Context, d *Draft) Result {
		before, after, err := candidate(d)
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = p.Save(before, after)
		}
		if err != nil {
			return Result{Errors: map[string]error{p.Definition.ID: err}}
		}
		result := Result{Applied: d.Changed(), Values: map[string]string{}}
		if saved, readErr := p.Read(); readErr == nil {
			for _, id := range result.Applied {
				result.Values[id] = StructValue(saved, id)
			}
		}
		return result
	}
	return d, nil
}
func structField(value reflect.Value, id string) reflect.Value {
	if value.Kind() == reflect.Pointer {
		value = value.Elem()
	}
	parts := strings.Split(id, ".")
	return value.FieldByName(parts[len(parts)-1])
}
func StructValue(value any, id string) string {
	v := structField(reflect.ValueOf(value), id)
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10)
	case reflect.String:
		return v.String()
	}
	return ""
}
func SetStructValue(target any, id, value string) error {
	v := structField(reflect.ValueOf(target), id)
	if !v.IsValid() || !v.CanSet() {
		return fmt.Errorf("invalid setting field %s", id)
	}
	switch v.Kind() {
	case reflect.Bool:
		n, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		v.SetBool(n)
	case reflect.Int, reflect.Int64:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.String:
		v.SetString(value)
	default:
		return fmt.Errorf("unsupported field %s", id)
	}
	return nil
}

func Scalar(id, category, group, label, description string, kind Kind) Field {
	return Field{ID: id, Category: category, Group: group, Label: Text{English: label}, Description: Text{English: description}, Kind: kind, Timing: "Apply"}
}
func Choices(values ...string) []Choice {
	var result []Choice
	for _, value := range values {
		parts := strings.SplitN(value, ":", 2)
		label := parts[0]
		if len(parts) > 1 {
			label = parts[1]
		}
		result = append(result, Choice{Value: parts[0], Label: Text{English: label}})
	}
	return result
}
