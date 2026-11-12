package resources_control

import (
	"context"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"testing"
	"time"
)

func TestStructureResponsesDispatchByNameOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	resources := &Resources{structureRequest: NewStructureRequestCallback(), listener: NewPacketListener(ctx)}
	replies := make(chan *packet.StructureTemplateDataResponse, 2)
	request := &packet.StructureTemplateDataRequest{StructureName: "nexfatal:export_wanted"}
	resources.StructureRequest().SetStructureRequestCallback(request, func(p *packet.StructureTemplateDataResponse) { replies <- p })
	resources.handlePacket(&packet.StructureTemplateDataResponse{StructureName: "unrelated", Success: true})
	if _, ok := resources.structureRequest.callback.Load(request.StructureName); !ok {
		t.Fatal("unrelated response consumed export callback")
	}
	wanted := &packet.StructureTemplateDataResponse{StructureName: request.StructureName, Success: true, StructureTemplate: map[string]any{"marker": "preserved"}}
	resources.handlePacket(wanted)
	resources.handlePacket(wanted)
	select {
	case got := <-replies:
		if got != wanted {
			t.Fatal("structure payload changed during dispatch")
		}
	case <-time.After(time.Second):
		t.Fatal("Flowers did not dispatch structure response")
	}
	if _, ok := resources.structureRequest.callback.Load(request.StructureName); ok {
		t.Fatal("finished callback leaked")
	}
	select {
	case <-replies:
		t.Fatal("duplicate response invoked callback twice")
	case <-time.After(20 * time.Millisecond):
	}
	resources.StructureRequest().SetStructureRequestCallback(request, func(p *packet.StructureTemplateDataResponse) { replies <- p })
	resources.StructureRequest().DeleteStructureRequestCallback(request.StructureName)
	if _, ok := resources.structureRequest.callback.Load(request.StructureName); ok {
		t.Fatal("canceled callback leaked")
	}
}
