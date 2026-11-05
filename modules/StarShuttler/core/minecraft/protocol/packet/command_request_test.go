package packet

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

func TestCommandRequestMarshalFieldOrder(t *testing.T) {
	request := &CommandRequest{
		CommandLine: "say test",
		CommandOrigin: protocol.CommandOrigin{
			Origin:    protocol.CommandOriginAutomationPlayer,
			UUID:      uuid.MustParse("bd903e4a-8617-4053-8321-821365777777"),
			RequestID: "request-id",
		},
		IsInternal: true,
		UnLimit:    false,
		Version:    "Latest",
	}

	actual := bytes.NewBuffer(nil)
	request.Marshal(protocol.NewWriter(actual, 0))

	expected := bytes.NewBuffer(nil)
	writer := protocol.NewWriter(expected, 0)
	writer.String(&request.CommandLine)
	protocol.CommandOriginDataString(writer, &request.CommandOrigin)
	isInternal := true
	unLimit := false
	version := "Latest"
	writer.Bool(&isInternal)
	writer.Bool(&unLimit)
	writer.String(&version)

	if !bytes.Equal(actual.Bytes(), expected.Bytes()) {
		t.Fatalf("command request fields were not marshalled in protocol order")
	}
}
