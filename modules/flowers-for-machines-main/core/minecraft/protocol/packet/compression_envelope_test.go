package packet

import (
	"bytes"
	"testing"
)

func TestCompressionEnvelopeCompressionID(t *testing.T) {
	packetData := [][]byte{{0x01, 0x02, 0x03}}

	for _, test := range []struct {
		name    string
		writeID bool
		readID  bool
	}{
		{name: "default online envelope", writeID: true, readID: true},
		{name: "local lobby envelope", writeID: false, readID: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var encoded bytes.Buffer
			encoder := NewEncoder(&encoded)
			if test.writeID {
				encoder.EnableCompression(CompressionAlgorithmFlate)
			} else {
				encoder.EnableCompressionWithID(CompressionAlgorithmFlate, false)
			}
			if err := encoder.Encode(packetData); err != nil {
				t.Fatalf("encode packet batch: %v", err)
			}

			decoder := NewDecoder(bytes.NewReader(encoded.Bytes()))
			if test.readID {
				decoder.EnableCompression(1024, true)
			} else {
				decoder.EnableCompressionWithID(CompressionAlgorithmFlate, false, 1024)
			}
			decoded, err := decoder.Decode()
			if err != nil {
				t.Fatalf("decode packet batch: %v", err)
			}
			if len(decoded) != 1 || !bytes.Equal(decoded[0], packetData[0]) {
				t.Fatalf("decoded packet batch = %v, want %v", decoded, packetData)
			}
		})
	}
}

func TestBatchHeaderSelection(t *testing.T) {
	packetData := [][]byte{{0x01, 0x02, 0x03}}
	for _, test := range []struct {
		name        string
		writeHeader bool
	}{
		{name: "standard transport", writeHeader: true},
		{name: "transport framed", writeHeader: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var encoded bytes.Buffer
			encoder := NewEncoder(&encoded)
			decoder := NewDecoder(&encoded)
			if !test.writeHeader {
				encoder = NewEncoderWithoutHeader(&encoded)
				decoder = NewDecoderWithoutHeader(&encoded)
			}
			if err := encoder.Encode(packetData); err != nil {
				t.Fatalf("encode packet batch: %v", err)
			}
			decoded, err := decoder.Decode()
			if err != nil {
				t.Fatalf("decode packet batch: %v", err)
			}
			if len(decoded) != 1 || !bytes.Equal(decoded[0], packetData[0]) {
				t.Fatalf("decoded packet batch = %v, want %v", decoded, packetData)
			}
		})
	}
}
