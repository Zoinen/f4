package vtui

import (
	"encoding/base64"
	"strconv"
	"strings"
)

const (
	// kittyChunkSize is the maximum amount of base64 payload per escape
	// sequence mandated by the protocol.
	kittyChunkSize = 4096

	// kittyIDBase keeps our image identifiers away from the low numbers
	// other clients running in the same terminal tend to pick.
	kittyIDBase = 0x76740000

	// kittyCacheLimit bounds how many uploaded images we keep alive in the
	// terminal. Gallery mode walks through many files, and every kept image
	// costs memory on the terminal side.
	kittyCacheLimit = 48
)

type kittyPlacementRef struct {
	image     uint32
	placement uint32
}

// kittyEncoder speaks the kitty graphics protocol. It keeps track of what has
// already been uploaded so that panning or scrolling only re-sends the cheap
// placement commands instead of the pixels.
type kittyEncoder struct {
	uploaded map[uint64]uint32
	order    []uint64
	nextID   uint32
	placed   []kittyPlacementRef
}

func newKittyEncoder() *kittyEncoder {
	return &kittyEncoder{
		uploaded: make(map[uint64]uint32),
		nextID:   kittyIDBase,
	}
}

// Reset drops every uploaded image, both locally and in the terminal. Used
// when our idea of the terminal state can no longer be trusted (reattach,
// resize, hard reset), because stale placements would otherwise linger on
// top of the freshly painted text.
func (k *kittyEncoder) Reset(sb *strings.Builder) {
	if sb != nil && (len(k.placed) > 0 || len(k.uploaded) > 0) {
		sb.WriteString("\x1b_Ga=d,d=A,q=2\x1b\\")
	}
	k.uploaded = make(map[uint64]uint32)
	k.order = k.order[:0]
	k.placed = k.placed[:0]
}

// Render replaces the currently visible placements with the given list.
func (k *kittyEncoder) Render(sb *strings.Builder, list []ImagePlacement) {
	k.removePlacements(sb)
	pid := uint32(0)
	for i := range list {
		p := &list[i]
		if !p.Surface.Valid() || p.Cols <= 0 || p.Rows <= 0 {
			continue
		}
		pid++
		k.emit(sb, p, pid)
	}
}

func (k *kittyEncoder) removePlacements(sb *strings.Builder) {
	for _, ref := range k.placed {
		sb.WriteString("\x1b_Ga=d,d=i,q=2,i=")
		sb.WriteString(strconv.FormatUint(uint64(ref.image), 10))
		sb.WriteString(",p=")
		sb.WriteString(strconv.FormatUint(uint64(ref.placement), 10))
		sb.WriteString("\x1b\\")
	}
	k.placed = k.placed[:0]
}

func (k *kittyEncoder) emit(sb *strings.Builder, p *ImagePlacement, pid uint32) {
	sx, sy, sw, sh := p.Source()
	if sw <= 0 || sh <= 0 {
		return
	}
	key := kittyCacheKey(p.Surface.Hash(), sx, sy, sw, sh)
	id, known := k.uploaded[key]
	if !known {
		id = k.nextID
		k.nextID++
		k.uploaded[key] = id
		k.order = append(k.order, key)
		k.evict(sb)
		k.upload(sb, p.Surface, sx, sy, sw, sh, id)
	}
	k.place(sb, p, id, pid)
	k.placed = append(k.placed, kittyPlacementRef{image: id, placement: pid})
}

func kittyCacheKey(hash uint64, x, y, w, h int) uint64 {
	key := hash
	for _, v := range [...]int{x, y, w, h} {
		key ^= uint64(uint32(v))
		key *= fnvPrime64
	}
	return key
}

func (k *kittyEncoder) evict(sb *strings.Builder) {
	for len(k.order) > kittyCacheLimit {
		oldest := k.order[0]
		k.order = k.order[1:]
		id, ok := k.uploaded[oldest]
		if !ok {
			continue
		}
		delete(k.uploaded, oldest)
		sb.WriteString("\x1b_Ga=d,d=I,q=2,i=")
		sb.WriteString(strconv.FormatUint(uint64(id), 10))
		sb.WriteString("\x1b\\")
	}
}

func (k *kittyEncoder) upload(sb *strings.Builder, surf *ImageSurface, sx, sy, sw, sh int, id uint32) {
	raw := make([]byte, 0, sw*sh*4)
	for row := 0; row < sh; row++ {
		off := (sy+row)*surf.Stride + sx*4
		raw = append(raw, surf.Pix[off:off+sw*4]...)
	}
	payload := base64.StdEncoding.EncodeToString(raw)

	first := true
	for {
		chunk := payload
		if len(chunk) > kittyChunkSize {
			chunk = chunk[:kittyChunkSize]
		}
		payload = payload[len(chunk):]
		more := "0"
		if len(payload) > 0 {
			more = "1"
		}

		sb.WriteString("\x1b_G")
		if first {
			sb.WriteString("a=t,q=2,f=32,t=d,i=")
			sb.WriteString(strconv.FormatUint(uint64(id), 10))
			sb.WriteString(",s=")
			sb.WriteString(strconv.Itoa(sw))
			sb.WriteString(",v=")
			sb.WriteString(strconv.Itoa(sh))
			sb.WriteString(",m=")
			first = false
		} else {
			sb.WriteString("m=")
		}
		sb.WriteString(more)
		sb.WriteByte(';')
		sb.WriteString(chunk)
		sb.WriteString("\x1b\\")

		if len(payload) == 0 {
			break
		}
	}
}

func (k *kittyEncoder) place(sb *strings.Builder, p *ImagePlacement, id, pid uint32) {
	sb.WriteString("\x1b[")
	sb.WriteString(strconv.Itoa(p.Row + 1))
	sb.WriteByte(';')
	sb.WriteString(strconv.Itoa(p.Col + 1))
	sb.WriteByte('H')

	sb.WriteString("\x1b_Ga=p,q=2,C=1,i=")
	sb.WriteString(strconv.FormatUint(uint64(id), 10))
	sb.WriteString(",p=")
	sb.WriteString(strconv.FormatUint(uint64(pid), 10))
	sb.WriteString(",c=")
	sb.WriteString(strconv.Itoa(p.Cols))
	sb.WriteString(",r=")
	sb.WriteString(strconv.Itoa(p.Rows))
	sb.WriteString(",z=")
	sb.WriteString(strconv.Itoa(p.ZIndex))
	sb.WriteString("\x1b\\")
}

// RenderGraphics implements GraphicsRenderer for the ANSI text backend.
func (r *AnsiRenderer) RenderGraphics(layer *GraphicsLayer, buf, shadow []CharInfo, w, h int, force bool) {
	if layer == nil {
		return
	}

	proto := layer.Protocol()
	if proto != r.gfxProto {
		r.gfxKitty = nil
		r.gfxProto = proto
		force = true
	}

	gen := layer.Generation()
	if !force && gen == r.gfxGen && !layer.DirtyUnder(buf, shadow, w, h) {
		return
	}
	r.gfxGen = gen

	switch proto {
	case GraphicsKitty:
		if r.gfxKitty == nil {
			r.gfxKitty = newKittyEncoder()
		}
		if force {
			r.gfxKitty.Reset(&r.frameOut)
		}
		r.gfxList, _ = layer.Snapshot(r.gfxList)
		r.gfxKitty.Render(&r.frameOut, r.gfxList)
	}
}
