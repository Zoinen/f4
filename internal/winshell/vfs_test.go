package winshell

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/unxed/f4/vfs"
)

type fakeShellClient struct {
	mu          sync.Mutex
	nodes       map[string]Node
	children    map[string][]Node
	imports     []string
	deletes     []deleteRequest
	renames     []renameRequest
	transfers   []transferRequest
	createdDirs []newItemRequest
}

func (f *fakeShellClient) Describe(_ context.Context, parsingName string) (Node, error) {
	if node, ok := f.nodes[parsingName]; ok {
		return node, nil
	}
	return Node{}, os.ErrNotExist
}

func (f *fakeShellClient) Enumerate(ctx context.Context, parsingName string) ([]Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]Node(nil), f.children[parsingName]...), nil
}

func (f *fakeShellClient) CreateDir(_ context.Context, parent, name string) error {
	f.mu.Lock()
	f.createdDirs = append(f.createdDirs, newItemRequest{ParentParsingName: parent, Name: name})
	f.mu.Unlock()
	return nil
}

func (f *fakeShellClient) Rename(_ context.Context, parsingName, name string) error {
	f.mu.Lock()
	f.renames = append(f.renames, renameRequest{ParsingName: parsingName, NewName: name})
	f.mu.Unlock()
	return nil
}

func (f *fakeShellClient) Delete(_ context.Context, parsingName string, recycle bool) error {
	f.mu.Lock()
	f.deletes = append(f.deletes, deleteRequest{ParsingName: parsingName, Recycle: recycle})
	f.mu.Unlock()
	return nil
}

func (f *fakeShellClient) ImportPath(_ context.Context, sourcePath, parent, name string, move bool) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.imports = append(f.imports, parent+"|"+name+"|"+string(data))
	f.mu.Unlock()
	return nil
}

func (f *fakeShellClient) Transfer(_ context.Context, source, destination, name string, move bool) error {
	f.mu.Lock()
	f.transfers = append(f.transfers, transferRequest{
		SourceParsingName: source, DestinationParsingName: destination, Name: name, Move: move,
	})
	f.mu.Unlock()
	return nil
}

func (*fakeShellClient) Materialize(context.Context, string) (MaterializedFile, error) {
	return MaterializedFile{}, os.ErrNotExist
}

func shellVFSTestFixture(t *testing.T) (*ShellVFS, *fakeShellClient, Node, []Node) {
	t.Helper()
	root := Node{URI: URIFromParsingName("root"), ParsingName: "root", Name: "Home", Folder: true}
	children := []Node{
		{URI: URIFromParsingName("root/a"), ParsingName: "root/a", ParentParsingName: "root", Name: "Folder", Folder: true, HasChildren: true, CanMove: true, CanRename: true, CanDelete: true},
		{URI: URIFromParsingName("root/b"), ParsingName: "root/b", ParentParsingName: "root", Name: "Folder", Folder: true, CanMove: true, CanRename: true, CanDelete: true},
		{URI: URIFromParsingName("root/file"), ParsingName: "root/file", ParentParsingName: "root", Name: "file.txt", Size: 7, SizeKnown: true, CanMove: true, CanRename: true, CanDelete: true},
	}
	client := &fakeShellClient{
		nodes:    map[string]Node{"root": root},
		children: map[string][]Node{"root": children},
	}
	for _, child := range children {
		client.nodes[child.ParsingName] = child
	}
	filesystem, err := newShellVFS(client, root, nil)
	if err != nil {
		t.Fatal(err)
	}
	return filesystem, client, root, children
}

func TestShellVFSReadDirUsesStableOpaquePaths(t *testing.T) {
	filesystem, _, root, children := shellVFSTestFixture(t)
	var items []string
	err := filesystem.ReadDir(t.Context(), root.URI, func(chunk []vfs.VFSItem) {
		for _, item := range chunk {
			items = append(items, item.Name)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(items, ","), "Folder,Folder (2),file.txt"; got != want {
		t.Fatalf("names = %q, want %q", got, want)
	}
	if got := filesystem.Join(root.URI, "Folder (2)"); got != children[1].URI {
		t.Fatalf("Join duplicate = %q, want %q", got, children[1].URI)
	}
	if got := filesystem.Dir(children[0].URI); got != root.URI {
		t.Fatalf("Dir(child) = %q, want %q", got, root.URI)
	}
}

func TestShellVFSMutationsUseNativeClient(t *testing.T) {
	filesystem, client, root, children := shellVFSTestFixture(t)
	if err := filesystem.ReadDir(t.Context(), root.URI, nil); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Rename(t.Context(), children[0].URI, filesystem.Join(root.URI, "Renamed")); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.MoveToTrash(t.Context(), children[1].URI); err != nil {
		t.Fatal(err)
	}
	if err := filesystem.Copy(t.Context(), children[2].URI, filesystem.Join(root.URI, "copy.txt")); err != nil {
		t.Fatal(err)
	}
	if len(client.renames) != 1 || client.renames[0].NewName != "Renamed" {
		t.Fatalf("renames = %#v", client.renames)
	}
	if len(client.deletes) != 1 || !client.deletes[0].Recycle {
		t.Fatalf("deletes = %#v", client.deletes)
	}
	if len(client.transfers) != 1 || client.transfers[0].Name != "copy.txt" || client.transfers[0].Move {
		t.Fatalf("transfers = %#v", client.transfers)
	}
}

func TestShellVFSWriterCommitsAndAbortDoesNot(t *testing.T) {
	filesystem, client, root, _ := shellVFSTestFixture(t)
	writer, err := filesystem.Create(t.Context(), filesystem.Join(root.URI, "created.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(writer, "payload"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if len(client.imports) != 1 || client.imports[0] != "root|created.txt|payload" {
		t.Fatalf("imports = %#v", client.imports)
	}

	abortWriter, err := filesystem.Create(t.Context(), filesystem.Join(root.URI, "discard.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(abortWriter, "discard"); err != nil {
		t.Fatal(err)
	}
	if err := abortWriter.(interface{ Abort() error }).Abort(); err != nil {
		t.Fatal(err)
	}
	if len(client.imports) != 1 {
		t.Fatalf("abort committed an import: %#v", client.imports)
	}
}
