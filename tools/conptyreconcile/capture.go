package main

import (
	"fmt"
	"sync"
	"time"
)

type streamKind uint8

const (
	streamInput streamKind = iota
	streamObservedOutput
	streamResize
)

type streamEvent struct {
	Kind         streamKind `json:"kind"`
	Bytes        []byte     `json:"bytes,omitempty"`
	OutputOffset int        `json:"output_offset,omitempty"`
	Width        int        `json:"width,omitempty"`
	Height       int        `json:"height,omitempty"`
	Cause        string     `json:"cause,omitempty"`
}

type capture struct {
	Seed       int64         `json:"seed,omitempty"`
	HostPath   string        `json:"host_path,omitempty"`
	HostSHA256 string        `json:"host_sha256,omitempty"`
	Events     []streamEvent `json:"events"`
}

func (c *capture) append(kind streamKind, data []byte, cause string) {
	c.Events = append(c.Events, streamEvent{Kind: kind, Bytes: append([]byte(nil), data...), Cause: cause})
}

type hostCaptureRecorder struct {
	mu          sync.Mutex
	capture     capture
	width       int
	height      int
	outputBytes int
}

func newHostCaptureRecorder(_ int64, width, height int) *hostCaptureRecorder {
	return &hostCaptureRecorder{width: width, height: height}
}

func (r *hostCaptureRecorder) append(kind streamKind, data []byte, cause string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capture.append(kind, data, cause)
	if kind == streamObservedOutput {
		r.outputBytes += len(data)
	}
}

func (r *hostCaptureRecorder) resize(width, height int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.width, r.height = width, height
	r.capture.Events = append(r.capture.Events, streamEvent{Kind: streamResize, OutputOffset: r.outputBytes, Width: width, Height: height, Cause: "pinned-host-resize"})
}

func (r *hostCaptureRecorder) snapshot() capture {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := capture{Events: make([]streamEvent, len(r.capture.Events))}
	copy(result.Events, r.capture.Events)
	return result
}

// observedOutputSnapshot returns only bytes read from the host output pipe so
// a native session can wait for a semantic marker without using a sleep as a
// synchronization primitive.
func (r *hostCaptureRecorder) observedOutputSnapshot() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	var result []byte
	for _, event := range r.capture.Events {
		if event.Kind == streamObservedOutput {
			result = append(result, event.Bytes...)
		}
	}
	return result
}

func (r *hostCaptureRecorder) outputOffset() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.outputBytes
}

// waitOutputQuiescent gives the pinned renderer a bounded opportunity to
// flush after the child exits. Child exit alone does not imply that ConPTY has
// finished emitting its final paint; closing the pseudoconsole first can
// truncate the tail or leave the reader blocked on the host-owned pipe.
func waitOutputQuiescent(recorder *hostCaptureRecorder, quiet, timeout time.Duration) error {
	if recorder == nil {
		return fmt.Errorf("output recorder is nil")
	}
	if quiet <= 0 {
		quiet = 300 * time.Millisecond
	}
	if timeout < quiet {
		timeout = quiet
	}
	deadline := time.Now().Add(timeout)
	last := recorder.outputOffset()
	stable := time.Duration(0)
	const interval = 100 * time.Millisecond
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		now := recorder.outputOffset()
		if now == last {
			stable += interval
			if stable >= quiet {
				return nil
			}
		} else {
			last = now
			stable = 0
		}
	}
	return fmt.Errorf("pinned output did not quiesce within %s", timeout)
}
