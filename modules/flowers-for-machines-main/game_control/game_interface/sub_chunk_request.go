package game_interface

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	mcnbt "github.com/OmineDev/flowers-for-machines/core/minecraft/nbt"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol"
	"github.com/OmineDev/flowers-for-machines/core/minecraft/protocol/packet"
	"github.com/OmineDev/flowers-for-machines/utils"
	"github.com/TriM-Organization/bedrock-world-operator/block"
	bwochunk "github.com/TriM-Organization/bedrock-world-operator/chunk"
	bwodefine "github.com/TriM-Organization/bedrock-world-operator/define"
)

type SubChunkRequest struct {
	api     *ResourcesWrapper
	mu      sync.Mutex
	write   func(packet.Packet) error
	listen  func([]uint32, func(packet.Packet, error)) (string, error)
	destroy func(string)
}

type BlockData struct {
	Name   string
	States map[string]any
	NBT    map[string]any
}

func NewSubChunkRequest(api *ResourcesWrapper) *SubChunkRequest {
	return &SubChunkRequest{api: api, write: api.WritePacket, listen: api.Resources.PacketListener().ListenPacket, destroy: api.Resources.PacketListener().DestroyListener}
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

func (s *SubChunkRequest) SendSubChunkRequestWithRespTimeout(request *packet.SubChunkRequest, timeout time.Duration) (*packet.SubChunk, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request == nil || len(request.Offsets) == 0 {
		return nil, false, errors.New("empty subchunk request")
	}
	expected := make(map[protocol.SubChunkOffset]bool, len(request.Offsets))
	for _, offset := range request.Offsets {
		expected[offset] = true
	}
	responses := make(chan *packet.SubChunk, len(expected)+1)
	closed := make(chan error, 1)
	id, err := s.listen([]uint32{packet.IDSubChunk}, func(p packet.Packet, connErr error) {
		if connErr != nil {
			select {
			case closed <- connErr:
			default:
			}
			return
		}
		response, ok := p.(*packet.SubChunk)
		if !ok || response.Dimension != request.Dimension || response.Position != request.Position {
			return
		}
		select {
		case responses <- response:
		default:
		}
	})
	if err != nil {
		return nil, false, err
	}
	defer s.destroy(id)
	if err := s.write(request); err != nil {
		return nil, false, err
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	combined := &packet.SubChunk{Dimension: request.Dimension, Position: request.Position}
	for {
		select {
		case err := <-closed:
			return nil, false, err
		case <-timer.C:
			return nil, true, nil
		case response := <-responses:
			combined.CacheEnabled = response.CacheEnabled
			for _, entry := range response.SubChunkEntries {
				if expected[entry.Offset] {
					combined.SubChunkEntries = append(combined.SubChunkEntries, entry)
					delete(expected, entry.Offset)
				}
			}
			if len(expected) == 0 {
				return combined, false, nil
			}
		}
	}
}

func (s *SubChunkRequest) MakeSubChunkRequestBySubChunkPos(dimension int32, subChunksPos []protocol.SubChunkPos) *packet.SubChunkRequest {
	if len(subChunksPos) == 0 {
		return nil
	}
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

func (s *SubChunkRequest) SetChunkRadius(radius int32) (int32, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if radius <= 0 {
		return 0, errors.New("chunk radius must be positive")
	}
	response := make(chan int32, 1)
	closed := make(chan error, 1)
	id, err := s.listen([]uint32{packet.IDChunkRadiusUpdated}, func(p packet.Packet, connErr error) {
		if connErr != nil {
			select {
			case closed <- connErr:
			default:
			}
			return
		}
		if value, ok := p.(*packet.ChunkRadiusUpdated); ok {
			select {
			case response <- value.ChunkRadius:
			default:
			}
		}
	})
	if err != nil {
		return 0, err
	}
	defer s.destroy(id)
	if err := s.write(&packet.RequestChunkRadius{ChunkRadius: radius, MaxChunkRadius: radius}); err != nil {
		return 0, err
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case r := <-response:
		return r, nil
	case err := <-closed:
		return 0, err
	case <-timer.C:
		return 0, errors.New("chunk radius response timed out")
	}
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
