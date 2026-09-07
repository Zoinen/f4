package id3editor

import (
	"os"
	"strings"
	"testing"

	"github.com/unxed/id3-go"
	v1 "github.com/unxed/id3-go/v1"
	"github.com/unxed/vtui"
)

func init() {
	vtui.AddStrings(map[string]string{
		"ID3Editor.LocalOnly": "ID3 Tag Editor only supports local files.",
		"ID3Editor.OnlyMP3":   "Only MP3 files are supported.",
	})
}

func TestID3Editor_Roundtrip(t *testing.T) {
	tempFile, err := os.CreateTemp("", "test_id3_roundtrip_*.mp3")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Remove(tempFile.Name()); err != nil {
			t.Errorf("remove temporary MP3: %v", err)
		}
	})

	dummyAudio := make([]byte, 100)
	if _, err := tempFile.Write(dummyAudio); err != nil {
		t.Fatal(err)
	}

	tag := &v1.Tag{}
	tag.SetTitle("Initial Title")
	tag.SetArtist("Initial Artist")
	tag.SetAlbum("Initial Album")
	tag.SetYear("2020")
	tag.SetGenre("Rock")
	tag.SetComment("Initial Comment")

	if _, err := tempFile.Write(tag.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("close temporary MP3: %v", err)
	}

	file, err := id3.Open(tempFile.Name())
	if err != nil {
		t.Fatalf("failed to open tag: %v", err)
	}

	if strings.TrimRight(file.Title(), "\x00") != "Initial Title" {
		t.Errorf("expected initial title, got %q", file.Title())
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close initial tag: %v", err)
	}

	file2, err := id3.Open(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}

	file2.SetTitle("New Title")
	file2.SetArtist("New Artist")
	setComment(file2.Tagger, "New Comment")
	err = file2.Close()
	if err != nil {
		t.Fatalf("failed to save tags: %v", err)
	}

	file3, err := id3.Open(tempFile.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file3.Close() }()

	title := strings.TrimSpace(strings.TrimRight(file3.Title(), "\x00"))
	artist := strings.TrimSpace(strings.TrimRight(file3.Artist(), "\x00"))

	comment := ""
	if comments := file3.Comments(); len(comments) > 0 {
		comment = strings.TrimSpace(strings.TrimRight(comments[0], "\x00"))
	}

	if title != "New Title" {
		t.Errorf("expected 'New Title', got %q", title)
	}
	if artist != "New Artist" {
		t.Errorf("expected 'New Artist', got %q", artist)
	}
	if comment != "New Comment" {
		t.Errorf("expected 'New Comment', got %q", comment)
	}
}
