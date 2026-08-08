// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"sync"
	"testing"
)

func TestCompressionStats_Increments(t *testing.T) {
	var stats CompressionStats
	stats.inc(trimReasonPreventive)
	stats.inc(trimReasonExceed)
	stats.inc(trimReasonToolLoop)

	snap := stats.snapshot()
	if snap.PreventiveTrimmed != 1 || snap.ExceedTrimmed != 1 || snap.ToolLoopTrimmed != 1 || snap.TotalTrimmed != 3 {
		t.Fatalf("unexpected snapshot: %+v", snap)
	}
}

func TestCompressionStats_Empty(t *testing.T) {
	var stats CompressionStats
	snap := stats.snapshot()
	if snap.TotalTrimmed != 0 {
		t.Fatalf("expected empty stats, got %+v", snap)
	}
}

func TestCompressionStats_Concurrent(t *testing.T) {
	var stats CompressionStats
	const n = 1000
	var wg sync.WaitGroup
	wg.Add(3)
	for range 3 {
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				stats.inc(trimReasonPreventive)
				stats.inc(trimReasonExceed)
				stats.inc(trimReasonToolLoop)
			}
		}()
	}
	wg.Wait()
	snap := stats.snapshot()
	if snap.TotalTrimmed != 3*n*3 {
		t.Fatalf("expected %d total trims, got %d", 3*n*3, snap.TotalTrimmed)
	}
}
