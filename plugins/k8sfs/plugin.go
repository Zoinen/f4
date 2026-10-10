// Package k8sfs shows a Kubernetes cluster in a file panel: namespaces, pods
// and containers are folders, and inside a container is its file system, read
// by running ls, stat and cat in it through the API server's exec endpoint (a
// WebSocket, like kubectl exec). Nothing is needed beyond a kubeconfig with a
// token or client certificate: no kubectl, no client-go, no CGO.
//
// Reading needs ls, stat and cat in the container; writing (small files, new
// folders, delete, rename) needs sh, base64, mkdir, rm and mv. Busybox has them all.
package k8sfs

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

// driveName is what the drive menu (Alt+F1) calls the panel.
const driveName = "Kubernetes"

// Plugin registers the Kubernetes drive.
type Plugin struct{}

// NewPlugin constructs the built-in Kubernetes plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (*Plugin) GetName() string { return "Kubernetes" }

// Init only registers the drive; the kubeconfig is not read until the panel is
// opened, so a machine without a cluster pays nothing.
func (*Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("Kubernetes: nil host API")
	}
	api.RegisterDrive(driveName, func() vfs.VFS { return newK8sVFS(openFromKubeconfig) })
	// k8s:///<path> reopens the panel from a bookmark, history or a saved session.
	// One more drive per context of the kubeconfig.
	if cfgPath, err := kubeconfigPath(); err == nil {
		for _, name := range listContexts(cfgPath) {
			api.RegisterDrive(contextDriveName(name), func() vfs.VFS { return newContextVFS(name) })
		}
	}
	return api.RegisterURIProvider(uriProvider{open: openFromKubeconfig, openContext: contextOpener})
}

func (*Plugin) Close() error {
	vfs.UnregisterURIProvider("k8s")
	return nil
}

// openFromKubeconfig connects with the user's kubeconfig; without one, inside
// a pod, with the pod's service account.
func openFromKubeconfig() (*restClient, error) {
	p, err := kubeconfigPath()
	if err != nil {
		return nil, err
	}
	ep, err := loadEndpoint(p)
	if errors.Is(err, errNoKubeconfig) {
		if ic, icErr := inClusterEndpoint(inClusterDir); icErr == nil {
			return newRESTClient(ic), nil
		}
	}
	if err != nil {
		return nil, err
	}
	return newRESTClient(ep), nil
}
