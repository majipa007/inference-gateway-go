package metrics

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestIncTotalRequests(t *testing.T) {
	resetGlobals()

	const n = 100
	for i := 0; i < n; i++ {
		IncTotalRequests()
	}

	snap := Snapshot()
	if snap.TotalRequests != uint64(n) {
		t.Errorf("expected %d total requests, got %d", n, snap.TotalRequests)
	}
}

func TestIncAndDecInFlight(t *testing.T) {
	resetGlobals()

	IncInFlight()
	IncInFlight()
	IncInFlight()

	snap := Snapshot()
	if snap.InFlight != 3 {
		t.Errorf("expected in_flight=3, got %d", snap.InFlight)
	}

	DecInFlight()
	DecInFlight()

	snap = Snapshot()
	if snap.InFlight != 1 {
		t.Errorf("expected in_flight=1 after decrements, got %d", snap.InFlight)
	}

	DecInFlight()
	snap = Snapshot()
	if snap.InFlight != 0 {
		t.Errorf("expected in_flight=0 after all decrements, got %d", snap.InFlight)
	}
}

func TestIncRejected(t *testing.T) {
	resetGlobals()

	const n = 50
	for i := 0; i < n; i++ {
		IncRejected()
	}

	snap := Snapshot()
	if snap.Rejected != uint64(n) {
		t.Errorf("expected %d rejected, got %d", n, snap.Rejected)
	}
}

func TestIncTimedOut(t *testing.T) {
	resetGlobals()

	const n = 25
	for i := 0; i < n; i++ {
		IncTimedOut()
	}

	snap := Snapshot()
	if snap.TimedOut != uint64(n) {
		t.Errorf("expected %d timed_out, got %d", n, snap.TimedOut)
	}
}

func TestSnapshotAllFields(t *testing.T) {
	resetGlobals()

	IncTotalRequests()
	IncTotalRequests()
	IncInFlight()
	IncInFlight()
	IncInFlight()
	IncRejected()
	IncTimedOut()

	snap := Snapshot()

	if snap.TotalRequests != 2 {
		t.Errorf("expected TotalRequests=2, got %d", snap.TotalRequests)
	}
	if snap.InFlight != 3 {
		t.Errorf("expected InFlight=3, got %d", snap.InFlight)
	}
	if snap.Rejected != 1 {
		t.Errorf("expected Rejected=1, got %d", snap.Rejected)
	}
	if snap.TimedOut != 1 {
		t.Errorf("expected TimedOut=1, got %d", snap.TimedOut)
	}
}

func TestConcurrentAccess(t *testing.T) {
	resetGlobals()

	const goroutines = 100
	const incs = 1000

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incs; j++ {
				IncTotalRequests()
				IncInFlight()
				DecInFlight()
				if j%10 == 0 {
					IncRejected()
				}
				if j%20 == 0 {
					IncTimedOut()
				}
			}
		}()
	}

	wg.Wait()

	snap := Snapshot()
	expected := uint64(goroutines * incs)
	if snap.TotalRequests != expected {
		t.Errorf("expected TotalRequests=%d, got %d", expected, snap.TotalRequests)
	}
	if snap.Rejected != expected/10 {
		t.Errorf("expected Rejected=%d, got %d", expected/10, snap.Rejected)
	}
	if snap.TimedOut != expected/20 {
		t.Errorf("expected TimedOut=%d, got %d", expected/20, snap.TimedOut)
	}
	if snap.InFlight != 0 {
		t.Errorf("expected InFlight=0, got %d", snap.InFlight)
	}
}

func TestInFlightGauge(t *testing.T) {
	resetGlobals()

	IncInFlight()
	snap1 := Snapshot()
	if snap1.InFlight != 1 {
		t.Errorf("expected InFlight=1, got %d", snap1.InFlight)
	}

	IncInFlight()
	IncInFlight()
	snap2 := Snapshot()
	if snap2.InFlight != 3 {
		t.Errorf("expected InFlight=3, got %d", snap2.InFlight)
	}

	DecInFlight()
	snap3 := Snapshot()
	if snap3.InFlight != 2 {
		t.Errorf("expected InFlight=2 after DecInFlight, got %d", snap3.InFlight)
	}

	DecInFlight()
	DecInFlight()
	snap4 := Snapshot()
	if snap4.InFlight != 0 {
		t.Errorf("expected InFlight=0 after all decrements, got %d", snap4.InFlight)
	}
}

func resetGlobals() {
	atomic.StoreInt64(&inFlight, 0)
	atomic.StoreUint64(&rejected, 0)
	atomic.StoreUint64(&timedOut, 0)
	atomic.StoreUint64(&totalRequests, 0)
	atomic.StoreUint64(&batched, 0)
	atomic.StoreUint64(&totalBatches, 0)
	atomic.StoreUint64(&cacheHits, 0)
	atomic.StoreUint64(&cacheMisses, 0)
	lockLatency()
	latencySum = 0
	latencyMax = 0
	totalResponses = 0
	for i := range latencyCounts {
		latencyCounts[i] = 0
	}
	unlockLatency()
}

func TestIncBatched(t *testing.T) {
	resetGlobals()

	const n = 10
	for i := 0; i < n; i++ {
		IncBatched()
	}

	snap := Snapshot()
	if snap.Batched != uint64(n) {
		t.Errorf("expected %d batched, got %d", n, snap.Batched)
	}
}

