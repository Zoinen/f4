package afcproto

import "time"

type Mode uint64

const (
	ModeReadOnly Mode = iota + 1
	ModeReadWriteCreate
	ModeWriteOnlyCreateTruncate
	ModeReadWriteCreateTruncate
	ModeWriteOnlyCreateAppend
	ModeReadWriteCreateAppend
)

func (m Mode) valid() bool { return m >= ModeReadOnly && m <= ModeReadWriteCreateAppend }

func (m Mode) truncates() bool {
	return m == ModeWriteOnlyCreateTruncate || m == ModeReadWriteCreateTruncate
}

func (m Mode) creates() bool { return m != ModeReadOnly }

func (m Mode) appends() bool {
	return m == ModeWriteOnlyCreateAppend || m == ModeReadWriteCreateAppend
}

type FileType string

const (
	TypeDirectory FileType = "S_IFDIR"
	TypeRegular   FileType = "S_IFREG"
	TypeSymlink   FileType = "S_IFLNK"
)

type FileInfo struct {
	Name       string
	Type       FileType
	Mode       uint32
	Size       int64
	ModTime    time.Time
	BirthTime  time.Time
	LinkTarget string
	Values     map[string]string
}

func (i FileInfo) IsDir() bool     { return i.Type == TypeDirectory }
func (i FileInfo) IsSymlink() bool { return i.Type == TypeSymlink }

type DeviceInfo struct {
	Model      string
	TotalBytes uint64
	FreeBytes  uint64
	BlockSize  uint64
	Values     map[string]string
}
