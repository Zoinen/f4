//go:build windows

package winshell

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func assertReadableShellNodeURI(t *testing.T, node Node) {
	t.Helper()
	if !strings.HasPrefix(strings.ToLower(node.URI), Scheme+"://") {
		t.Fatalf("Shell node %q exposes non-Windows URI %q", node.Name, node.URI)
	}
	parsingName, err := ParsingNameFromURI(node.URI)
	if err != nil || !strings.EqualFold(parsingName, node.ParsingName) {
		t.Fatalf("Shell node %q URI %q resolves to %q, %v; want %q", node.Name, node.URI, parsingName, err, node.ParsingName)
	}
}

func TestBrokerIntegration(t *testing.T) {
	executable := os.Getenv("F4_SHELL_INTEGRATION_EXE")
	if executable == "" {
		t.Skip("set F4_SHELL_INTEGRATION_EXE to the canonical f4.exe")
	}
	client := NewClient(executable)
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("broker ping: %v", err)
	}
	roots, err := client.Roots(ctx)
	if err != nil {
		t.Fatalf("enumerate Explorer navigation roots: %v", err)
	}
	nonSeparators := 0
	icons := 0
	homeFound := false
	logTree := os.Getenv("F4_SHELL_LOG_TREE") != ""
	for _, node := range roots {
		if node.Separator {
			continue
		}
		if logTree {
			t.Logf("root name=%q uri=%q parsing=%q filesystem=%q folder=%v children=%v", node.Name, node.URI, node.ParsingName, node.FileSystemPath, node.Folder, node.HasChildren)
		}
		nonSeparators++
		if node.IconWidth > 0 && node.IconHeight > 0 && len(node.IconRGBA) == node.IconWidth*node.IconHeight*4 {
			icons++
		}
		if node.Name == "" || node.ParsingName == "" || !IsURI(node.URI) {
			t.Fatalf("invalid navigation node: %#v", node)
		}
		assertReadableShellNodeURI(t, node)
		if isCLSIDRoot(node.ParsingName, homeCLSID) {
			homeFound = true
			if node.URI != homeURI {
				t.Fatalf("Home URI = %q, want %q", node.URI, homeURI)
			}
		}
	}
	if nonSeparators < 3 {
		t.Fatalf("Explorer navigation roots = %d, want at least 3", nonSeparators)
	}
	if icons == 0 {
		t.Fatal("Explorer navigation roots returned no system bitmap icons")
	}
	if homeFound {
		t.Logf("Home uses readable URI %s", homeURI)
	}
	var first Node
	for _, node := range roots {
		if !node.Separator {
			first = node
			break
		}
	}
	described, err := client.Describe(ctx, first.ParsingName)
	if err != nil || described.Name == "" {
		t.Fatalf("describe first navigation root: node=%#v err=%v", described, err)
	}
	firstChildren, err := client.Enumerate(ctx, described.ParsingName)
	if err != nil {
		t.Fatalf("enumerate first navigation root nodes: %v", err)
	}
	for _, child := range firstChildren {
		assertReadableShellNodeURI(t, child)
		if logTree {
			t.Logf("%s child name=%q uri=%q parsing=%q filesystem=%q folder=%v children=%v", described.Name, child.Name, child.URI, child.ParsingName, child.FileSystemPath, child.Folder, child.HasChildren)
		}
	}
	filesystem, err := newShellVFS(client, described, nil)
	if err != nil {
		t.Fatalf("open first navigation root as Shell VFS: %v", err)
	}
	if err := filesystem.ReadDir(ctx, filesystem.GetPath(), nil); err != nil {
		t.Fatalf("enumerate first navigation root through Shell VFS: %v", err)
	}
	menu, err := client.ContextMenu(ctx, first.ParsingName)
	if err != nil {
		t.Fatalf("query real Shell context menu for %q: %v", first.Name, err)
	}
	if menu.Token == 0 || len(menu.Commands) == 0 {
		t.Fatalf("empty Shell context menu for %q: %#v", first.Name, menu)
	}
	if err := client.DismissContextMenu(ctx, menu.Token); err != nil {
		t.Fatalf("dismiss Shell context menu: %v", err)
	}
	for _, node := range roots {
		if strings.Contains(strings.ToUpper(node.ParsingName), strings.ToUpper(thisPCCLSID)) {
			children, childErr := client.NavigationChildren(ctx, node.ParsingName)
			if childErr != nil {
				t.Fatalf("enumerate This PC navigation children: %v", childErr)
			}
			if len(children) == 0 {
				t.Fatal("This PC returned no navigation children")
			}
			for _, child := range children {
				assertReadableShellNodeURI(t, child)
			}
			if logTree {
				for _, child := range children {
					t.Logf("This PC child name=%q uri=%q parsing=%q filesystem=%q folder=%v children=%v", child.Name, child.URI, child.ParsingName, child.FileSystemPath, child.Folder, child.HasChildren)
				}
			}
			break
		}
	}
	if logTree {
		for _, node := range roots {
			if !isCLSIDRoot(node.ParsingName, linuxCLSID) {
				continue
			}
			children, childErr := client.NavigationChildren(ctx, node.ParsingName)
			if childErr != nil {
				t.Logf("Linux children error: %v", childErr)
				break
			}
			for _, child := range children {
				assertReadableShellNodeURI(t, child)
				t.Logf("Linux child name=%q uri=%q parsing=%q filesystem=%q folder=%v children=%v", child.Name, child.URI, child.ParsingName, child.FileSystemPath, child.Folder, child.HasChildren)
			}
			break
		}
	}
	for _, special := range []struct {
		name  string
		clsid string
	}{
		{name: "Gallery", clsid: galleryCLSID},
		{name: "Network", clsid: networkCLSID},
	} {
		for _, node := range roots {
			if !isCLSIDRoot(node.ParsingName, special.clsid) {
				continue
			}
			t.Run(special.name, func(t *testing.T) {
				timeout := 5 * time.Second
				if special.clsid == networkCLSID {
					timeout = 10 * time.Second
				}
				rootCtx, rootCancel := context.WithTimeout(context.Background(), timeout)
				defer rootCancel()
				items, rootErr := client.Enumerate(rootCtx, node.ParsingName)
				if special.clsid == galleryCLSID && node.RequiresIndexing {
					if !errors.Is(rootErr, ErrGalleryIndexingRequired) {
						t.Fatalf("Gallery indexing error = %v, want %v", rootErr, ErrGalleryIndexingRequired)
					}
					t.Log("Gallery correctly reports that Windows Search indexing is required")
					return
				}
				if special.clsid == galleryCLSID && os.Getenv("F4_SHELL_EXPECT_GALLERY_INDEXING_REQUIRED") != "" {
					t.Fatal("Gallery root did not report disabled Windows Search indexing")
				}
				if rootErr != nil {
					t.Fatalf("enumerate %s: %v", special.name, rootErr)
				}
				if special.clsid == networkCLSID && os.Getenv("F4_SHELL_EXPECT_NETWORK_ITEMS") != "" && len(items) == 0 {
					t.Fatal("Network returned no items after bounded asynchronous discovery")
				}
				names := make([]string, 0, len(items))
				localComputerFound := false
				hostname, _ := os.Hostname()
				for _, item := range items {
					assertReadableShellNodeURI(t, item)
					names = append(names, item.Name)
					if special.clsid == networkCLSID && strings.EqualFold(item.ParsingName, `\\`+hostname) {
						localComputerFound = item.Folder
					}
					if logTree {
						t.Logf("%s child name=%q uri=%q parsing=%q filesystem=%q folder=%v children=%v", special.name, item.Name, item.URI, item.ParsingName, item.FileSystemPath, item.Folder, item.HasChildren)
					}
				}
				if special.clsid == networkCLSID && !localComputerFound {
					t.Fatalf("Network does not contain the local computer %q", hostname)
				}
				t.Logf("%s returned %d items: %v", special.name, len(items), names)
			})
			break
		}
	}
}
