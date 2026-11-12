package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"

	"github.com/google/uuid"
)

// CopeChallenge preserves the existing API.
func (m *MCPCheckChallengesSolver) CopeChallenge() error {
	return m.CopeChallengeContext(context.Background())
}

func (m *MCPCheckChallengesSolver) CopeChallengeContext(parent context.Context) error {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	conn := m.client.connection
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline, _ := ctx.Deadline()
	if err := conn.SetReadDeadline(deadline); err != nil {
		return err
	}
	defer conn.SetReadDeadline(time.Time{})
	var cached []packet.Packet
	var requestID uuid.UUID
	sent := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		pk, err := conn.ReadPacket()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("MCP challenge: %w", err)
		}
		if rpc, ok := pk.(*packet.PyRpc); ok {
			if err := m.onPyRpc(ctx, rpc); err != nil {
				return err
			}
		} else if response, ok := pk.(*packet.CommandOutput); ok && sent && response.CommandOrigin.UUID == requestID {
			packets := make(chan packet.Packet, len(cached))
			for _, p := range cached {
				packets <- p
			}
			close(packets)
			m.client.cachedPacket = packets
			return nil
		} else {
			if len(cached) >= 32767 {
				return fmt.Errorf("MCP challenge packet buffer exceeded")
			}
			cached = append(cached, pk)
		}
		if m.client.getCheckNumEverPassed && !sent {
			requestID = uuid.New()
			origin := protocol.CommandOrigin{Origin: protocol.CommandOriginAutomationPlayer, UUID: requestID, RequestID: "96045347-a6a3-4114-94c0-1bc4cc561694"}
			if m.client.IsDomainGame() {
				origin.Origin = protocol.CommandOriginPlayer
				origin.PlayerUniqueID = conn.GameData().EntityUniqueID
			}
			if err := conn.WritePacket(&packet.CommandRequest{CommandLine: "list", CommandOrigin: origin, Version: 44}); err != nil {
				return err
			}
			sent = true
		}
	}
}

// onPyRpc ..
func (m *MCPCheckChallengesSolver) onPyRpc(ctx context.Context, p *packet.PyRpc) error {
	// prepare
	conn := m.client.connection
	client := m.client.authClient
	if p.Value == nil {
		return nil
	}

	// unmarshal
	content, err := py_rpc.Unmarshal(p.Value)
	if err != nil {
		return fmt.Errorf("onPyRpc: %v", err)
	}

	// do some actions for some specific PyRpc packets
	switch c := content.(type) {
	case *py_rpc.StartType:
		// get data and send packet
		c.Content, err = client.TransferDataContext(ctx, c.Content)
		if err != nil {
			return fmt.Errorf("onPyRpc: %v", err)
		}
		c.Type = py_rpc.StartTypeResponse
		conn.WritePacket(&packet.PyRpc{
			Value:         py_rpc.Marshal(c),
			OperationType: packet.PyRpcOperationTypeSend,
		})
	case *py_rpc.GetMCPCheckNum:
		// if the challenges has been down,
		// then we do NOTHING
		if m.client.getCheckNumEverPassed {
			break
		}
		// create request to the auth server and get response
		arg, _ := json.Marshal([]any{
			c.FirstArg,
			c.SecondArg.Arg,
			conn.GameData().EntityUniqueID,
		})
		ret, err := client.TransferCheckNumContext(ctx, string(arg))
		if err != nil {
			return fmt.Errorf("onPyRpc: %v", err)
		}
		// unmarshal response and adjust the data included
		ret_p := []any{}
		json.Unmarshal([]byte(ret), &ret_p)
		if len(ret_p) > 7 {
			ret6, ok := ret_p[6].(float64)
			if ok {
				ret_p[6] = int64(ret6)
			}
		}
		// send packet and mark this challenges was finished
		conn.WritePacket(&packet.PyRpc{
			Value:         py_rpc.Marshal(&py_rpc.SetMCPCheckNum{ret_p}),
			OperationType: packet.PyRpcOperationTypeSend,
		})
		m.client.getCheckNumEverPassed = true
	}

	// return
	return nil
}
