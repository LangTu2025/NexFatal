package define

type Size struct {
	Width, Height, Length int
}

func (s Size) GetWidth() int {
	return s.Width
}

func (s Size) GetHeight() int {
	return s.Height
}

func (s Size) GetLength() int {
	return s.Length
}

func (s Size) GetVolume() int {
	if s.Width <= 0 || s.Height <= 0 || s.Length <= 0 {
		return 0
	}
	return s.Width * s.Height * s.Length
}

func (s Size) GetChunkXCount() int {
	return chunkCount(s.Width)
}

func (s Size) GetChunkZCount() int {
	return chunkCount(s.Length)
}

func (s Size) GetChunkCount() int {
	return s.GetChunkXCount() * s.GetChunkZCount()
}

func chunkCount(blockCount int) int {
	if blockCount <= 0 {
		return 0
	}
	return (blockCount + 15) / 16
}
