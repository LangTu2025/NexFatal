package resources_control

import (
	"sync"
	"time"
)

const (
	outboundPacketWindow = time.Second / 20
	// mcpe_server 会按 tick 统计会话 IP 发包量，10000 以上会踢出；这里保留 500 包余量。
	outboundPacketLimitPerTick = 9500
)

var globalOutboundPacketLimiter = newPacketWindowLimiter(outboundPacketLimitPerTick, outboundPacketWindow)

type packetWindowLimiter struct {
	mu     sync.Mutex
	window time.Duration
	limit  int
	sentAt []time.Time
	head   int
	count  int
}

func newPacketWindowLimiter(limit int, window time.Duration) *packetWindowLimiter {
	return &packetWindowLimiter{
		window: window,
		limit:  limit,
		sentAt: make([]time.Time, limit),
	}
}

func (l *packetWindowLimiter) Wait() {
	if l == nil || l.limit <= 0 || l.window <= 0 {
		return
	}
	for {
		now := time.Now()
		wait := l.reserve(now)
		if wait <= 0 {
			return
		}
		time.Sleep(wait)
	}
}

func (l *packetWindowLimiter) reserve(now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	for l.count > 0 && now.Sub(l.sentAt[l.head]) >= l.window {
		l.head = (l.head + 1) % l.limit
		l.count--
	}
	if l.count < l.limit {
		index := (l.head + l.count) % l.limit
		l.sentAt[index] = now
		l.count++
		return 0
	}

	wait := l.window - now.Sub(l.sentAt[l.head])
	if wait <= 0 {
		return time.Microsecond
	}
	return wait
}
