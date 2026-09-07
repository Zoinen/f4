package mediainfo

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestParseMacroArgumentsKeepsLegacyLastValueSemantics(t *testing.T) {
	arguments, err := parseMacroArguments([]any{
		nil,
		int64(0),
		"first.wav",
		[]any{"Width", nil},
		"{Format,Duration}",
		true,
		"last.wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	if arguments.path != "last.wav" || !arguments.technical || !arguments.filterUsed {
		t.Fatalf("parsed arguments = %#v", arguments)
	}
	for _, key := range []string{"Width", "Format", "Duration"} {
		if _, ok := arguments.filter[key]; !ok {
			t.Errorf("filter does not contain %q: %#v", key, arguments.filter)
		}
	}

	arguments, err = parseMacroArguments(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !arguments.technical || arguments.filterUsed || arguments.path != "" {
		t.Fatalf("default arguments = %#v", arguments)
	}
}

func TestParseMacroArgumentsRejectsUnsupportedValues(t *testing.T) {
	for _, arguments := range [][]any{
		{float64(1)},
		{[]any{"Format", int64(2)}},
	} {
		if _, err := parseMacroArguments(arguments); err == nil {
			t.Fatalf("arguments %#v were accepted", arguments)
		}
	}
}

func TestMacroResultFiltersExactKeysAndPreservesDuplicates(t *testing.T) {
	report := Report{
		General: General{Format: "Matroska", Duration: 2 * time.Second},
		Streams: []Stream{
			{ID: "1", Kind: StreamVideo, Format: "AVC"},
			{ID: "2", Kind: StreamAudio, Format: "AAC"},
		},
		Tags: []Field{
			{Name: "Title", Value: "First title"},
			{Name: "Title", Value: "Second title"},
			{Name: "Empty", Value: ""},
		},
	}
	pairs := macroTechnicalPairs(report)
	arguments, err := parseMacroArguments([]any{"{Format}"})
	if err != nil {
		t.Fatal(err)
	}
	result := macroResult(true, pairs, arguments)
	if !reflect.DeepEqual(result, []any{
		true,
		int64(3),
		[]string{"Format", "Format", "Format"},
		[]string{"Matroska", "AVC", "AAC"},
	}) {
		t.Fatalf("filtered result = %#v", result)
	}

	arguments, err = parseMacroArguments([]any{"{format}"})
	if err != nil {
		t.Fatal(err)
	}
	result = macroResult(true, pairs, arguments)
	if result[1] != int64(0) || len(result[2].([]string)) != 0 || len(result[3].([]string)) != 0 {
		t.Fatalf("case-insensitive filter match = %#v", result)
	}

	arguments, err = parseMacroArguments([]any{"{Title}"})
	if err != nil {
		t.Fatal(err)
	}
	result = macroResult(true, pairs, arguments)
	if !reflect.DeepEqual(result[3], []string{"First title", "Second title"}) {
		t.Fatalf("duplicate metadata values = %#v", result)
	}
}

func TestMacroImageSelectorReturnsOnlyImageMetadata(t *testing.T) {
	takenAt := time.Date(2024, 6, 7, 8, 9, 10, 0, time.UTC)
	report := Report{
		General: General{Format: "JPEG"},
		Streams: []Stream{{
			ID:     "1",
			Kind:   StreamImage,
			Format: "JPEG",
			Image: &Image{
				Width:       4032,
				Height:      3024,
				CameraMake:  "Example Camera Co.",
				CameraModel: "Model One",
				TakenAt:     &takenAt,
			},
		}},
		Tags: []Field{
			{Name: "Keyword", Value: "one"},
			{Name: "Keyword", Value: "two"},
		},
	}
	pairs := macroImagePairs(report)
	if macroPairValues(pairs, "Format") != nil {
		t.Fatalf("image backend exposed technical Format: %#v", pairs)
	}
	if got := macroPairValues(pairs, "Width"); !reflect.DeepEqual(got, []string{"4032"}) {
		t.Fatalf("Width = %#v", got)
	}
	if got := macroPairValues(pairs, "Keyword"); !reflect.DeepEqual(got, []string{"one", "two"}) {
		t.Fatalf("Keyword = %#v", got)
	}
}

func TestCallMacroReadsCurrentVFSWithoutMaterializing(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "tone.wav")
	if err := os.WriteFile(path, minimalWave(), 0o600); err != nil {
		t.Fatal(err)
	}
	filesystem := &macroNoMaterializeVFS{VFS: vfs.NewOSVFS(directory)}
	plugin := NewPlugin(t.TempDir())

	result, err := plugin.callMacro(context.Background(), vfs.MacroCallContext{Current: vfs.FileRef{
		VFS:  filesystem,
		Dir:  directory,
		Name: "tone.wav",
		Path: path,
	}}, []any{nil, int64(1), "{Format}"})
	if err != nil {
		t.Fatal(err)
	}
	if result[0] != true || result[1] != int64(2) {
		t.Fatalf("result = %#v", result)
	}
	if !reflect.DeepEqual(result[2], []string{"Format", "Format"}) || !reflect.DeepEqual(result[3], []string{"Wave", "PCM"}) {
		t.Fatalf("format pairs = %#v", result)
	}
	relativeResult, err := plugin.callMacro(context.Background(), vfs.MacroCallContext{Current: vfs.FileRef{
		VFS: filesystem, Dir: directory,
	}}, []any{"tone.wav", "{CodecID}"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(relativeResult[3], []string{"RIFF", "0x0001"}) {
		t.Fatalf("relative-path codec pairs = %#v", relativeResult)
	}

	// reader::read in the original plugin ignores exif_read's false return:
	// a readable non-image is still a successful image-mode call with no
	// pairs. Some macros use that distinction from a missing/unreadable file.
	imageResult, err := plugin.callMacro(context.Background(), vfs.MacroCallContext{Current: vfs.FileRef{
		VFS: filesystem, Path: path,
	}}, []any{false})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imageResult, []any{true, int64(0), []string{}, []string{}}) {
		t.Fatalf("non-image image-mode result = %#v", imageResult)
	}
	if filesystem.opens != 1 || filesystem.creates != 0 {
		t.Fatalf("VFS calls: opens=%d creates=%d", filesystem.opens, filesystem.creates)
	}
}

func TestCallMacroFailureAndCancellation(t *testing.T) {
	plugin := NewPlugin(t.TempDir())
	result, err := plugin.callMacro(context.Background(), vfs.MacroCallContext{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result, []any{false, int64(0), []string{}, []string{}}) {
		t.Fatalf("missing source result = %#v", result)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := plugin.callMacro(ctx, vfs.MacroCallContext{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func macroPairValues(pairs []macroPair, key string) []string {
	var values []string
	for _, pair := range pairs {
		if pair.key == key {
			values = append(values, pair.value)
		}
	}
	return values
}

type macroNoMaterializeVFS struct {
	vfs.VFS
	opens   int
	creates int
}

func (filesystem *macroNoMaterializeVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	filesystem.opens++
	return filesystem.VFS.Open(ctx, path)
}

func (filesystem *macroNoMaterializeVFS) Create(context.Context, string) (io.WriteCloser, error) {
	filesystem.creates++
	return nil, errors.New("Create must not be called by MediaInfo macros")
}

func minimalWave() []byte {
	data := make([]byte, 44)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], mediaFixtureUint32(len(data)-8))
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	binary.LittleEndian.PutUint32(data[16:20], 16)
	binary.LittleEndian.PutUint16(data[20:22], 1)
	binary.LittleEndian.PutUint16(data[22:24], 1)
	binary.LittleEndian.PutUint32(data[24:28], 8000)
	binary.LittleEndian.PutUint32(data[28:32], 8000)
	binary.LittleEndian.PutUint16(data[32:34], 1)
	binary.LittleEndian.PutUint16(data[34:36], 8)
	copy(data[36:40], "data")
	return data
}
