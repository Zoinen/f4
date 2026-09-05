package mediainfo

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// CanonicalSection is the stable English-key representation used by Inform
// templates and macro consumers. Fields preserve their source order and may
// repeat.
type CanonicalSection struct {
	Name   string
	Index  int
	Fields []Field
}

// CanonicalSections returns General first, then streams in report order, then
// chapters as Menu sections. It never localizes field names.
func CanonicalSections(r Report) []CanonicalSection {
	counts := map[StreamKind]int{}
	for _, s := range r.Streams {
		counts[s.Kind]++
	}
	targetedTags := make(map[string][]int)
	for index, tag := range r.Tags {
		if tag.Target != "" {
			targetedTags[tag.Target] = append(targetedTags[tag.Target], index)
		}
	}
	g := r.General
	general := []Field{
		{Name: "CompleteName", Value: g.FileName}, {Name: "FileName", Value: strings.TrimSuffix(g.FileName, filepath.Ext(g.FileName))},
		{Name: "FileExtension", Value: strings.TrimPrefix(filepath.Ext(g.FileName), ".")}, {Name: "Format", Value: g.Format},
		{Name: "Format_Profile", Value: g.FormatProfile}, {Name: "CodecID", Value: g.CodecID}, {Name: "FileSize", Value: fmt.Sprint(g.FileSize)},
		{Name: "FileSize_String", Value: formatBytes(g.FileSize)}, {Name: "Duration", Value: formatDuration(g.Duration)}, {Name: "PlayTime", Value: formatDuration(g.Duration)},
		{Name: "OverallBitRate", Value: fmt.Sprint(g.OverallBitRate)}, {Name: "OverallBitRate_String", Value: formatBitRate(g.OverallBitRate)},
		{Name: "FrameRate", Value: plainFloat(g.FrameRate)}, {Name: "FrameCount", Value: plainInt(g.FrameCount)}, {Name: "StreamCount", Value: fmt.Sprint(len(r.Streams))},
		{Name: "VideoCount", Value: fmt.Sprint(counts[StreamVideo])}, {Name: "AudioCount", Value: fmt.Sprint(counts[StreamAudio])},
		{Name: "TextCount", Value: fmt.Sprint(counts[StreamText])}, {Name: "ImageCount", Value: fmt.Sprint(counts[StreamImage])},
		{Name: "MenuCount", Value: fmt.Sprint(counts[StreamMenu])}, {Name: "Encoded_Application", Value: g.MuxingApp}, {Name: "Encoded_Library", Value: g.WritingApp},
	}
	for _, tag := range r.Tags {
		if tag.Target == "" {
			general = append(general, Field{Name: tag.Name, Value: tag.Value})
		}
	}
	sections := []CanonicalSection{{Name: "General", Index: 0, Fields: general}}
	kindIndex := map[StreamKind]int{}
	// A malformed container can repeat a stream ID. Targeted metadata belongs
	// to that identifier, not to every duplicate Stream struct: multiplying the
	// same MaxTags collection by MaxStreams would turn a bounded report into a
	// million-field macro/template allocation. Preserve tag order and repeats,
	// but attach each target's collection to its first stream in report order.
	attachedTargets := make(map[string]struct{}, len(targetedTags))
	for _, s := range r.Streams {
		idx := kindIndex[s.Kind]
		kindIndex[s.Kind]++
		fields := canonicalStreamFields(s)
		if _, attached := attachedTargets[s.ID]; !attached && s.ID != "" {
			for _, tagIndex := range targetedTags[s.ID] {
				tag := r.Tags[tagIndex]
				fields = append(fields, Field{Name: tag.Name, Value: tag.Value})
			}
			attachedTargets[s.ID] = struct{}{}
		}
		for _, tag := range s.Tags {
			fields = append(fields, Field{Name: tag.Name, Value: tag.Value})
		}
		sections = append(sections, CanonicalSection{Name: string(s.Kind), Index: idx, Fields: fields})
	}
	for i, c := range r.Chapters {
		sections = append(sections, CanonicalSection{Name: "Menu", Index: i, Fields: []Field{{Name: "ID", Value: c.ID}, {Name: "Start", Value: formatDuration(c.Start)}, {Name: "End", Value: formatDuration(c.End)}, {Name: "Title", Value: c.Title}, {Name: "Language", Value: c.Language}}})
	}
	return sections
}

