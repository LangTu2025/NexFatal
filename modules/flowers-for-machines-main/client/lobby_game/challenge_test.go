package lobby_game

import (
	"context"
	"errors"
	"github.com/OmineDev/flowers-for-machines/core/minecraft"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/core/py_rpc"
	"github.com/google/uuid"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeChallengeConn struct {
	incoming    []packet.Packet
	written     []packet.Packet
	deadline    time.Time
	writeErr    error
	acknowledge bool
	emptyErr    error
}

func (c *fakeChallengeConn) ReadPacket() (packet.Packet, error) {
	if len(c.incoming) > 0 {
		p := c.incoming[0]
		c.incoming = c.incoming[1:]
		return p, nil
	}
	if c.emptyErr != nil {
		return nil, c.emptyErr
	}
	if !c.deadline.IsZero() {
		time.Sleep(max(0, time.Until(c.deadline)))
		return nil, context.DeadlineExceeded
	}
	return nil, io.EOF
}
func (c *fakeChallengeConn) WritePacket(p packet.Packet) error {
	if c.writeErr != nil {
		return c.writeErr
	}
	c.written = append(c.written, p)
	if request, ok := p.(*packet.CommandRequest); ok && c.acknowledge {
		c.incoming = append(c.incoming, &packet.CommandOutput{CommandOrigin: request.CommandOrigin})
	}
	return nil
}
func (c *fakeChallengeConn) Flush() error                      { return nil }
func (c *fakeChallengeConn) SetReadDeadline(d time.Time) error { c.deadline = d; return nil }
func (c *fakeChallengeConn) GameData() minecraft.GameData {
	return minecraft.GameData{EntityUniqueID: 4682670425802129213}
}

type fakeChallengeAuth struct {
	start, check int
	response     string
	err          error
	checkArg     string
}

func (a *fakeChallengeAuth) TransferData(s string) (string, error) { a.start++; return "answer", a.err }
func (a *fakeChallengeAuth) TransferCheckNum(s string) (string, error) {
	a.check++
	a.checkArg = s
	return a.response, a.err
}

func checkRequest() *packet.PyRpc {
	return &packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.GetMCPCheckNum{FirstArg: "first", SecondArg: py_rpc.GetMCPCheckNumSecondArg{Arg: "second", FirstExtraData: []any{}, SecondExtraData: []any{}}})}
}

func TestChallengeHandoffAndCommandProbe(t *testing.T) {
	conn := &fakeChallengeConn{acknowledge: true, incoming: []packet.Packet{
		&packet.Text{Message: "cached"},
		&packet.CommandOutput{CommandOrigin: protocol.CommandOrigin{UUID: uuid.New()}},
		&packet.PyRpc{Value: py_rpc.Marshal(&py_rpc.StartType{Type: py_rpc.StartTypeRequest, Content: "start"})},
		checkRequest(), checkRequest(),
	}}
	auth := &fakeChallengeAuth{response: `[1,2,3,4,5,6,4682670425802129213,8]`}
	cached, err := solveChallenge(conn, auth, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !conn.deadline.IsZero() || auth.start != 1 || auth.check != 1 {
		t.Fatalf("bad handoff or repeated challenge: %+v", auth)
	}
	if !strings.Contains(auth.checkArg, "4682670425802129213") {
		t.Fatalf("lost unique ID: %s", auth.checkArg)
	}
	var count int
	for range cached {
		count++
	}
	if count != 6 {
		t.Fatalf("cached packet count = %d, want 6", count)
	}
	if len(conn.written) != 3 {
		t.Fatalf("outgoing count = %d", len(conn.written))
	}
	probe := conn.written[2].(*packet.CommandRequest)
	if probe.CommandLine != "list" || probe.CommandOrigin.Origin != protocol.CommandOriginAutomationPlayer {
		t.Fatalf("wrong command origin: %+v", probe)
	}
	response := conn.written[1].(*packet.PyRpc).Value.([]any)
	values := response[1].([]any)[0].([]any)
	if values[6] != int64(4682670425802129213) {
		t.Fatalf("integer was rounded: %#v", values[6])
	}
}

func TestChallengeTimeoutAndFailure(t *testing.T) {
	for _, ackMissing := range []bool{false, true} {
		conn := &fakeChallengeConn{}
		if ackMissing {
			conn.incoming = []packet.Packet{checkRequest()}
		}
		_, err := solveChallenge(conn, &fakeChallengeAuth{response: `[1]`}, 5*time.Millisecond)
		if !errors.Is(err, context.DeadlineExceeded) || !conn.deadline.IsZero() {
			t.Fatalf("timeout did not clean up: %v", err)
		}
	}
	sentinel := errors.New("write failed")
	conn := &fakeChallengeConn{incoming: []packet.Packet{checkRequest()}, writeErr: sentinel}
	if _, err := solveChallenge(conn, &fakeChallengeAuth{response: `[1]`}, time.Second); !errors.Is(err, sentinel) {
		t.Fatalf("lost write error: %v", err)
	}
	conn = &fakeChallengeConn{emptyErr: io.EOF}
	if _, err := solveChallenge(conn, &fakeChallengeAuth{}, time.Second); !errors.Is(err, io.EOF) {
		t.Fatalf("lost read error: %v", err)
	}
}

func TestDecodeCheckNumRejectsBadJSON(t *testing.T) {
	for _, raw := range []string{"bad", "null", "[]", "{}"} {
		if _, err := decodeCheckNum(raw); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}
