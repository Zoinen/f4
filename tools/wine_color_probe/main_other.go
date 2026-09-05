//go:build !windows

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Println("wine_color_probe is designed to run under Wine and must be compiled for Windows.")
	fmt.Println("Please build with: GOOS=windows GOARCH=amd64 go build -o wine_color_probe.exe ./tools/wine_color_probe")
	os.Exit(1)
}