func canonicalStreamFields(s Stream) []Field {
	f := []Field{{Name: "StreamKind", Value: string(s.Kind)}, {Name: "StreamKindID", Value: fmt.Sprint(s.Index)}, {Name: "StreamOrder", Value: fmt.Sprint(s.Index)}, {Name: "ID", Value: s.ID}, {Name: "Format", Value: s.Format}, {Name: "Format_Profile", Value: s.Profile}, {Name: "Format_Level", Value: s.Level}, {Name: "CodecID", Value: s.CodecID}, {Name: "Codec", Value: s.CodecName}, {Name: "Title", Value: s.Title}, {Name: "Language", Value: s.Language}, {Name: "Duration", Value: formatDuration(s.Duration)}, {Name: "PlayTime", Value: formatDuration(s.Duration)}, {Name: "BitRate", Value: plainInt(s.BitRate)}, {Name: "BitRate_String", Value: formatBitRate(s.BitRate)}, {Name: "BitRate_Mode", Value: s.BitRateMode}, {Name: "FrameRate", Value: plainFloat(s.FrameRate)}, {Name: "FrameCount", Value: plainInt(s.FrameCount)}, {Name: "Default", Value: yesNo(s.Default)}, {Name: "Forced", Value: yesNo(s.Forced)}, {Name: "Encryption", Value: yesNo(s.Encrypted)}}
	if v := s.Video; v != nil {
		f = append(f, Field{Name: "Width", Value: plainInt(int64(v.Width))}, Field{Name: "Height", Value: plainInt(int64(v.Height))}, Field{Name: "DisplayAspectRatio", Value: plainFloat(v.DisplayAspectRatio)}, Field{Name: "PixelAspectRatio", Value: plainFloat(v.PixelAspectRatio)}, Field{Name: "Rotation", Value: plainFloat(v.Rotation)}, Field{Name: "BitDepth", Value: plainInt(int64(v.BitDepth))}, Field{Name: "ColorSpace", Value: v.ColorSpace}, Field{Name: "ChromaSubsampling", Value: v.ChromaSubsampling}, Field{Name: "ScanType", Value: v.ScanType}, Field{Name: "ColorRange", Value: v.ColorRange}, Field{Name: "colour_primaries", Value: v.ColorPrimaries}, Field{Name: "transfer_characteristics", Value: v.TransferCharacteristics}, Field{Name: "matrix_coefficients", Value: v.MatrixCoefficients}, Field{Name: "HDR_Format", Value: v.HDRFormat})
	}
	if a := s.Audio; a != nil {
		f = append(f, Field{Name: "Channel_s_", Value: plainInt(int64(a.Channels))}, Field{Name: "ChannelLayout", Value: a.ChannelLayout}, Field{Name: "SamplingRate", Value: plainInt(int64(a.SampleRate))}, Field{Name: "BitDepth", Value: plainInt(int64(a.BitDepth))}, Field{Name: "Compression_Mode", Value: a.CompressionMode})
	}
	if x := s.Text; x != nil {
		f = append(f, Field{Name: "Format_Version", Value: x.FormatVersion}, Field{Name: "Encoding", Value: x.Encoding}, Field{Name: "ElementCount", Value: plainInt(x.CueCount)}, Field{Name: "FirstFrameTimeCode", Value: formatDuration(x.FirstCue)}, Field{Name: "LastFrameTimeCode", Value: formatDuration(x.LastCue)})
	}
	if im := s.Image; im != nil {
		f = append(f,
			Field{Name: "Width", Value: plainInt(int64(im.Width))}, Field{Name: "Height", Value: plainInt(int64(im.Height))},
			Field{Name: "BitDepth", Value: plainInt(int64(im.BitDepth))}, Field{Name: "ColorSpace", Value: im.ColorModel},
			Field{Name: "Compression_Mode", Value: im.Compression}, Field{Name: "FrameCount", Value: plainInt(int64(im.FrameCount))},
			Field{Name: "Duration", Value: formatDuration(im.AnimationDuration)}, Field{Name: "Orientation", Value: plainInt(int64(im.Orientation))},
			Field{Name: "DPIX", Value: plainFloat(im.DPIX)}, Field{Name: "DPIY", Value: plainFloat(im.DPIY)},
			Field{Name: "Make", Value: im.CameraMake}, Field{Name: "Model", Value: im.CameraModel}, Field{Name: "LensModel", Value: im.LensModel},
		)
		if im.TakenAt != nil {
			f = append(f, Field{Name: "DateTimeOriginal", Value: im.TakenAt.Format("2006-01-02 15:04:05")})
		}
		if im.Latitude != nil {
			f = append(f, Field{Name: "GPSLatitude", Value: plainFloat(*im.Latitude)})
		}
		if im.Longitude != nil {
			f = append(f, Field{Name: "GPSLongitude", Value: plainFloat(*im.Longitude)})
		}
		if im.GPSAltitude != nil {
			f = append(f, Field{Name: "GPSAltitude", Value: plainFloat(*im.GPSAltitude)})
		}
		f = append(f, im.EXIF...)
	}
	return f
}

func yesNo(v *bool) string {
	if v == nil {
		return ""
	}
	if *v {
		return "Yes"
	}
	return "No"
}
func plainInt(v int64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprint(v)
}
func plainFloat(v float64) string {
	if v == 0 {
		return ""
	}
	return fmt.Sprintf("%.6g", v)
}

