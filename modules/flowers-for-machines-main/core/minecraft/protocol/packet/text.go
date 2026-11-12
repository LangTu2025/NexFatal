package packet

import (
	"strings"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
)

const (
	TextTypeRaw = iota
	TextTypeChat
	TextTypeTranslation
	TextTypePopup
	TextTypeJukeboxPopup
	TextTypeTip
	TextTypeSystem
	TextTypeWhisper
	TextTypeAnnouncement
	TextTypeObjectWhisper
	TextTypeObject
	TextTypeObjectAnnouncement
)

const (
	textCategoryMessageOnly = iota
	textCategoryAuthoredMessage
	textCategoryMessageWithParameters
)

// Text is sent by the client to the server to send chat messages, and by the server to the client to forward
// or send messages, which may be chat, popups, tips etc.
type Text struct {
	// TextType is the type of the text sent. When a client sends this to the server, it should always be
	// TextTypeChat. If the server sends it, it may be one of the other text types above.
	TextType byte
	// NeedsTranslation specifies if any of the messages need to be translated. It seems that where % is found
	// in translatable text types, these are translated regardless of this bool. Translatable text types
	// include TextTypeTranslation, TextTypeTip, TextTypePopup and TextTypeJukeboxPopup.
	NeedsTranslation bool
	// SourceName is the name of the source of the messages. This source is displayed in text types such as
	// the TextTypeChat and TextTypeWhisper, where typically the username is shown.
	SourceName string
	// Message is the message of the packet. This field is set for each TextType and is the main component of
	// the packet.
	Message string
	// Parameters is a list of parameters that should be filled into the message. These parameters are only
	// written if the type of the packet is TextTypeTranslation, TextTypeTip, TextTypePopup or TextTypeJukeboxPopup.
	Parameters []string
	// XUID is the XBOX Live user ID of the player that sent the message. It is only set for packets of
	// TextTypeChat. When sent to a player, the player will only be shown the chat message if a player with
	// this XUID is present in the player list and not muted, or if the XUID is empty.
	XUID string
	// PlatformChatID is an identifier only set for particular platforms when chatting (presumably only for
	// Nintendo Switch). It is otherwise an empty string, and is used to decide which players are able to
	// chat with each other.
	PlatformChatID string
	// FilteredMessage is a filtered version of Message with all the profanity removed. The client will use
	// this over Message if this field is not empty and they have the "Filter Profanity" setting enabled.
	FilteredMessage string

	// PhoenixBuilder specific fields.
	// Author: Liliya233, Happy2018new
	//
	// The following fields are NetEase specific.
	NeteaseExtraData []string
	// Unknown1 is a compatibility alias for NeteaseExtraData.
	Unknown1 []string
	Unknown2 string
}

// ID ...
func (*Text) ID() uint32 {
	return IDText
}

func (pk *Text) Marshal(io protocol.IO) {
	io.Bool(&pk.NeedsTranslation)

	var categoryType uint8
	switch pk.TextType {
	case TextTypeRaw, TextTypeTip, TextTypeSystem, TextTypeObjectWhisper, TextTypeObjectAnnouncement, TextTypeObject:
		categoryType = textCategoryMessageOnly
	case TextTypeChat, TextTypeWhisper, TextTypeAnnouncement:
		categoryType = textCategoryAuthoredMessage
	default:
		categoryType = textCategoryMessageWithParameters
	}
	io.Uint8(&categoryType)
	switch categoryType {
	case textCategoryMessageOnly:
		textStringConst(io, "raw")
		textStringConst(io, "tip")
		textStringConst(io, "systemMessage")
		textStringConst(io, "textObjectWhisper")
		textStringConst(io, "textObjectAnnouncement")
		textStringConst(io, "textObject")
	case textCategoryAuthoredMessage:
		textStringConst(io, "chat")
		textStringConst(io, "whisper")
		textStringConst(io, "announcement")
	default:
		textStringConst(io, "translate")
		textStringConst(io, "popup")
		textStringConst(io, "jukeboxPopup")
	}

	io.Uint8(&pk.TextType)
	switch pk.TextType {
	case TextTypeChat, TextTypeWhisper, TextTypeAnnouncement:
		io.String(&pk.SourceName)
		io.String(&pk.Message)
	case TextTypeRaw, TextTypeTip, TextTypeSystem, TextTypeObject, TextTypeObjectWhisper, TextTypeObjectAnnouncement:
		io.String(&pk.Message)
	case TextTypeTranslation, TextTypePopup, TextTypeJukeboxPopup:
		io.String(&pk.Message)
		protocol.FuncSlice(io, &pk.Parameters, io.String)
	}

	// PhoenixBuilder specific changes.
	// Author: Liliya233, Happy2018new
	//
	// Netease
	{
		switch pk.TextType {
		case TextTypeChat:
			extraData := pk.NeteaseExtraData
			_, reading := io.(*protocol.Reader)
			if reading {
				extraData = nil
			} else if extraData == nil {
				extraData = pk.Unknown1
			}
			protocol.FuncSlice(io, &extraData, io.String)
			if reading {
				pk.NeteaseExtraData, pk.Unknown1 = extraData, extraData
			}
		case TextTypePopup:
			io.String(&pk.Unknown2)
		}
	}

	io.String(&pk.XUID)
	io.String(&pk.PlatformChatID)
	filteredMessage := protocol.Optional[string]{}
	if pk.FilteredMessage != "" {
		filteredMessage = protocol.Option(pk.FilteredMessage)
	}
	protocol.OptionalFunc(io, &filteredMessage, io.String)
	if value, present := filteredMessage.Value(); present {
		pk.FilteredMessage = value
	} else {
		pk.FilteredMessage = ""
	}
}

func textStringConst(io protocol.IO, expected string) {
	value := expected
	io.String(&value)
	if !strings.EqualFold(value, expected) {
		io.InvalidValue(value, "text category string", "unexpected value")
	}
}
