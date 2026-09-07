package mediainfo

import (
	"container/list"
	"fmt"
	"reflect"
	"sync"
	"unsafe"

	"github.com/unxed/f4/vfs"
)

const (
	reportCacheCapacity      = 64
	reportCacheByteCapacity  = 16 << 20
	backendCacheVersion      = "purego-v1"
	reportCacheEntryOverhead = 256
)

type cachedReport struct {
	key    string
	report Report
	weight int64
}

type reportCache struct {
	mu        sync.Mutex
	capacity  int
	maxBytes  int64
	usedBytes int64
	items     map[string]*list.Element
	lru       *list.List
}

func newReportCache(capacity int) *reportCache {
	return newReportCacheWithLimit(capacity, reportCacheByteCapacity)
}

func newReportCacheWithLimit(capacity int, maxBytes int64) *reportCache {
	if capacity <= 0 {
		capacity = reportCacheCapacity
	}
	if maxBytes <= 0 {
		maxBytes = reportCacheByteCapacity
	}
	return &reportCache{capacity: capacity, maxBytes: maxBytes, items: make(map[string]*list.Element), lru: list.New()}
}

func (cache *reportCache) get(key string) (Report, bool) {
	if cache == nil || key == "" {
		return Report{}, false
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	element, ok := cache.items[key]
	if !ok {
		return Report{}, false
	}
	cache.lru.MoveToFront(element)
	return element.Value.(cachedReport).report, true
}

func (cache *reportCache) put(key string, report Report) {
	if cache == nil || key == "" {
		return
	}
	weight := cachedReportWeight(key, report)
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if weight > cache.maxBytes {
		if element, ok := cache.items[key]; ok {
			cache.removeElement(element)
		}
		return
	}
	if element, ok := cache.items[key]; ok {
		old := element.Value.(cachedReport)
		cache.usedBytes -= old.weight
		element.Value = cachedReport{key: key, report: report, weight: weight}
		cache.usedBytes += weight
		cache.lru.MoveToFront(element)
	} else {
		element := cache.lru.PushFront(cachedReport{key: key, report: report, weight: weight})
		cache.items[key] = element
		cache.usedBytes += weight
	}
	for cache.lru.Len() > cache.capacity || cache.usedBytes > cache.maxBytes {
		oldest := cache.lru.Back()
		if oldest == nil {
			break
		}
		cache.removeElement(oldest)
	}
}

func (cache *reportCache) removeElement(element *list.Element) {
	entry := element.Value.(cachedReport)
	delete(cache.items, entry.key)
	cache.usedBytes -= entry.weight
	if cache.usedBytes < 0 {
		cache.usedBytes = 0
	}
	cache.lru.Remove(element)
}

func (cache *reportCache) clear() {
	if cache == nil {
		return
	}
	cache.mu.Lock()
	cache.items = make(map[string]*list.Element)
	cache.lru.Init()
	cache.usedBytes = 0
	cache.mu.Unlock()
}

// cachedReportWeight deliberately overestimates retained heap payload. The
// fixed struct headers are counted with unsafe.Sizeof, slice backing arrays are
// counted by capacity, and all referenced string bytes are counted separately.
// Saturating arithmetic prevents a hostile synthetic Report from wrapping the
// admission calculation.
func cachedReportWeight(key string, report Report) int64 {
	w := cacheWeight{value: reportCacheEntryOverhead}
	w.add(int64(unsafe.Sizeof(report)))
	w.addStringTwice(key) // map key plus cachedReport key, conservatively.
	addGeneralWeight(&w, report.General)
	w.addSlice(cap(report.Streams), unsafe.Sizeof(Stream{}))
	for _, stream := range report.Streams {
		addStreamWeight(&w, stream)
	}
	w.addSlice(cap(report.Chapters), unsafe.Sizeof(Chapter{}))
	for _, chapter := range report.Chapters {
		w.addStrings(chapter.ID, chapter.Title, chapter.Language)
	}
	addFieldsWeight(&w, report.Tags)
	w.addSlice(cap(report.Warnings), unsafe.Sizeof(Warning{}))
	for _, warning := range report.Warnings {
		w.addStrings(warning.Code, warning.Message)
	}
	return w.value
}

type cacheWeight struct{ value int64 }

const maxCacheWeight = int64(^uint64(0) >> 1)

func (w *cacheWeight) add(value int64) {
	if value <= 0 || w.value == maxCacheWeight {
		return
	}
	if value > maxCacheWeight-w.value {
		w.value = maxCacheWeight
		return
	}
	w.value += value
}

func (w *cacheWeight) addSlice(capacity int, elementSize uintptr) {
	if capacity <= 0 {
		return
	}
	if elementSize == 0 || uint64(capacity) > uint64(maxCacheWeight)/uint64(elementSize) {
		w.value = maxCacheWeight
		return
	}
	// #nosec G115 -- the product was checked against maxCacheWeight immediately above.
	w.add(int64(uint64(capacity) * uint64(elementSize)))
}

func (w *cacheWeight) addStrings(values ...string) {
	for _, value := range values {
		w.add(int64(len(value)))
	}
}

func (w *cacheWeight) addStringTwice(value string) {
	w.add(int64(len(value)))
	w.add(int64(len(value)))
}

func addGeneralWeight(w *cacheWeight, general General) {
	w.addStrings(general.FileName, general.Format, general.FormatProfile, general.CodecID, general.MIME, general.MuxingApp, general.WritingApp)
	w.addSlice(cap(general.CompatibleBrands), unsafe.Sizeof(""))
	for _, brand := range general.CompatibleBrands {
		w.addStrings(brand)
	}
	if general.EncodedDate != nil {
		w.add(int64(unsafe.Sizeof(*general.EncodedDate)))
	}
	if general.TaggedDate != nil {
		w.add(int64(unsafe.Sizeof(*general.TaggedDate)))
	}
	if general.Streamable != nil {
		w.add(1)
	}
}

func addStreamWeight(w *cacheWeight, stream Stream) {
	w.addStrings(stream.ID, string(stream.Kind), stream.Format, stream.Profile, stream.Level, stream.CodecID, stream.CodecName, stream.Title, stream.Language, stream.BitRateMode)
	if stream.Default != nil {
		w.add(1)
	}
	if stream.Forced != nil {
		w.add(1)
	}
	if stream.Encrypted != nil {
		w.add(1)
	}
	if stream.Video != nil {
		w.add(int64(unsafe.Sizeof(*stream.Video)))
		w.addStrings(stream.Video.ColorSpace, stream.Video.ChromaSubsampling, stream.Video.ScanType, stream.Video.ScanOrder, stream.Video.ColorRange, stream.Video.ColorPrimaries, stream.Video.TransferCharacteristics, stream.Video.MatrixCoefficients, stream.Video.HDRFormat)
	}
	if stream.Audio != nil {
		w.add(int64(unsafe.Sizeof(*stream.Audio)))
		w.addStrings(stream.Audio.ChannelLayout, stream.Audio.CompressionMode)
	}
	if stream.Text != nil {
		w.add(int64(unsafe.Sizeof(*stream.Text)))
		w.addStrings(stream.Text.FormatVersion, stream.Text.Encoding)
	}
	if stream.Image != nil {
		w.add(int64(unsafe.Sizeof(*stream.Image)))
		w.addStrings(stream.Image.ColorModel, stream.Image.Compression, stream.Image.CameraMake, stream.Image.CameraModel, stream.Image.LensModel)
		if stream.Image.TakenAt != nil {
			w.add(int64(unsafe.Sizeof(*stream.Image.TakenAt)))
		}
		if stream.Image.Latitude != nil {
			w.add(int64(unsafe.Sizeof(*stream.Image.Latitude)))
		}
		if stream.Image.Longitude != nil {
			w.add(int64(unsafe.Sizeof(*stream.Image.Longitude)))
		}
		if stream.Image.GPSAltitude != nil {
			w.add(int64(unsafe.Sizeof(*stream.Image.GPSAltitude)))
		}
		addFieldsWeight(w, stream.Image.EXIF)
	}
	addFieldsWeight(w, stream.Tags)
}

func addFieldsWeight(w *cacheWeight, fields []Field) {
	w.addSlice(cap(fields), unsafe.Sizeof(Field{}))
	for _, field := range fields {
		w.addStrings(field.Target, field.Name, field.Value)
	}
}

func reportCacheKey(fs vfs.VFS, path string, item vfs.VFSItem, mode Mode) string {
	if item.Revision == "" && item.Size == 0 && !item.SizeKnown && item.MTime.IsZero() {
		return ""
	}
	identity := vfsCacheIdentity(fs)
	if identity == "" {
		// A value-backed VFS without SessionIdentity has no safe, stable
		// instance key. Bypass the cache instead of sharing metadata across
		// unrelated accounts that happen to use the same implementation type.
		return ""
	}
	return fmt.Sprintf("%s|%q|%q|%q|%d|%d|%d|%d",
		backendCacheVersion, identity, path, item.Revision,
		item.Size, item.MTime.UnixNano(), boolInt(item.SizeKnown), mode)
}

func vfsCacheIdentity(fs vfs.VFS) string {
	if fs == nil {
		return "<nil>"
	}
	if session, ok := fs.(vfs.SessionIdentity); ok {
		if key := session.SessionKey(); key != nil {
			value := reflect.ValueOf(key)
			if !value.Type().Comparable() {
				return ""
			}
			switch value.Kind() {
			case reflect.Chan, reflect.Pointer, reflect.UnsafePointer:
				if !value.IsNil() {
					return fmt.Sprintf("session:%T:%x", key, value.Pointer())
				}
			default:
				return fmt.Sprintf("session:%T:%v", key, key)
			}
		}
	}
	value := reflect.ValueOf(fs)
	if value.Kind() == reflect.Pointer && !value.IsNil() {
		return fmt.Sprintf("instance:%T:%x", fs, value.Pointer())
	}
	return ""
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