// RenderOptions controls deterministic human-readable report output.
type RenderOptions struct {
	// Language selects display labels. "ru" uses Russian; other values use
	// English. Template and macro field keys remain canonical English.
	Language string
	// Technical includes codec IDs, profiles, flags, and parser warnings.
	Technical bool
	// Compact omits empty separators and less useful secondary fields.
	Compact bool
}

const (
	maxRenderedTextBytes         = maxTemplateOutputBytes
	maxRenderedCollectionItems   = 8 << 10
	maxRenderedTextLines         = 8 << 10
	renderTextTruncationNotice   = "[Report truncated]"
	renderTextTruncationNoticeRU = "[Отчёт сокращён]"
)

// RenderText renders aligned MediaInfo-style sections with canonical keys.
func RenderText(r Report, o RenderOptions) string {
	out := newReportTextWriter(maxRenderedTextBytes, renderedTruncationNotice(o.Language))
	out.labelWidth = renderedTextLabelWidth(r, o, out.contentCap)
	if walkRenderedTextSections(r, o, func(title string, fields [][2]string) bool {
		renderTextSection(out, title, fields, o.Language)
		return !out.truncated
	}) {
		out.truncated = true
	}
	return out.String()
}

// renderedTextLabelWidth uses the same bounded section walk as the writer so
// every visible field is aligned to one report-wide column. Values and labels
// are normalized here exactly as they are by renderTextSection; empty fields
// therefore cannot widen the report.
func renderedTextLabelWidth(r Report, o RenderOptions, contentCap int) int {
	width := 0
	walkRenderedTextSections(r, o, func(_ string, fields [][2]string) bool {
		for _, field := range fields {
			if normalizeRenderedInline(field[1]) == "" {
				continue
			}
			label := normalizeRenderedInline(renderLabel(o.Language, field[0]))
			if len(label) > contentCap {
				return false
			}
			if current := runewidth.StringWidth(label); current > width {
				width = current
			}
		}
		return true
	})
	return width
}

