package minecraft

import (
	"bytes"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
)

type packetHeaderConn struct {
	net.Conn
	withHeader bool
}

func (c packetHeaderConn) PacketHeader() (byte, bool) {
	return 0xfe, c.withHeader
}

func TestNewConnUsesTransportPacketHeader(t *testing.T) {
	for _, withHeader := range []bool{true, false} {
		t.Run(map[bool]string{true: "header included", false: "transport framed"}[withHeader], func(t *testing.T) {
			clientSide, serverSide := net.Pipe()
			defer serverSide.Close()

			conn := newConn(
				packetHeaderConn{Conn: clientSide, withHeader: withHeader},
				nil,
				slog.New(slog.NewTextHandler(io.Discard, nil)),
				DefaultProtocol,
				-time.Second,
				false,
			)
			defer conn.Close()

			frame := make(chan []byte, 1)
			readErr := make(chan error, 1)
			go func() {
				buf := make([]byte, 1024)
				n, err := serverSide.Read(buf)
				if err != nil {
					readErr <- err
					return
				}
				frame <- append([]byte(nil), buf[:n]...)
			}()

			if err := conn.WritePacket(&packet.RequestNetworkSettings{ClientProtocol: protocol.CurrentProtocol}); err != nil {
				t.Fatalf("buffer request packet: %v", err)
			}
			if err := conn.Flush(); err != nil {
				t.Fatalf("flush request packet: %v", err)
			}

			var data []byte
			select {
			case data = <-frame:
			case err := <-readErr:
				t.Fatalf("read request frame: %v", err)
			case <-time.After(time.Second):
				t.Fatal("timed out reading request frame")
			}
			if len(data) == 0 || (data[0] == 0xfe) != withHeader {
				t.Fatalf("frame header = %#x, withHeader = %t", data[0], withHeader)
			}

			decoder := packet.NewDecoder(bytes.NewReader(data))
			if !withHeader {
				decoder = packet.NewDecoderWithoutHeader(bytes.NewReader(data))
			}
			packets, err := decoder.Decode()
			if err != nil {
				t.Fatalf("decode request frame: %v", err)
			}
			if len(packets) != 1 {
				t.Fatalf("decoded %d packets, want 1", len(packets))
			}
		})
	}
}
