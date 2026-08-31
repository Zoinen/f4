//go:build windows

package winshell

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/registry"

	"github.com/zzl/go-win32api/v2/win32"
)

var clsidFileOperation = syscall.GUID{
	Data1: 0x3AD05575,
	Data2: 0x8857,
	Data3: 0x4850,
	Data4: [8]byte{0x92, 0x77, 0x11, 0xB8, 0x5B, 0xDB, 0x8E, 0x09},
}

func clsidParsingName(clsid string) string { return "shell:::" + clsid }

func shellHRESULT(operation string, hr win32.HRESULT) error {
	return fmt.Errorf("%s: %s", operation, win32.HRESULT_ToString(hr))
}

func openShellItem(parsingName string) (*win32.IShellItem, error) {
	if strings.TrimSpace(parsingName) == "" {
		return nil, fmt.Errorf("empty Windows Shell parsing name")
	}
	var item *win32.IShellItem
	hr := win32.SHCreateItemFromParsingName(
		win32.StrToPwstr(parsingName),
		nil,
		&win32.IID_IShellItem,
		unsafe.Pointer(&item),
	)
	if win32.FAILED(hr) || item == nil {
		return nil, shellHRESULT("resolve Windows Shell item", hr)
	}
	return item, nil
}

func shellDisplayName(item *win32.IShellItem, kind win32.SIGDN) (string, error) {
	var value win32.PWSTR
	hr := item.GetDisplayName(kind, &value)
	if win32.FAILED(hr) || value == nil {
		return "", shellHRESULT("read Windows Shell item name", hr)
	}
	result := win32.PwstrToStr(value)
	win32.CoTaskMemFree(unsafe.Pointer(value))
	return result, nil
}

func describeParsingName(parsingName string) (Node, error) {
	item, err := openShellItem(parsingName)
	if err != nil {
		return Node{}, err
	}
	defer item.Release()
	node, err := describeShellItem(item, parsingName)
	if err == nil && (isCLSIDRoot(parsingName, galleryCLSID) || isCLSIDRoot(node.ParsingName, galleryCLSID)) {
		node.RequiresIndexing = windowsSearchIndexingDisabled()
	}
	return node, err
}

func describeShellItem(item *win32.IShellItem, fallbackParsingName string) (Node, error) {
	name, err := shellDisplayName(item, win32.SIGDN_NORMALDISPLAY)
	if err != nil {
		return Node{}, err
	}
	parsingName, parsingErr := shellDisplayName(item, win32.SIGDN_DESKTOPABSOLUTEPARSING)
	if parsingErr != nil || parsingName == "" {
		parsingName = fallbackParsingName
	}
	if parsingName == "" {
		return Node{}, fmt.Errorf("Windows Shell item %q has no persistent parsing identity", name)
	}

	fileSystemPath, _ := shellDisplayName(item, win32.SIGDN_FILESYSPATH)
	mask := win32.SFGAO_FOLDER | win32.SFGAO_HASSUBFOLDER | win32.SFGAO_HIDDEN |
		win32.SFGAO_READONLY | win32.SFGAO_CANCOPY | win32.SFGAO_CANMOVE |
		win32.SFGAO_CANLINK | win32.SFGAO_CANRENAME | win32.SFGAO_CANDELETE |
		win32.SFGAO_DROPTARGET
	var attributes win32.SFGAO_FLAGS
	_ = item.GetAttributes(mask, &attributes)

	node := Node{
		URI:            URIFromParsingName(parsingName),
		ParsingName:    parsingName,
		Name:           name,
		FileSystemPath: fileSystemPath,
		Folder:         attributes&win32.SFGAO_FOLDER != 0,
		HasChildren:    attributes&win32.SFGAO_HASSUBFOLDER != 0,
		Hidden:         attributes&win32.SFGAO_HIDDEN != 0,
		ReadOnly:       attributes&win32.SFGAO_READONLY != 0,
		CanCopy:        attributes&win32.SFGAO_CANCOPY != 0,
		CanMove:        attributes&win32.SFGAO_CANMOVE != 0,
		CanLink:        attributes&win32.SFGAO_CANLINK != 0,
		CanRename:      attributes&win32.SFGAO_CANRENAME != 0,
		CanDelete:      attributes&win32.SFGAO_CANDELETE != 0,
		DropTarget:     attributes&win32.SFGAO_DROPTARGET != 0,
	}

	var parent *win32.IShellItem
	if hr := item.GetParent(&parent); !win32.FAILED(hr) && parent != nil {
		node.ParentParsingName, _ = shellDisplayName(parent, win32.SIGDN_DESKTOPABSOLUTEPARSING)
		parent.Release()
	}
	if fileSystemPath != "" {
		if stat, statErr := os.Stat(fileSystemPath); statErr == nil {
			node.Size = stat.Size()
			node.SizeKnown = !stat.IsDir()
			node.Modified = stat.ModTime()
		}
	}
	return node, nil
}

