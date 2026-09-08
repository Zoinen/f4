//go:build windows

package vfs

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

const (
	registryVFSRoot          = "registry://"
	registryDefaultValueName = "(Default)"
)

type registryHive struct {
	name string
	key  registry.Key
}

var registryHives = []registryHive{
	{name: "HKEY_CLASSES_ROOT", key: registry.CLASSES_ROOT},
	{name: "HKEY_CURRENT_USER", key: registry.CURRENT_USER},
	{name: "HKEY_LOCAL_MACHINE", key: registry.LOCAL_MACHINE},
	{name: "HKEY_USERS", key: registry.USERS},
	{name: "HKEY_CURRENT_CONFIG", key: registry.CURRENT_CONFIG},
}

// RegistryVFS exposes the Windows registry as a read-only virtual filesystem.
// Registry keys are directories and registry values are virtual text files.
// The VFS deliberately opens keys with registry.READ only: this first slice
// must not change the user's registry, even when the process has write access.
type RegistryVFS struct {
	currentPath string
}

func NewRegistryVFS() *RegistryVFS {
	return &RegistryVFS{currentPath: registryVFSRoot}
}

func (v *RegistryVFS) GetPath() string { return v.currentPath }

func (v *RegistryVFS) IsAbs(p string) bool {
	return strings.HasPrefix(p, registryVFSRoot)
}

func (v *RegistryVFS) IsAtRoot() bool {
	return v.currentPath == registryVFSRoot
}

func (v *RegistryVFS) SetPath(p string) error {
	if p == "" || p == "/" {
		p = registryVFSRoot
	}
	segments, err := registryPathSegments(p)
	if err != nil {
		return err
	}
	if len(segments) == 0 {
		v.currentPath = registryVFSRoot
		return nil
	}
	key, owned, err := openRegistryKey(segments)
	if err != nil {
		return err
	}
	if owned {
		defer key.Close()
	}
	v.currentPath = registryPath(segments)
	return nil
}

