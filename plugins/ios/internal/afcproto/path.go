package afcproto

import (
	"io/fs"
	"path"
	"strings"
)

func cleanPath(op, value string, mutation bool) (string, error) {
	if value == "" || len(value) > maxPathBytes || strings.IndexByte(value, 0) >= 0 {
		return "", &fs.PathError{Op: op, Path: value, Err: ErrUnsafePath}
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".." {
			return "", &fs.PathError{Op: op, Path: value, Err: ErrUnsafePath}
		}
	}
	clean := path.Clean(value)
	if mutation && (clean == "." || clean == "/") {
		return "", &fs.PathError{Op: op, Path: value, Err: ErrUnsafePath}
	}
	return clean, nil
}

func pathBytes(value string) []byte {
	return append([]byte(value), 0)
}

func validEntryName(name string) bool {
	return name != "" && name != "." && name != ".." &&
		len(name) <= maxPathBytes && !strings.ContainsAny(name, "/\x00")
}
