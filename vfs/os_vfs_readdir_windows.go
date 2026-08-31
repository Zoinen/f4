//go:build windows

package vfs

import (
	"context"
	"fmt"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileFullDirectoryInfo mirrors FILE_FULL_DIR_INFORMATION. Windows already
// returns every field needed by the deferred base catalog in this record.
// Parsing it directly avoids allocating a fileStat plus two interface wrappers
// per row in os.File.ReadDir/DirEntry.Info.
type fileFullDirectoryInfo struct {
	NextEntryOffset uint32
	FileIndex       uint32
	CreationTime    windows.Filetime
	LastAccessTime  windows.Filetime
	LastWriteTime   windows.Filetime
	ChangeTime      windows.Filetime
	EndOfFile       int64
	AllocationSize  int64
	FileAttributes  uint32
	FileNameLength  uint32
	EaSize          uint32
	FileName        [1]uint16
}

const directoryQueryBufferSize = 64 * 1024

type completeOSDirectoryQuery func(windows.Handle, bool, []byte) error

func queryCompleteOSDirectoryWin32(handle windows.Handle, restart bool, buffer []byte) error {
	infoClass := uint32(windows.FileFullDirectoryInfo)
	if restart {
		infoClass = uint32(windows.FileFullDirectoryRestartInfo)
	}
	return windows.GetFileInformationByHandleEx(
		handle, infoClass, &buffer[0], uint32(len(buffer)))
}

type completeOSDirectoryReadStats struct {
	Rows          int
	QueryCount    int
	QueryDuration time.Duration
	ParseDuration time.Duration
	NameBytes     int
}

// completeOSDirectoryRecord is deliberately compact. VFSItem is currently
// 200 bytes, so growing a []VFSItem from a small capacity copies tens of
// megabytes for a directory such as WinSxS. Parse into these records and one
// segmented UTF-8 name arena first, then materialize the exact-sized result
// once.
type completeOSDirectoryRecord struct {
	name           string
	attributes     uint32
	size           int64
	physicalSize   int64
	modifiedTimeNs int64
}

const (
	directoryRecordChunkSize = 512
	directoryNameChunkSize   = 64 * 1024
	// One details viewport currently paints about 39 rows. Keep four spare rows
	// overscan without making the first semantic/QML hand-off normalize and
	// bind rows which cannot contribute to the first frame.
	directoryPreviewLimit = 42
)

type completeOSDirectoryBuilder struct {
	recordChunks   [][]completeOSDirectoryRecord
	records        []completeOSDirectoryRecord
	nameChunks     [][]byte
	names          []byte
	rowCount       int
	directoryCount int
	nameBytes      int
}

func (builder *completeOSDirectoryBuilder) appendName(source []uint16) string {
	// A UTF-16 code unit expands to at most three UTF-8 bytes. Reserving the
	// worst case makes it impossible for append to reallocate the current name
	// chunk after a string has begun referring to it.
	maximumBytes := len(source) * 3
	if cap(builder.names)-len(builder.names) < maximumBytes {
		chunkSize := directoryNameChunkSize
		if maximumBytes > chunkSize {
			chunkSize = maximumBytes
		}
		builder.names = make([]byte, 0, chunkSize)
		builder.nameChunks = append(builder.nameChunks, builder.names)
	}
	start := len(builder.names)
	builder.names = appendUTF16DirectoryName(builder.names, source)
	builder.nameChunks[len(builder.nameChunks)-1] = builder.names
	nameBytes := builder.names[start:]
	builder.nameBytes += len(nameBytes)
	return unsafe.String(unsafe.SliceData(nameBytes), len(nameBytes))
}

func (builder *completeOSDirectoryBuilder) appendRecord(record completeOSDirectoryRecord) {
	if len(builder.records) == cap(builder.records) {
		builder.records = make([]completeOSDirectoryRecord, 0, directoryRecordChunkSize)
		builder.recordChunks = append(builder.recordChunks, builder.records)
	}
	builder.records = append(builder.records, record)
	builder.recordChunks[len(builder.recordChunks)-1] = builder.records
	builder.rowCount++
	if record.attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
		builder.directoryCount++
	}
}