// walkRenderedTextSections centralizes section and field selection for both
// the width preflight and output pass. It returns true when the report must be
// marked truncated, either because source collections exceed their rendering
// limits or because the visitor cannot accept another section.
func walkRenderedTextSections(r Report, o RenderOptions, visit func(string, [][2]string) bool) bool {
	g := r.General
	gf := [][2]string{{"File name", g.FileName}, {"Format", g.Format}, {"Format profile", g.FormatProfile}, {"File size", formatBytes(g.FileSize)}, {"Duration", formatDuration(g.Duration)}, {"Overall bit rate", formatBitRate(g.OverallBitRate)}, {"Frame rate", formatFloat(g.FrameRate)}, {"Muxing application", g.MuxingApp}, {"Writing application", g.WritingApp}}
	if o.Compact {
		gf = [][2]string{{"Format", g.Format}, {"File size", formatBytes(g.FileSize)}, {"Duration", formatDuration(g.Duration)}, {"Overall bit rate", formatBitRate(g.OverallBitRate)}}
	}
	brandsTruncated := false
	if o.Technical {
		brands, complete := joinRenderedStrings(g.CompatibleBrands, ", ", maxRenderedTextBytes)
		brandsTruncated = !complete
		gf = append(gf, [2]string{"Codec ID", g.CodecID}, [2]string{"Compatible brands", brands})
	}
	if !visit(renderLabel(o.Language, "General"), gf) || brandsTruncated {
		return true
	}
	for _, s := range r.Streams {
		title := renderLabel(o.Language, string(s.Kind))
		if s.ID != "" {
			if !renderedConcatFits(title, " #", s.ID) {
				return true
			}
			title += " #" + s.ID
		}
		f := [][2]string{{"Format", s.Format}, {"Format profile", s.Profile}, {"Title", s.Title}, {"Language", s.Language}, {"Duration", formatDuration(s.Duration)}, {"Bit rate", formatBitRate(s.BitRate)}, {"Frame rate", formatFloat(s.FrameRate)}}
		if o.Compact {
			f = [][2]string{{"Format", s.Format}, {"Title", s.Title}, {"Language", s.Language}, {"Bit rate", formatBitRate(s.BitRate)}}
		}
		if o.Technical {
			f = append(f, [2]string{"Codec ID", s.CodecID}, [2]string{"Codec name", s.CodecName}, [2]string{"Frame count", formatInt(s.FrameCount)})
		}
		if s.Video != nil {
			f = append(f, [2]string{"Width", formatPixels(s.Video.Width)}, [2]string{"Height", formatPixels(s.Video.Height)})
			if !o.Compact {
				f = append(f, [2]string{"Bit depth", formatBits(s.Video.BitDepth)}, [2]string{"Color space", s.Video.ColorSpace}, [2]string{"HDR format", s.Video.HDRFormat})
			}
		}
		if s.Audio != nil {
			f = append(f, [2]string{"Channels", formatInt(int64(s.Audio.Channels))}, [2]string{"Sample rate", formatHz(s.Audio.SampleRate)})
			if !o.Compact {
				f = append(f, [2]string{"Bit depth", formatBits(s.Audio.BitDepth)}, [2]string{"Compression mode", s.Audio.CompressionMode})
			}
		}
		if s.Text != nil {
			f = append(f, [2]string{"Encoding", s.Text.Encoding}, [2]string{"Cue count", formatInt(s.Text.CueCount)})
			if !o.Compact {
				f = append(f, [2]string{"First cue", formatDuration(s.Text.FirstCue)}, [2]string{"Last cue", formatDuration(s.Text.LastCue)})
			}
		}
		if s.Image != nil {
			f = append(f, [2]string{"Width", formatPixels(s.Image.Width)}, [2]string{"Height", formatPixels(s.Image.Height)}, [2]string{"Frame count", formatInt(int64(s.Image.FrameCount))})
			if !o.Compact {
				f = append(f,
					[2]string{"Bit depth", formatBits(s.Image.BitDepth)}, [2]string{"Color model", s.Image.ColorModel}, [2]string{"Compression", s.Image.Compression},
					[2]string{"Animation duration", formatDuration(s.Image.AnimationDuration)}, [2]string{"Camera make", s.Image.CameraMake},
					[2]string{"Camera model", s.Image.CameraModel}, [2]string{"Lens model", s.Image.LensModel}, [2]string{"Captured at", formatImageTakenAt(s.Image.TakenAt)},
					[2]string{"Orientation", formatImageOrientation(s.Image.Orientation)}, [2]string{"Horizontal resolution", formatImageDPI(s.Image.DPIX)},
					[2]string{"Vertical resolution", formatImageDPI(s.Image.DPIY)}, [2]string{"GPS latitude", formatGPSCoordinate(s.Image.Latitude)},
					[2]string{"GPS longitude", formatGPSCoordinate(s.Image.Longitude)}, [2]string{"GPS altitude", formatGPSAltitude(s.Image.GPSAltitude)},
				)
				for _, field := range s.Image.EXIF {
					f = append(f, [2]string{field.Name, field.Value})
				}
			}
		}
		if !visit(title, f) {
			return true
		}
	}
	chapterLimit := len(r.Chapters)
	if o.Compact && chapterLimit > 3 {
		chapterLimit = 3
	}
	for index := 0; index < chapterLimit; index++ {
		chapter := r.Chapters[index]
		title := renderLabel(o.Language, "Menu")
		if chapter.ID != "" {
			if !renderedConcatFits(title, " #", chapter.ID) {
				return true
			}
			title += " #" + chapter.ID
		}
		fields := [][2]string{
			{"Start", formatDuration(chapter.Start)},
			{"End", formatDuration(chapter.End)},
			{"Title", chapter.Title},
			{"Language", chapter.Language},
		}
		if o.Technical {
			fields = append([][2]string{{"ID", chapter.ID}}, fields...)
		}
		if !visit(title, fields) {
			return true
		}
	}
	if len(r.Tags) > 0 {
		tagCount := len(r.Tags)
		if tagCount > maxRenderedCollectionItems {
			tagCount = maxRenderedCollectionItems
		}
		tags := append([]Field(nil), r.Tags[:tagCount]...)
		sort.SliceStable(tags, func(i, j int) bool {
			if tags[i].Target == tags[j].Target {
				return tags[i].Name < tags[j].Name
			}
			return tags[i].Target < tags[j].Target
		})
		f := make([][2]string, 0, len(tags))
		tagsTruncated := !o.Compact && tagCount < len(r.Tags)
		for _, tag := range tags {
			if o.Compact && len(f) >= 6 {
				break
			}
			name := tag.Name
			if tag.Target != "" {
				if !renderedConcatFits(name, " [", tag.Target, "]") {
					tagsTruncated = true
					break
				}
				name += " [" + tag.Target + "]"
			}
			f = append(f, [2]string{name, tag.Value})
		}
		if !visit(renderLabel(o.Language, "Metadata"), f) || tagsTruncated {
			return true
		}
	}
	if o.Technical && len(r.Warnings) > 0 {
		warningCount := len(r.Warnings)
		if warningCount > maxRenderedCollectionItems {
			warningCount = maxRenderedCollectionItems
		}
		f := make([][2]string, 0, warningCount)
		for _, w := range r.Warnings[:warningCount] {
			f = append(f, [2]string{w.Code, w.Message})
		}
		if !visit(renderLabel(o.Language, "Warnings"), f) || warningCount < len(r.Warnings) {
			return true
		}
	}
	return false
}

