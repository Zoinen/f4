package vfs

import (
	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/vmihailenco/msgpack/v5"
	"testing"
	"time"
)

func TestGroupMetadataCompatibility(t *testing.T) {
	item := VFSItem{KnownMetadata: MetadataExplicit | MetadataUID | MetadataGID | MetadataPermissions | MetadataPhysicalSize}
	for _, field := range []MetadataFields{MetadataUID, MetadataGID, MetadataPermissions, MetadataPhysicalSize} {
		if !item.HasMetadata(field) {
			t.Fatal(field)
		}
	}
	item.ATime = time.Now()
	if item.HasMetadata(MetadataATime) {
		t.Fatal("explicit absence lost")
	}
	legacy := VFSItem{}
	if legacy.HasMetadata(MetadataUID) || legacy.HasMetadata(MetadataPermissions) {
		t.Fatal("ambiguous zero treated as known")
	}
	legacy.Uid = 12
	if !legacy.HasMetadata(MetadataUID) {
		t.Fatal("legacy nonzero owner")
	}
	packet, err := msgpack.Marshal(f4plugin.VFSItem{Name: "root", KnownMetadata: f4plugin.MetadataExplicit | f4plugin.MetadataUID})
	if err != nil {
		t.Fatal(err)
	}
	var decoded VFSItem
	if err := msgpack.Unmarshal(packet, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.HasMetadata(MetadataUID) || decoded.Uid != 0 {
		t.Fatalf("RPC root owner lost: %+v", decoded)
	}
	oldPacket, _ := msgpack.Marshal(map[string]any{"Name": "legacy", "Size": int64(3)})
	if err := msgpack.Unmarshal(oldPacket, &decoded); err != nil {
		t.Fatal(err)
	}
	var old VFSItem
	_ = msgpack.Unmarshal(oldPacket, &old)
	if old.HasMetadata(MetadataUID) {
		t.Fatal("old packet owner")
	}
}