func appendUTF16DirectoryName(destination []byte, source []uint16) []byte {
	for index := 0; index < len(source); index++ {
		unit := uint32(source[index])
		switch {
		case unit < 0x80:
			destination = append(destination, byte(unit))
		case unit < 0x800:
			destination = append(destination,
				byte(0xc0|(unit>>6)),
				byte(0x80|(unit&0x3f)))
		case unit >= 0xd800 && unit <= 0xdbff && index+1 < len(source):
			low := uint32(source[index+1])
			if low < 0xdc00 || low > 0xdfff {
				destination = append(destination, 0xef, 0xbf, 0xbd)
				continue
			}
			index++
			codePoint := uint32(0x10000) + (unit-0xd800)<<10 + low - 0xdc00
			destination = append(destination,
				byte(0xf0|(codePoint>>18)),
				byte(0x80|((codePoint>>12)&0x3f)),
				byte(0x80|((codePoint>>6)&0x3f)),
				byte(0x80|(codePoint&0x3f)))
		case unit >= 0xd800 && unit <= 0xdfff:
			destination = append(destination, 0xef, 0xbf, 0xbd)
		default:
			destination = append(destination,
				byte(0xe0|(unit>>12)),
				byte(0x80|((unit>>6)&0x3f)),
				byte(0x80|(unit&0x3f)))
		}
	}
	return destination
}

func completeOSDirectoryItem(record completeOSDirectoryRecord) VFSItem {
	attributes := record.attributes
	return VFSItem{
		Name:         record.name,
		Size:         record.size,
		SizeKnown:    true,
		IsDir:        attributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0,
		MTime:        time.Unix(0, record.modifiedTimeNs),
		IsHidden:     attributes&windows.FILE_ATTRIBUTE_HIDDEN != 0,
		IsSymlink:    attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0,
		PhysicalSize: record.physicalSize,
		WinAttrs:     attributes,
	}
}

func materializeCompleteOSDirectoryBase(builder *completeOSDirectoryBuilder) []VFSItem {
	items := make([]VFSItem, 0, builder.rowCount)
	for _, records := range builder.recordChunks {
		for _, record := range records {
			items = append(items, completeOSDirectoryItem(record))
		}
	}
	// VFSItem.Name now retains every referenced chunk. Keep the builder itself
	// live through the complete copy so the unsafe zero-copy strings cannot
	// outlive their backing storage during materialization.
	runtime.KeepAlive(builder)
	return items
}

func materializeCompleteOSDirectoryPreview(
	builder *completeOSDirectoryBuilder,
	limit int,
) []VFSItem {
	items := make([]VFSItem, 0, min(limit, builder.directoryCount))
	for _, records := range builder.recordChunks {
		for _, record := range records {
			if record.attributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
				continue
			}
			items = append(items, completeOSDirectoryItem(record))
			if len(items) == limit {
				runtime.KeepAlive(builder)
				return items
			}
		}
	}
	runtime.KeepAlive(builder)
	return items
}

// readCompleteOSDirectoryBase handles the fast local Windows path. handled is
// false when the optimized query cannot be used, allowing ReadDirPhased to
// retain its existing junction, privilege and portable fallbacks.
func readCompleteOSDirectoryBase(ctx context.Context, path string) (items []VFSItem, handled bool, err error) {
	return readCompleteOSDirectoryBasePhased(ctx, path, nil)
}

func readCompleteOSDirectoryBasePhased(
	ctx context.Context,
	path string,
	onPreview func([]VFSItem),
) (items []VFSItem, handled bool, err error) {
	return readCompleteOSDirectoryBaseWithQuery(
		ctx, path, directoryQueryBufferSize, queryCompleteOSDirectoryWin32, onPreview, nil)
}

func readCompleteOSDirectoryBaseWithBuffer(
	ctx context.Context,
	path string,
	bufferSize int,
	onStats func(completeOSDirectoryReadStats),
) (items []VFSItem, handled bool, err error) {
	return readCompleteOSDirectoryBaseWithQuery(
		ctx, path, bufferSize, queryCompleteOSDirectoryWin32, nil, onStats)
}

