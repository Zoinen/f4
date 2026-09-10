package archive

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const sfxProbeLimit = 64 << 20

type sfxSignature struct {
	magic  []byte
	format string
	suffix string
}

var sfxSignatures = []sfxSignature{
	{magic: []byte("PK\x03\x04"), format: "zip", suffix: ".zip"},
	{magic: []byte("PK\x05\x06"), format: "zip", suffix: ".zip"},
	{magic: []byte("7z\xBC\xAF\x27\x1C"), format: "fallback", suffix: ".7z"},
	{magic: []byte("Rar!\x1A\x07\x00"), format: "fallback", suffix: ".rar"},
	{magic: []byte("Rar!\x1A\x07\x01\x00"), format: "fallback", suffix: ".rar"},
}

type embeddedArchive struct {
	format string
	suffix string
	offset int64
}

func findEmbeddedArchive(filename string) (embeddedArchive, bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return embeddedArchive{}, false, err
	}
	defer func() { _ = file.Close() }()

	const chunkSize = 64 << 10
	maxMagic := 0
	for _, signature := range sfxSignatures {
		if len(signature.magic) > maxMagic {
			maxMagic = len(signature.magic)
		}
	}

	chunk := make([]byte, chunkSize)
	var carry []byte
	var scanned int64
	for scanned < sfxProbeLimit {
		want := int64(len(chunk))
		if remaining := sfxProbeLimit - scanned; remaining < want {
			want = remaining
		}
		n, readErr := file.Read(chunk[:int(want)])
		if n > 0 {
			block := make([]byte, 0, len(carry)+n)
			block = append(block, carry...)
			block = append(block, chunk[:n]...)
			blockStart := scanned - int64(len(carry))

			bestIndex := -1
			var best sfxSignature
			for _, signature := range sfxSignatures {
				index := bytes.Index(block, signature.magic)
				if index >= 0 && (bestIndex < 0 || index < bestIndex) {
					bestIndex = index
					best = signature
				}
			}
			if bestIndex >= 0 {
				return embeddedArchive{
					format: best.format,
					suffix: best.suffix,
					offset: blockStart + int64(bestIndex),
				}, true, nil
			}

			keep := maxMagic - 1
			if len(block) > keep {
				carry = append(carry[:0], block[len(block)-keep:]...)
			} else {
				carry = append(carry[:0], block...)
			}
			scanned += int64(n)
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return embeddedArchive{}, false, readErr
		}
	}

	return embeddedArchive{}, false, nil
}

type sfxBacking struct {
	path string
	dir  string
	once sync.Once
	err  error
}

func (b *sfxBacking) Close() error {
	b.once.Do(func() {
		if b.dir != "" {
			b.err = os.RemoveAll(b.dir)
		} else {
			b.err = os.Remove(b.path)
			if errors.Is(b.err, os.ErrNotExist) {
				b.err = nil
			}
		}
	})
	return b.err
}

type sfxVolume struct {
	source string
	target string
}

type sfxVolumePlan struct {
	first      string
	companions []sfxVolume
}

func sfxVolumePlanFor(filename string, embedded embeddedArchive) (sfxVolumePlan, error) {
	base := filepath.Base(filename)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	plan := sfxVolumePlan{first: stem + embedded.suffix}

	entries, err := os.ReadDir(filepath.Dir(filename))
	if err != nil {
		return sfxVolumePlan{}, err
	}

	switch strings.ToLower(embedded.suffix) {
	case ".zip":
		for volume := 1; ; volume++ {
			target := fmt.Sprintf("%s.z%02d", stem, volume)
			source := findCaseInsensitiveEntry(entries, target)
			if source == "" {
				break
			}
			plan.companions = append(plan.companions, sfxVolume{
				source: filepath.Join(filepath.Dir(filename), source),
				target: target,
			})
		}
	case ".7z":
		for volume := 2; ; volume++ {
			target := fmt.Sprintf("%s.7z.%03d", stem, volume)
			source := findCaseInsensitiveEntry(entries, target)
			if source == "" {
				break
			}
			plan.companions = append(plan.companions, sfxVolume{
				source: filepath.Join(filepath.Dir(filename), source),
				target: target,
			})
		}
		if len(plan.companions) > 0 {
			plan.first = stem + ".7z.001"
		}
	case ".rar":
		plan = planRARVolumes(filepath.Dir(filename), stem, entries)
	}

	return plan, nil
}

func findCaseInsensitiveEntry(entries []os.DirEntry, target string) string {
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), target) {
			return entry.Name()
		}
	}
	return ""
}