func TestIncTotalBatches(t *testing.T) {
	resetGlobals()

	const n = 5
	for i := 0; i < n; i++ {
		IncTotalBatches()
	}

	snap := Snapshot()
	if snap.TotalBatches != uint64(n) {
		t.Errorf("expected %d total_batches, got %d", n, snap.TotalBatches)
	}
}

func TestIncCacheHits(t *testing.T) {
	resetGlobals()

	const n = 8
	for i := 0; i < n; i++ {
		IncCacheHits()
	}

	snap := Snapshot()
	if snap.CacheHits != uint64(n) {
		t.Errorf("expected %d cache_hits, got %d", n, snap.CacheHits)
	}
}

func TestIncCacheMisses(t *testing.T) {
	resetGlobals()

	const n = 3
	for i := 0; i < n; i++ {
		IncCacheMisses()
	}

	snap := Snapshot()
	if snap.CacheMisses != uint64(n) {
		t.Errorf("expected %d cache_misses, got %d", n, snap.CacheMisses)
	}
}

func TestRecordLatency(t *testing.T) {
	resetGlobals()

	// Record some latencies in the 100-250ms bucket
	RecordLatency(150)
	RecordLatency(200)

	// One in the 500-1000ms bucket
	RecordLatency(750)

	snap := Snapshot()

	if snap.TotalResponses != 3 {
		t.Errorf("expected 3 total responses, got %d", snap.TotalResponses)
	}

	if snap.AvgLatencyMs < 350 || snap.AvgLatencyMs > 450 {
		t.Errorf("expected avg ~367ms, got %.1fms", snap.AvgLatencyMs)
	}

	if snap.MaxLatencyMs < 700 || snap.MaxLatencyMs > 800 {
		t.Errorf("expected max ~750ms, got %.1fms", snap.MaxLatencyMs)
	}
}

func TestLatencyPercentiles(t *testing.T) {
	resetGlobals()

	// Record 50 latencies at 100ms -> bucket index 2 (end=250)
	for i := 0; i < 50; i++ {
		RecordLatency(100)
	}

	// Record 30 latencies at 500ms -> bucket index 4 (end=1000)
	for i := 0; i < 30; i++ {
		RecordLatency(500)
	}

	// Record 20 latencies at 5000ms -> bucket index 7 (end=10000)
	for i := 0; i < 20; i++ {
		RecordLatency(5000)
	}

	snap := Snapshot()

	if snap.TotalResponses != 100 {
		t.Errorf("expected 100 total responses, got %d", snap.TotalResponses)
	}

	// Bucket 2: 50 entries (100ms), Bucket 4: 30 entries (500ms), Bucket 7: 20 entries (5000ms)
	// P50: 50th entry is at boundary of bucket 2 -> ~250ms
	if snap.P50LatencyMs < 200 || snap.P50LatencyMs > 300 {
		t.Errorf("expected p50 ~250ms, got %.1fms", snap.P50LatencyMs)
	}

	// P90: 90th entry is in bucket 7 -> ~10000ms
	if snap.P90LatencyMs < 9000 || snap.P90LatencyMs > 11000 {
		t.Errorf("expected p90 ~10000ms, got %.1fms", snap.P90LatencyMs)
	}

	// P99: 99th entry is in bucket 7 -> ~10000ms
	if snap.P99LatencyMs < 9000 || snap.P99LatencyMs > 11000 {
		t.Errorf("expected p99 ~10000ms, got %.1fms", snap.P99LatencyMs)
	}
}

func TestBucketRanges(t *testing.T) {
	ranges := BucketRanges()
	if len(ranges) != 11 {
		t.Errorf("expected 11 buckets, got %d", len(ranges))
	}

	// First bucket should end at 50ms
	if ranges[0] != 50 {
		t.Errorf("expected first bucket end=50, got %.0f", ranges[0])
	}

	// Last bucket (catch-all) should have end=0
	if ranges[len(ranges)-1] != 0 {
		t.Errorf("expected last bucket end=0, got %.0f", ranges[len(ranges)-1])
	}
}

func TestSnapshotLatencyBuckets(t *testing.T) {
	resetGlobals()

	RecordLatency(500)
	RecordLatency(500)

	snap := Snapshot()

	if snap.LatencyBuckets == nil {
		t.Fatal("expected non-nil LatencyBuckets")
	}

	if len(snap.LatencyBuckets) != 11 {
		t.Errorf("expected 11 latency buckets, got %d", len(snap.LatencyBuckets))
	}

	// The 500ms values go into bucket index 4 (end=1000) because
	// the condition is ms < end: 500 < 50 is false, 500 < 100 is false,
	// 500 < 250 is false, 500 < 500 is false, 500 < 1000 is true
	if snap.LatencyBuckets[4] != 2 {
		t.Errorf("expected bucket 4 (500ms) count=2, got %.0f", snap.LatencyBuckets[4])
	}
}

func TestConcurrentLatencyRecording(t *testing.T) {
	resetGlobals()

	const goroutines = 100
	const records = 1000

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < records; j++ {
				RecordLatency(float64(j % 1000))
			}
		}()
	}

	wg.Wait()

	snap := Snapshot()
	expected := goroutines * records
	if snap.TotalResponses != int64(expected) {
		t.Errorf("expected %d total responses, got %d", expected, snap.TotalResponses)
	}

	// Sum of all buckets should equal total responses
	var bucketSum float64
	for _, count := range snap.LatencyBuckets {
		bucketSum += count
	}

	if int(bucketSum) != expected {
		t.Errorf("expected bucket sum %d, got %.0f", expected, bucketSum)
	}
}
