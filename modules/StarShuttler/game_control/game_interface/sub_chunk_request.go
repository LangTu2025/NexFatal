package game_interface

import (
	"bytes"
	"fmt"
	"time"

	"github.com/TriM-Organization/bedrock-world-operator/block"
	bwochunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
	mcnbt "github.com/xingbaiawa/StarShuttler/core/minecraft/nbt"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol"
	"github.com/xingbaiawa/StarShuttler/core/minecraft/protocol/packet"
	"github.com/xingbaiawa/StarShuttler/utils"
)

type SubChunkRequest struct {
	api *ResourcesWrapper
}

type BlockData struct {
	Name   string
	States map[string]any
	NBT    map[string]any
}

func NewSubChunkRequest(api *ResourcesWrapper) *SubChunkRequest {
	return &SubChunkRequest{api: api}
}

func (s *SubChunkRequest) SendSubChunkRequestWithResp(request *packet.SubChunkRequest) (*packet.SubChunk, error) {
	resp, isTimeout, err := s.SendSubChunkRequestWithRespTimeout(request, 0)
	if err != nil {
		return nil, fmt.Errorf("SendSubChunkRequestWithResp: %w", err)
	}
	if isTimeout {
		return nil, fmt.Errorf("SendSubChunkRequestWithResp: request timed out")
	}
	return resp, nil
}

func (s *SubChunkRequest) SendSubChunkRequestWithRespTimeout(
	request *packet.SubChunkRequest,
	timeout time.Duration,
) (resp *packet.SubChunk, isTimeout bool, err error) {
	api := s.api
	responseChan := make(chan *packet.SubChunk, 1)

	api.Resources.SubChunkRequest().SetSubChunkRequestCallback(
		request,
		func(p *packet.SubChunk) {
			select {
			case responseChan <- p:
			default:
			}
		},
	)
	if err = api.WritePacket(request); err != nil {
		api.Resources.SubChunkRequest().DeleteSubChunkRequestCallback(request.Position)
		return nil, false, fmt.Errorf("write packet: %w", err)
	}

	var (
		timer     *time.Timer
		timeoutCh <-chan time.Time
	)
	if timeout > 0 {
		timer = time.NewTimer(timeout)
		timeoutCh = timer.C
	}

	select {
	case resp = <-responseChan:
		if timer != nil && !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		return resp, false, nil
	case <-timeoutCh:
		api.Resources.SubChunkRequest().DeleteSubChunkRequestCallback(request.Position)
		return nil, true, nil
	}
}

func (s *SubChunkRequest) MakeSubChunkRequestBySubChunkPos(dimension int32, subChunksPos []protocol.SubChunkPos) *packet.SubChunkRequest {
	offsets := make([]protocol.SubChunkOffset, 0, len(subChunksPos))

	centerChunkX := subChunksPos[0].X()
	centerChunkZ := subChunksPos[0].Z()

	for _, pos := range subChunksPos {
		offsets = append(offsets, protocol.SubChunkOffset{
			int8(pos.X() - centerChunkX),
			int8(pos.Y()),
			int8(pos.Z() - centerChunkZ),
		})
	}

	return &packet.SubChunkRequest{
		Dimension: dimension,
		Position:  protocol.SubChunkPos{centerChunkX, 0, centerChunkZ},
		Offsets:   offsets,
	}
}

func (s *SubChunkRequest) SetChunkRadius(chunkRadius int32) (resp int32, err error) {
	if chunkRadius < 0 || chunkRadius > 255 {
		return -1, fmt.Errorf("SetChunkRadius: chunk radius %d out of uint8 range", chunkRadius)
	}

	api := s.api
	channel := make(chan struct{})

	api.Resources.SubChunkRequest().SetRequestChunkRadiusCallback(
		func(c int32) {
			resp = c
			close(channel)
		},
	)
	request := &packet.RequestChunkRadius{
		ChunkRadius:    chunkRadius,
		MaxChunkRadius: uint8(chunkRadius),
	}
	err = api.WritePacket(request)
	if err != nil {
		api.Resources.SubChunkRequest().ClearRequestChunkRadiusCallback()
		return -1, fmt.Errorf("SetChunkRadius: %w", err)
	}

	<-channel
	return resp, nil
}

