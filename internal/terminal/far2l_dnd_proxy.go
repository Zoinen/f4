package terminal

// The semantic proxy of nested terminals (unxed/f4#1628, specification v0.2
// § 11): when the terminal f4 runs in hands it a drop and a program inside
// f4's own built-in terminal is bound for drops too, the drop is not passed
// through as bytes. The child gets an offer of its own, whose objects are read
// from the parent's offer on demand: DNDProxySource is the DNDSource that does
// it, so OfferDrop on the child terminal is all the wiring the drop needs.
//
//   - item IDs are this hop's own (1, 2, ...) and are mapped to the parent's
//     explicitly; the parent's IDs, RIDs and reference URIs are never shown
//     to the child, and no name is interpreted as a path;
//   - a READ of the child is served by READs of the parent, in as many parent
//     chunks as the parent's max_chunk needs, and returns when the child's
//     buffer is full or the item ends: nothing is cached, so memory is bounded
//     by the child's max_chunk times the child's window;
//   - Close releases the parent's offer, once, when the child's offer ends.

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/unxed/f4/internal/terminal/far2ldnd"
)

// dndProxyCloseTimeout bounds the CLOSE of the parent's offer.
const dndProxyCloseTimeout = 10 * time.Second

// dndParentOffer is the part of DNDClient the proxy needs.
type dndParentOffer interface {
	ListAll(ctx context.Context, offer far2ldnd.ID) ([]far2ldnd.Entry, error)
	Read(ctx context.Context, offer far2ldnd.ID, itemID, offset uint64, length uint32) (far2ldnd.ReadReply, error)
	Close(ctx context.Context, offer far2ldnd.ID, reason uint8) error
}

// DNDProxySource offers the objects of a parent offer to a child terminal.
type DNDProxySource struct {
	client   dndParentOffer
	offer    far2ldnd.ID
	maxChunk uint32
	entries  []far2ldnd.Entry
	parent   map[uint64]uint64 // this hop's item_id -> the parent's

	ctx    context.Context
	cancel context.CancelFunc
	once   sync.Once
}

// NewDNDProxySource lists the parent's offer and builds the source. maxChunk
// is the max_chunk the parent granted; a parent READ is never longer. Items
// without a STREAM representation cannot be forwarded and are left out; a
// reference is metadata of the parent's side and is not passed on.
func NewDNDProxySource(ctx context.Context, client dndParentOffer, offer far2ldnd.ID, maxChunk uint32) (*DNDProxySource, error) {
	if maxChunk == 0 {
		return nil, errors.New("terminal: DND proxy: no read chunk size granted by the parent")
	}
	in, err := client.ListAll(ctx, offer)
	if err != nil {
		return nil, err
	}
	s := &DNDProxySource{client: client, offer: offer, maxChunk: maxChunk, parent: make(map[uint64]uint64, len(in))}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	for _, e := range in {
		if e.Kind != far2ldnd.KindFile || e.Flags&far2ldnd.ItemStream == 0 {
			continue
		}
		local := uint64(len(s.entries)) + 1
		s.parent[local] = e.ItemID
		e.ItemID = local
		e.Flags &^= far2ldnd.ItemReference
		e.ReferenceURI = ""
		e.SourceNamespace = ""
		e.NativeEncoding, e.NativeName = far2ldnd.NativeNone, nil
		s.entries = append(s.entries, e)
	}
	return s, nil
}

// Entries implements DNDSource.
func (s *DNDProxySource) Entries() []far2ldnd.Entry {
	return append([]far2ldnd.Entry(nil), s.entries...)
}

// ReadAt implements DNDSource: it fills p from the parent's item, one parent
// chunk at a time, until p is full or the item ends.
func (s *DNDProxySource) ReadAt(itemID uint64, p []byte, off uint64) (int, bool, error) {
	pid, ok := s.parent[itemID]
	if !ok {
		return 0, false, errors.New("terminal: DND proxy: unknown item")
	}
	total := 0
	for total < len(p) {
		want := len(p) - total
		if uint64(want) > uint64(s.maxChunk) {
			want = int(s.maxChunk)
		}
		reply, err := s.client.Read(s.ctx, s.offer, pid, off+uint64(total), uint32(want)) //nolint:gosec // want <= max_chunk
		if err != nil {
			var se *far2ldnd.StatusError
			if errors.As(err, &se) && se.Status == far2ldnd.StatusChanged {
				return total, false, errors.Join(ErrDNDSourceChanged, err)
			}
			return total, false, err
		}
		if len(reply.Data) > want {
			return total, false, errors.New("terminal: DND proxy: the parent returned more than was asked")
		}
		total += copy(p[total:], reply.Data)
		if reply.Flags&far2ldnd.ReadEOF != 0 {
			return total, true, nil
		}
	}
	return total, false, nil
}

// Close implements DNDSource: it stops the reads still waiting on the parent
// and releases the parent's offer, once. The CLOSE goes out on its own
// goroutine, because the terminal calls Close with its state locked and the
// reply comes through the same event loop.
func (s *DNDProxySource) Close() { s.CloseWith(far2ldnd.CloseProcessed) }

// CloseWith is Close with the reason the parent's offer is released for: a
// child that never took the offer at all releases it as rejected.
func (s *DNDProxySource) CloseWith(reason uint8) {
	s.once.Do(func() {
		s.cancel()
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), dndProxyCloseTimeout)
			defer cancel()
			_ = s.client.Close(ctx, s.offer, reason)
		}()
	})
}
