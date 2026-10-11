//go:build !lite

package plughost

import (
	"github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/plugins/dockerfs"
	"github.com/unxed/f4/plugins/k8sfs"
	"github.com/unxed/f4/plugins/mongofs"
	"github.com/unxed/f4/plugins/netfox"
)

// optionalVFSPlugins are the VFS providers a lite build cuts down or drops
// (f4#1178): the native-library archive plugin goes entirely, and netfox
// keeps only FISH+ (over a subprocess ssh dialer instead of this build's
// golang.org/x/crypto/ssh one) in place of the FTP/SFTP/FISH+ trio here.
// See plugins_lite.go for the other half of this build tag's single point
// of truth.
//
// Cloud storage (plugins/cloudfox: S3, Google Drive, Yandex Disk, WebDAV)
// no longer lives here at all, in either build. It moved out to its own
// module and its own subprocess RPC plugin binary
// (plugins/cloudfox/cmd/cloudfox-plugin, plugins/cloudfox/go.mod) so its
// ~30 MB of cloud SDKs (aws-sdk-go-v2, google.golang.org/api,
// golang.org/x/oauth2, github.com/zalando/go-keyring) never enter this
// module's build list, full build included (f4#1178, part 1 of the plan at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851218447). f4
// itself still runs it exactly the way it runs any other native plugin --
// see docs/PLUGINS.md and PlugRing (internal/plughost/plugring.go); wiring
// an actual install path is part 2/3 of that plan, not done here.
func optionalVFSPlugins() []Plugin {
	return []Plugin{
		&archive.ArchivePlugin{},
		&netfox.NetFoxPlugin{},
		// Docker containers as a read-only drive (f4#1663); talks to the
		// daemon over its unix socket with the standard library only.
		dockerfs.NewPlugin(),
		// Kubernetes namespaces, pods and containers as a read-only drive
		// (f4#1663); the API server over the standard library and x/net.
		k8sfs.NewPlugin(),
		// MongoDB databases, collections and documents as a read-only drive
		// (f4#1663); the wire protocol is spoken directly, no driver.
		mongofs.NewPlugin(),
	}
}
