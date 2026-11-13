package task

import (
	"testing"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

func TestDomainRepairParsesLegacyTeleportCoordinates(t *testing.T) {
	output := &packet.CommandOutput{SuccessCount: 1, OutputMessages: []protocol.CommandOutputMessage{{Success: true, Message: "commands.tp.success.coordinates", Parameters: []string{"12345", "704.25", "-57", "5.75"}}}}
	position, ok := domainRepairTeleportCoordinates(output)
	if !ok || position != ([3]float64{704.25, -57, 5.75}) {
		t.Fatalf("position=%v ok=%v", position, ok)
	}
}

func TestDomainRepairRejectsUnsuccessfulAndAmbiguousTeleportOutput(t *testing.T) {
	for _, output := range []*packet.CommandOutput{
		{OutputMessages: []protocol.CommandOutputMessage{{Parameters: []string{"123", "456", "789"}}}},
		{SuccessCount: 1, OutputMessages: []protocol.CommandOutputMessage{{Success: true, Message: "commands.tp.success", Parameters: []string{"123", "456", "789"}}}},
		{SuccessCount: 1, OutputMessages: []protocol.CommandOutputMessage{{Success: true, Message: "commands.tp.success.coordinates", Parameters: []string{"123", "NaN", "0", "0"}}}},
	} {
		if position, ok := domainRepairTeleportCoordinates(output); ok {
			t.Fatalf("unverified coordinates accepted: %v", position)
		}
	}
}

func TestDomainRepairDoesNotUsePlayerNameDigitsAsCoordinates(t *testing.T) {
	output := &packet.CommandOutput{OutputMessages: []protocol.CommandOutputMessage{{Parameters: []string{"player 浪兔工作室02 not found"}}}}
	if position, ok := domainRepairTeleportCoordinates(output); ok || position != ([3]float64{}) {
		t.Fatalf("invalid output parsed as %v", position)
	}
}
