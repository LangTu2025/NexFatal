package packet

import (
	"strconv"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

// NeteaseCommandRequestVersion retains the numeric version used by existing
// Go and JSON callers. New requests use NeteaseCommandRequestWireVersion.
const NeteaseCommandRequestVersion int32 = 44

const NeteaseCommandRequestWireVersion = "Latest"

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
	// Internal specifies if the command request internal. Setting it to false seems to work and the usage of
	// this field is not known.
	Internal bool
	// PhoenixBuilder specific fields.
	// Author: LNSSPsd
	//
	// NetEase specified field, encoded immediately after Internal.
	UnLimited bool
	// Version retains the numeric Go and JSON API while the wire field is a string.
	Version        int32
	versionRaw     string
	versionValue   int32
	versionDecoded bool
}

// ID ...
func (*CommandRequest) ID() uint32 {
	return IDCommandRequest
}

func (pk *CommandRequest) Marshal(io protocol.IO) {
	io.String(&pk.CommandLine)
	protocol.CommandOriginData(io, &pk.CommandOrigin)
	io.Bool(&pk.Internal)
	io.Bool(&pk.UnLimited)
	version := NeteaseCommandRequestWireVersion
	if pk.versionDecoded && pk.Version == pk.versionValue {
		version = pk.versionRaw
	}
	io.String(&version)
	if _, reading := io.(*protocol.Reader); reading {
		pk.versionRaw, pk.versionDecoded = version, true
		pk.Version = 0
		if parsedVersion, err := strconv.ParseInt(version, 10, 32); err == nil {
			pk.Version = int32(parsedVersion)
		}
		pk.versionValue = pk.Version
	}
}
