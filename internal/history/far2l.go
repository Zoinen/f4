package history

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func ExtractNames(recs []HistoryRecord) []string {
	var res []string
	for _, r := range recs {
		res = append(res, r.Name)
	}
	return res
}

func DecodeFar2lTime(hexStr string) (time.Time, error) {
	if len(hexStr) != 16 {
		return time.Time{}, fmt.Errorf("invalid time length")
	}
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		return time.Time{}, err
	}
	val := binary.LittleEndian.Uint64(b)
	// FILETIME's maximum uint64 value becomes at most 1.85e12 seconds
	// after division, far below int64's limit.
	// #nosec G115 -- division by 10,000,000 bounds the result to int64.
	sec := int64(val / 10000000)
	nsec := int64(val%10000000) * 100
	sec -= 11644473600
	return time.Unix(sec, nsec), nil
}

func UnescapeFar2lString(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	s = strings.ReplaceAll(s, "\\n", "\n")
	s = strings.ReplaceAll(s, "\\r", "\r")
	s = strings.ReplaceAll(s, "\\\"", "\"")
	s = strings.ReplaceAll(s, "\\\\", "\\")
	return s
}

// Far2lHistoryFile is the slice of an ini reader this import needs. Taking it
// as an argument keeps the configuration loader out of a leaf package.
type Far2lHistoryFile interface {
	GetString(section, key, fallback string) string
}

// ImportFar2lHistory reads a far2l SavedHistory block. path names the file only
// so that a failure can say which one.
func ImportFar2lHistory(ini Far2lHistoryFile, path string) ([]HistoryRecord, error) {
	linesStr := ini.GetString("SavedHistory", "Lines", "")
	extrasStr := ini.GetString("SavedHistory", "Extras", "")
	locksStr := ini.GetString("SavedHistory", "Locks", "")
	timesStr := ini.GetString("SavedHistory", "Times", "")

	if linesStr == "" {
		return nil, fmt.Errorf("no Lines found in %s", path)
	}

	linesStr = UnescapeFar2lString(linesStr)
	extrasStr = UnescapeFar2lString(extrasStr)

	lines := strings.Split(linesStr, "\n")
	var extras []string
	if extrasStr != "" {
		extras = strings.Split(extrasStr, "\n")
	}
	times := strings.Fields(timesStr)

	var res []HistoryRecord
	for i, name := range lines {
		rec := HistoryRecord{Name: name}
		if i < len(extras) {
			rec.Extra = extras[i]
		}
		if i < len(locksStr) && locksStr[i] != '0' {
			rec.Lock = true
		}
		if i < len(times) {
			if t, err := DecodeFar2lTime(times[i]); err == nil {
				rec.Timestamp = t
			}
		}
		res = append(res, rec)
	}

	return res, nil
}
