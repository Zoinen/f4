package mongofs

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// A small BSON codec: enough to send the handful of commands the panel uses
// and to read whatever a server answers with. Documents are ordered, because a
// command is recognised by its first key.

// bsonE is one key/value pair of a document.
type bsonE struct {
	Key   string
	Value any
}

// bsonD is an ordered document.
type bsonD []bsonE

// get returns the value of key, or nil.
func (d bsonD) get(key string) any {
	for _, e := range d {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// objectID is a 12-byte BSON ObjectId.
type objectID [12]byte

func (o objectID) hex() string { return hex.EncodeToString(o[:]) }

// bsonBinary is a BSON binary value.
type bsonBinary struct {
	Subtype byte
	Data    []byte
}

// bsonTimestamp is the internal replication timestamp type.
type bsonTimestamp struct{ T, I uint32 }

// bsonRaw is a value of a type this codec does not interpret; it is kept so a
// document can still be shown.
type bsonRaw struct {
	Type byte
	Data []byte
}

var errBSON = errors.New("mongofs: malformed BSON")

// BSON element types.
const (
	tDouble    = 0x01
	tString    = 0x02
	tDocument  = 0x03
	tArray     = 0x04
	tBinary    = 0x05
	tObjectID  = 0x07
	tBool      = 0x08
	tDateTime  = 0x09
	tNull      = 0x0A
	tInt32     = 0x10
	tTimestamp = 0x11
	tInt64     = 0x12
	tDecimal   = 0x13
)

func appendCString(b []byte, s string) []byte {
	b = append(b, s...)
	return append(b, 0)
}

func appendInt32(b []byte, v int32) []byte {
	return binary.LittleEndian.AppendUint32(b, uint32(v)) // #nosec G115 -- two's complement is what BSON stores
}

// encode writes a document.
func (d bsonD) encode() ([]byte, error) {
	out := appendInt32(nil, 0)
	for _, e := range d {
		var err error
		if out, err = appendElement(out, e.Key, e.Value); err != nil {
			return nil, err
		}
	}
	out = append(out, 0)
	binary.LittleEndian.PutUint32(out, uint32(len(out))) // #nosec G115 -- a command is small
	return out, nil
}

func appendElement(b []byte, key string, v any) ([]byte, error) {
	add := func(t byte) []byte { return appendCString(append(b, t), key) }
	switch x := v.(type) {
	case nil:
		return add(tNull), nil
	case string:
		b = add(tString)
		b = appendInt32(b, int32(len(x)+1)) // #nosec G115 -- a command string is small
		b = append(b, x...)
		return append(b, 0), nil
	case bool:
		b = add(tBool)
		if x {
			return append(b, 1), nil
		}
		return append(b, 0), nil
	case int32:
		return appendInt32(add(tInt32), x), nil
	case int:
		if x > math.MaxInt32 || x < math.MinInt32 {
			return binary.LittleEndian.AppendUint64(add(tInt64), uint64(int64(x))), nil // #nosec G115 -- two's complement is what BSON stores
		}
		return appendInt32(add(tInt32), int32(x)), nil // #nosec G115 -- range checked above
	case int64:
		return binary.LittleEndian.AppendUint64(add(tInt64), uint64(x)), nil // #nosec G115 -- two's complement is what BSON stores
	case float64:
		return binary.LittleEndian.AppendUint64(add(tDouble), math.Float64bits(x)), nil
	case objectID:
		return append(add(tObjectID), x[:]...), nil
	case time.Time:
		return binary.LittleEndian.AppendUint64(add(tDateTime), uint64(x.UnixMilli())), nil // #nosec G115 -- two's complement is what BSON stores
	case bsonTimestamp:
		b = add(tTimestamp)
		b = binary.LittleEndian.AppendUint32(b, x.I)
		return binary.LittleEndian.AppendUint32(b, x.T), nil
	case bsonBinary:
		b = add(tBinary)
		b = appendInt32(b, int32(len(x.Data))) // #nosec G115 -- a command payload is small
		b = append(b, x.Subtype)
		return append(b, x.Data...), nil
	case []byte:
		b = add(tBinary)
		b = appendInt32(b, int32(len(x))) // #nosec G115 -- a command payload is small
		b = append(b, 0)
		return append(b, x...), nil
	case bsonRaw:
		if x.Type != tDecimal || len(x.Data) != 16 {
			return nil, fmt.Errorf("mongofs: cannot encode a raw value of type 0x%02x", x.Type)
		}
		return append(add(x.Type), x.Data...), nil
	case bsonD:
		sub, err := x.encode()
		if err != nil {
			return nil, err
		}
		return append(add(tDocument), sub...), nil
	case []any:
		doc := make(bsonD, len(x))
		for i, item := range x {
			doc[i] = bsonE{Key: fmt.Sprint(i), Value: item}
		}
		sub, err := doc.encode()
		if err != nil {
			return nil, err
		}
		return append(add(tArray), sub...), nil
	}
	return nil, fmt.Errorf("mongofs: cannot encode %T", v)
}

// decodeDoc reads one document from the start of data.
func decodeDoc(data []byte) (bsonD, error) {
	if len(data) < 5 {
		return nil, errBSON
	}
	size := int(binary.LittleEndian.Uint32(data))
	if size < 5 || size > len(data) || data[size-1] != 0 {
		return nil, errBSON
	}
	body := data[4 : size-1]
	var doc bsonD
	for len(body) > 0 {
		t := body[0]
		end := indexZero(body[1:])
		if end < 0 {
			return nil, errBSON
		}
		key := string(body[1 : 1+end])
		body = body[2+end:]
		val, n, err := decodeValue(t, body)
		if err != nil {
			return nil, err
		}
		body = body[n:]
		doc = append(doc, bsonE{Key: key, Value: val})
	}
	return doc, nil
}

func indexZero(b []byte) int {
	for i, c := range b {
		if c == 0 {
			return i
		}
	}
	return -1
}

func decodeValue(t byte, b []byte) (any, int, error) {
	need := func(n int) error {
		if n < 0 || len(b) < n {
			return errBSON
		}
		return nil
	}
	switch t {
	case tDouble:
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return math.Float64frombits(binary.LittleEndian.Uint64(b)), 8, nil
	case tString:
		if err := need(4); err != nil {
			return nil, 0, err
		}
		n := int(binary.LittleEndian.Uint32(b))
		if n < 1 || need(4+n) != nil {
			return nil, 0, errBSON
		}
		return string(b[4 : 4+n-1]), 4 + n, nil
	case tDocument, tArray:
		if err := need(4); err != nil {
			return nil, 0, err
		}
		n := int(binary.LittleEndian.Uint32(b))
		if n < 5 || need(n) != nil {
			return nil, 0, errBSON
		}
		doc, err := decodeDoc(b[:n])
		if err != nil {
			return nil, 0, err
		}
		if t == tArray {
			items := make([]any, len(doc))
			for i, e := range doc {
				items[i] = e.Value
			}
			return items, n, nil
		}
		return doc, n, nil
	case tBinary:
		if err := need(5); err != nil {
			return nil, 0, err
		}
		n := int(binary.LittleEndian.Uint32(b))
		if n < 0 || need(5+n) != nil {
			return nil, 0, errBSON
		}
		return bsonBinary{Subtype: b[4], Data: append([]byte(nil), b[5:5+n]...)}, 5 + n, nil
	case tObjectID:
		if err := need(12); err != nil {
			return nil, 0, err
		}
		var o objectID
		copy(o[:], b)
		return o, 12, nil
	case tBool:
		if err := need(1); err != nil {
			return nil, 0, err
		}
		return b[0] != 0, 1, nil
	case tDateTime:
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return time.UnixMilli(int64(binary.LittleEndian.Uint64(b))).UTC(), 8, nil // #nosec G115 -- two's complement is what BSON stores
	case tNull:
		return nil, 0, nil
	case tInt32:
		if err := need(4); err != nil {
			return nil, 0, err
		}
		return int32(binary.LittleEndian.Uint32(b)), 4, nil // #nosec G115 -- two's complement is what BSON stores
	case tTimestamp:
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return bsonTimestamp{I: binary.LittleEndian.Uint32(b), T: binary.LittleEndian.Uint32(b[4:])}, 8, nil
	case tInt64:
		if err := need(8); err != nil {
			return nil, 0, err
		}
		return int64(binary.LittleEndian.Uint64(b)), 8, nil // #nosec G115 -- two's complement is what BSON stores
	case tDecimal:
		if err := need(16); err != nil {
			return nil, 0, err
		}
		return bsonRaw{Type: t, Data: append([]byte(nil), b[:16]...)}, 16, nil
	}
	return nil, 0, fmt.Errorf("%w: unsupported element type 0x%02x", errBSON, t)
}

// toJSON renders a decoded document as relaxed extended JSON, indented, so a
// document reads as a file.
func toJSON(v any, indent string) string {
	var sb strings.Builder
	writeJSON(&sb, v, indent, "")
	return sb.String()
}

func writeJSON(sb *strings.Builder, v any, unit, cur string) {
	next := cur + unit
	nl := func(pad string) {
		if unit != "" {
			sb.WriteString("\n" + pad)
		}
	}
	switch x := v.(type) {
	case nil:
		sb.WriteString("null")
	case string:
		sb.WriteString(jsonQuote(x))
	case bool:
		fmt.Fprint(sb, x)
	case int32:
		fmt.Fprint(sb, x)
	case int64:
		if x >= math.MinInt32 && x <= math.MaxInt32 {
			// A plain number this small would read back as an int32.
			fmt.Fprintf(sb, `{"$numberLong": "%d"}`, x)
		} else {
			fmt.Fprint(sb, x)
		}
	case float64:
		switch {
		case math.IsNaN(x):
			sb.WriteString(`{"$numberDouble": "NaN"}`)
		case math.IsInf(x, 1):
			sb.WriteString(`{"$numberDouble": "Infinity"}`)
		case math.IsInf(x, -1):
			sb.WriteString(`{"$numberDouble": "-Infinity"}`)
		default:
			text := strconv.FormatFloat(x, 'g', -1, 64)
			if !strings.ContainsAny(text, ".e") {
				text += ".0" // so that it reads back as a double, not an int
			}
			sb.WriteString(text)
		}
	case objectID:
		fmt.Fprintf(sb, `{"$oid": %q}`, x.hex())
	case time.Time:
		fmt.Fprintf(sb, `{"$date": %q}`, x.Format("2006-01-02T15:04:05.000Z"))
	case bsonBinary:
		fmt.Fprintf(sb, `{"$binary": {"base64": %q, "subType": "%02x"}}`, base64Std(x.Data), x.Subtype)
	case bsonTimestamp:
		fmt.Fprintf(sb, `{"$timestamp": {"t": %d, "i": %d}}`, x.T, x.I)
	case bsonRaw:
		fmt.Fprintf(sb, `{"$unsupported": {"type": %d, "hex": %q}}`, x.Type, hex.EncodeToString(x.Data))
	case bsonD:
		if len(x) == 0 {
			sb.WriteString("{}")
			return
		}
		sb.WriteString("{")
		for i, e := range x {
			if i > 0 {
				sb.WriteString(",")
			}
			nl(next)
			sb.WriteString(jsonQuote(e.Key) + ": ")
			writeJSON(sb, e.Value, unit, next)
		}
		nl(cur)
		sb.WriteString("}")
	case []any:
		if len(x) == 0 {
			sb.WriteString("[]")
			return
		}
		sb.WriteString("[")
		for i, item := range x {
			if i > 0 {
				sb.WriteString(",")
			}
			nl(next)
			writeJSON(sb, item, unit, next)
		}
		nl(cur)
		sb.WriteString("]")
	default:
		sb.WriteString(jsonQuote(fmt.Sprint(x)))
	}
}
