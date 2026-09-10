package history_test

import (
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/history"
)

// Exercise the production path policy without reversing the package dependency.
func init() { history.PathIdentity = fileops.FolderHistoryPathIdentity }
