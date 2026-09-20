package mediainfo

import (
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"
)

func isoCoverageProbe(t *testing.T, data []byte, mode Mode) *probe {
	t.Helper()
	p, err := newProbe(context.Background(), Source{
		Name:   "fixture.mp4",
		Size:   int64(len(data)),
		Reader: memorySource(data),
	}, DefaultOptions(mode))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestISOBoxBoundariesAndClassifiers(t *testing.T) {
	normal := make([]byte, 12)
	binary.BigEndian.PutUint32(normal[:4], 12)
	copy(normal[4:8], "free")
	p := isoCoverageProbe(t, normal, ModeFast)
	box, err := readISOBox(p, 0, int64(len(normal)))
	if err != nil {
		t.Fatal(err)
	}
	if box.typ != "free" || box.start != 0 || box.data != 8 || box.size != 12 || box.payload != 4 {
		t.Fatalf("normal ISO box = %#v", box)
	}

	extended := make([]byte, 24)
	binary.BigEndian.PutUint32(extended[:4], 1)
	copy(extended[4:8], "wide")
	binary.BigEndian.PutUint64(extended[8:16], 24)
	p = isoCoverageProbe(t, extended, ModeFast)
	box, err = readISOBox(p, 0, int64(len(extended)))
	if err != nil || box.data != 16 || box.size != 24 || box.payload != 8 {
		t.Fatalf("extended ISO box = %#v, err=%v", box, err)
	}

	toEnd := make([]byte, 12)
	copy(toEnd[4:8], "tail")
	p = isoCoverageProbe(t, toEnd, ModeFast)
	box, err = readISOBox(p, 0, int64(len(toEnd)))
	if err != nil || box.size != 12 || box.payload != 4 {
		t.Fatalf("zero-sized ISO box = %#v, err=%v", box, err)
	}

	invalid := make([]byte, 8)
	binary.BigEndian.PutUint32(invalid[:4], 4)
	copy(invalid[4:8], "bad!")
	p = isoCoverageProbe(t, invalid, ModeFast)
	if _, err := readISOBox(p, 0, int64(len(invalid))); err == nil {
		t.Fatal("undersized ISO box was accepted")
	}
	if _, err := readISOBox(p, 1, int64(len(invalid))); err != io.EOF {
		t.Fatalf("short ISO header error = %v, want io.EOF", err)
	}

	tooLarge := make([]byte, 16)
	binary.BigEndian.PutUint32(tooLarge[:4], 1)
	binary.BigEndian.PutUint64(tooLarge[8:16], ^uint64(0)>>1+1)
	copy(tooLarge[4:8], "huge")
	p = isoCoverageProbe(t, tooLarge, ModeFast)
	if _, err := readISOBox(p, 0, int64(len(tooLarge))); err == nil {
		t.Fatal("oversized extended ISO box was accepted")
	}

	for _, typ := range []string{"moov", "trak", "mdia", "minf", "stbl", "edts", "dinf", "udta", "ilst", "meta", "mvex", "moof", "traf", "mfra", "tref", "wave"} {
		if !isoContainer(typ) {
			t.Errorf("isoContainer(%q) = false", typ)
		}
	}
	for _, typ := range []string{"free", "stsd", "", "long"} {
		if isoContainer(typ) {
			t.Errorf("isoContainer(%q) = true", typ)
		}
	}
	for _, typ := range []string{"\xa9nam", "\xa9ART", "aART", "trkn", "disk", "----"} {
		if !isISOTagBox(typ) {
			t.Errorf("isISOTagBox(%q) = false", typ)
		}
	}
	for _, typ := range []string{"covr", "free", "short", ""} {
		if isISOTagBox(typ) {
			t.Errorf("isISOTagBox(%q) = true", typ)
		}
	}
}

func TestISOFileTypeMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, major, format, mime string
	}{
		{name: "M4A", major: "M4A ", mime: "audio/mp4"},
		{name: "3GPP", major: "3gp4", format: "3GPP", mime: "video/3gpp"},
		{name: "3GPP2", major: "3g2a", format: "3GPP2", mime: "video/3gpp2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := append([]byte(tc.major+"\x00\x00\x00\x00"), []byte("isommp42")...)
			p := isoCoverageProbe(t, data, ModeFast)
			parseFTYP(p, isoBox{data: 0, payload: int64(len(data))})
			if p.report.General.CodecID != tc.major || p.report.General.MIME != tc.mime {
				t.Fatalf("FTYP report = %#v", p.report.General)
			}
			if tc.format != "" && p.report.General.Format != tc.format {
				t.Fatalf("FTYP format = %q, want %q", p.report.General.Format, tc.format)
			}
			if len(p.report.General.CompatibleBrands) != 2 {
				t.Fatalf("compatible brands = %v, want two brands", p.report.General.CompatibleBrands)
			}
		})
	}
}

