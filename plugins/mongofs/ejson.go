package mongofs

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"time"
)

var errEJSON = errors.New("mongofs: bad document")

// parseEJSON reads what toJSON writes: relaxed extended JSON, keeping the order
// of keys, with the $-wrappers ($oid, $date, $numberLong, $numberInt,
// $numberDouble, $binary, $timestamp, $unsupported) turned back into their
// BSON types. A number without a fraction or exponent is an int32 when it fits
// and an int64 otherwise; anything else is a double.
func parseEJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := readValue(dec)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errEJSON, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: text after the document", errEJSON)
	}
	return v, nil
}

func readValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch t := tok.(type) {
	case json.Delim:
		switch t {
		case '{':
			var doc bsonD
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := keyTok.(string)
				val, err := readValue(dec)
				if err != nil {
					return nil, err
				}
				doc = append(doc, bsonE{Key: key, Value: val})
			}
			if _, err := dec.Token(); err != nil { // the closing brace
				return nil, err
			}
			return unwrap(doc)
		case '[':
			items := []any{}
			for dec.More() {
				val, err := readValue(dec)
				if err != nil {
					return nil, err
				}
				items = append(items, val)
			}
			if _, err := dec.Token(); err != nil {
				return nil, err
			}
			return items, nil
		}
		return nil, fmt.Errorf("unexpected %v", t)
	case json.Number:
		return numberValue(t)
	default: // string, bool, nil
		return t, nil
	}
}

func numberValue(n json.Number) (any, error) {
	s := n.String()
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		if i >= math.MinInt32 && i <= math.MaxInt32 {
			return int32(i), nil // #nosec G115 -- range checked
		}
		return i, nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("bad number %q", s)
	}
	return f, nil
}

// unwrap turns a one-key $-wrapper document into its value.
func unwrap(doc bsonD) (any, error) {
	if len(doc) == 0 || len(doc[0].Key) == 0 || doc[0].Key[0] != '$' {
		return doc, nil
	}
	key, val := doc[0].Key, doc[0].Value
	str, _ := val.(string)
	switch key {
	case "$oid":
		raw, err := hex.DecodeString(str)
		if err != nil || len(raw) != 12 {
			return nil, fmt.Errorf("bad $oid %q", str)
		}
		var o objectID
		copy(o[:], raw)
		return o, nil
	case "$date":
		if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
			return t.UTC(), nil
		}
		switch ms := val.(type) { // {"$date": {"$numberLong": "..."}} or a bare number of milliseconds
		case int64:
			return time.UnixMilli(ms).UTC(), nil
		case int32:
			return time.UnixMilli(int64(ms)).UTC(), nil
		}
		return nil, fmt.Errorf("bad $date %v", val)
	case "$numberLong":
		n, err := strconv.ParseInt(str, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad $numberLong %q", str)
		}
		return n, nil
	case "$numberInt":
		n, err := strconv.ParseInt(str, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("bad $numberInt %q", str)
		}
		return int32(n), nil
	case "$numberDouble":
		switch str {
		case "NaN":
			return math.NaN(), nil
		case "Infinity":
			return math.Inf(1), nil
		case "-Infinity":
			return math.Inf(-1), nil
		}
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return nil, fmt.Errorf("bad $numberDouble %q", str)
		}
		return f, nil
	case "$binary":
		inner, _ := val.(bsonD)
		data, err := base64.StdEncoding.DecodeString(fmt.Sprint(inner.get("base64")))
		sub, subErr := strconv.ParseUint(fmt.Sprint(inner.get("subType")), 16, 8)
		if err != nil || subErr != nil {
			return nil, errors.New("bad $binary")
		}
		return bsonBinary{Subtype: byte(sub), Data: data}, nil
	case "$timestamp":
		inner, _ := val.(bsonD)
		t, tErr := strconv.ParseUint(fmt.Sprint(inner.get("t")), 10, 32)
		i, iErr := strconv.ParseUint(fmt.Sprint(inner.get("i")), 10, 32)
		if tErr != nil || iErr != nil {
			return nil, errors.New("bad $timestamp")
		}
		return bsonTimestamp{T: uint32(t), I: uint32(i)}, nil
	case "$unsupported":
		inner, _ := val.(bsonD)
		typ, tErr := strconv.ParseUint(fmt.Sprint(inner.get("type")), 10, 8)
		data, hErr := hex.DecodeString(fmt.Sprint(inner.get("hex")))
		if tErr != nil || hErr != nil || byte(typ) != tDecimal || len(data) != 16 {
			return nil, errors.New("a value of a type this panel cannot store")
		}
		return bsonRaw{Type: byte(typ), Data: data}, nil
	}
	return doc, nil // a real key that starts with $ ($set and the like)
}
