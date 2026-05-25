package metrics

import "sync/atomic"

const numLatencyBuckets = 11

type snapshot struct {
	InFlight       int64    `json:"in_flight"`
	Rejected       uint64   `json:"rejected"`
	TimedOut       uint64   `json:"timed_out"`
	TotalRequests  uint64   `json:"total_requests"`
	Batched        uint64   `json:"batched"`
	TotalBatches   uint64   `json:"total_batches"`
	CacheHits      uint64   `json:"cache_hits"`
	CacheMisses    uint64   `json:"cache_misses"`
	AvgLatencyMs   float64  `json:"avg_latency_ms"`
	MaxLatencyMs   float64  `json:"max_latency_ms"`
	P50LatencyMs   float64  `json:"p50_latency_ms"`
	P90LatencyMs   float64  `json:"p90_latency_ms"`
	P99LatencyMs   float64  `json:"p99_latency_ms"`
	TotalResponses int64    `json:"total_responses"`
	LatencyBuckets []float64 `json:"latency_buckets,omitempty"`
}

var (
	inFlight      int64
	rejected      uint64
	timedOut      uint64
	totalRequests uint64
	batched       uint64
	totalBatches  uint64
	cacheHits     uint64
	cacheMisses   uint64
)

// latencyBucket stores histogram data for latencies using fixed buckets.
type latencyBucket struct {
	end float64 // upper bound (inclusive of lower, exclusive of upper)
}

var latencyBuckets = []latencyBucket{
	{end: 50},
	{end: 100},
	{end: 250},
	{end: 500},
	{end: 1000},
	{end: 2500},
	{end: 5000},
	{end: 10000},
	{end: 30000},
	{end: 60000},
	{end: 0}, // catch-all for >60s
}

var (
	latencyCounts [numLatencyBuckets]int64
	latencySum    float64
	latencyMax    float64
	totalResponses int64
	latencyMu     int32
)

func lockLatency() {
	for !atomic.CompareAndSwapInt32(&latencyMu, 0, 1) {
	}
}

func unlockLatency() {
	atomic.StoreInt32(&latencyMu, 0)
}

func RecordLatency(ms float64) {
	lockLatency()
	latencySum += ms
	if ms > latencyMax {
		latencyMax = ms
	}
	totalResponses++

	for i := range latencyBuckets {
		if ms < latencyBuckets[i].end || latencyBuckets[i].end == 0 {
			latencyCounts[i]++
			break
		}
	}
	unlockLatency()
}

func IncTotalRequests() {
	atomic.AddUint64(&totalRequests, 1)
}

func IncInFlight() {
	atomic.AddInt64(&inFlight, 1)
}

func DecInFlight() {
	atomic.AddInt64(&inFlight, -1)
}

func IncRejected() {
	atomic.AddUint64(&rejected, 1)
}

func IncTimedOut() {
	atomic.AddUint64(&timedOut, 1)
}

func IncBatched() {
	atomic.AddUint64(&batched, 1)
}

func IncTotalBatches() {
	atomic.AddUint64(&totalBatches, 1)
}

func IncCacheHits() {
	atomic.AddUint64(&cacheHits, 1)
}

func IncCacheMisses() {
	atomic.AddUint64(&cacheMisses, 1)
}

func Snapshot() snapshot {
	lockLatency()
	buckets := make([]float64, numLatencyBuckets)
	var totalInBuckets int64
	for i := range latencyBuckets {
		buckets[i] = float64(latencyCounts[i])
		totalInBuckets += latencyCounts[i]
	}

	// Calculate percentiles
	var p50, p90, p99 float64
	if totalInBuckets > 0 {
		target50 := float64(totalInBuckets) * 0.50
		target90 := float64(totalInBuckets) * 0.90
		target99 := float64(totalInBuckets) * 0.99

		cumulative := 0.0
		for i, count := range buckets {
			cumulative += count
			if p50 == 0 && cumulative >= target50 {
				p50 = latencyBuckets[i].end
			}
			if p90 == 0 && cumulative >= target90 {
				p90 = latencyBuckets[i].end
			}
			if p99 == 0 && cumulative >= target99 {
				p99 = latencyBuckets[i].end
			}
		}
	}
	unlockLatency()

	return snapshot{
		InFlight:       atomic.LoadInt64(&inFlight),
		Rejected:       atomic.LoadUint64(&rejected),
		TimedOut:       atomic.LoadUint64(&timedOut),
		TotalRequests:  atomic.LoadUint64(&totalRequests),
		Batched:        atomic.LoadUint64(&batched),
		TotalBatches:   atomic.LoadUint64(&totalBatches),
		CacheHits:      atomic.LoadUint64(&cacheHits),
		CacheMisses:    atomic.LoadUint64(&cacheMisses),
		AvgLatencyMs:   func() float64 { if totalResponses > 0 { return latencySum / float64(totalResponses) }; return 0 }(),
		MaxLatencyMs:   latencyMax,
		P50LatencyMs:   p50,
		P90LatencyMs:   p90,
		P99LatencyMs:   p99,
		TotalResponses: totalResponses,
		LatencyBuckets: buckets,
	}
}

// BucketRanges returns the upper bounds of each latency bucket in ms.
func BucketRanges() []float64 {
	ranges := make([]float64, numLatencyBuckets)
	for i, b := range latencyBuckets {
		ranges[i] = b.end
	}
	return ranges
}