func renderedTruncationNotice(language string) string {
	if strings.EqualFold(language, "ru") {
		return renderTextTruncationNoticeRU
	}
	return renderTextTruncationNotice
}

type reportTextWriter struct {
	out        *cappedTemplateWriter
	limit      int
	notice     string
	truncated  bool
	finalized  bool
	contentCap int
	labelWidth int
}

func newReportTextWriter(limit int, notice string) *reportTextWriter {
	contentCap := limit - len(notice) - 2 // reserve a blank line and notice
	if contentCap < 0 {
		contentCap = 0
	}
	return &reportTextWriter{
		out:        newCappedTemplateWriter(contentCap),
		limit:      limit,
		notice:     notice,
		contentCap: contentCap,
	}
}

func (writer *reportTextWriter) String() string {
	if !writer.truncated || writer.finalized {
		return writer.out.String()
	}
	writer.finalized = true
	writer.out.limit = writer.limit
	if writer.out.b.Len() > 0 {
		_ = writer.out.WriteString("\n\n")
	}
	_ = writer.out.WriteString(writer.notice)
	return writer.out.String()
}

func (writer *reportTextWriter) reserve(size int) bool {
	if writer.truncated || size < 0 || size > writer.contentCap-writer.out.b.Len() {
		writer.truncated = true
		return false
	}
	return true
}

func renderTextSection(out *reportTextWriter, title string, fields [][2]string, language string) {
	if out.truncated {
		return
	}
	title = normalizeRenderedInline(title)
	filtered := fields[:0]
	width := out.labelWidth
	for _, f := range fields {
		f[1] = normalizeRenderedInline(f[1])
		if f[1] == "" {
			continue
		}
		f[0] = normalizeRenderedInline(renderLabel(language, f[0]))
		if len(f[0]) > out.contentCap {
			out.truncated = true
			return
		}
		filtered = append(filtered, f)
		if n := runewidth.StringWidth(f[0]); n > width {
			width = n
		}
	}
	if len(filtered) == 0 {
		return
	}
	prefixBytes := 0
	if out.out.b.Len() > 0 {
		prefixBytes = 2
	}
	first := filtered[0]
	firstLineBytes := renderedLineBytes(first[0], first[1], width)
	if firstLineBytes < 0 || !out.reserve(prefixBytes+len(title)+1+firstLineBytes) {
		return
	}
	if prefixBytes > 0 {
		_ = out.out.WriteString("\n\n")
	}
	_ = out.out.WriteString(title)
	_ = out.out.WriteByte('\n')
	writeRenderedLine(out.out, first[0], first[1], width)
	for _, f := range filtered[1:] {
		lineBytes := renderedLineBytes(f[0], f[1], width)
		if lineBytes < 0 || !out.reserve(1+lineBytes) {
			return
		}
		_ = out.out.WriteByte('\n')
		writeRenderedLine(out.out, f[0], f[1], width)
	}
}

// normalizeRenderedInline keeps the default human-readable report a sequence
// of actual report lines. Metadata is untrusted: embedded CR/LF, tabs, NULs
// and terminal control bytes must not inject rows or terminal commands into a
// label or value. Inform templates deliberately retain their raw field-value
// semantics and do not call this helper.
func normalizeRenderedInline(value string) string {
	if strings.IndexFunc(value, isRenderedInlineControl) < 0 {
		return value
	}
	var normalized strings.Builder
	normalized.Grow(len(value))
	lastSpace := false
	for _, r := range value {
		if isRenderedInlineControl(r) {
			if normalized.Len() > 0 && !lastSpace {
				normalized.WriteByte(' ')
				lastSpace = true
			}
			continue
		}
		normalized.WriteRune(r)
		lastSpace = unicode.IsSpace(r)
	}
	return strings.TrimSpace(normalized.String())
}

func isRenderedInlineControl(r rune) bool {
	return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
}

func renderedLineBytes(label, value string, width int) int {
	padding := width - runewidth.StringWidth(label)
	if padding < 0 || len(label) > int(^uint(0)>>1)-padding-3 || len(value) > int(^uint(0)>>1)-len(label)-padding-3 {
		return -1
	}
	return len(label) + padding + 3 + len(value)
}

func writeRenderedLine(out *cappedTemplateWriter, label, value string, width int) {
	_ = out.WriteString(label)
	padding := width - runewidth.StringWidth(label)
	const spaces = "                                                                "
	for padding > len(spaces) {
		_ = out.WriteString(spaces)
		padding -= len(spaces)
	}
	if padding > 0 {
		_ = out.WriteString(spaces[:padding])
	}
	_ = out.WriteString(" : ")
	_ = out.WriteString(value)
}

