package minecraft

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

func TestListenConnPreservesReceiveFailureAsCloseCause(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		frame      []byte
		compressed bool
		want       string
	}{
		{name: "batch header", frame: []byte{0}, want: "decode batch: invalid header"},
		{name: "packet header", frame: []byte{0xfe, 1, 0x80}, want: "read packet header"},
		{name: "compressed batch", frame: []byte{0xfe, 0}, compressed: true, want: "decompress batch"},
		{name: "transport EOF", want: "read batch: EOF"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			clientSide, serverSide := net.Pipe()
			defer serverSide.Close()
			conn := newConn(clientSide, nil, slog.New(slog.NewTextHandler(io.Discard, nil)), DefaultProtocol, -time.Second, false)
			defer conn.Close()
			if scenario.compressed {
				conn.dec.EnableCompressionWithID(packet.CompressionAlgorithmFlate, false, 1024)
			}
			loginCtx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				listenConn(conn, make(chan struct{}), make(chan struct{}), cancel)
			}()
			if len(scenario.frame) > 0 {
				if _, err := serverSide.Write(scenario.frame); err != nil {
					t.Fatal(err)
				}
			} else {
				_ = serverSide.Close()
			}
			select {
			case <-finished:
			case <-time.After(time.Second):
				t.Fatal("packet listener did not close the failed connection")
			}
			_, err := conn.ReadPacket()
			if err == nil || !strings.Contains(err.Error(), scenario.want) {
				t.Fatalf("ReadPacket error = %v, want original receive failure %q", err, scenario.want)
			}
			cause := context.Cause(conn.Context())
			if cause == nil || !errors.Is(cause, context.Cause(loginCtx)) {
				t.Fatalf("connection cause = %v, want login receive cause %v", cause, context.Cause(loginCtx))
			}
			if err := conn.Close(); err != nil {
				t.Fatal(err)
			}
			if context.Cause(conn.Context()) != cause {
				t.Fatal("cleanup replaced the first connection failure")
			}
		})
	}
}