func enumerateParsingName(parsingName string) ([]Node, error) {
	if isCLSIDRoot(parsingName, networkCLSID) {
		return enumerateNetworkRoot(parsingName)
	}
	return enumerateShellItems(parsingName)
}

func enumerateShellItems(parsingName string) ([]Node, error) {
	parent, err := openShellItem(parsingName)
	if err != nil {
		return nil, err
	}
	defer parent.Release()

	var enum *win32.IEnumShellItems
	hr := parent.BindToHandler(nil, &win32.BHID_EnumItems, &win32.IID_IEnumShellItems, unsafe.Pointer(&enum))
	if win32.FAILED(hr) || enum == nil {
		return nil, shellHRESULT("enumerate Windows Shell folder", hr)
	}
	defer enum.Release()

	items := make([]Node, 0, 32)
	for {
		var child *win32.IShellItem
		var fetched uint32
		hr = enum.Next(1, &child, &fetched)
		if fetched == 0 || child == nil {
			if isGalleryIndexingRequired(parsingName, hr) {
				return nil, ErrGalleryIndexingRequired
			}
			if win32.FAILED(hr) && !isShellEnumerationEnd(hr) {
				return nil, shellHRESULT("read Windows Shell folder", hr)
			}
			break
		}
		node, describeErr := describeShellItem(child, "")
		child.Release()
		if describeErr == nil {
			items = append(items, node)
		}
		if hr != 0 {
			break
		}
	}
	return items, nil
}

func enumerateNetworkRoot(parsingName string) ([]Node, error) {
	flags := uint32(win32.SHCONTF_FOLDERS | win32.SHCONTF_NONFOLDERS |
		win32.SHCONTF_INIT_ON_FIRST_NEXT | win32.SHCONTF_ENABLE_ASYNC)
	nodes, enumerateErr := collectEnumerationSnapshots(8, func() {
		time.Sleep(150 * time.Millisecond)
	}, func() ([]Node, error) {
		return enumerateShellFolder(parsingName, flags)
	})

	// Explorer's Function Discovery view includes the local machine even when
	// the asynchronous network providers have only returned fast SSDP devices.
	// Resolve the UNC identity through Shell so the item remains a real,
	// navigable namespace node with the same parent and capabilities.
	if hostname, err := os.Hostname(); err == nil && strings.TrimSpace(hostname) != "" {
		if local, describeErr := describeParsingName(`\\` + hostname); describeErr == nil {
			nodes = appendUniqueNodes(nodes, local)
		}
	}
	if len(nodes) > 0 {
		return nodes, nil
	}
	return nodes, enumerateErr
}

func collectEnumerationSnapshots(attempts int, pause func(), enumerate func() ([]Node, error)) ([]Node, error) {
	if attempts < 1 {
		attempts = 1
	}
	var result []Node
	for attempt := 0; attempt < attempts; attempt++ {
		snapshot, err := enumerate()
		if err != nil {
			if len(result) > 0 {
				return result, nil
			}
			return nil, err
		}
		result = appendUniqueNodes(result, snapshot...)
		if attempt+1 < attempts && pause != nil {
			pause()
		}
	}
	return result, nil
}

