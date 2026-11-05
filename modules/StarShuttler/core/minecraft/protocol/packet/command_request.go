package packet

import (
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

// CommandRequest is sent by the client to request the execution of a server-side command. Although some
// servers support sending commands using the Text packet, this packet is guaranteed to have the correct
// result.
type CommandRequest struct {
	// CommandLine is the raw entered command line. The client does no parsing of the command line by itself
	// (unlike it did in the early stages), but lets the server do that.
	CommandLine string
	// CommandOrigin is the data specifying the origin of the command. In other words, the source that the
	// command was from, such as the player itself or a websocket server.
	CommandOrigin protocol.CommandOrigin
	// IsInternal specifies if the command request is internal. Setting it to false seems to work and the usage of
	// this field is not known.
	IsInternal bool
	// UnLimit is a Netease extension controlling whether the command is unlimited.
	UnLimit bool
	// Version is the version of the command that is being executed. This field currently has no purpose or functionality.
	Version string
}

// ID ...
func (*CommandRequest) ID() uint32 {
	return IDCommandRequest
}

func (pk *CommandRequest) Marshal(io protocol.IO) {
	io.String(&pk.CommandLine)
	protocol.CommandOriginDataString(io, &pk.CommandOrigin)
	io.Bool(&pk.IsInternal)
	io.Bool(&pk.UnLimit)
	if pk.Version == "" {
		pk.Version = "Latest"
	}
	io.String(&pk.Version)
}
