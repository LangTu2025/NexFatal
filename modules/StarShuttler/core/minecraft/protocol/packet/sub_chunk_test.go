package packet

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
)

func TestSubChunkRoundTrip120(t *testing.T) {
	for _, cacheEnabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "no_cache", true: "cache"}[cacheEnabled], func(t *testing.T) {
			original := &SubChunk{
				CacheEnabled: cacheEnabled,
				Dimension:    0,
				Position:     protocol.SubChunkPos{12, -4, 34},
				SubChunkEntries: []protocol.SubChunkEntry{{
					Offset:              protocol.SubChunkOffset{1, -2, 3},
					Result:              protocol.SubChunkResultSuccess,
					RawPayload:          []byte{9, 1, 0, 3, 5, 7, 11, 13},
					HeightMapType:       protocol.HeightMapDataNone,
					RenderHeightMapType: protocol.HeightMapDataAllCopied,
				}},
			}
			if cacheEnabled {
				original.SubChunkEntries[0].BlobHash = 0x0102030405060708
			}

			buf := bytes.NewBuffer(nil)
			original.Marshal(protocol.NewWriter(buf, 0))

			decoded := &SubChunk{}
			readBuf := bytes.NewBuffer(buf.Bytes())
			decoded.Marshal(protocol.NewReader(readBuf, 0, false))

			if readBuf.Len() != 0 {
				t.Fatalf("sub chunk decode left %d unread bytes", readBuf.Len())
			}
			if !reflect.DeepEqual(decoded, original) {
				t.Fatalf("sub chunk mismatch:\n got: %#v\nwant: %#v", decoded, original)
			}
		})
	}
}