func appendUniqueNodes(nodes []Node, candidates ...Node) []Node {
	seen := make(map[string]struct{}, len(nodes)+len(candidates))
	key := func(node Node) string {
		identity := node.ParsingName
		if identity == "" {
			identity = node.URI
		}
		return strings.ToLower(identity)
	}
	for _, node := range nodes {
		seen[key(node)] = struct{}{}
	}
	for _, candidate := range candidates {
		candidateKey := key(candidate)
		if candidateKey == "" {
			continue
		}
		if _, exists := seen[candidateKey]; exists {
			continue
		}
		seen[candidateKey] = struct{}{}
		nodes = append(nodes, candidate)
	}
	return nodes
}

func enumerateShellFolder(parsingName string, flags uint32) ([]Node, error) {
	parent, err := openShellItem(parsingName)
	if err != nil {
		return nil, err
	}
	defer parent.Release()

	var folder *win32.IShellFolder
	hr := parent.BindToHandler(nil, &win32.BHID_SFObject, &win32.IID_IShellFolder, unsafe.Pointer(&folder))
	if win32.FAILED(hr) || folder == nil {
		return nil, shellHRESULT("open Windows Shell folder", hr)
	}
	defer folder.Release()

	var enum *win32.IEnumIDList
	hr = folder.EnumObjects(0, flags, &enum)
	if enum == nil {
		if isShellEnumerationEnd(hr) {
			return []Node{}, nil
		}
		return nil, shellHRESULT("enumerate Windows Shell folder", hr)
	}
	if win32.FAILED(hr) {
		enum.Release()
		return nil, shellHRESULT("enumerate Windows Shell folder", hr)
	}
	defer enum.Release()

	var parentPIDL *win32.ITEMIDLIST
	hr = win32.SHGetIDListFromObject(&parent.IUnknown, &parentPIDL)
	if win32.FAILED(hr) || parentPIDL == nil {
		return nil, shellHRESULT("read Windows Shell folder identity", hr)
	}
	defer win32.CoTaskMemFree(unsafe.Pointer(parentPIDL))

	return shellNodesFromIDList(folder, parentPIDL, enum)
}

func shellNodesFromIDList(folder *win32.IShellFolder, parentPIDL *win32.ITEMIDLIST, enum *win32.IEnumIDList) ([]Node, error) {
	items := make([]Node, 0, 32)
	for {
		var childPIDL *win32.ITEMIDLIST
		var fetched uint32
		hr := enum.Next(1, &childPIDL, &fetched)
		if fetched == 0 || childPIDL == nil {
			if win32.FAILED(hr) && !isShellEnumerationEnd(hr) {
				return nil, shellHRESULT("read Windows Shell folder", hr)
			}
			break
		}

		var child *win32.IShellItem
		createHR := win32.SHCreateItemWithParent(parentPIDL, folder, childPIDL,
			&win32.IID_IShellItem, unsafe.Pointer(&child))
		win32.CoTaskMemFree(unsafe.Pointer(childPIDL))
		if !win32.FAILED(createHR) && child != nil {
			node, describeErr := describeShellItem(child, "")
			child.Release()
			if describeErr == nil {
				items = append(items, node)
			}
		}
		if hr != 0 {
			if win32.FAILED(hr) && !isShellEnumerationEnd(hr) {
				return nil, shellHRESULT("read Windows Shell folder", hr)
			}
			break
		}
	}
	return items, nil
}

func isShellEnumerationEnd(hr win32.HRESULT) bool {
	switch uint32(hr) {
	case uint32(win32.S_FALSE),
		0x80070012, // HRESULT_FROM_WIN32(ERROR_NO_MORE_FILES)
		0x80070103, // HRESULT_FROM_WIN32(ERROR_NO_MORE_ITEMS)
		0x800710d2: // HRESULT_FROM_WIN32(ERROR_EMPTY)
		return true
	default:
		return false
	}
}

