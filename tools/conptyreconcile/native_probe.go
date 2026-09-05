package main

import "time"

// probeResize and nativeProbeSession are shared by the Windows implementation
// and the non-Windows compile-only stubs. Keeping the report shape
// platform-neutral lets the standalone module be checked on the Linux lint
// runner while the actual host session remains Windows-only.
type probeResize struct {
	At     time.Time `json:"at"`
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Error  string    `json:"error,omitempty"`
}

type nativeProbeSession struct {
	InitialWidth      int                   `json:"initial_width"`
	InitialHeight     int                   `json:"initial_height"`
	Command           string                `json:"command"`
	ExpectedInput     []byte                `json:"expected_input"`
	HostCommand       string                `json:"host_command"`
	HostPID           uint32                `json:"host_pid"`
	ChildPID          uint32                `json:"child_pid"`
	HostProcess       pinnedHostIdentity    `json:"host_process"`
	StartedAt         time.Time             `json:"started_at"`
	FinishedAt        time.Time             `json:"finished_at"`
	ExitCode          uint32                `json:"exit_code"`
	Resizes           []probeResize         `json:"resizes"`
	Markers           []string              `json:"markers"`
	MarkerWarnings    []string              `json:"marker_warnings,omitempty"`
	RawSHA256         string                `json:"raw_sha256"`
	RawOutput         []byte                `json:"raw_output"`
	LogicalLines      []logicalLine         `json:"logical_lines"`
	RenderedLines     []renderedHistoryLine `json:"rendered_lines,omitempty"`
	Frames            []hostFrame           `json:"frames"`
	RepaintFrames     []repaintFrame        `json:"repaint_frames"`
	ResizeOffsets     []int                 `json:"resize_offsets"`
	Events            []streamEvent         `json:"events"`
	Assertions        []payloadAssertion    `json:"assertions,omitempty"`
	AssertionFailures []string              `json:"assertion_failures,omitempty"`
	Chunking          []chunkingAssertion   `json:"chunking_assertions,omitempty"`
	ConsumerChecks    []seedConsumerCheck   `json:"consumer_checks,omitempty"`
	ChildExited       bool                  `json:"child_exited"`
	HostExited        bool                  `json:"host_exited"`
	HandlesClosed     bool                  `json:"handles_closed"`
	Error             string                `json:"error,omitempty"`
}
