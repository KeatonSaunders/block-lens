package observer

import (
	"context"
	"sync"
	"time"

	"github.com/keato/btc-observer/internal/metrics"
)

const seenExpiry = 10 * time.Minute

// seenTxs tracks transactions we've already requested
var seenTxs = struct {
	sync.RWMutex
	m map[[32]byte]time.Time
}{m: make(map[[32]byte]time.Time)}

// seenBlocks tracks blocks we've already requested
var seenBlocks = struct {
	sync.RWMutex
	m map[[32]byte]time.Time
}{m: make(map[[32]byte]time.Time)}

// MarkSeenTx returns true if this is the first time seeing this tx hash
func MarkSeenTx(hash [32]byte) bool {
	seenTxs.Lock()
	defer seenTxs.Unlock()
	if _, exists := seenTxs.m[hash]; exists {
		return false
	}
	seenTxs.m[hash] = time.Now()
	return true
}

// MarkSeenBlock returns true if this is the first time seeing this block hash
func MarkSeenBlock(hash [32]byte) bool {
	seenBlocks.Lock()
	defer seenBlocks.Unlock()
	if _, exists := seenBlocks.m[hash]; exists {
		return false
	}
	seenBlocks.m[hash] = time.Now()
	return true
}

// CleanupSeenMaps removes entries older than seenExpiry
func CleanupSeenMaps() {
	cutoff := time.Now().Add(-seenExpiry)

	seenTxs.Lock()
	for hash, t := range seenTxs.m {
		if t.Before(cutoff) {
			delete(seenTxs.m, hash)
		}
	}
	metrics.SeenMapSize.WithLabelValues("tx").Set(float64(len(seenTxs.m)))
	seenTxs.Unlock()

	seenBlocks.Lock()
	for hash, t := range seenBlocks.m {
		if t.Before(cutoff) {
			delete(seenBlocks.m, hash)
		}
	}
	metrics.SeenMapSize.WithLabelValues("block").Set(float64(len(seenBlocks.m)))
	seenBlocks.Unlock()
}

// StartCleanupRoutine starts periodic cleanup of seen maps
func StartCleanupRoutine(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				CleanupSeenMaps()
			}
		}
	}()
}