func renderedConcatFits(parts ...string) bool {
	remaining := maxRenderedTextBytes
	for _, part := range parts {
		if len(part) > remaining {
			return false
		}
		remaining -= len(part)
	}
	return true
}

func joinRenderedStrings(values []string, separator string, limit int) (string, bool) {
	length := 0
	for index, value := range values {
		if index > 0 {
			if len(separator) > limit-length {
				return "", false
			}
			length += len(separator)
		}
		if len(value) > limit-length {
			return "", false
		}
		length += len(value)
	}
	return strings.Join(values, separator), true
}

func renderLabel(language, key string) string {
	if !strings.EqualFold(language, "ru") {
		return key
	}
	if value, ok := russianLabels[key]; ok {
		return value
	}
	return key
}

var russianLabels = map[string]string{
	"General": "Общее", "Video": "Видео", "Audio": "Аудио", "Text": "Текст", "Image": "Изображение", "Menu": "Меню",
	"Metadata": "Метаданные", "Warnings": "Предупреждения", "File name": "Имя файла", "Format": "Формат",
	"Format profile": "Профиль формата", "File size": "Размер файла", "Duration": "Продолжительность",
	"Overall bit rate": "Общий битрейт", "Frame rate": "Частота кадров", "Muxing application": "Приложение-мультиплексор",
	"Writing application": "Приложение записи", "Codec ID": "Идентификатор кодека", "Compatible brands": "Совместимые марки",
	"Title": "Название", "Language": "Язык", "Bit rate": "Битрейт", "Codec name": "Название кодека", "Frame count": "Количество кадров",
	"Width": "Ширина", "Height": "Высота", "Bit depth": "Глубина цвета", "Color space": "Цветовое пространство", "HDR format": "Формат HDR",
	"Channels": "Каналы", "Sample rate": "Частота дискретизации", "Compression mode": "Режим сжатия", "Encoding": "Кодировка",
	"Cue count": "Количество реплик", "First cue": "Первая реплика", "Last cue": "Последняя реплика", "Color model": "Цветовая модель",
	"Camera make": "Производитель камеры", "Camera model": "Модель камеры", "Artist": "Исполнитель", "Album": "Альбом", "Comment": "Комментарий",
	"Compression": "Сжатие", "Animation duration": "Длительность анимации", "Lens model": "Модель объектива", "Captured at": "Дата съёмки",
	"Orientation": "Ориентация", "Horizontal resolution": "Разрешение по горизонтали", "Vertical resolution": "Разрешение по вертикали",
	"GPS latitude": "Широта GPS", "GPS longitude": "Долгота GPS", "GPS altitude": "Высота GPS", "Exposure time": "Выдержка",
	"F-number": "Диафрагма", "ISO speed": "Светочувствительность ISO", "Exposure bias": "Экспокоррекция", "Focal length": "Фокусное расстояние",
	"Focal length in 35mm": "Фокусное расстояние в 35-мм эквиваленте", "Metering mode": "Режим экспозамера", "Flash": "Вспышка",
	"White balance": "Баланс белого", "Exposure program": "Программа экспозиции", "Exposure mode": "Режим экспозиции",
	"Brightness": "Яркость", "Subject distance": "Расстояние до объекта", "Light source": "Источник света", "Digital zoom": "Цифровой зум",
	"Scene capture type": "Тип сцены", "Contrast": "Контраст", "Saturation": "Насыщенность", "Sharpness": "Резкость",
	"Software": "Программа", "Copyright": "Авторские права", "Camera serial number": "Серийный номер камеры", "Lens make": "Производитель объектива",
	"Lens serial number": "Серийный номер объектива", "User comment": "Комментарий пользователя", "Image unique ID": "Уникальный ID изображения",
	"Start": "Начало", "End": "Конец", "ID": "Идентификатор",
}

// ExecuteTemplate executes a bounded MediaInfo Inform template. The supported
// subset is `Section;template`, %Field%, optional `[ ... ]` groups, common
// backslash escapes, and Section_Begin/Section_Middle/Section_End. Go template
// actions remain available as an extension when the source contains `{{`.
func ExecuteTemplate(r Report, src string) (string, error) {
	if len(src) > maxTemplateSourceBytes {
		return "", errors.New("template is too large")
	}
	if !utf8.ValidString(src) {
		return "", errors.New("template is not valid UTF-8")
	}
	src = strings.TrimPrefix(src, "\ufeff")
	if !strings.Contains(src, "{{") {
		return executeInformTemplate(r, src)
	}
	funcs := template.FuncMap{"duration": formatDuration, "bytes": formatBytes, "bitrate": formatBitRate, "join": strings.Join}
	t, e := template.New("mediainfo").Option("missingkey=zero").Funcs(funcs).Parse(src)
	if e != nil {
		return "", e
	}
	b := newCappedTemplateWriter(maxTemplateOutputBytes)
	if e = t.Execute(b, r); e != nil {
		return "", e
	}
	return b.String(), nil
}

