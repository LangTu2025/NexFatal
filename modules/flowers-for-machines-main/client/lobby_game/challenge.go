package lobby_game

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	"github.com/google/uuid"
)

type challengeConnection interface {
	ReadPacket() (packet.Packet, error)
	WritePacket(packet.Packet) error
	Flush() error
	SetReadDeadline(time.Time) error
	GameData() minecraft.GameData
}

type challengeAuthenticator interface {
	TransferData(string) (string, error)
	TransferCheckNum(string) (string, error)
}

// One reader owns the connection until handoff, including timeout and retries.
// This keeps late reads from racing the game interface after login.
func solveChallenge(conn challengeConnection, authClient challengeAuthenticator, timeout time.Duration) (chan packet.Packet, error) {
	deadline := time.Now().Add(timeout)
	defer conn.SetReadDeadline(time.Time{})
	var cached []packet.Packet
	passed := false
	requestID := uuid.New()
	nextProbe := deadline
	for {
		now := time.Now()
		if !now.Before(deadline) {
			return nil, fmt.Errorf("LobbyGame MCP challenge: %w", context.DeadlineExceeded)
		}
		if passed && !now.Before(nextProbe) {
			err := conn.WritePacket(&packet.CommandRequest{
				CommandLine:   "list",
				CommandOrigin: protocol.CommandOrigin{Origin: protocol.CommandOriginAutomationPlayer, UUID: requestID, RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694"},
				Version:       packet.NeteaseCommandRequestVersion,
			})
			if err != nil {
				return nil, fmt.Errorf("LobbyGame MCP command probe: %w", err)
			}
			if err := conn.Flush(); err != nil {
				return nil, err
			}
			nextProbe = time.Now().Add(time.Second / 20)
		}
		readDeadline := deadline
		if passed && nextProbe.Before(readDeadline) {
			readDeadline = nextProbe
		}
		if err := conn.SetReadDeadline(readDeadline); err != nil {
			return nil, err
		}
		pk, err := conn.ReadPacket()
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			return nil, fmt.Errorf("LobbyGame MCP read: %w", err)
		}
		if len(cached) >= 32767 {
			return nil, errors.New("LobbyGame MCP packet cache is full")
		}
		cached = append(cached, pk)
		switch p := pk.(type) {
		case *packet.PyRpc:
			wasPassed := passed
			passed, err = handleChallenge(conn, authClient, p, passed)
			if err != nil {
				return nil, fmt.Errorf("LobbyGame MCP response: %w", err)
			}
			if passed && !wasPassed {
				nextProbe = time.Now()
			}
		case *packet.CommandOutput:
			if passed && p.CommandOrigin.UUID == requestID {
				packets := make(chan packet.Packet, len(cached))
				for _, cachedPacket := range cached {
					packets <- cachedPacket
				}
				close(packets)
				return packets, nil
			}
		}
	}
}

func handleChallenge(conn challengeConnection, authClient challengeAuthenticator, pk *packet.PyRpc, passed bool) (bool, error) {
	if pk.Value == nil {
		return passed, nil
	}
	content, err := py_rpc.Unmarshal(pk.Value)
	if err != nil {
		return passed, err
	}
	var response py_rpc.PyRpc
	switch c := content.(type) {
	case *py_rpc.StartType:
		if c.Type != py_rpc.StartTypeRequest {
			return passed, nil
		}
		c.Content, err = authClient.TransferData(c.Content)
		if err != nil {
			return passed, err
		}
		c.Type = py_rpc.StartTypeResponse
		response = c
	case *py_rpc.GetMCPCheckNum:
		if passed {
			return passed, nil
		}
		arg, err := json.Marshal([]any{c.FirstArg, c.SecondArg.Arg, conn.GameData().EntityUniqueID})
		if err != nil {
			return passed, err
		}
		ret, err := authClient.TransferCheckNum(string(arg))
		if err != nil {
			return passed, err
		}
		values, err := decodeCheckNum(ret)
		if err != nil {
			return passed, err
		}
		response = &py_rpc.SetMCPCheckNum{values}
	default:
		return passed, nil
	}
	if err := conn.WritePacket(&packet.PyRpc{Value: py_rpc.Marshal(response), OperationType: packet.PyRpcOperationTypeSend}); err != nil {
		return passed, err
	}
	if err := conn.Flush(); err != nil {
		return passed, err
	}
	_, checkNum := content.(*py_rpc.GetMCPCheckNum)
	return passed || checkNum, nil
}

func decodeCheckNum(raw string) ([]any, error) {
	var values []any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, errors.New("empty LobbyGame MCP response")
	}
	// Preserve the integer at index 6 exactly; float64 would round 64-bit IDs.
	if len(values) > 6 {
		var fields []json.RawMessage
		_ = json.Unmarshal([]byte(raw), &fields)
		var number json.Number
		if json.Unmarshal(fields[6], &number) == nil {
			if integer, err := number.Int64(); err == nil {
				values[6] = integer
			}
		}
	}
	return values, nil
}
