package main

import (
	"fmt"
	"os"

	"github.com/unxed/f4/tools/hardcode"
)

// find_hardcoded prints every UI caption that is still a Go string literal.
// The same scanner is used by TestNoNewHardcodedUIStrings, which keeps CI red
// whenever a new one appears. See L10N_PLAN.md.
func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	findings, err := hardcode.Scan(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scan failed: %v\n", err)
		os.Exit(1)
	}

	for _, f := range findings {
		fmt.Println(f.String())
	}
	fmt.Printf("%d hardcoded caption(s) found in %s\n", len(findings), root)
}
