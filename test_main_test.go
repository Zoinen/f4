package main

import (
	"github.com/unxed/f4/vfs"
	//"github.com/unxed/vtui"
	"os"
	"testing"
)

func TestMain(m *testing.M) {
	vfs.InitSudoClient("/usr/bin/f4", "")

	tmpDir, err := os.MkdirTemp("", "f4-test-config-*")
	if err == nil {
		defer os.RemoveAll(tmpDir)
		os.Setenv("XDG_CONFIG_HOME", tmpDir)
		os.Setenv("APPDATA", tmpDir)
		resetConfigDirForTest()
	}

	result := m.Run()
	if result != 0 {
		// disabled for now
		//vtui.DumpLogsToFile("_failed_tests_f4.log")
	}
	os.Exit(result)
}