func isGalleryIndexingRequired(parsingName string, hr win32.HRESULT) bool {
	return isCLSIDRoot(parsingName, galleryCLSID) && uint32(hr) == 0x800710d2
}

func enumerateNavigationChildren(parsingName string) ([]Node, error) {
	nodes, err := enumerateParsingName(parsingName)
	if err != nil {
		return nil, err
	}
	result := make([]Node, 0, len(nodes))
	for _, node := range nodes {
		if !node.Folder {
			continue
		}
		attachNodeIcon(&node)
		result = append(result, node)
	}
	return result, nil
}

func buildRootModel() ([]Node, error) {
	var result []Node
	seen := make(map[string]bool)
	appendNode := func(node Node) {
		key := strings.ToLower(node.ParsingName)
		if node.FileSystemPath != "" {
			key = "path:" + strings.ToLower(filepath.Clean(node.FileSystemPath))
		}
		if key == "" || seen[key] {
			return
		}
		seen[key] = true
		result = append(result, node)
	}

	for _, clsid := range []string{homeCLSID, galleryCLSID} {
		if node, err := describeParsingName(clsidParsingName(clsid)); err == nil {
			node.Section = SectionPrimary
			appendNode(node)
		}
	}
	if len(result) > 0 {
		result = append(result, Node{Separator: true, Section: SectionQuickAccess})
	}

	quickItems, _ := enumerateParsingName(clsidParsingName(quickAccessCLSID))
	quickCount := 0
	for _, node := range quickItems {
		if !node.Folder {
			continue
		}
		node.Section = SectionQuickAccess
		node.Pinned = true
		before := len(result)
		appendNode(node)
		if len(result) != before {
			quickCount++
		}
	}
	if quickCount > 0 {
		result = append(result, Node{Separator: true, Section: SectionNamespace})
	}

	showAll := navPaneShowAllFolders()
	desktopItems, _ := enumerateParsingName("shell:Desktop")
	for _, node := range desktopItems {
		if !node.Folder || (!showAll && !isNavigationRoot(node)) {
			continue
		}
		node.Section = SectionNamespace
		appendNode(node)
	}

	// Some namespace providers are absent from the Desktop enumerator until
	// first use. Resolve the standard Explorer roots explicitly, while still
	// feature-detecting each one.
	for _, clsid := range []string{thisPCCLSID, networkCLSID, linuxCLSID} {
		if node, err := describeParsingName(clsidParsingName(clsid)); err == nil {
			node.Section = SectionNamespace
			appendNode(node)
		}
	}

	for len(result) > 0 && result[len(result)-1].Separator {
		result = result[:len(result)-1]
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("Windows Shell navigation tree is empty")
	}
	for index := range result {
		if !result[index].Separator {
			attachNodeIcon(&result[index])
		}
	}
	return result, nil
}