func (s *SubChunkRequest) RequestBlockState(dimension int32, pos protocol.BlockPos, timeout time.Duration) (string, map[string]any, error) {
	data, err := s.RequestBlockData(dimension, pos, timeout)
	if err != nil {
		return "", nil, err
	}
	return data.Name, data.States, nil
}

func (s *SubChunkRequest) RequestBlockData(dimension int32, pos protocol.BlockPos, timeout time.Duration) (BlockData, error) {
	subChunkPos := protocol.SubChunkPos{pos[0] >> 4, pos[1] >> 4, pos[2] >> 4}
	request := s.MakeSubChunkRequestBySubChunkPos(dimension, []protocol.SubChunkPos{subChunkPos})

	resp, isTimeout, err := s.SendSubChunkRequestWithRespTimeout(request, timeout)
	if err != nil {
		return BlockData{}, fmt.Errorf("RequestBlockData: %w", err)
	}
	if isTimeout {
		return BlockData{}, fmt.Errorf("RequestBlockData: request timed out")
	}
	if resp == nil || len(resp.SubChunkEntries) == 0 {
		return BlockData{}, fmt.Errorf("RequestBlockData: empty sub chunk response")
	}

	entry := resp.SubChunkEntries[0]
	switch entry.Result {
	case protocol.SubChunkResultSuccess:
		subChunk, nbts, err := decodeSubChunkPayload(entry.RawPayload)
		if err != nil {
			return BlockData{}, fmt.Errorf("RequestBlockData: %w", err)
		}
		runtimeID := subChunk.Block(byte(pos[0]&15), byte(pos[1]&15), byte(pos[2]&15), 0)
		name, states, found := block.RuntimeIDToState(runtimeID)
		if !found {
			return BlockData{}, fmt.Errorf("RequestBlockData: unknown runtime id %d", runtimeID)
		}
		return BlockData{
			Name:   name,
			States: utils.DeepCopyNBT(states),
			NBT:    findBlockEntityNBT(nbts, pos),
		}, nil
	case protocol.SubChunkResultSuccessAllAir:
		return BlockData{Name: "minecraft:air", States: map[string]any{}, NBT: map[string]any{}}, nil
	default:
		return BlockData{}, fmt.Errorf("RequestBlockData: unexpected sub chunk result %d", entry.Result)
	}
}

func decodeSubChunkPayload(data []byte) (*bwochunk.SubChunk, []map[string]any, error) {
	buf := bytes.NewBuffer(data)
	subChunk, _, err := bwochunk.DecodeSubChunk(buf, bwodefine.Range{0, 15}, bwochunk.NetworkEncoding)
	if err != nil {
		return nil, nil, fmt.Errorf("decode sub chunk payload: %w", err)
	}

	nbts := make([]map[string]any, 0)
	for buf.Len() > 0 {
		var blockData map[string]any
		if err := mcnbt.NewDecoderWithEncoding(buf, mcnbt.NetworkLittleEndian).Decode(&blockData); err != nil {
			break
		}
		nbts = append(nbts, blockData)
	}
	return subChunk, nbts, nil
}

func findBlockEntityNBT(nbts []map[string]any, pos protocol.BlockPos) map[string]any {
	for _, blockNBT := range nbts {
		x, xok := anyToInt32(blockNBT["x"])
		y, yok := anyToInt32(blockNBT["y"])
		z, zok := anyToInt32(blockNBT["z"])
		if !xok || !yok || !zok {
			continue
		}
		if x == pos[0] && y == pos[1] && z == pos[2] {
			return utils.DeepCopyNBT(blockNBT)
		}
	}
	return map[string]any{}
}

func anyToInt32(v any) (int32, bool) {
	switch val := v.(type) {
	case int32:
		return val, true
	case int16:
		return int32(val), true
	case int8:
		return int32(val), true
	case int:
		return int32(val), true
	case int64:
		return int32(val), true
	case uint8:
		return int32(val), true
	case uint16:
		return int32(val), true
	case uint32:
		return int32(val), true
	case uint64:
		return int32(val), true
	default:
		return 0, false
	}
}
