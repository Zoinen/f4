package androidfs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInstalledADBCommandHasNoConsole(t *testing.T) {
	if os.Getenv("F4_TEST_ADB_CONSOLE_CHILD") == "1" {
		getConsoleWindow := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetConsoleWindow")
		window, _, _ := getConsoleWindow.Call()
		fmt.Printf("console=%d", window)
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := installedADBCommand(context.Background(), executable, "-test.run=^TestInstalledADBCommandHasNoConsole$")
	command.Env = append(os.Environ(), "F4_TEST_ADB_CONSOLE_CHILD=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("background command: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != "console=0" {
		t.Fatalf("background command allocated a console: %s", output)
	}
}