const (
	maxTemplateSourceBytes = 64 << 10
	maxTemplateOutputBytes = 2 << 20
	maxInformNesting       = 64
)

var errTemplateOutputTooLarge = errors.New("template output is too large")

// cappedTemplateWriter rejects a write before it can grow the backing buffer
// beyond the configured limit. This matters for templates that range over a
// large chapter or stream list: checking the size after execution is too late.
type cappedTemplateWriter struct {
	b          strings.Builder
	limit      int
	overflowed bool
}

func newCappedTemplateWriter(limit int) *cappedTemplateWriter {
	return &cappedTemplateWriter{limit: limit}
}

func (writer *cappedTemplateWriter) Write(data []byte) (int, error) {
	if writer.overflowed {
		return 0, errTemplateOutputTooLarge
	}
	if len(data) > writer.limit-writer.b.Len() {
		writer.overflowed = true
		return 0, errTemplateOutputTooLarge
	}
	return writer.b.Write(data)
}

func (writer *cappedTemplateWriter) WriteString(value string) error {
	if writer.overflowed {
		return errTemplateOutputTooLarge
	}
	if len(value) > writer.limit-writer.b.Len() {
		writer.overflowed = true
		return errTemplateOutputTooLarge
	}
	_, _ = writer.b.WriteString(value)
	return nil
}

func (writer *cappedTemplateWriter) WriteByte(value byte) error {
	if writer.overflowed {
		return errTemplateOutputTooLarge
	}
	if writer.b.Len() >= writer.limit {
		writer.overflowed = true
		return errTemplateOutputTooLarge
	}
	return writer.b.WriteByte(value)
}

func (writer *cappedTemplateWriter) String() string { return writer.b.String() }

func executeInformTemplate(r Report, src string) (string, error) {
	definitions := map[string]string{}
	directives := 0
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, ";", 2)
		if len(parts) != 2 {
			continue
		}
		key := canonicalInformSection(parts[0])
		if key == "" {
			continue
		}
		definitions[key] += parts[1]
		directives++
	}
	sections := CanonicalSections(r)
	if directives == 0 {
		out := newCappedTemplateWriter(maxTemplateOutputBytes)
		if err := renderInformFragment(out, src, sectionLookup(sections[0]), 0); err != nil {
			return "", err
		}
		return out.String(), nil
	}
	byKind := map[string][]CanonicalSection{}
	for _, s := range sections {
		byKind[s.Name] = append(byKind[s.Name], s)
	}
	out := newCappedTemplateWriter(maxTemplateOutputBytes)
	for _, kind := range []string{"General", "Video", "Audio", "Text", "Image", "Menu"} {
		items := byKind[kind]
		if len(items) == 0 {
			continue
		}
		main := definitions[kind]
		begin := definitions[kind+"_Begin"]
		middle := definitions[kind+"_Middle"]
		end := definitions[kind+"_End"]
		if begin != "" {
			if err := renderInformFragment(out, begin, sectionLookup(items[0]), 0); err != nil {
				return "", err
			}
		}
		for i, item := range items {
			fields := sectionLookup(item)
			if i > 0 && middle != "" {
				if err := renderInformFragment(out, middle, fields, 0); err != nil {
					return "", err
				}
			}
			if main != "" {
				if err := renderInformFragment(out, main, fields, 0); err != nil {
					return "", err
				}
			}
		}
		if end != "" {
			if err := renderInformFragment(out, end, sectionLookup(items[len(items)-1]), 0); err != nil {
				return "", err
			}
		}
	}
	return out.String(), nil
}

func canonicalInformSection(raw string) string {
	raw = strings.TrimSpace(raw)
	suffix := ""
	for _, s := range []string{"_Begin", "_Middle", "_End"} {
		if strings.HasSuffix(strings.ToLower(raw), strings.ToLower(s)) {
			raw = raw[:len(raw)-len(s)]
			suffix = s
			break
		}
	}
	for _, kind := range []string{"General", "Video", "Audio", "Text", "Image", "Menu"} {
		if strings.EqualFold(raw, kind) {
			return kind + suffix
		}
	}
	return ""
}

func sectionLookup(section CanonicalSection) map[string]string {
	m := make(map[string]string, len(section.Fields)*2)
	for _, f := range section.Fields {
		k := normalizeInformField(f.Name)
		if _, exists := m[k]; !exists {
			m[k] = f.Value
		}
	}
	return m
}

func normalizeInformField(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if r == '_' || r == ' ' || r == '/' || r == '-' || r == '(' || r == ')' {
			continue
		}
		b.WriteRune(r)
	}
	key := b.String()
	switch key {
	case "playtime", "durationstring", "playtimestring":
		return "duration"
	case "formatprofile":
		return "formatprofile"
	case "channels", "channel", "channelsstring":
		return "channels"
	}
	return key
}

