package panel

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
)

// A drive that reports itself as a DVD drive is DVD-ROM in the drive menu,
// any other optical drive is CD-ROM (mitek1977-coder, f4#1148).
func TestDriveMenuOpticalKindTellsDVDFromCD(t *testing.T) {
	if got := driveMenuOpticalKind(fileDeviceDVD, true); got != driveMenuKindDVD {
		t.Errorf("FILE_DEVICE_DVD: kind %d, want DVD", got)
	}
	const fileDeviceCDROM = 0x02
	if got := driveMenuOpticalKind(fileDeviceCDROM, true); got != driveMenuKindCD {
		t.Errorf("FILE_DEVICE_CD_ROM: kind %d, want CD", got)
	}
	if got := driveMenuOpticalKind(fileDeviceDVD, false); got != driveMenuKindCD {
		t.Errorf("a drive that did not answer: kind %d, want CD", got)
	}

	if cd, dvd := driveMenuKindLabel(driveMenuKindCD), driveMenuKindLabel(driveMenuKindDVD); cd != i18n.Msg("Drive.TypeCD") || dvd != i18n.Msg("Drive.TypeDVD") || cd == dvd {
		t.Errorf("labels CD %q, DVD %q", cd, dvd)
	}

	// "Show CD drive parameters" covers DVD drives too.
	on := config.DriveMenuShowCD
	if !driveMenuKindShowsParameters(driveMenuKindDVD, on) || driveMenuKindShowsParameters(driveMenuKindDVD, 0) {
		t.Error("the CD parameters switch must govern DVD drives as well")
	}
}
