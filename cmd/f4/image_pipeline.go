package main

// The decoding pipeline: the one place that turns files into pixels. It
// remembers what it has decoded, decodes one picture only once no matter how
// many parts of the interface ask for it, and decodes in advance what is
// likely to be asked for next.

import (
	"context"
	"fmt"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const (
	// imageCacheLimit bounds the decoded pixels the pipeline keeps. Pictures
	// are large and the cache exists to make going back and forth instant,
	// not to hold a whole directory.
	imageCacheLimit = 192 << 20

	// imageWorkers is how many pictures are decoded at the same time.
	imageWorkers = 2
)

// ImageResult is what a request for a picture eventually produces. A preview
// is a provisional answer: the small copy the file carries inside itself,
// handed over while the picture proper is still being decoded.
type ImageResult struct {
	Path    string
	Surface *vtui.ImageSurface
	Decoder string
	Preview bool
	Err     error
}

// imageCacheKey identifies a picture. The same path in two different file
// systems is two different pictures.
type imageCacheKey struct {
	Source string
	Path   string
}

type imageEntry struct {
	res   ImageResult
	bytes int64
}

// imageWaiter is somebody waiting for a picture. Requests made from the
// interface are answered on the UI thread; a caller that already runs in the
// background is answered where the decoding happened.
type imageWaiter struct {
	fn func(ImageResult)
	ui bool
}

type imageJob struct {
	key     imageCacheKey
	v       vfs.VFS
	path    string
	urgent  bool
	waiters []imageWaiter
}

// imageLoader turns a path into pixels. The pipeline keeps it as a field so
// that tests do not need a file system.
type imageLoader func(ctx context.Context, v vfs.VFS, path string) (*vtui.ImageSurface, string, error)

// ImagePipeline decodes pictures in the background and caches the results.
type ImagePipeline struct {
	mu    sync.Mutex
	cache map[imageCacheKey]*imageEntry
	lru   []imageCacheKey
	bytes int64
	limit int64

	jobs    map[imageCacheKey]*imageJob
	queue   []*imageJob
	busy    int
	workers int

	previews     map[imageCacheKey]ImageResult
	previewOrder []imageCacheKey

	load     imageLoader
	preview  imageLoader
	dispatch func(func())
}

// ImagePipe is the pipeline the application uses.
var ImagePipe = NewImagePipeline()

func NewImagePipeline() *ImagePipeline {
	return &ImagePipeline{
		cache:    make(map[imageCacheKey]*imageEntry),
		jobs:     make(map[imageCacheKey]*imageJob),
		limit:    imageCacheLimit,
		workers:  imageWorkers,
		previews: make(map[imageCacheKey]ImageResult),
		load:     LoadImage,
		preview:  imageQuickPreview,
		// dispatch is left nil on purpose. The default is the frame manager
		// read where a job is started rather than one read from inside the
		// worker: a decode outlives the call that asked for it, and reading
		// the global from the worker races anything that reassigns
		// vtui.FrameManager while the picture is still being decoded. Tests
		// set this field to run the callbacks inline.
	}
}

// imageSource names the file system a path belongs to.
func imageSource(v vfs.VFS) string {
	if v == nil {
		return ""
	}
	if t, ok := v.(vfs.TitleProvider); ok {
		if title := t.GetTitle(); title != "" {
			return fmt.Sprintf("%T:%s", v, title)
		}
	}
	return fmt.Sprintf("%T", v)
}

// Cached returns a picture that has already been decoded.
func (p *ImagePipeline) Cached(v vfs.VFS, path string) (ImageResult, bool) {
	key := imageCacheKey{Source: imageSource(v), Path: path}

	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.cache[key]
	if !ok {
		return ImageResult{}, false
	}
	p.touch(key)
	return entry.res, true
}

// PreviewSync returns the best picture that can be had without decoding the
// whole file: one that is already decoded, a thumbnail seen earlier, or the
// thumbnail the file carries inside itself. The second value says whether
// there is anything at all to show.
func (p *ImagePipeline) PreviewSync(ctx context.Context, v vfs.VFS, path string) (ImageResult, bool) {
	if res, ok := p.Cached(v, path); ok {
		return res, true
	}
	key := imageCacheKey{Source: imageSource(v), Path: path}

	p.mu.Lock()
	res, ok := p.previews[key]
	p.mu.Unlock()
	if ok {
		return res, true
	}

	surf, decoder, err := p.preview(ctx, v, path)
	if err != nil || !surf.Valid() {
		return ImageResult{}, false
	}
	res = ImageResult{Path: path, Surface: surf, Decoder: decoder, Preview: true}

	p.mu.Lock()
	p.storePreview(key, res)
	p.mu.Unlock()
	return res, true
}

// storePreview keeps a thumbnail around. The caller holds the lock.
func (p *ImagePipeline) storePreview(key imageCacheKey, res ImageResult) {
	if _, ok := p.previews[key]; !ok {
		p.previewOrder = append(p.previewOrder, key)
	}
	p.previews[key] = res
	for len(p.previewOrder) > imagePreviewCacheLimit {
		delete(p.previews, p.previewOrder[0])
		p.previewOrder = p.previewOrder[1:]
	}
}

func (p *ImagePipeline) dropPreview(key imageCacheKey) {
	if _, ok := p.previews[key]; !ok {
		return
	}
	delete(p.previews, key)
	for i, k := range p.previewOrder {
		if k == key {
			p.previewOrder = append(p.previewOrder[:i], p.previewOrder[i+1:]...)
			break
		}
	}
}

// Load asks for a picture. A picture that is already decoded is handed over
// before Load returns, on the calling thread; otherwise the callback runs on
// the UI thread once the picture is ready. Several requests for the same
// picture share one decoding job.
func (p *ImagePipeline) Load(v vfs.VFS, path string, done func(ImageResult)) {
	if res, ok := p.Cached(v, path); ok {
		if done != nil {
			done(res)
		}
		return
	}
	p.request(v, path, true, imageWaiter{fn: done, ui: true})
}

// LoadSync decodes a picture and waits for it. It is meant for callers that
// already run in the background and would rather block than be called back.
func (p *ImagePipeline) LoadSync(ctx context.Context, v vfs.VFS, path string) ImageResult {
	if res, ok := p.Cached(v, path); ok {
		return res
	}
	ch := make(chan ImageResult, 1)
	p.request(v, path, true, imageWaiter{fn: func(res ImageResult) { ch <- res }})

	if ctx == nil {
		return <-ch
	}
	select {
	case res := <-ch:
		return res
	case <-ctx.Done():
		return ImageResult{Path: path, Err: ctx.Err()}
	}
}

// Prefetch decodes pictures nobody has asked for yet. The list replaces the
// previous one: a neighbour that is no longer near the picture on screen
// gives up its place in the queue to the new neighbours.
func (p *ImagePipeline) Prefetch(v vfs.VFS, paths []string) {
	source := imageSource(v)
	wanted := make(map[imageCacheKey]bool, len(paths))
	for _, path := range paths {
		wanted[imageCacheKey{Source: source, Path: path}] = true
	}

	p.mu.Lock()
	kept := p.queue[:0]
	for _, job := range p.queue {
		// A job somebody is waiting for is not a prefetch any more.
		if len(job.waiters) == 0 && !wanted[job.key] {
			delete(p.jobs, job.key)
			continue
		}
		kept = append(kept, job)
	}
	p.queue = kept
	p.mu.Unlock()

	for _, path := range paths {
		key := imageCacheKey{Source: source, Path: path}
		p.mu.Lock()
		_, cached := p.cache[key]
		p.mu.Unlock()
		if cached {
			continue
		}
		p.request(v, path, false, imageWaiter{})
	}
}

// Invalidate forgets one picture, so that the next request decodes it again.
func (p *ImagePipeline) Invalidate(v vfs.VFS, path string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := imageCacheKey{Source: imageSource(v), Path: path}
	p.drop(key)
	p.dropPreview(key)
}

// Clear forgets every picture.
func (p *ImagePipeline) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cache = make(map[imageCacheKey]*imageEntry)
	p.lru = nil
	p.bytes = 0
	p.previews = make(map[imageCacheKey]ImageResult)
	p.previewOrder = nil
}

