package structure

import (
	"errors"
	"fmt"
	"io"

	"github.com/xingbaiawa/bdump/command"
)

var errBDXReadStalled = errors.New("bdx read stalled")

type bdxCommandReadGuard struct {
	reader       *countingReader
	stalledLimit int
	stalledCount int
}

func newBDXCommandReadGuard(reader io.Reader, stalledLimit int) *bdxCommandReadGuard {
	if stalledLimit <= 0 {
		stalledLimit = 8
	}
	return &bdxCommandReadGuard{
		reader:       &countingReader{reader: reader},
		stalledLimit: stalledLimit,
	}
}

func (g *bdxCommandReadGuard) ReadCommand() (command.Command, error) {
	before := g.reader.readBytes
	cmd, err := command.ReadCommand(g.reader)
	after := g.reader.readBytes

	if err == nil {
		g.stalledCount = 0
		return cmd, nil
	}
	if err == io.EOF {
		return nil, err
	}

	if after == before {
		g.stalledCount++
		if g.stalledCount >= g.stalledLimit {
			return nil, fmt.Errorf("%w: no forward progress for %d consecutive reads, last error: %v", errBDXReadStalled, g.stalledCount, err)
		}
	} else {
		g.stalledCount = 0
	}

	return nil, err
}

func isBDXReadStalledError(err error) bool {
	return errors.Is(err, errBDXReadStalled)
}

type countingReader struct {
	reader    io.Reader
	readBytes uint64
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.readBytes += uint64(n)
	return n, err
}
