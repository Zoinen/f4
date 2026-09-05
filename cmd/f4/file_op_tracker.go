package main

import (
	"github.com/unxed/f4/vfs"
	"sync"
)

// FileOpTracker aggregates statistics from a running file operation.
// It is thread-safe and provides normalized progress data for the UI.
type FileOpTracker struct {
	mu sync.RWMutex

	total     vfs.OpStats // Constant results from pre-scan
	processed vfs.OpStats // Accumulated results of finished items

	currentFileName  string
	currentFileBytes int64
	currentFileSize  int64
	currentSizeKnown bool
	// Some remote providers can report a percentage before the materialized
	// reader reveals its size (native exports, WebDAV without Content-Length).
	currentFilePercent  int
	currentPercentKnown bool

	completedBytes int64 // Sum of sizes of fully copied files
}

func NewFileOpTracker(total vfs.OpStats) *FileOpTracker {
	return &FileOpTracker{
		total: total,
	}
}

// StartFile marks the beginning of a new file transfer.
func (t *FileOpTracker) StartFile(name string, size int64) {
	t.StartFileKnown(name, size, true)
}

// StartFileKnown distinguishes a genuine zero-byte file from a provider
// placeholder whose size will only be known after Open.
func (t *FileOpTracker) StartFileKnown(name string, size int64, sizeKnown bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.currentFileName = name
	t.currentFileSize = size
	t.currentSizeKnown = sizeKnown || size > 0
	t.currentFileBytes = 0
	t.currentFilePercent = 0
	t.currentPercentKnown = false
}

// SetCurrentSize supplies a size learned after Open. It preserves any earlier
// provider percentage, allowing an unknown-length network materialization to
// transition to byte-based progress without jumping backwards.
func (t *FileOpTracker) SetCurrentSize(size int64) {
	if size <= 0 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.currentFileName == "" || t.currentFileSize > 0 {
		return
	}
	t.currentFileSize = size
	t.currentSizeKnown = true
	if t.currentPercentKnown {
		t.currentFileBytes = size * int64(t.currentFilePercent) / 100
	}
}

// BytesBetweenPercents converts progress within one provider phase to bytes
// for speed accounting. It does not alter logical file progress, so a second
// Downloading/Uploading phase can report throughput without moving total
// progress backwards.
func (t *FileOpTracker) BytesBetweenPercents(previous, current int) int {
	if current <= previous {
		return 0
	}
	t.mu.RLock()
	size := t.currentFileSize
	t.mu.RUnlock()
	if size <= 0 {
		return 0
	}
	delta := size*int64(current)/100 - size*int64(previous)/100
	if delta <= 0 {
		return 0
	}
	if delta > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(delta)
}

// UpdateBytes records progress within the current file.
func (t *FileOpTracker) UpdateBytes(n int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.currentFileBytes += int64(n)
	// Safety: don't let current file progress exceed 100%
	if t.currentFileSize > 0 && t.currentFileBytes > t.currentFileSize {
		t.currentFileBytes = t.currentFileSize
	}
}

// SetCurrentPercent replaces the staged byte count with provider-reported
// transfer progress. It returns the positive byte delta for speed accounting.
func (t *FileOpTracker) SetCurrentPercent(percent int) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		percent = 100
	}
	// Retries (for example a stale Digest nonce) may start another 0..99
	// sequence. UI and aggregate byte accounting must never move backwards.
	if t.currentPercentKnown && percent < t.currentFilePercent {
		percent = t.currentFilePercent
	}
	t.currentFilePercent = percent
	t.currentPercentKnown = true
	if t.currentFileSize <= 0 {
		return 0
	}
	previous := t.currentFileBytes
	target := t.currentFileSize * int64(percent) / 100
	if target < previous {
		target = previous
	}
	t.currentFileBytes = target
	delta := t.currentFileBytes - previous
	if delta <= 0 {
		return 0
	}
	if delta > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(delta)
}

// FileDone completes the current file and updates global counters.
func (t *FileOpTracker) FileDone() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.processed.Files++
	t.completedBytes += t.currentFileSize

	t.currentFileName = ""
	t.currentFileBytes = 0
	t.currentFileSize = 0
	t.currentSizeKnown = false
	t.currentFilePercent = 0
	t.currentPercentKnown = false
}

// FileSkipped records that a file was bypassed (e.g. user chose Skip)
func (t *FileOpTracker) FileSkipped() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.processed.Files++
	t.completedBytes += t.currentFileSize

	t.currentFileName = ""
	t.currentFileBytes = 0
	t.currentFileSize = 0
	t.currentSizeKnown = false
	t.currentFilePercent = 0
	t.currentPercentKnown = false
}

// DirDone records a successfully created/processed directory.
func (t *FileOpTracker) DirDone() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.processed.Dirs++
}

// GetProgress returns data for both progress bars.
func (t *FileOpTracker) GetProgress() (filePct, totalPct int, currentName string) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	currentName = t.currentFileName

	// 1. Current File Percentage
	if t.currentFileSize > 0 {
		filePct = int((t.currentFileBytes * 100) / t.currentFileSize)
	} else if t.currentPercentKnown {
		filePct = t.currentFilePercent
	} else if t.currentFileName != "" && t.currentSizeKnown {
		// A zero-byte stream may still spend significant time authenticating or
		// committing remotely. Only FileDone (or explicit provider percentage)
		// proves completion.
		filePct = 0
	}

	// 2. Total Percentage
	if t.total.Bytes > 0 && t.total.UnknownSizeFiles == 0 {
		currentTotalBytes := t.completedBytes + t.currentFileBytes
		totalPct = int((currentTotalBytes * 100) / t.total.Bytes)
	} else {
		// FALLBACK: If total volume is 0 (e.g. copying empty folders or 0-byte files),
		// calculate progress based on item count to avoid a "stuck" bar.
		totalItems := t.total.Files + t.total.Dirs
		processedItems := t.processed.Files + t.processed.Dirs
		if totalItems > 0 {
			progressUnits := processedItems * 100
			if t.currentFileName != "" {
				progressUnits += int64(filePct)
			}
			totalPct = int(progressUnits / totalItems)
		} else {
			totalPct = 100
		}
	}

	// Safety clamp
	if totalPct > 100 {
		totalPct = 100
	}
	// Reading or writing the last byte is not the commit point: Close can still
	// fail while flushing a local file or waiting for a remote server response.
	// Reserve total 100% until FileDone records a confirmed close/commit.
	totalItems := t.total.Files + t.total.Dirs
	processedItems := t.processed.Files + t.processed.Dirs
	if totalPct >= 100 && totalItems > 0 && processedItems < totalItems {
		totalPct = 99
	}
	return
}

// GetStats returns the raw accumulated statistics.
func (t *FileOpTracker) GetStats() (processed vfs.OpStats, total vfs.OpStats) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	processed = t.processed
	processed.Bytes = t.completedBytes + t.currentFileBytes
	return processed, t.total
}
