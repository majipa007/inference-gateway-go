package metrics

import "sync/atomic"

type snapshot struct {
	InFlight      int64  `json:"in_flight"`
	Rejected      uint64 `json:"rejected"`
	TimedOut      uint64 `json:"timed_out"`
	TotalRequests uint64 `json:"total_requests"`
}

var (
	inFlight      int64
	rejected      uint64
	timedOut      uint64
	totalRequests uint64
)

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

func Snapshot() snapshot {
	return snapshot{
		InFlight:      atomic.LoadInt64(&inFlight),
		Rejected:      atomic.LoadUint64(&rejected),
		TimedOut:      atomic.LoadUint64(&timedOut),
		TotalRequests: atomic.LoadUint64(&totalRequests),
	}
}
