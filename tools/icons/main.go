// Command icons regenerates every platform icon from assets/icon/f4.svg and
// optional size-specific assets/icon/f4-N.svg overrides.
//
// Run it from anywhere in the repository with:
//
//	go generate
//
// or directly with:
//
//	go -C tools/icons run .
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

const winresVersion = "v0.3.3"

var sizes = []int{16, 24, 28, 30, 32, 36, 42, 48, 56, 64, 128, 256, 512, 1024}

var windowsSizes = []int{16, 24, 28, 30, 32, 36, 42, 48, 56, 64, 128, 256}

func main() {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	check(err)
	iconDir := filepath.Join(root, "assets", "icon")
	outDir := filepath.Join(root, "assets", "icon", "generated")
	check(os.MkdirAll(outDir, 0o755))

	pngs := make(map[int][]byte, len(sizes))
	for _, size := range sizes {
		source, err := sourceForSize(iconDir, size)
		check(err)
		data, err := renderPNG(source, size)
		check(err)
		pngs[size] = data
		check(writeFile(filepath.Join(outDir, fmt.Sprintf("f4-%d.png", size)), data))
	}

	check(writeFile(filepath.Join(outDir, "f4.ico"), makeICO(pngs)))
	check(writeFile(filepath.Join(outDir, "f4.icns"), makeICNS(pngs)))
	check(makeWindowsResources(root, filepath.Join(outDir, "f4.ico")))
	fmt.Println("generated platform icon resources")
}

func sourceForSize(iconDir string, size int) (string, error) {
	specific := filepath.Join(iconDir, fmt.Sprintf("f4-%d.svg", size))
	if _, err := os.Stat(specific); err == nil {
		return specific, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect size-specific SVG %q: %w", specific, err)
	}
	return filepath.Join(iconDir, "f4.svg"), nil
}

func renderPNG(source string, size int) ([]byte, error) {
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	icon, err := oksvg.ReadIconStream(f)
	if err != nil {
		return nil, fmt.Errorf("parse SVG: %w", err)
	}
	icon.SetTarget(0, 0, float64(size), float64(size))
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, img, img.Bounds())
	dasher := rasterx.NewDasher(size, size, scanner)
	icon.Draw(dasher, 1)

	var out bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestCompression}
	if err := encoder.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func makeICO(images map[int][]byte) []byte {
	// ICO supports PNG payloads. A zero width/height byte means 256 pixels.
	headerSize := 6 + 16*len(windowsSizes)
	var out bytes.Buffer
	writeLE(&out, uint16(0))
	writeLE(&out, uint16(1))
	writeLE(&out, uint16(len(windowsSizes)))
	offset := headerSize
	for _, size := range windowsSizes {
		dimension := byte(size)
		if size == 256 {
			dimension = 0
		}
		out.WriteByte(dimension)
		out.WriteByte(dimension)
		out.WriteByte(0)
		out.WriteByte(0)
		writeLE(&out, uint16(1))
		writeLE(&out, uint16(32))
		writeLE(&out, uint32(len(images[size])))
		writeLE(&out, uint32(offset))
		offset += len(images[size])
	}
	for _, size := range windowsSizes {
		out.Write(images[size])
	}
	return out.Bytes()
}

func makeICNS(images map[int][]byte) []byte {
	// Modern macOS accepts PNG-compressed icon elements in an ICNS container.
	types := []struct {
		name string
		size int
	}{
		{"icp4", 16}, {"icp5", 32}, {"icp6", 64},
		{"ic07", 128}, {"ic08", 256}, {"ic09", 512}, {"ic10", 1024},
		{"ic11", 32}, {"ic12", 64}, {"ic13", 256}, {"ic14", 512},
	}
	total := 8
	for _, entry := range types {
		total += 8 + len(images[entry.size])
	}
	var out bytes.Buffer
	out.WriteString("icns")
	writeBE(&out, uint32(total))
	for _, entry := range types {
		out.WriteString(entry.name)
		writeBE(&out, uint32(8+len(images[entry.size])))
		out.Write(images[entry.size])
	}
	return out.Bytes()
}

func makeWindowsResources(root, icon string) error {
	args := []string{
		"run", "github.com/tc-hib/go-winres@" + winresVersion,
		"simply",
		"--arch", "amd64,arm64",
		"--out", "rsrc",
		"--manifest", "gui",
		"--product-name", "f4",
		"--file-description", "f4 file manager",
		"--original-filename", "f4.exe",
		"--icon", icon,
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("generate Windows resources: %w", err)
	}
	return nil
}

func writeFile(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return nil
	}
	return os.WriteFile(path, data, 0o644)
}

func writeLE(w io.Writer, value any) {
	check(binary.Write(w, binary.LittleEndian, value))
}

func writeBE(w io.Writer, value any) {
	check(binary.Write(w, binary.BigEndian, value))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "icon generator:", err)
		os.Exit(1)
	}
}