func TestISOMovieAndTrackHeaders(t *testing.T) {
	mvhd0 := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd0[12:16], 1000)
	binary.BigEndian.PutUint32(mvhd0[16:20], 2500)
	p := isoCoverageProbe(t, mvhd0, ModeFast)
	parseMVHD(p, isoBox{data: 0, payload: 20})
	if p.report.General.Duration != 2500*time.Millisecond {
		t.Fatalf("version 0 movie duration = %v", p.report.General.Duration)
	}

	mvhd1 := make([]byte, 32)
	mvhd1[0] = 1
	binary.BigEndian.PutUint32(mvhd1[20:24], 1000)
	binary.BigEndian.PutUint64(mvhd1[24:32], 2500)
	p = isoCoverageProbe(t, mvhd1, ModeFast)
	parseMVHD(p, isoBox{data: 0, payload: 32})
	if p.report.General.Duration != 2500*time.Millisecond {
		t.Fatalf("version 1 movie duration = %v", p.report.General.Duration)
	}

	for _, tc := range []struct {
		name string
		a, b uint32
		want float64
	}{
		{name: "90", b: 0x10000, want: 90},
		{name: "180", a: 0xffff0000, want: 180},
		{name: "270", b: 0xffff0000, want: 270},
	} {
		t.Run("rotation "+tc.name, func(t *testing.T) {
			data := make([]byte, 96)
			binary.BigEndian.PutUint32(data[12:16], 7)
			binary.BigEndian.PutUint32(data[40:44], tc.a)
			binary.BigEndian.PutUint32(data[44:48], tc.b)
			binary.BigEndian.PutUint32(data[76:80], 640<<16)
			binary.BigEndian.PutUint32(data[80:84], 480<<16)
			stream := Stream{}
			p := isoCoverageProbe(t, data, ModeFast)
			parseTKHD(p, isoBox{data: 0, payload: 96}, &stream)
			if stream.ID != "7" || stream.Video == nil || stream.Video.DisplayWidth != 640 || stream.Video.DisplayHeight != 480 || stream.Video.Rotation != tc.want {
				t.Fatalf("track header stream = %#v", stream)
			}
		})
	}

	mdhd0 := make([]byte, 22)
	binary.BigEndian.PutUint32(mdhd0[12:16], 1000)
	binary.BigEndian.PutUint32(mdhd0[16:20], 3000)
	binary.BigEndian.PutUint16(mdhd0[20:22], uint16(5<<10|14<<5|7))
	stream := Stream{}
	p = isoCoverageProbe(t, mdhd0, ModeFast)
	parseMDHD(p, isoBox{data: 0, payload: 22}, &stream)
	if stream.Duration != 3*time.Second || stream.Language != "eng" {
		t.Fatalf("version 0 media header stream = %#v", stream)
	}

	mdhd1 := make([]byte, 34)
	mdhd1[0] = 1
	binary.BigEndian.PutUint32(mdhd1[20:24], 1000)
	binary.BigEndian.PutUint64(mdhd1[24:32], 3000)
	binary.BigEndian.PutUint16(mdhd1[32:34], uint16(5<<10|14<<5|7))
	stream = Stream{}
	p = isoCoverageProbe(t, mdhd1, ModeFast)
	parseMDHD(p, isoBox{data: 0, payload: 34}, &stream)
	if stream.Duration != 3*time.Second || stream.Language != "eng" {
		t.Fatalf("version 1 media header stream = %#v", stream)
	}
}

