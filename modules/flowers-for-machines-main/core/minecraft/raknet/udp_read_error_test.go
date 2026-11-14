package raknet

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/raknet/internal/message"
)

func oversizedUDPReadError(op string) error {
	errno := syscall.EMSGSIZE
	if runtime.GOOS == "windows" {
		errno = syscall.Errno(10040)
	}
	return &net.OpError{Op: "read", Net: "udp", Err: os.NewSyscallError(op, errno)}
}

func TestIsOversizedUDPReadError(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil"},
		{name: "read", err: oversizedUDPReadError("read"), want: true},
		{name: "recvmsg", err: oversizedUDPReadError("recvmsg"), want: true},
		{name: "bare errno", err: errors.Unwrap(errors.Unwrap(oversizedUDPReadError("read"))), want: true},
		{name: "message alone", err: errors.New("message too long")},
		{name: "closed", err: net.ErrClosed},
		{name: "deadline", err: os.ErrDeadlineExceeded},
		{name: "other syscall", err: os.NewSyscallError("recvmsg", syscall.EINVAL)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := isOversizedUDPReadError(tt.err); got != tt.want {
				t.Fatalf("isOversizedUDPReadError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestHandshakeUDPReadErrors(t *testing.T) {
	reply1, err := (&message.OpenConnectionReply1{ServerGUID: 1, MTU: 1200}).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	reply2, err := (&message.OpenConnectionReply2{
		ServerGUID: 1, ClientAddress: netip.MustParseAddrPort("127.0.0.1:19132"), MTU: 1200,
	}).MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []struct {
		name  string
		reply []byte
		run   func(*connState, context.Context) error
	}{
		{name: "discover MTU", reply: reply1, run: (*connState).discoverMTU},
		{name: "open connection", reply: reply2, run: (*connState).openConnection},
	} {
		t.Run(phase.name, func(t *testing.T) {
			for _, op := range []string{"read", "recvmsg"} {
				t.Run(op, func(t *testing.T) {
					conn := &scriptedUDPConn{}
					// More errors than MaxTransientErrors: discarded datagrams must
					// not consume the budget for unrelated network failures. Include
					// partial packets, which must never reach the RakNet decoder.
					for range 12 {
						conn.reads = append(conn.reads, udpReadResult{data: phase.reply[:2], err: oversizedUDPReadError(op)})
					}
					conn.reads = append(conn.reads, udpReadResult{data: phase.reply})
					state := testUDPConnState(t, conn)
					if err := phase.run(state, t.Context()); err != nil {
						t.Fatalf("handshake failed after oversized datagrams: %v", err)
					}
					if state.mtu != 1200 || conn.closed || state.transientErrorCount != 0 {
						t.Fatalf("mtu=%d, closed=%v, transient errors=%d", state.mtu, conn.closed, state.transientErrorCount)
					}
				})
			}
			for _, fatalErr := range []error{net.ErrClosed, os.ErrDeadlineExceeded, syscall.EINVAL} {
				t.Run(fatalErr.Error(), func(t *testing.T) {
					conn := &scriptedUDPConn{reads: []udpReadResult{
						{err: oversizedUDPReadError("recvmsg")}, {err: fatalErr}, {data: phase.reply},
					}}
					if err := phase.run(testUDPConnState(t, conn), t.Context()); !errors.Is(err, fatalErr) {
						t.Fatalf("handshake error = %v, want %v", err, fatalErr)
					}
					if !conn.closed || conn.readCount != 2 {
						t.Fatalf("fatal error did not stop handshake: closed=%v, reads=%d", conn.closed, conn.readCount)
					}
				})
			}
		})
	}
}

func TestClientListenUDPReadErrors(t *testing.T) {
	conn := &scriptedUDPConn{reads: []udpReadResult{
		{data: []byte{0xff}, err: oversizedUDPReadError("read")},
		{data: []byte{0xff}, err: oversizedUDPReadError("recvmsg")},
		{},
		{err: io.ErrUnexpectedEOF},
		{err: net.ErrClosed},
	}}
	var logs bytes.Buffer
	dialer := Dialer{ErrorLog: slog.New(slog.NewTextHandler(&logs, nil))}
	dialer.clientListen(&Conn{mtu: 1200}, conn)
	if conn.readCount != 5 {
		t.Fatalf("read %d times, want 5", conn.readCount)
	}
	if got := logs.String(); strings.Count(got, "handle packet:") != 1 || !strings.Contains(got, io.ErrUnexpectedEOF.Error()) {
		t.Fatalf("expected only the unrelated read error to be logged, got %q", got)
	}
}

func testUDPConnState(t *testing.T, conn net.Conn) *connState {
	t.Helper()
	ticker := time.NewTicker(time.Second)
	t.Cleanup(ticker.Stop)
	return &connState{conn: conn, raddr: conn.RemoteAddr(), mtu: 1492, ticker: ticker, maxTransientErrors: 10}
}

type udpReadResult struct {
	data []byte
	err  error
}

type scriptedUDPConn struct {
	net.Conn
	reads     []udpReadResult
	readCount int
	closed    bool
}

func (conn *scriptedUDPConn) Read(b []byte) (int, error) {
	conn.readCount++
	if len(conn.reads) == 0 {
		return 0, net.ErrClosed
	}
	result := conn.reads[0]
	conn.reads = conn.reads[1:]
	return copy(b, result.data), result.err
}

func (*scriptedUDPConn) Write(b []byte) (int, error) { return len(b), nil }
func (*scriptedUDPConn) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 19132}
}
func (conn *scriptedUDPConn) Close() error {
	conn.closed = true
	return nil
}