func (v *RegistryVFS) ReadDir(ctx context.Context, p string, onChunk func([]VFSItem)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	segments, err := registryPathSegments(p)
	if err != nil {
		return err
	}

	if len(segments) == 0 {
		items := make([]VFSItem, 0, len(registryHives))
		for _, hive := range registryHives {
			items = append(items, VFSItem{
				Name:        hive.name,
				IsDir:       true,
				NoExtension: true,
			})
		}
		if onChunk != nil {
			onChunk(items)
		}
		return nil
	}

	key, owned, err := openRegistryKey(segments)
	if err != nil {
		return err
	}
	if owned {
		defer key.Close()
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	subkeys, err := key.ReadSubKeyNames(0)
	if err != nil {
		return registryVFSMapError(err)
	}
	values, err := key.ReadValueNames(0)
	if err != nil {
		return registryVFSMapError(err)
	}
	sort.Strings(subkeys)
	sort.Strings(values)

	items := make([]VFSItem, 0, len(subkeys)+len(values))
	for _, name := range subkeys {
		if err := ctx.Err(); err != nil {
			return err
		}
		items = append(items, VFSItem{
			Name:        name,
			IsDir:       true,
			NoExtension: true,
		})
	}
	for _, name := range values {
		if err := ctx.Err(); err != nil {
			return err
		}
		item, err := registryValueItem(key, name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				// A value can disappear while the key is being listed.
				continue
			}
			return err
		}
		items = append(items, item)
	}
	if len(items) > 0 && onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (v *RegistryVFS) Stat(ctx context.Context, p string) (VFSItem, error) {
	if err := ctx.Err(); err != nil {
		return VFSItem{}, err
	}
	segments, err := registryPathSegments(p)
	if err != nil {
		return VFSItem{}, err
	}
	if len(segments) == 0 {
		return VFSItem{Name: "Registry", IsDir: true, NoExtension: true}, nil
	}

	key, owned, keyErr := openRegistryKey(segments)
	if keyErr == nil {
		if owned {
			defer key.Close()
		}
		info, err := key.Stat()
		if err != nil {
			return VFSItem{}, registryVFSMapError(err)
		}
		return VFSItem{
			Name:        registryDisplayName(segments[len(segments)-1]),
			IsDir:       true,
			MTime:       info.ModTime(),
			NoExtension: true,
		}, nil
	}
	if !errors.Is(keyErr, os.ErrNotExist) {
		return VFSItem{}, keyErr
	}
	if len(segments) < 2 {
		return VFSItem{}, keyErr
	}

	parent, owned, err := openRegistryKey(segments[:len(segments)-1])
	if err != nil {
		return VFSItem{}, err
	}
	if owned {
		defer parent.Close()
	}
	valueName := registryValueName(segments[len(segments)-1])
	data, valueType, err := readRegistryValue(parent, valueName)
	if err != nil {
		return VFSItem{}, registryVFSMapError(err)
	}
	content := formatRegistryValue(valueName, valueType, data)
	return VFSItem{
		Name:        registryDisplayName(segments[len(segments)-1]),
		Size:        int64(len(content)),
		SizeKnown:   true,
		Mode:        registryValueTypeName(valueType),
		NoExtension: true,
		Revision:    registryValueRevision(valueType, data),
	}, nil
}

func (v *RegistryVFS) Join(elem ...string) string {
	segments := make([]string, 0)
	for _, part := range elem {
		if part == "" {
			continue
		}
		if strings.HasPrefix(part, registryVFSRoot) {
			parsed, err := registryPathSegments(part)
			if err != nil {
				return registryVFSRoot
			}
			segments = append(segments, parsed...)
			continue
		}
		switch part {
		case ".":
			continue
		case "..":
			if len(segments) > 0 {
				segments = segments[:len(segments)-1]
			}
		default:
			segments = append(segments, part)
		}
	}
	return registryPath(segments)
}

func (v *RegistryVFS) Abs(p string) (string, error) {
	if p == "" {
		return v.currentPath, nil
	}
	if v.IsAbs(p) {
		segments, err := registryPathSegments(p)
		if err != nil {
			return "", err
		}
		return registryPath(segments), nil
	}
	return v.Join(v.currentPath, p), nil
}

func (v *RegistryVFS) Base(p string) string {
	segments, err := registryPathSegments(p)
	if err != nil || len(segments) == 0 {
		return "Registry"
	}
	return registryDisplayName(segments[len(segments)-1])
}

func (v *RegistryVFS) Dir(p string) string {
	segments, err := registryPathSegments(p)
	if err != nil || len(segments) == 0 {
		return registryVFSRoot
	}
	return registryPath(segments[:len(segments)-1])
}

func (v *RegistryVFS) MkDir(ctx context.Context, _ string) error {
	return registryVFSReadOnlyError(ctx)
}

func (v *RegistryVFS) Remove(ctx context.Context, _ string) error {
	return registryVFSReadOnlyError(ctx)
}

func (v *RegistryVFS) Rename(ctx context.Context, _, _ string) error {
	return registryVFSReadOnlyError(ctx)
}

func (v *RegistryVFS) GetCapabilities() VFSCapabilities {
	return VFSCapabilities{
		HasRandomAccess: true,
		HasWrite:        false,
	}
}

func (v *RegistryVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, nil
}

func (v *RegistryVFS) Open(ctx context.Context, p string) (ReadAtCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	segments, err := registryPathSegments(p)
	if err != nil {
		return nil, err
	}
	if len(segments) < 2 {
		return nil, os.ErrInvalid
	}

	// A key takes precedence over a value with the same name, matching the
	// directory-first presentation in ReadDir.
	if key, owned, keyErr := openRegistryKey(segments); keyErr == nil {
		if owned {
			key.Close()
		}
		return nil, os.ErrInvalid
	} else if !errors.Is(keyErr, os.ErrNotExist) {
		return nil, keyErr
	}

	parent, owned, err := openRegistryKey(segments[:len(segments)-1])
	if err != nil {
		return nil, err
	}
	if owned {
		defer parent.Close()
	}
	data, valueType, err := readRegistryValue(parent, registryValueName(segments[len(segments)-1]))
	if err != nil {
		return nil, registryVFSMapError(err)
	}
	return &registryReader{data: formatRegistryValue(registryValueName(segments[len(segments)-1]), valueType, data)}, nil
}

func (v *RegistryVFS) Create(ctx context.Context, _ string) (io.WriteCloser, error) {
	return nil, registryVFSReadOnlyError(ctx)
}

func (v *RegistryVFS) SetAttributes(ctx context.Context, _ string, _ VFSItem) error {
	return registryVFSReadOnlyError(ctx)
}

func (v *RegistryVFS) ParentVFS() VFS { return nil }

func (v *RegistryVFS) Clone() VFS {
	return &RegistryVFS{currentPath: v.currentPath}
}

func (v *RegistryVFS) Close() error { return nil }

func registryPathSegments(p string) ([]string, error) {
	if p == "" || p == "/" {
		return nil, nil
	}
	if !strings.HasPrefix(p, registryVFSRoot) {
		return nil, os.ErrInvalid
	}
	rest := strings.TrimPrefix(p, registryVFSRoot)
	if rest == "" {
		return nil, nil
	}
	if strings.HasPrefix(rest, "/") {
		return nil, os.ErrInvalid
	}
	encoded := strings.Split(rest, "/")
	segments := make([]string, 0, len(encoded))
	for _, part := range encoded {
		if part == "" {
			return nil, os.ErrInvalid
		}
		decoded, err := url.PathUnescape(part)
		if err != nil || decoded == "" || strings.ContainsRune(decoded, '\x00') {
			return nil, os.ErrInvalid
		}
		segments = append(segments, decoded)
	}
	return segments, nil
}

func registryPath(segments []string) string {
	if len(segments) == 0 {
		return registryVFSRoot
	}
	encoded := make([]string, len(segments))
	for i, segment := range segments {
		encoded[i] = url.PathEscape(segment)
	}
	return registryVFSRoot + strings.Join(encoded, "/")
}

func registryHiveByName(name string) (registryHive, bool) {
	for _, hive := range registryHives {
		if strings.EqualFold(name, hive.name) {
			return hive, true
		}
	}
	return registryHive{}, false
}

func openRegistryKey(segments []string) (registry.Key, bool, error) {
	if len(segments) == 0 {
		return 0, false, os.ErrInvalid
	}
	hive, ok := registryHiveByName(segments[0])
	if !ok {
		return 0, false, os.ErrNotExist
	}
	if len(segments) == 1 {
		return hive.key, false, nil
	}
	for _, segment := range segments[1:] {
		if strings.ContainsRune(segment, '\\') {
			return 0, false, os.ErrInvalid
		}
	}
	key, err := registry.OpenKey(hive.key, strings.Join(segments[1:], `\`), registry.READ)
	if err != nil {
		return 0, false, registryVFSMapError(err)
	}
	return key, true, nil
}

func registryVFSMapError(err error) error {
	if errors.Is(err, registry.ErrNotExist) {
		return os.ErrNotExist
	}
	return err
}

func registryVFSReadOnlyError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return os.ErrPermission
}

func registryValueName(name string) string {
	if name == registryDefaultValueName {
		return ""
	}
	return name
}

func registryDisplayName(name string) string {
	if name == "" {
		return registryDefaultValueName
	}
	return name
}

func readRegistryValue(key registry.Key, name string) ([]byte, uint32, error) {
	for attempt := 0; attempt < 3; attempt++ {
		n, valueType, err := key.GetValue(name, nil)
		if err != nil && !errors.Is(err, registry.ErrShortBuffer) {
			return nil, valueType, err
		}
		if n < 0 {
			return nil, valueType, os.ErrInvalid
		}
		data := make([]byte, n)
		n, valueType, err = key.GetValue(name, data)
		if errors.Is(err, registry.ErrShortBuffer) {
			continue
		}
		if err != nil {
			return nil, valueType, err
		}
		return data[:n], valueType, nil
	}
	return nil, 0, os.ErrInvalid
}

func registryValueItem(key registry.Key, name string) (VFSItem, error) {
	data, valueType, err := readRegistryValue(key, name)
	if err != nil {
		return VFSItem{}, registryVFSMapError(err)
	}
	return VFSItem{
		Name:        registryDisplayName(name),
		Size:        int64(len(formatRegistryValue(name, valueType, data))),
		SizeKnown:   true,
		Mode:        registryValueTypeName(valueType),
		NoExtension: true,
		Revision:    registryValueRevision(valueType, data),
	}, nil
}

func registryValueRevision(valueType uint32, data []byte) string {
	hash := sha256.New()
	var typeBytes [4]byte
	binary.LittleEndian.PutUint32(typeBytes[:], valueType)
	_, _ = hash.Write(typeBytes[:])
	_, _ = hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil))
}

func formatRegistryValue(name string, valueType uint32, data []byte) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "Name: %s\nType: %s\n", registryDisplayName(name), registryValueTypeName(valueType))
	switch valueType {
	case registry.SZ, registry.EXPAND_SZ:
		if value, ok := registryUTF16String(data); ok {
			fmt.Fprintf(&out, "Value: %q\n", value)
			break
		}
		fmt.Fprintf(&out, "Data: %s\n", hex.EncodeToString(data))
	case registry.MULTI_SZ:
		if values, ok := registryUTF16Strings(data); ok {
			for i, value := range values {
				fmt.Fprintf(&out, "Value[%d]: %q\n", i, value)
			}
			if len(values) == 0 {
				out.WriteString("Value: \"\"\n")
			}
			break
		}
		fmt.Fprintf(&out, "Data: %s\n", hex.EncodeToString(data))
	case registry.DWORD:
		if len(data) == 4 {
			value := binary.LittleEndian.Uint32(data)
			fmt.Fprintf(&out, "Value: %d (0x%08X)\n", value, value)
			break
		}
		fmt.Fprintf(&out, "Data: %s\n", hex.EncodeToString(data))
	case registry.DWORD_BIG_ENDIAN:
		if len(data) == 4 {
			value := binary.BigEndian.Uint32(data)
			fmt.Fprintf(&out, "Value: %d (0x%08X)\n", value, value)
			break
		}
		fmt.Fprintf(&out, "Data: %s\n", hex.EncodeToString(data))
	case registry.QWORD:
		if len(data) == 8 {
			value := binary.LittleEndian.Uint64(data)
			fmt.Fprintf(&out, "Value: %d (0x%016X)\n", value, value)
			break
		}
		fmt.Fprintf(&out, "Data: %s\n", hex.EncodeToString(data))
	default:
		fmt.Fprintf(&out, "Data: %s\n", hex.EncodeToString(data))
	}
	return []byte(out.String())
}

func registryUTF16String(data []byte) (string, bool) {
	if len(data)%2 != 0 {
		return "", false
	}
	words := make([]uint16, len(data)/2)
	for i := range words {
		words[i] = binary.LittleEndian.Uint16(data[i*2:])
	}
	return strings.TrimRight(string(utf16.Decode(words)), "\x00"), true
}

func registryUTF16Strings(data []byte) ([]string, bool) {
	value, ok := registryUTF16String(data)
	if !ok {
		return nil, false
	}
	if value == "" {
		return nil, true
	}
	parts := strings.Split(value, "\x00")
	return parts, true
}

func registryValueTypeName(valueType uint32) string {
	switch valueType {
	case registry.NONE:
		return "REG_NONE"
	case registry.SZ:
		return "REG_SZ"
	case registry.EXPAND_SZ:
		return "REG_EXPAND_SZ"
	case registry.BINARY:
		return "REG_BINARY"
	case registry.DWORD:
		return "REG_DWORD"
	case registry.DWORD_BIG_ENDIAN:
		return "REG_DWORD_BIG_ENDIAN"
	case registry.LINK:
		return "REG_LINK"
	case registry.MULTI_SZ:
		return "REG_MULTI_SZ"
	case registry.RESOURCE_LIST:
		return "REG_RESOURCE_LIST"
	case registry.FULL_RESOURCE_DESCRIPTOR:
		return "REG_FULL_RESOURCE_DESCRIPTOR"
	case registry.RESOURCE_REQUIREMENTS_LIST:
		return "REG_RESOURCE_REQUIREMENTS_LIST"
	case registry.QWORD:
		return "REG_QWORD"
	default:
		return fmt.Sprintf("REG_%d", valueType)
	}
}

type registryReader struct {
	mu     sync.Mutex
	data   []byte
	offset int64
	closed bool
}

func (r *registryReader) Size() int64 { return int64(len(r.data)) }

func (r *registryReader) Read(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, os.ErrClosed
	}
	if r.offset >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	r.offset += int64(n)
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *registryReader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 {
		return 0, os.ErrInvalid
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return 0, os.ErrClosed
	}
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *registryReader) Close() error {
	r.mu.Lock()
	r.closed = true
	r.mu.Unlock()
	return nil
}