// CacheStats reports how much the pipeline is holding on to.
func (p *ImagePipeline) CacheStats() (int, int64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.cache), p.bytes
}

func (p *ImagePipeline) request(v vfs.VFS, path string, urgent bool, w imageWaiter) {
	key := imageCacheKey{Source: imageSource(v), Path: path}

	p.mu.Lock()
	defer p.mu.Unlock()

	if job, ok := p.jobs[key]; ok {
		if w.fn != nil {
			job.waiters = append(job.waiters, w)
		}
		if urgent {
			job.urgent = true
		}
		return
	}

	job := &imageJob{key: key, v: v, path: path, urgent: urgent}
	if w.fn != nil {
		job.waiters = append(job.waiters, w)
	}
	p.jobs[key] = job
	p.queue = append(p.queue, job)
	p.pump()
}

// pump starts as many queued jobs as there are free workers. The caller
// holds the lock.
func (p *ImagePipeline) pump() {
	for p.busy < p.workers {
		job := p.nextJob()
		if job == nil {
			return
		}
		p.busy++
		// Read here: pump runs on the goroutine that asked for the picture,
		// while p.run is the worker that outlives it.
		go p.run(job, vtui.FrameManager)
	}
}

// nextJob takes the most deserving job out of the queue: somebody is looking
// at an urgent one right now, a prefetched one may still be needed later.
func (p *ImagePipeline) nextJob() *imageJob {
	best := -1
	for i, job := range p.queue {
		if best < 0 || (job.urgent && !p.queue[best].urgent) {
			best = i
		}
	}
	if best < 0 {
		return nil
	}
	job := p.queue[best]
	p.queue = append(p.queue[:best], p.queue[best+1:]...)
	return job
}

