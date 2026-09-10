package viewer

import (
	testing "testing"
)

func TestNativeViewerContentRevisionInvalidatesPendingConstruction(t *testing.T) {
	viewer := cachedSemanticViewer([]byte("old text"))
	defer viewer.Close()
	before := viewer.constructionKey(0, 20, 4, 2)
	viewer.Backend.DropCache()
	after := viewer.constructionKey(0, 20, 4, 2)
	if before == after {
		t.Fatal("same-size cache invalidation reused a pending construction key")
	}
}
