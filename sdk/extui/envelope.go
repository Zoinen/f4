package extui

import "fmt"

const (
	EnvelopeType    = "extui"
	EnvelopeVersion = 4

	KindSnapshot = "snapshot"
	KindPatch    = "patch"
	KindReset    = "reset"
	KindAppend   = "append"
	KindMetadata = "metadata"
	KindRows     = "rows"
	KindIntent   = "intent"
)

// Envelope is the only semantic ExtUI v4 wire envelope. Transport/control
// messages (hello, palette, fallback cell frame, cursor and quit) stay on the
// same length-prefixed MessagePack channel but are not semantic streams.
//
// Sequence orders the connection globally. Revision orders one StreamID;
// BaseRevision is omitted for a self-contained snapshot and required for every
// incremental message. A receiver can therefore resynchronize one stream
// without invalidating panels, documents, menus, or any other unrelated state.
type Envelope struct {
	Sequence     uint64
	StreamID     string
	Revision     uint64
	BaseRevision *uint64
	Kind         string
	Payload      any
}

func (e Envelope) Validate() error {
	if e.Sequence == 0 {
		return fmt.Errorf("extui envelope sequence must be non-zero")
	}
	if e.StreamID == "" {
		return fmt.Errorf("extui envelope streamId must be non-empty")
	}
	if e.Revision == 0 {
		return fmt.Errorf("extui envelope revision must be non-zero")
	}
	if e.Kind == "" {
		return fmt.Errorf("extui envelope kind must be non-empty")
	}
	if e.Kind == KindSnapshot {
		if e.BaseRevision != nil {
			return fmt.Errorf("extui snapshot must not have baseRevision")
		}
	} else if e.BaseRevision == nil {
		return fmt.Errorf("extui incremental envelope requires baseRevision")
	} else if *e.BaseRevision >= e.Revision {
		return fmt.Errorf("extui baseRevision must precede revision")
	}
	return nil
}

func (e Envelope) ToMap() M {
	out := M{
		"type":     EnvelopeType,
		"version":  EnvelopeVersion,
		"sequence": e.Sequence,
		"streamId": e.StreamID,
		"revision": e.Revision,
		"kind":     e.Kind,
		"payload":  e.Payload,
	}
	if e.BaseRevision != nil {
		out["baseRevision"] = *e.BaseRevision
	}
	return out
}

func Revision(value uint64) *uint64 {
	return &value
}