func (p *ImagePipeline) run(job *imageJob, frames *vtui.FrameManagerType) {
	surf, decoder, err := p.load(context.Background(), job.v, job.path)
	res := ImageResult{Path: job.path, Surface: surf, Decoder: decoder, Err: err}

	p.mu.Lock()
	delete(p.jobs, job.key)
	p.busy--
	if err == nil && surf.Valid() {
		p.store(job.key, res)
	}
	waiters := job.waiters
	job.waiters = nil
	p.pump()
	p.mu.Unlock()

	var onUI []imageWaiter
	for _, w := range waiters {
		if w.ui {
			onUI = append(onUI, w)
			continue
		}
		w.fn(res)
	}
	if len(onUI) > 0 {
		dispatch := p.dispatch
		if dispatch == nil {
			dispatch = func(fn func()) {
				if frames != nil {
					frames.PostTask(fn)
				}
			}
		}
		dispatch(func() {
			for _, w := range onUI {
				w.fn(res)
			}
		})
	}
}

// store puts a picture into the cache and throws out the ones nobody has
// looked at for the longest. The caller holds the lock.
func (p *ImagePipeline) store(key imageCacheKey, res ImageResult) {
	bytes := int64(res.Surface.Width) * int64(res.Surface.Height) * 4
	p.drop(key)
	p.cache[key] = &imageEntry{res: res, bytes: bytes}
	p.lru = append(p.lru, key)
	p.bytes += bytes

	// The newest picture is the one on screen, so it stays even when it
	// alone is larger than the whole budget.
	for p.bytes > p.limit && len(p.lru) > 1 {
		p.drop(p.lru[0])
	}
}

func (p *ImagePipeline) drop(key imageCacheKey) {
	entry, ok := p.cache[key]
	if !ok {
		return
	}
	delete(p.cache, key)
	p.bytes -= entry.bytes
	for i, k := range p.lru {
		if k == key {
			p.lru = append(p.lru[:i], p.lru[i+1:]...)
			break
		}
	}
}

// touch moves a picture to the young end of the eviction order.
func (p *ImagePipeline) touch(key imageCacheKey) {
	for i, k := range p.lru {
		if k == key {
			p.lru = append(p.lru[:i], p.lru[i+1:]...)
			break
		}
	}
	p.lru = append(p.lru, key)
}

// ImageNeighbourhood picks the pictures worth decoding before they are asked
// for: the nearest ones on both sides of the current position, forward
// first, because that is the way people usually go.
func ImageNeighbourhood(paths []string, index, radius int) []string {
	if index < 0 || index >= len(paths) || radius <= 0 {
		return nil
	}
	out := make([]string, 0, 2*radius)
	for step := 1; step <= radius; step++ {
		if i := index + step; i < len(paths) {
			out = append(out, paths[i])
		}
		if i := index - step; i >= 0 {
			out = append(out, paths[i])
		}
	}
	return out
}
