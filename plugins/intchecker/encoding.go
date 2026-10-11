package intchecker

import (
	"errors"
	"fmt"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// utf8Codepage is the codepage id of UTF-8 in f4's codepage table.
const utf8Codepage = 65001

// autoDetectEncoding asks "Validate files" to detect the encoding.
var autoDetectEncoding = fileEncoding{Codepage: vfs.CodepageAutoDetect}

// utf8BOM is the byte order mark a "UTF-8 with BOM" checksum file starts with.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// fileEncoding is how a checksum file stores its text: a codepage from f4's
// table (vfs.AvailableCodepages), as the editor and the viewer use them. The
// zero value is UTF-8 without a byte order mark, what the plugin wrote before
// the encoding could be chosen.
type fileEncoding struct {
	// Codepage is the codepage id; 0 means UTF-8. For reading,
	// vfs.CodepageAutoDetect asks to detect it.
	Codepage int
	// BOM starts a UTF-8 file with a byte order mark.
	BOM bool
}

func (e fileEncoding) codepage() int {
	if e.Codepage == 0 {
		return utf8Codepage
	}
	return vfs.NormalizeCodepageID(e.Codepage)
}

func (e fileEncoding) isUTF8() bool { return e.codepage() == utf8Codepage }

// name is the encoding as reports show it.
func (e fileEncoding) name() string {
	if e.isUTF8() && e.BOM {
		return vtui.Msg("IntChecker.EncodingUTF8BOM")
	}
	return vfs.DisplayCodepageName(e.codepage())
}

// errUnencodableName means a file name has characters the chosen encoding
// cannot store.
var errUnencodableName = errors.New("file name cannot be stored in this encoding")

// canStore says whether name survives a round trip through the encoding. A
// codepage that silently replaces what it cannot map (the Windows converters
// do) is caught by the round trip, not only by an encoder error.
func (e fileEncoding) canStore(name string) bool {
	if e.isUTF8() {
		return true
	}
	encoded, err := vfs.EncodeBytes([]byte(name), e.codepage())
	if err != nil {
		return false
	}
	decoded, err := vfs.DecodeBytes(encoded, e.codepage())
	return err == nil && string(decoded) == name
}

// encode turns the UTF-8 text of a checksum file into the bytes to write. It
// fails with errUnencodableName when the encoding cannot store the text;
// callers leave such names out beforehand (see canStore).
func (e fileEncoding) encode(text string) ([]byte, error) {
	if e.isUTF8() {
		if e.BOM {
			return append(append([]byte{}, utf8BOM...), text...), nil
		}
		return []byte(text), nil
	}
	if !e.canStore(text) {
		return nil, fmt.Errorf("%w: %s", errUnencodableName, e.name())
	}
	return vfs.EncodeBytes([]byte(text), e.codepage())
}

// decodeChecksumFile turns the bytes of a checksum file into UTF-8 text and
// returns the codepage it read them in. A byte order mark always wins, as it
// does in the viewer. Otherwise codepage says how the file is encoded;
// vfs.CodepageAutoDetect detects it the way the viewer does: valid UTF-8 is
// UTF-8, anything else goes through f4's legacy codepage detection and falls
// back to the system ANSI codepage, what Windows tools write. UTF-8 is passed
// through unchanged, so names that are not valid UTF-8 (raw bytes of a Unix
// file name) still find their files.
func decodeChecksumFile(data []byte, codepage int) ([]byte, int, error) {
	cp, ok := vfs.DetectBOM(data)
	if !ok {
		switch codepage {
		case vfs.CodepageAutoDetect:
			cp = vfs.DetectEncoding(data, true, vfs.SystemANSICodepage())
		case 0:
			cp = utf8Codepage
		default:
			cp = codepage
		}
	}
	cp = vfs.NormalizeCodepageID(cp)
	if cp == utf8Codepage {
		return data, cp, nil
	}
	decoded, err := vfs.DecodeBytes(data, cp)
	if err != nil {
		return nil, cp, err
	}
	return decoded, cp, nil
}

// encodingChoice is one entry of an encoding combo box.
type encodingChoice struct {
	label    string
	encoding fileEncoding
}

// systemEncodingChoices are the system ANSI and OEM codepages, as IntChecker
// offers them next to UTF-8. One that is UTF-8 itself (a UTF-8 system) or
// repeats the other is left out.
func systemEncodingChoices() []encodingChoice {
	var out []encodingChoice
	seen := map[int]bool{utf8Codepage: true}
	for _, sys := range []struct {
		key string
		id  int
	}{
		{"IntChecker.EncodingANSI", vfs.SystemANSICodepage()},
		{"IntChecker.EncodingOEM", vfs.SystemOEMCodepage()},
	} {
		id := vfs.NormalizeCodepageID(sys.id)
		if seen[id] {
			continue
		}
		if cp, ok := vfs.FindCodepage(id); !ok || cp.Enc == nil {
			continue
		}
		seen[id] = true
		out = append(out, encodingChoice{label: fmt.Sprintf(vtui.Msg(sys.key), id), encoding: fileEncoding{Codepage: id}})
	}
	return out
}

// writeEncodingChoices are the encodings "Generate hashes" can write, the
// default (UTF-8, what md5sum and sha256sum read) first.
func writeEncodingChoices() []encodingChoice {
	return append([]encodingChoice{
		{label: "UTF-8", encoding: fileEncoding{Codepage: utf8Codepage}},
		{label: vtui.Msg("IntChecker.EncodingUTF8BOM"), encoding: fileEncoding{Codepage: utf8Codepage, BOM: true}},
	}, systemEncodingChoices()...)
}

// readEncodingChoices are the encodings "Validate files" can read a checksum
// file in, detection (the default) first.
func readEncodingChoices() []encodingChoice {
	return append([]encodingChoice{
		{label: vtui.Msg("IntChecker.EncodingAuto"), encoding: autoDetectEncoding},
		{label: "UTF-8", encoding: fileEncoding{Codepage: utf8Codepage}},
	}, systemEncodingChoices()...)
}

// encodingCombo is a drop-down list of encoding choices.
type encodingCombo struct {
	box     *vtui.ComboBox
	choices []encodingChoice
}

// newEncodingCombo builds the combo box with the choice for current
// selected (the first one when current is not offered).
func newEncodingCombo(width int, choices []encodingChoice, current fileEncoding) *encodingCombo {
	labels := make([]string, len(choices))
	selected := 0
	for i, c := range choices {
		labels[i] = c.label
		if c.encoding == current {
			selected = i
		}
	}
	combo := vtui.NewComboBox(0, 0, width, labels)
	combo.DropdownOnly = true
	combo.Menu.SetSelectPos(selected)
	combo.Edit.SetText(labels[selected])
	return &encodingCombo{box: combo, choices: choices}
}

// selected is the chosen encoding.
func (c *encodingCombo) selected() fileEncoding {
	pos := c.box.Menu.SelectPos
	if pos < 0 || pos >= len(c.choices) {
		pos = 0
	}
	return c.choices[pos].encoding
}
