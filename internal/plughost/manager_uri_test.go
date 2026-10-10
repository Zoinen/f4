package plughost

import (
	"testing"
	"testing/synctest"
)

func TestExtractedRestoreLeavesExternalApprovalUnblocked(t *testing.T) {
	for _, scheme := range []string{"ios", "android", "cloud"} {
		t.Run(scheme, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				pm := NewPluginManager(newLuaTestHostAPI())
				provider := pm.api.(*extractedURIHost).prepare(scheme)
				if provider == nil {
					t.Fatal("URI registration failed")
				}
				restoreDone := make(chan error, 1)
				go func() {
					_, err := provider.OpenURI(t.Context(), nil, scheme+"://")
					restoreDone <- err
				}()
				synctest.Wait()

				approvalRequested := make(chan struct{})
				approvalDeferred := make(chan struct{})
				pm.externalLoader = func() {
					// Model an interactive loader posting a permission request
					// and waiting for the UI's response, without granting access.
					approvalRequested <- struct{}{}
					<-approvalDeferred
				}
				pm.StartExternal()
				// This is the startup/UI caller, not another worker: it must
				// reach the permission request while restoration is pending.
				<-approvalRequested
				select {
				case err := <-restoreDone:
					t.Fatalf("restore ended before permission response: %v", err)
				default:
				}
				close(approvalDeferred)
				if err := <-restoreDone; err == nil {
					t.Fatal("deferred permission unexpectedly opened a mount")
				}
			})
		})
	}
}
