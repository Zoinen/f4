package update

import (
	"fmt"
	"strings"
)

const (
	HelperFlag        = "--update-helper"
	RestoreHelperFlag = "--update-restore-helper"
)

// ParseHelperArgs extracts the private command line used by the
// elevated updater. It deliberately accepts the flag only when it is the
// complete helper invocation, so a normal f4 launch cannot accidentally skip
// UI startup because a similarly named user argument was supplied.
func ParseHelperArgs(args []string) (archivePath, archiveKind string, found bool, err error) {
	for i, arg := range args {
		if arg != HelperFlag {
			continue
		}
		if len(args) != i+3 {
			return "", "", true, fmt.Errorf("%s requires archive path and archive kind", HelperFlag)
		}
		var helperArgs [2]string
		copy(helperArgs[:], args[i+1:])
		archivePath = strings.TrimSpace(helperArgs[0])
		archiveKind = strings.TrimSpace(helperArgs[1])
		if archivePath == "" || archiveKind == "" {
			return "", "", true, fmt.Errorf("%s requires non-empty archive path and archive kind", HelperFlag)
		}
		return archivePath, archiveKind, true, nil
	}
	return "", "", false, nil
}

// ParseRestoreHelperArgs extracts the private command line used to restore a
// failed update from an elevated helper process.
func ParseRestoreHelperArgs(args []string) (backupPath string, found bool, err error) {
	for i, arg := range args {
		if arg != RestoreHelperFlag {
			continue
		}
		if len(args) != i+2 {
			return "", true, fmt.Errorf("%s requires backup path", RestoreHelperFlag)
		}
		var helperArgs [1]string
		copy(helperArgs[:], args[i+1:])
		backupPath = strings.TrimSpace(helperArgs[0])
		if backupPath == "" {
			return "", true, fmt.Errorf("%s requires non-empty backup path", RestoreHelperFlag)
		}
		return backupPath, true, nil
	}
	return "", false, nil
}
