package extui

import (
	"reflect"
	"testing"
)

func TestEnvelopeSnapshotShape(t *testing.T) {
	envelope := Envelope{
		Sequence: 4, StreamID: "panel/1", Revision: 7,
		Kind: KindSnapshot, Payload: M{"totalCount": 30000},
	}
	if err := envelope.Validate(); err != nil {
		t.Fatal(err)
	}
	want := M{
		"type": EnvelopeType, "version": EnvelopeVersion,
		"sequence": uint64(4), "streamId": "panel/1",
		"revision": uint64(7), "kind": KindSnapshot,
		"payload": M{"totalCount": 30000},
	}
	if got := envelope.ToMap(); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %#v, want %#v", got, want)
	}
}

func TestEnvelopePatchRequiresOrderedBaseRevision(t *testing.T) {
	valid := Envelope{
		Sequence: 9, StreamID: "menus", Revision: 3,
		BaseRevision: Revision(2), Kind: KindPatch, Payload: M{},
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := valid
	invalid.BaseRevision = nil
	if err := invalid.Validate(); err == nil {
		t.Fatal("incremental envelope without base revision was accepted")
	}
	invalid.BaseRevision = Revision(3)
	if err := invalid.Validate(); err == nil {
		t.Fatal("non-increasing stream revision was accepted")
	}
}
