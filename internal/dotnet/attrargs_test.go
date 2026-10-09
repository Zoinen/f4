package dotnet

import (
	"strings"
	"testing"
)

func TestAttributeTextDecodesArguments(t *testing.T) {
	str := func(s string) []byte { return append([]byte{byte(len(s) & 0x7F)}, s...) } // #nosec G115 -- short test strings
	cat := func(parts ...[]byte) []byte {
		out := []byte{0x01, 0x00}
		for _, p := range parts {
			out = append(out, p...)
		}
		return out
	}
	cases := []struct {
		name   string
		params []attrType
		blob   []byte
		want   string
	}{
		{"string and bool", []attrType{{code: 0x0e}, {code: 0x02}}, cat(str("msg"), []byte{1}, []byte{0, 0}), `"msg", true`},
		{"no arguments", nil, cat([]byte{0, 0}), ""},
		{"a null string", []attrType{{code: 0x0e}}, cat([]byte{0xFF}, []byte{0, 0}), "null"},
		{"numbers", []attrType{{code: 0x08}, {code: 0x05}, {code: 0x04}, {code: 0x06}, {code: 0x07}, {code: 0x09}},
			cat([]byte{0xFE, 0xFF, 0xFF, 0xFF}, []byte{200}, []byte{0xFF}, []byte{0xFE, 0xFF}, []byte{0x2C, 0x01}, []byte{1, 0, 0, 0}), "-2, 200, -1, -2, 300, 1"},
		{"wide numbers", []attrType{{code: 0x0a}, {code: 0x0b}, {code: 0x0c}, {code: 0x0d}},
			cat([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}, []byte{2, 0, 0, 0, 0, 0, 0, 0}, []byte{0, 0, 0xC0, 0x3F}, []byte{0, 0, 0, 0, 0, 0, 0xF8, 0x3F}), "-1, 2, 1.5, 1.5"},
		{"a char", []attrType{{code: 0x03}}, cat([]byte{'x', 0}), `'x'`},
		{"System.Type", []attrType{{code: attrTypeObj}}, cat(str("System.Int32")), "typeof(System.Int32)"},
		{"an array", []attrType{{code: 0x1d, elem: &attrType{code: 0x08}}}, cat([]byte{2, 0, 0, 0}, []byte{1, 0, 0, 0}, []byte{2, 0, 0, 0}), "{1, 2}"},
		{"a null array", []attrType{{code: 0x1d, elem: &attrType{code: 0x08}}}, cat([]byte{0xFF, 0xFF, 0xFF, 0xFF}), "null"},
		{"a boxed int", []attrType{{code: 0x1c}}, cat([]byte{0x08}, []byte{7, 0, 0, 0}), "7"},
		{"a boxed Type", []attrType{{code: 0x1c}}, cat([]byte{0x50}, str("Foo")), "typeof(Foo)"},
		{"a boxed array", []attrType{{code: 0x1c}}, cat([]byte{0x1d, 0x05}, []byte{1, 0, 0, 0}, []byte{9}), "{9}"},
		{"an enum", []attrType{{code: attrEnum, enum: "My.Color", wide: 0x08}}, cat([]byte{2, 0, 0, 0}), "My.Color(2)"},
		{"named arguments", nil, cat([]byte{2, 0}, []byte{0x54, 0x0e}, str("Name"), str("x"), []byte{0x53, 0x02}, str("On"), []byte{1}), `Name = "x", On = true`},
		{"a named Type and array", nil, cat([]byte{2, 0}, []byte{0x54, 0x50}, str("T"), str("A"), []byte{0x53, 0x1d, 0x05}, str("L"), []byte{1, 0, 0, 0}, []byte{4}), "T = typeof(A), L = {4}"},
		{"a long string is cut", []attrType{{code: 0x0e}}, cat([]byte{0x81, 0x2C}, []byte(strings.Repeat("a", 300))), `"` + strings.Repeat("a", maxAttrText-1) + "..."},
	}
	for _, c := range cases {
		if got := attributeText(c.params, c.blob); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}

	bad := []struct {
		name   string
		params []attrType
		blob   []byte
	}{
		{"no prolog", nil, []byte{0, 0, 0, 0}},
		{"a truncated string", []attrType{{code: 0x0e}}, []byte{1, 0, 5, 'a'}},
		{"a truncated number", []attrType{{code: 0x08}}, []byte{1, 0, 1}},
		{"a truncated char", []attrType{{code: 0x03}}, []byte{1, 0, 1}},
		{"an unknown type", []attrType{{code: 0x55}}, []byte{1, 0, 1}},
		{"a boxed enum", []attrType{{code: 0x1c}}, []byte{1, 0, 0x55, 0}},
		{"a nested boxed value", []attrType{{code: 0x1c}}, []byte{1, 0, 0x51, 0}},
		{"an array without an element type", []attrType{{code: 0x1d}}, []byte{1, 0, 1, 0, 0, 0}},
		{"an array too long", []attrType{{code: 0x1d, elem: &attrType{code: 0x05}}}, []byte{1, 0, 0xFF, 0, 0, 0}},
		{"a truncated array", []attrType{{code: 0x1d, elem: &attrType{code: 0x05}}}, []byte{1, 0, 2, 0, 0, 0, 1}},
		{"a truncated named count", nil, []byte{1, 0, 1}},
		{"too many named arguments", nil, []byte{1, 0, 0xFF, 0}},
		{"a bad named kind", nil, []byte{1, 0, 1, 0, 0x99, 0x08}},
		{"a named enum", nil, []byte{1, 0, 1, 0, 0x53, 0x55}},
		{"a truncated named argument", nil, []byte{1, 0, 1, 0, 0x53, 0x08, 1, 'N'}},
		{"a named value that is cut", nil, []byte{1, 0, 1, 0, 0x53, 0x08, 1, 'N', 1}},
		{"a long string length", []attrType{{code: 0x0e}}, []byte{1, 0, 0xC0, 0xFF, 0xFF, 0xFF}},
		{"an unreadable string length", []attrType{{code: 0x0e}}, []byte{1, 0, 0xF0}},
		{"a truncated null-check string", []attrType{{code: attrTypeObj}}, []byte{1, 0}},
	}
	for _, c := range bad {
		if got := attributeText(c.params, c.blob); got != "" {
			t.Errorf("%s: got %q, want none", c.name, got)
		}
	}
	// A nesting deeper than allowed is refused.
	deep := attrType{code: 0x1d, elem: &attrType{code: 0x1d, elem: &attrType{code: 0x1d, elem: &attrType{code: 0x1d, elem: &attrType{code: 0x1d, elem: &attrType{code: 0x05}}}}}}
	if got := attributeText([]attrType{deep}, []byte{1, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 1}); got != "" {
		t.Errorf("a too deeply nested array gave %q", got)
	}
}

