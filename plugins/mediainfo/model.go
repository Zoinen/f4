// Package mediainfo provides a bounded, pure-Go media metadata analyzer.
//
// It intentionally exposes a container-neutral report.  Parsers may leave
// fields at their zero value when a format does not carry the information.
package mediainfo

import "time"

// Mode selects how much of a file the analyzer may inspect.
type Mode uint8

const (
	// ModeFast reads structural metadata only and is suitable for Quick View.
	ModeFast Mode = iota
	// ModeDetailed may scan bounded indexes, frame headers and subtitle text.
	ModeDetailed
)

// StreamKind identifies the logical kind of a stream.
type StreamKind string

const (
	StreamVideo StreamKind = "Video"
	StreamAudio StreamKind = "Audio"
	StreamText  StreamKind = "Text"
	StreamImage StreamKind = "Image"
	StreamMenu  StreamKind = "Menu"
)

// Field stores a repeatable metadata value. Target is empty for file-level
// fields and contains a stream ID for stream-specific fields.
type Field struct {
	Target string
	Name   string
	Value  string
}

// Warning describes non-fatal damage, unsupported details, or a resource cap.
type Warning struct {
	Code    string
	Message string
	Offset  int64
}

// General describes the file/container as a whole.
type General struct {
	FileName                string
	FileSize                int64
	Format                  string
	FormatProfile           string
	CodecID                 string
	MIME                    string
	CompatibleBrands        []string
	Duration                time.Duration
	DurationEstimated       bool
	OverallBitRate          int64
	OverallBitRateEstimated bool
	FrameCount              int64
	FrameRate               float64
	MuxingApp               string
	WritingApp              string
	EncodedDate             *time.Time
	TaggedDate              *time.Time
	Streamable              *bool
}

// Video contains video-specific stream properties.
type Video struct {
	Width                   int
	Height                  int
	DisplayWidth            int
	DisplayHeight           int
	PixelAspectRatio        float64
	DisplayAspectRatio      float64
	Rotation                float64
	BitDepth                int
	ColorSpace              string
	ChromaSubsampling       string
	ScanType                string
	ScanOrder               string
	ColorRange              string
	ColorPrimaries          string
	TransferCharacteristics string
	MatrixCoefficients      string
	HDRFormat               string
	MaxCLL                  int
	MaxFALL                 int
}

// Audio contains audio-specific stream properties.
type Audio struct {
	Channels        int
	ChannelLayout   string
	SampleRate      int
	BitDepth        int
	CompressionMode string
	Delay           time.Duration
	EncoderPadding  int64
}

// Text contains subtitle/text stream properties.
type Text struct {
	FormatVersion string
	Encoding      string
	CueCount      int64
	FirstCue      time.Duration
	LastCue       time.Duration
	StyleCount    int
}

// Image contains still/animated image properties.
type Image struct {
	Width             int
	Height            int
	BitDepth          int
	ColorModel        string
	Compression       string
	FrameCount        int
	Animated          bool
	AnimationDuration time.Duration
	Orientation       int
	DPIX              float64
	DPIY              float64
	CameraMake        string
	CameraModel       string
	LensModel         string
	TakenAt           *time.Time
	Latitude          *float64
	Longitude         *float64
	GPSAltitude       *float64
	// EXIF contains useful camera fields which do not need dedicated typed
	// accessors. Names are stable English keys and values are display-ready.
	EXIF []Field
}

// Stream describes one elementary stream.
type Stream struct {
	Index              int
	ID                 string
	Kind               StreamKind
	Format             string
	Profile            string
	Level              string
	CodecID            string
	CodecName          string
	Title              string
	Language           string
	Duration           time.Duration
	DurationEstimated  bool
	BitRate            int64
	BitRateEstimated   bool
	BitRateMode        string
	FrameCount         int64
	FrameRate          float64
	FrameRateEstimated bool
	Default            *bool
	Forced             *bool
	Encrypted          *bool
	Video              *Video
	Audio              *Audio
	Text               *Text
	Image              *Image
	Tags               []Field
}

// Chapter describes a chapter/menu entry.
type Chapter struct {
	ID       string
	Start    time.Duration
	End      time.Duration
	Title    string
	Language string
}

// Report is the normalized result returned by Analyze.
type Report struct {
	Mode      Mode
	General   General
	Streams   []Stream
	Chapters  []Chapter
	Tags      []Field
	Warnings  []Warning
	Truncated bool
}