func TestISOHandlerAndCodecMetadata(t *testing.T) {
	for _, tc := range []struct {
		typ    string
		kind   StreamKind
		format string
	}{
		{typ: "vide", kind: StreamVideo, format: "Video"},
		{typ: "soun", kind: StreamAudio, format: "Audio"},
		{typ: "text", kind: StreamText, format: "Text"},
		{typ: "pict", kind: StreamImage, format: "Image"},
		{typ: "meta", kind: StreamMenu, format: "Metadata"},
	} {
		t.Run(tc.typ, func(t *testing.T) {
			data := make([]byte, 32)
			copy(data[8:12], tc.typ)
			copy(data[24:], "Title\x00")
			stream := Stream{}
			p := isoCoverageProbe(t, data, ModeFast)
			parseHDLR(p, isoBox{data: 0, payload: 32}, &stream)
			if stream.Kind != tc.kind || stream.Format != tc.format || stream.Title != "Title" {
				t.Fatalf("handler stream = %#v", stream)
			}
		})
	}

	for _, tc := range []struct {
		id, want string
	}{
		{"avc1", "AVC"}, {"hvc1", "HEVC"}, {"av01", "AV1"}, {"vp08", "VP8"},
		{"vp09", "VP9"}, {"mp4v", "MPEG-4 Visual"}, {"jpeg", "Motion JPEG"},
		{"mp4a", "AAC"}, {"alac", "ALAC"}, {"ac-3", "AC-3"}, {"ec-3", "E-AC-3"},
		{"Opus", "Opus"}, {"fLaC", "FLAC"}, {"lpcm", "PCM"}, {"tx3g", "Timed Text"},
		{"wvtt", "WebVTT"}, {"stpp", "TTML"},
	} {
		if got := isoCodec(tc.id, StreamAudio); got != tc.want {
			t.Errorf("isoCodec(%q) = %q, want %q", tc.id, got, tc.want)
		}
	}
	if got := isoCodec("custom", StreamVideo); got != "custom" {
		t.Fatalf("unknown video codec = %q", got)
	}
	if got := isoCodec("custom", StreamAudio); got != "custom" {
		t.Fatalf("unknown audio codec = %q", got)
	}
}

func TestISOTrackTablesAndColorMetadata(t *testing.T) {
	stts := make([]byte, 16)
	binary.BigEndian.PutUint32(stts[4:8], 1)
	binary.BigEndian.PutUint32(stts[8:12], 3)
	binary.BigEndian.PutUint32(stts[12:16], 2)
	stream := Stream{Duration: 3 * time.Second}
	p := isoCoverageProbe(t, stts, ModeDetailed)
	parseSTTS(p, isoBox{data: 0, payload: 16}, &stream)
	if stream.FrameCount != 3 || stream.FrameRate != 1 {
		t.Fatalf("STTS stream = %#v", stream)
	}

	stszFixed := make([]byte, 12)
	binary.BigEndian.PutUint32(stszFixed[4:8], 100)
	binary.BigEndian.PutUint32(stszFixed[8:12], 3)
	stream = Stream{}
	p = isoCoverageProbe(t, stszFixed, ModeDetailed)
	if got := parseSTSZ(p, isoBox{data: 0, payload: 12}, &stream); got != 300 || stream.FrameCount != 3 {
		t.Fatalf("fixed STSZ = %d, stream = %#v", got, stream)
	}

	stszVariable := make([]byte, 20)
	binary.BigEndian.PutUint32(stszVariable[8:12], 2)
	binary.BigEndian.PutUint32(stszVariable[12:16], 4)
	binary.BigEndian.PutUint32(stszVariable[16:20], 6)
	stream = Stream{}
	p = isoCoverageProbe(t, stszVariable, ModeDetailed)
	if got := parseSTSZ(p, isoBox{data: 0, payload: 20}, &stream); got != 10 || stream.FrameCount != 2 {
		t.Fatalf("variable STSZ = %d, stream = %#v", got, stream)
	}

	btrt := make([]byte, 12)
	binary.BigEndian.PutUint32(btrt[8:12], 192000)
	stream = Stream{}
	p = isoCoverageProbe(t, btrt, ModeFast)
	parseBTRT(p, isoBox{data: 0, payload: 12}, &stream)
	if stream.BitRate != 192000 {
		t.Fatalf("BTRT bitrate = %d", stream.BitRate)
	}

	colr := make([]byte, 11)
	copy(colr[:4], "nclx")
	binary.BigEndian.PutUint16(colr[4:6], 1)
	binary.BigEndian.PutUint16(colr[6:8], 2)
	binary.BigEndian.PutUint16(colr[8:10], 3)
	colr[10] = 0x80
	video := &Video{}
	p = isoCoverageProbe(t, colr, ModeFast)
	parseCOLR(p, isoBox{data: 0, payload: 11}, video)
	if video.ColorPrimaries != "1" || video.TransferCharacteristics != "2" || video.MatrixCoefficients != "3" || video.ColorRange != "Full" {
		t.Fatalf("COLR video = %#v", video)
	}
}

