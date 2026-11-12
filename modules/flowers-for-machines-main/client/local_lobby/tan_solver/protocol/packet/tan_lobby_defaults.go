package packet

import "github.com/OmineDev/flowers-for-machines/client/local_lobby/tan_solver/protocol/encoding"

const (
	TanLobbyDefaultLevelID        = "World"
	TanLobbyDefaultGameType uint8 = 0
	TanLobbyRoomTipsText          = "test"
	TanLobbyDefaultVoice    int16 = 0
	// These values come from the captured 1.21.130.0 room tips.
	TanLobbyCurrentProtocolID  uint8 = 43
	TanLobbyCurrentGameVersion       = "1.21.130.0"
	TanLobbyPlatformPE         uint8 = 1
	TanLobbyLocalServerAddress       = "127.0.0.1|19132"
)

func DefaultTanRoomTips() encoding.RoomTips {
	return encoding.RoomTips{
		LevelID:            TanLobbyDefaultLevelID,
		GameType:           TanLobbyDefaultGameType,
		ConstantTestString: TanLobbyRoomTipsText,
		Vioce:              TanLobbyDefaultVoice,
		ProtocolID:         TanLobbyCurrentProtocolID,
		GameVersion:        TanLobbyCurrentGameVersion,
	}
}

func NewTanEnterRoomRequest(ownerID, roomID uint32, passcode, netherNetID string) TanEnterRoomRequest {
	return TanEnterRoomRequest{
		OwnerID:               ownerID,
		RoomID:                roomID,
		EnterPassword:         passcode,
		EnterTeamID:           0,
		EnterToken:            0,
		FollowTeamID:          0,
		NetherNetID:           netherNetID,
		SupportWebRTCCompress: true,
	}
}

func NewTanNotifyServerReady(netherNetID string) TanNotifyServerReady {
	return TanNotifyServerReady{
		ServerAddress:         TanLobbyLocalServerAddress,
		ServerRaknetGuid:      "",
		RTCRoomID:             "",
		NetherNetID:           netherNetID,
		WebRTCCompressEnabled: true,
	}
}