func readCompleteOSDirectoryBaseWithQuery(
	ctx context.Context,
	path string,
	bufferSize int,
	query completeOSDirectoryQuery,
	onPreview func([]VFSItem),
	onStats func(completeOSDirectoryReadStats),
) (items []VFSItem, handled bool, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, true, ctxErr
	}
	if bufferSize < int(unsafe.Sizeof(fileFullDirectoryInfo{})) {
		return nil, false, fmt.Errorf("directory query buffer is too small: %d", bufferSize)
	}
	pathUTF16, pathErr := windows.UTF16PtrFromString(path)
	if pathErr != nil {
		return nil, false, fmt.Errorf("encode directory path: %w", pathErr)
	}
	handle, openErr := windows.CreateFile(
		pathUTF16,
		windows.FILE_LIST_DIRECTORY|windows.FILE_READ_ATTRIBUTES,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS,
		0,
	)
	if openErr != nil {
		return nil, false, fmt.Errorf("open directory: %w", openErr)
	}
	defer windows.CloseHandle(handle)

	// Keep the first byte naturally machine-word aligned; directory info
	// records contain 64-bit fields and the Windows API rejects misaligned
	// output storage on some builds.
	bufferStorage := make([]uint64, (bufferSize+7)/8)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&bufferStorage[0])), bufferSize)
	builder := completeOSDirectoryBuilder{}
	tracePhases := OSVFSSetPathBenchmarkHook != nil || onStats != nil
	var queryDuration time.Duration
	var parseDuration time.Duration
	queryCount := 0
	restartScan := true
	previewPublished := false
	for {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, true, ctxErr
		}
		queryStarted := time.Time{}
		if tracePhases {
			queryStarted = time.Now()
		}
		queryErr := query(handle, restartScan, buffer)
		if tracePhases {
			queryDuration += time.Since(queryStarted)
			queryCount++
		}
		runtime.KeepAlive(buffer)
		if queryErr != nil {
			if queryErr == windows.ERROR_NO_MORE_FILES ||
				queryErr == windows.ERROR_FILE_NOT_FOUND {
				materializeStarted := time.Time{}
				if tracePhases {
					materializeStarted = time.Now()
				}
				items = materializeCompleteOSDirectoryBase(&builder)
				if tracePhases {
					parseDuration += time.Since(materializeStarted)
				}
				stats := completeOSDirectoryReadStats{
					Rows:          len(items),
					QueryCount:    queryCount,
					QueryDuration: queryDuration,
					ParseDuration: parseDuration,
					NameBytes:     builder.nameBytes,
				}
				if onStats != nil {
					onStats(stats)
				}
				if tracePhases {
					osVFSSetPathBenchmarkEvent(
						"osvfs.readdir_fast.end",
						"path", path,
						"rows", stats.Rows,
						"queryCount", stats.QueryCount,
						"queryNs", stats.QueryDuration.Nanoseconds(),
						"parseNs", stats.ParseDuration.Nanoseconds(),
						"bufferBytes", len(buffer))
				}
				return items, true, nil
			}
			// Some non-local filesystems reject the directory information
			// classes even though CreateFile succeeds. Let the portable reader
			// preserve its existing behavior in that case.
			return nil, false, fmt.Errorf("query directory: %w", queryErr)
		}
		restartScan = false

		parseStarted := time.Time{}
		if tracePhases {
			parseStarted = time.Now()
		}
		for offset := 0; ; {
			if offset < 0 || offset+int(unsafe.Offsetof(fileFullDirectoryInfo{}.FileName)) > len(buffer) {
				return nil, false, fmt.Errorf("invalid directory record offset %d", offset)
			}
			info := (*fileFullDirectoryInfo)(unsafe.Pointer(&buffer[offset]))
			nameBytes := int(info.FileNameLength)
			nameOffset := offset + int(unsafe.Offsetof(info.FileName))
			if nameBytes < 0 || nameBytes&1 != 0 || nameOffset+nameBytes > len(buffer) {
				return nil, false, fmt.Errorf(
					"invalid directory record name at offset %d: bytes=%d", offset, nameBytes)
			}
			nameUnits := unsafe.Slice(&info.FileName[0], nameBytes/2)
			isDotEntry := len(nameUnits) == 1 && nameUnits[0] == '.'
			isDotDotEntry := len(nameUnits) == 2 && nameUnits[0] == '.' && nameUnits[1] == '.'
			if !isDotEntry && !isDotDotEntry {
				name := builder.appendName(nameUnits)
				builder.appendRecord(completeOSDirectoryRecord{
					name:           name,
					attributes:     info.FileAttributes,
					size:           info.EndOfFile,
					physicalSize:   info.AllocationSize,
					modifiedTimeNs: info.LastWriteTime.Nanoseconds(),
				})
			}

			if info.NextEntryOffset == 0 {
				break
			}
			next := offset + int(info.NextEntryOffset)
			if next <= offset || next >= len(buffer) {
				return nil, false, fmt.Errorf(
					"invalid next directory record offset %d after %d", next, offset)
			}
			offset = next
		}
		if !previewPublished && onPreview != nil && builder.directoryCount >= directoryPreviewLimit {
			onPreview(materializeCompleteOSDirectoryPreview(&builder, directoryPreviewLimit))
			previewPublished = true
		}
		if tracePhases {
			parseDuration += time.Since(parseStarted)
		}
	}
}