func TestISODataTagsAndSampleDescriptions(t *testing.T) {
	data := make([]byte, 16)
	binary.BigEndian.PutUint16(data[10:12], 7)
	copy(data[12:], "name")
	p := isoCoverageProbe(t, data, ModeFast)
	parseISODataTag(p, isoBox{data: 0, payload: 16}, "trkn")
	if len(p.report.Tags) != 1 || p.report.Tags[0].Name != "trkn" || p.report.Tags[0].Value != "7" {
		t.Fatalf("track tag = %#v", p.report.Tags)
	}

	videoEntry := make([]byte, 78)
	binary.BigEndian.PutUint16(videoEntry[24:26], 640)
	binary.BigEndian.PutUint16(videoEntry[26:28], 480)
	binary.BigEndian.PutUint16(videoEntry[74:76], 24)
	videoDescription := make([]byte, 8)
	binary.BigEndian.PutUint32(videoDescription[4:8], 1)
	videoDescription = append(videoDescription, isoTestBox("avc1", videoEntry)...)
	stream := Stream{Kind: StreamVideo, Video: &Video{}}
	p = isoCoverageProbe(t, videoDescription, ModeFast)
	if err := parseSTSD(p, &isoState{}, isoBox{data: 0, size: int64(len(videoDescription)), payload: int64(len(videoDescription))}, &stream); err != nil {
		t.Fatal(err)
	}
	if stream.CodecID != "avc1" || stream.Format != "AVC" || stream.Video.Width != 640 || stream.Video.Height != 480 || stream.Video.BitDepth != 24 {
		t.Fatalf("video STSD stream = %#v", stream)
	}

	audioEntry := make([]byte, 28)
	binary.BigEndian.PutUint16(audioEntry[16:18], 2)
	binary.BigEndian.PutUint16(audioEntry[18:20], 16)
	binary.BigEndian.PutUint32(audioEntry[24:28], 48000<<16)
	audioDescription := make([]byte, 8)
	binary.BigEndian.PutUint32(audioDescription[4:8], 1)
	audioDescription = append(audioDescription, isoTestBox("mp4a", audioEntry)...)
	stream = Stream{Kind: StreamAudio, Audio: &Audio{}}
	p = isoCoverageProbe(t, audioDescription, ModeFast)
	if err := parseSTSD(p, &isoState{}, isoBox{data: 0, size: int64(len(audioDescription)), payload: int64(len(audioDescription))}, &stream); err != nil {
		t.Fatal(err)
	}
	if stream.CodecID != "mp4a" || stream.Format != "AAC" || stream.Audio.Channels != 2 || stream.Audio.BitDepth != 16 || stream.Audio.SampleRate != 48000 {
		t.Fatalf("audio STSD stream = %#v", stream)
	}
}

func TestISOSegmentIndexAndInvalidMetadataAreBounded(t *testing.T) {
	data := make([]byte, 40)
	binary.BigEndian.PutUint32(data[8:12], 1000)
	binary.BigEndian.PutUint32(data[20:24], 1)
	p := isoCoverageProbe(t, data, ModeDetailed)
	parseSIDX(p, isoBox{data: 0, payload: 20})

	data[0] = 1
	p = isoCoverageProbe(t, data, ModeDetailed)
	parseSIDX(p, isoBox{data: 0, payload: 20})

	data[8] = 0
	data[9] = 0
	data[10] = 0
	data[11] = 0
	p = isoCoverageProbe(t, data, ModeDetailed)
	parseSIDX(p, isoBox{data: 0, payload: 20})

	p = isoCoverageProbe(t, data, ModeFast)
	p.report.General.Duration = time.Second
	parseSIDX(p, isoBox{data: 0, payload: 20})
	parseISODataTag(p, isoBox{data: 0, payload: 8}, "title")
	parseISODataTag(p, isoBox{data: 0, payload: 9}, "covr")
	if len(p.report.Tags) != 0 {
		t.Fatalf("invalid metadata produced tags: %#v", p.report.Tags)
	}
}