func TestCtorParams(t *testing.T) {
	tab := &tables{}
	params, ok := tab.ctorParams([]byte{0x20, 0x02, 0x01, 0x0e, 0x02})
	if !ok || len(params) != 2 || params[0].code != 0x0e || params[1].code != 0x02 {
		t.Fatalf("ctorParams = %+v, %v", params, ok)
	}
	if params, ok := tab.ctorParams([]byte{0x20, 0x01, 0x01, 0x1d, 0x08}); !ok || params[0].code != 0x1d || params[0].elem.code != 0x08 {
		t.Fatalf("array parameter = %+v, %v", params, ok)
	}
	for name, sig := range map[string][]byte{
		"an empty signature":   nil,
		"too many parameters":  {0x20, 0x7F, 0x01},
		"a truncated list":     {0x20, 0x02, 0x01, 0x0e},
		"an unsupported type":  {0x20, 0x01, 0x01, 0x55},
		"a class parameter":    {0x20, 0x01, 0x01, 0x12, 0x00},
		"a value type":         {0x20, 0x01, 0x01, 0x11, 0x00},
		"a nested array chain": {0x20, 0x01, 0x01, 0x1d, 0x1d, 0x1d, 0x1d, 0x1d, 0x1d, 0x08},
	} {
		if _, ok := (&tables{}).ctorParams(sig); ok {
			t.Errorf("ctorParams accepted %s", name)
		}
	}
	// Without metadata a class or value type parameter cannot be resolved.
	if _, ok := (&sigReader{data: []byte{0x12, 0x00}}).argType(); ok {
		t.Error("a class type was accepted without tables")
	}
}
