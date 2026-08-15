package vtui

import (
	"fmt"
	"os"
)

// DumpLogsToFile exports memory logs to a file if a test fails.
func DumpLogsToFile(filename string) {
	logs := GetCurrentLogs()
	if len(logs) == 0 {
		return
	}

	f, err := os.Create(filename)
	if err != nil {
		fmt.Printf("Failed to create failure log: %v\n", err)
		return
	}
	defer f.Close()

	fmt.Fprintf(f, "=== TEST FAILURE LOG DUMP ===\n")
	for _, l := range logs {
		fmt.Fprintln(f, l)
	}
	fmt.Printf("\n[!] Tests failed. Log dump saved to: %s\n", filename)
}