func isNavigationRoot(node Node) bool {
	upper := strings.ToUpper(node.ParsingName)
	for _, clsid := range []string{thisPCCLSID, networkCLSID, linuxCLSID, homeCLSID, galleryCLSID} {
		if strings.Contains(upper, strings.ToUpper(clsid)) {
			return true
		}
	}
	clsid := firstCLSID(node.ParsingName)
	if clsid == "" {
		return false
	}
	for _, base := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		path := `Software\Classes\CLSID\` + clsid
		key, err := registry.OpenKey(base, path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, _, valueErr := key.GetIntegerValue("System.IsPinnedToNameSpaceTree")
		key.Close()
		if valueErr == nil {
			return value != 0
		}
	}
	return false
}

func firstCLSID(value string) string {
	start := strings.IndexByte(value, '{')
	if start < 0 || len(value)-start < 38 {
		return ""
	}
	candidate := value[start : start+38]
	if _, err := win32.StrToGuid(candidate); err != nil {
		return ""
	}
	return strings.ToUpper(candidate)
}

func isCLSIDRoot(parsingName, clsid string) bool {
	value := strings.TrimSpace(parsingName)
	if len(value) >= len("shell:") && strings.EqualFold(value[:len("shell:")], "shell:") {
		value = value[len("shell:"):]
	}
	return strings.EqualFold(value, "::"+clsid)
}

func navPaneShowAllFolders() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	value, _, err := key.GetIntegerValue("NavPaneShowAllFolders")
	return err == nil && value != 0
}

func windowsSearchIndexingDisabled() bool {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE,
		`SYSTEM\CurrentControlSet\Services\WSearch`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()
	start, _, err := key.GetIntegerValue("Start")
	return err == nil && start == 4 // SERVICE_DISABLED
}

func newFileOperation(recycle bool) (*win32.IFileOperation, error) {
	var operation *win32.IFileOperation
	hr := win32.CoCreateInstance(&clsidFileOperation, nil, win32.CLSCTX_INPROC_SERVER,
		&win32.IID_IFileOperation, unsafe.Pointer(&operation))
	if win32.FAILED(hr) || operation == nil {
		return nil, shellHRESULT("create Windows Shell file operation", hr)
	}
	flags := uint32(win32.FOF_SILENT | win32.FOF_NOCONFIRMATION | win32.FOF_NOCONFIRMMKDIR |
		win32.FOF_NOERRORUI | win32.FOFX_EARLYFAILURE | win32.FOFX_ADDUNDORECORD)
	if recycle {
		flags |= uint32(win32.FOFX_RECYCLEONDELETE)
	}
	if hr = operation.SetOperationFlags(flags); win32.FAILED(hr) {
		operation.Release()
		return nil, shellHRESULT("configure Windows Shell file operation", hr)
	}
	return operation, nil
}

func performFileOperation(operation *win32.IFileOperation, action string) error {
	hr := operation.PerformOperations()
	var aborted win32.BOOL
	abortHR := operation.GetAnyOperationsAborted(&aborted)
	if win32.FAILED(hr) {
		return shellHRESULT(action, hr)
	}
	if win32.FAILED(abortHR) {
		return shellHRESULT("query Windows Shell operation result", abortHR)
	}
	if aborted != 0 {
		return fmt.Errorf("%s was aborted", action)
	}
	return nil
}

func createFolder(parentParsingName, name string) error {
	parent, err := openShellItem(parentParsingName)
	if err != nil {
		return err
	}
	defer parent.Release()
	operation, err := newFileOperation(false)
	if err != nil {
		return err
	}
	defer operation.Release()
	hr := operation.NewItem(parent, uint32(win32.FILE_ATTRIBUTE_DIRECTORY), win32.StrToPwstr(name), nil, nil)
	if win32.FAILED(hr) {
		return shellHRESULT("queue Windows Shell folder creation", hr)
	}
	return performFileOperation(operation, "create Windows Shell folder")
}

func renameItem(parsingName, newName string) error {
	item, err := openShellItem(parsingName)
	if err != nil {
		return err
	}
	defer item.Release()
	operation, err := newFileOperation(false)
	if err != nil {
		return err
	}
	defer operation.Release()
	hr := operation.RenameItem(item, win32.StrToPwstr(newName), nil)
	if win32.FAILED(hr) {
		return shellHRESULT("queue Windows Shell rename", hr)
	}
	return performFileOperation(operation, "rename Windows Shell item")
}

func deleteItem(parsingName string, recycle bool) error {
	item, err := openShellItem(parsingName)
	if err != nil {
		return err
	}
	defer item.Release()
	operation, err := newFileOperation(recycle)
	if err != nil {
		return err
	}
	defer operation.Release()
	hr := operation.DeleteItem(item, nil)
	if win32.FAILED(hr) {
		return shellHRESULT("queue Windows Shell delete", hr)
	}
	return performFileOperation(operation, "delete Windows Shell item")
}

func importPath(sourcePath, parentParsingName, name string, move bool) error {
	source, err := openShellItem(sourcePath)
	if err != nil {
		return err
	}
	defer source.Release()
	destination, err := openShellItem(parentParsingName)
	if err != nil {
		return err
	}
	defer destination.Release()
	operation, err := newFileOperation(false)
	if err != nil {
		return err
	}
	defer operation.Release()
	var hr win32.HRESULT
	if move {
		hr = operation.MoveItem(source, destination, win32.StrToPwstr(name), nil)
	} else {
		hr = operation.CopyItem(source, destination, win32.StrToPwstr(name), nil)
	}
	if win32.FAILED(hr) {
		return shellHRESULT("queue Windows Shell import", hr)
	}
	return performFileOperation(operation, "import item into Windows Shell folder")
}

func transferShellItem(sourceParsingName, destinationParsingName, name string, move bool) error {
	source, err := openShellItem(sourceParsingName)
	if err != nil {
		return err
	}
	defer source.Release()
	destination, err := openShellItem(destinationParsingName)
	if err != nil {
		return err
	}
	defer destination.Release()
	operation, err := newFileOperation(false)
	if err != nil {
		return err
	}
	defer operation.Release()
	var hr win32.HRESULT
	if move {
		hr = operation.MoveItem(source, destination, win32.StrToPwstr(name), nil)
	} else {
		hr = operation.CopyItem(source, destination, win32.StrToPwstr(name), nil)
	}
	if win32.FAILED(hr) {
		return shellHRESULT("queue Windows Shell transfer", hr)
	}
	return performFileOperation(operation, "transfer Windows Shell item")
}

func materializeItem(parsingName string) (MaterializedFile, error) {
	node, err := describeParsingName(parsingName)
	if err != nil {
		return MaterializedFile{}, err
	}
	if node.Folder {
		return MaterializedFile{}, fmt.Errorf("cannot open Windows Shell folder as a stream")
	}
	if node.FileSystemPath != "" {
		stat, statErr := os.Stat(node.FileSystemPath)
		if statErr != nil {
			return MaterializedFile{}, statErr
		}
		return MaterializedFile{Path: node.FileSystemPath, Size: stat.Size()}, nil
	}

	item, err := openShellItem(parsingName)
	if err != nil {
		return MaterializedFile{}, err
	}
	defer item.Release()
	var stream *win32.IStream
	hr := item.BindToHandler(nil, &win32.BHID_Stream, &win32.IID_IStream, unsafe.Pointer(&stream))
	if win32.FAILED(hr) || stream == nil {
		return MaterializedFile{}, shellHRESULT("open Windows Shell item stream", hr)
	}
	defer stream.Release()

	extension := filepath.Ext(node.Name)
	temp, err := os.CreateTemp("", "f4-shell-*"+extension)
	if err != nil {
		return MaterializedFile{}, err
	}
	tempPath := temp.Name()
	committed := false
	defer func() {
		_ = temp.Close()
		if !committed {
			_ = os.Remove(tempPath)
		}
	}()

	buffer := make([]byte, 128*1024)
	var total int64
	for {
		var read uint32
		hr = stream.Read(unsafe.Pointer(&buffer[0]), uint32(len(buffer)), &read)
		if win32.FAILED(hr) {
			return MaterializedFile{}, shellHRESULT("read Windows Shell item stream", hr)
		}
		if read > 0 {
			written, writeErr := temp.Write(buffer[:read])
			total += int64(written)
			if writeErr != nil {
				return MaterializedFile{}, writeErr
			}
		}
		if read == 0 || hr != 0 {
			break
		}
	}
	if err := temp.Close(); err != nil {
		return MaterializedFile{}, err
	}
	committed = true
	return MaterializedFile{Path: tempPath, Size: total, Owned: true}, nil
}