func renderInformFragment(out *cappedTemplateWriter, src string, fields map[string]string, depth int) error {
	if depth > maxInformNesting {
		return errors.New("inform template nesting is too deep")
	}
	for i := 0; i < len(src); {
		switch src[i] {
		case '[':
			end := matchingInformBracket(src, i)
			if end < 0 {
				if err := out.WriteByte(src[i]); err != nil {
					return err
				}
				i++
				continue
			}
			inner := src[i+1 : end]
			if optionalInformGroupIncluded(inner, fields) {
				if err := renderInformFragment(out, inner, fields, depth+1); err != nil {
					return err
				}
			}
			i = end + 1
		case '%':
			relativeEnd := strings.IndexByte(src[i+1:], '%')
			if relativeEnd <= 0 {
				if err := out.WriteByte(src[i]); err != nil {
					return err
				}
				i++
				continue
			}
			end := i + 1 + relativeEnd
			if err := writeInformEscaped(out, fields[normalizeInformField(src[i+1:end])]); err != nil {
				return err
			}
			i = end + 1
		case '\\':
			if i+1 >= len(src) {
				return out.WriteByte(src[i])
			}
			i++
			value := src[i]
			switch value {
			case 'n':
				value = '\n'
			case 'r':
				value = '\r'
			case 't':
				value = '\t'
			}
			if err := out.WriteByte(value); err != nil {
				return err
			}
			i++
		default:
			if err := out.WriteByte(src[i]); err != nil {
				return err
			}
			i++
		}
	}
	return nil
}

func matchingInformBracket(src string, start int) int {
	depth := 1
	for i := start + 1; i < len(src); i++ {
		switch src[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func optionalInformGroupIncluded(src string, fields map[string]string) bool {
	foundField := false
	for i := 0; i < len(src); {
		if src[i] != '%' {
			i++
			continue
		}
		relativeEnd := strings.IndexByte(src[i+1:], '%')
		if relativeEnd <= 0 {
			i++
			continue
		}
		end := i + 1 + relativeEnd
		foundField = true
		if fields[normalizeInformField(src[i+1:end])] != "" {
			return true
		}
		i = end + 1
	}
	return !foundField
}

func writeInformEscaped(out *cappedTemplateWriter, value string) error {
	for i := 0; i < len(value); i++ {
		if value[i] != '\\' || i+1 >= len(value) {
			if err := out.WriteByte(value[i]); err != nil {
				return err
			}
			continue
		}
		i++
		next := value[i]
		switch next {
		case 'n':
			next = '\n'
		case 'r':
			next = '\r'
		case 't':
			next = '\t'
		}
		if err := out.WriteByte(next); err != nil {
			return err
		}
	}
	return nil
}
func formatDuration(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	h := int64(d / time.Hour)
	d -= time.Duration(h) * time.Hour
	m := int64(d / time.Minute)
	d -= time.Duration(m) * time.Minute
	s := float64(d) / float64(time.Second)
	if h > 0 {
		return fmt.Sprintf("%d h %02d min %06.3f s", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%d min %06.3f s", m, s)
	}
	return fmt.Sprintf("%.3f s", s)
}
func formatBytes(n int64) string {
	if n <= 0 {
		return ""
	}
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.2f %s", v, units[i])
}
func formatBitRate(n int64) string {
	if n <= 0 {
		return ""
	}
	if n >= 1000000 {
		return fmt.Sprintf("%.3f Mb/s", float64(n)/1e6)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.0f kb/s", float64(n)/1e3)
	}
	return fmt.Sprintf("%d b/s", n)
}
func formatFloat(v float64) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("%.3f FPS", v)
}

func formatImageTakenAt(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02 15:04:05")
}

func formatImageOrientation(value int) string {
	return map[int]string{
		1: "Normal", 2: "Mirrored horizontally", 3: "Rotated 180°", 4: "Mirrored vertically",
		5: "Transposed", 6: "Rotated 90° clockwise", 7: "Transverse", 8: "Rotated 270° clockwise",
	}[value]
}

func formatImageDPI(value float64) string {
	if value == 0 {
		return ""
	}
	return fmt.Sprintf("%.4g dpi", value)
}

func formatGPSCoordinate(value *float64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%.6f°", *value)
}

func formatGPSAltitude(value *float64) string {
	if value == nil {
		return ""
	}
	return fmt.Sprintf("%.3f m", *value)
}
func formatInt(v int64) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprint(v)
}
func formatPixels(v int) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("%d pixels", v)
}
func formatBits(v int) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("%d bits", v)
}
func formatHz(v int) string {
	if v <= 0 {
		return ""
	}
	return fmt.Sprintf("%d Hz", v)
}
