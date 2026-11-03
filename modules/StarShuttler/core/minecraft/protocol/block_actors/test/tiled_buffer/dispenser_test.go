package tiled_buffer

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/block_actors"
)

func TestDecodeDispenserAndDropperFixtures(t *testing.T) {
	for _, fixture := range NewPool() {
		if fixture.ID != block_actors.IDDispenser && fixture.ID != block_actors.IDDropper {
			continue
		}

		t.Run(fixture.ID, func(t *testing.T) {
			buffer := bytes.NewBuffer(fixture.Buffer)
			block, err := Decode(fixture.ID, buffer)
			if err != nil {
				t.Fatalf("Decode(%q): %v", fixture.ID, err)
			}
			if block.ID() != fixture.ID {
				t.Fatalf("Decode(%q) returned ID %q", fixture.ID, block.ID())
			}
			if buffer.Len() != 0 {
				t.Fatalf("Decode(%q) left %d bytes", fixture.ID, buffer.Len())
			}

			switch value := block.(type) {
			case *block_actors.Dispenser:
				if value.CustomName != "Liliya233" {
					t.Fatalf("CustomName = %q, want %q", value.CustomName, "Liliya233")
				}
				if len(value.Items) == 0 {
					t.Fatalf("Decode(%q) returned no items", fixture.ID)
				}
			case *block_actors.Dropper:
				if len(value.Items) == 0 {
					t.Fatalf("Decode(%q) returned no items", fixture.ID)
				}
			}

			id, encoded := Encode(block)
			decoded, err := Decode(id, bytes.NewBuffer(encoded))
			if err != nil {
				t.Fatalf("round-trip Decode(%q): %v", id, err)
			}
			if !reflect.DeepEqual(block, decoded) {
				t.Fatalf("%s did not round-trip: got %#v, want %#v", id, decoded, block)
			}
		})
	}
}
