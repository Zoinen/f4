package cloudfox

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestWritableCloudBackendsAdvertiseAtomicNoReplaceRename(t *testing.T) {
	backends := map[string]interface{ Capabilities() vfs.VFSCapabilities }{
		"Google Drive": &googleDriveBackend{},
		"S3":           &s3Backend{},
		"WebDAV":       &webDAVBackend{},
		"Yandex Disk":  &yandexDiskBackend{},
	}
	for name, backend := range backends {
		if !backend.Capabilities().HasAtomicNoReplaceRename {
			t.Errorf("%s does not advertise atomic no-replace rename", name)
		}
	}
}
