package main

import (
	"sync"
	"time"
)

// Snowflake ID generator — 64-bit IDs.
// Layout: 1 bit unused | 41 bits timestamp | 10 bits worker | 12 bits sequence
//
// Custom epoch: 2020-01-01T00:00:00Z (matches typical Supernote IDs).
const (
	sfEpoch       = 1577836800000 // 2020-01-01T00:00:00Z in millis
	sfWorkerBits  = 10
	sfSeqBits     = 12
	sfWorkerMax   = (1 << sfWorkerBits) - 1
	sfSeqMax      = (1 << sfSeqBits) - 1
	sfTimeShift   = sfWorkerBits + sfSeqBits
	sfWorkerShift = sfSeqBits
)

type snowflakeGen struct {
	mu       sync.Mutex
	worker   int64
	sequence int64
	lastTime int64
}

var idGen = &snowflakeGen{worker: 1}

func (g *snowflakeGen) Next() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli() - sfEpoch

	if now == g.lastTime {
		g.sequence = (g.sequence + 1) & sfSeqMax
		if g.sequence == 0 {
			// Wait for next millisecond.
			for now <= g.lastTime {
				now = time.Now().UnixMilli() - sfEpoch
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastTime = now

	return (now << sfTimeShift) | (g.worker << sfWorkerShift) | g.sequence
}

func nextID() int64 {
	return idGen.Next()
}
