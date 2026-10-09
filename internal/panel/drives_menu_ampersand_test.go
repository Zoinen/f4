package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// A volume label with an ampersand reads as that label in the drive menu: the
// menu takes a single "&" for a hotkey marker, so the row has to escape it
// without disturbing the columns after it (f4#1148).
func TestDriveMenuRowKeepsAmpersandOfVolumeLabel(t *testing.T) {
	options := uint32(config.DriveMenuShowLabel | config.DriveMenuShowFilesystem)
	rows := []DriveMenuPlatformRow{
		{Base: "D:", Label: "R&D", Filesystem: "NTFS"},
		{Base: "E:", Label: "Backup", Filesystem: "ext4"},
	}
	texts := DriveMenuPlatformRowsText(rows, options)

	if !strings.Contains(texts[0], "R&&D") {
		t.Fatalf("label not escaped for the menu: %q", texts[0])
	}
	shown := make([]string, len(texts))
	for i, text := range texts {
		plain, _, _ := vtui.ParseAmpersandString(text)
		shown[i] = plain
	}
	if !strings.Contains(shown[0], "R&D") {
		t.Fatalf("the menu shows %q, want the label R&D intact", shown[0])
	}
	if strings.Index(shown[0], "| NTFS") != strings.Index(shown[1], "| ext4") {
		t.Fatalf("columns drifted apart as shown: %q / %q", shown[0], shown[1])
	}
}