type rarVolumeMatch struct {
	name   string
	part   int
	width  int
	total  string
	legacy bool
}

func planRARVolumes(dir, stem string, entries []os.DirEntry) sfxVolumePlan {
	newPattern := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(stem) + `\.part([0-9]+)(?:of([0-9]+))?\.rar$`)
	legacyPattern := regexp.MustCompile(`(?i)^` + regexp.QuoteMeta(stem) + `\.r([0-9]+)$`)
	var modern []rarVolumeMatch
	var legacy []rarVolumeMatch
	for _, entry := range entries {
		name := entry.Name()
		if match := newPattern.FindStringSubmatch(name); match != nil {
			part, err := strconv.Atoi(match[1])
			if err == nil && part >= 2 {
				modern = append(modern, rarVolumeMatch{
					name:  name,
					part:  part,
					width: len(match[1]),
					total: match[2],
				})
			}
			continue
		}
		if match := legacyPattern.FindStringSubmatch(name); match != nil {
			part, err := strconv.Atoi(match[1])
			if err == nil {
				legacy = append(legacy, rarVolumeMatch{
					name:   name,
					part:   part,
					width:  len(match[1]),
					legacy: true,
				})
			}
		}
	}

	plan := sfxVolumePlan{first: stem + ".rar"}
	if len(modern) > 0 {
		sort.Slice(modern, func(i, j int) bool { return modern[i].part < modern[j].part })
		first := modern[0]
		firstPart := fmt.Sprintf("%0*d", first.width, 1)
		plan.first = stem + ".part" + firstPart
		if first.total != "" {
			plan.first += "of" + first.total
		}
		plan.first += ".rar"
		for _, volume := range modern {
			part := fmt.Sprintf("%0*d", first.width, volume.part)
			target := stem + ".part" + part
			if first.total != "" {
				target += "of" + first.total
			}
			plan.companions = append(plan.companions, sfxVolume{
				source: filepath.Join(dir, volume.name),
				target: target + ".rar",
			})
		}
		return plan
	}

	if len(legacy) > 0 {
		sort.Slice(legacy, func(i, j int) bool { return legacy[i].part < legacy[j].part })
		first := legacy[0]
		for _, volume := range legacy {
			target := stem + ".r" + fmt.Sprintf("%0*d", first.width, volume.part)
			plan.companions = append(plan.companions, sfxVolume{
				source: filepath.Join(dir, volume.name),
				target: target,
			})
		}
	}
	return plan
}

func copySFXFile(dst, source string, offset int64) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() { _ = input.Close() }()

	output, err := os.Create(dst)
	if err != nil {
		return err
	}
	removeOutput := true
	defer func() {
		_ = output.Close()
		if removeOutput {
			_ = os.Remove(dst)
		}
	}()

	if offset > 0 {
		stat, err := input.Stat()
		if err != nil {
			return err
		}
		if offset >= stat.Size() {
			return os.ErrInvalid
		}
		if _, err := io.Copy(output, io.NewSectionReader(input, offset, stat.Size()-offset)); err != nil {
			return err
		}
	} else if _, err := io.Copy(output, input); err != nil {
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	removeOutput = false
	return nil
}

func materializeEmbeddedArchive(filename string, embedded embeddedArchive) (string, io.Closer, error) {
	if embedded.offset <= 0 {
		return filename, nil, nil
	}

	plan, err := sfxVolumePlanFor(filename, embedded)
	if err != nil {
		return "", nil, err
	}
	if len(plan.companions) == 0 {
		target, err := os.CreateTemp("", "f4-sfx-*"+embedded.suffix)
		if err != nil {
			return "", nil, err
		}
		targetName := target.Name()
		if err := target.Close(); err != nil {
			_ = os.Remove(targetName)
			return "", nil, err
		}
		if err := copySFXFile(targetName, filename, embedded.offset); err != nil {
			_ = os.Remove(targetName)
			return "", nil, err
		}
		return targetName, &sfxBacking{path: targetName}, nil
	}

	dir, err := os.MkdirTemp("", "f4-sfx-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	targetName := filepath.Join(dir, plan.first)
	if err := copySFXFile(targetName, filename, embedded.offset); err != nil {
		cleanup()
		return "", nil, err
	}
	for _, volume := range plan.companions {
		if err := copySFXFile(filepath.Join(dir, volume.target), volume.source, 0); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return targetName, &sfxBacking{path: targetName, dir: dir}, nil
}
