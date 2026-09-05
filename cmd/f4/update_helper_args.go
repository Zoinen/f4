package main

import (
	"fmt"
	"strings"
)

const updateHelperFlag = "--update-helper"

// parseUpdateHelperArgs extracts the private command line used by the
// elevated updater. It deliberately accepts the flag only when it is the
// complete helper invocation, so a normal f4 launch cannot accidentally skip
// UI startup because a similarly named user argument was supplied.
func parseUpdateHelperArgs(args []string) (archivePath, archiveKind string, found bool, err error) {
	for i, arg := range args {
		if arg != updateHelperFlag {
			continue
		}
		if len(args) != i+3 {
			return "", "", true, fmt.Errorf("%s requires archive path and archive kind", updateHelperFlag)
		}
		var helperArgs [2]string
		copy(helperArgs[:], args[i+1:])
		archivePath = strings.TrimSpace(helperArgs[0])
		archiveKind = strings.TrimSpace(helperArgs[1])
		if archivePath == "" || archiveKind == "" {
			return "", "", true, fmt.Errorf("%s requires non-empty archive path and archive kind", updateHelperFlag)
		}
		return archivePath, archiveKind, true, nil
	}
	return "", "", false, nil
}
