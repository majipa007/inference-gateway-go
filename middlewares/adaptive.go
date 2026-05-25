package middlewares

import (
	"context"
	"net/http"
	"sync/atomic"

	"inference-gateway-go/metrics"
)

type adaptiveLimiter struct {
	maxLimit int
	hardSem  chan struct{}

	inFlight   int64
	softLimit  int64
}

var (
	adaptiveInFlight int64
	adaptiveSoft     int64
	adaptiveRejected int64
	adaptiveSeen     int64
)

// NewAdaptiveConcurrentLimit returns middleware that adaptively limits concurrency.
//
// When load is light, the limiter acts like ConcurrentLimitWare with maxLimit.
// When load exceeds the threshold, it proactively rejects requests to prevent
// backend saturation. This is a form of load shedding.
func NewAdaptiveConcurrentLimit(maxLimit int) func(next http.Handler) http.Handler {
	al := &adaptiveLimiter{
		maxLimit: maxLimit,
		hardSem:  make(chan struct{}, maxLimit),
	}
	atomic.StoreInt64(&adaptiveSoft, int64(maxLimit))

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !al.tryAccept(r.Context(), w) {
				return
			}

			atomic.AddInt64(&al.inFlight, 1)
			atomic.AddInt64(&adaptiveInFlight, 1)
			metrics.IncInFlight()

			next.ServeHTTP(w, r)

			atomic.AddInt64(&al.inFlight, -1)
			atomic.AddInt64(&adaptiveInFlight, -1)
			metrics.DecInFlight()

			<-al.hardSem
		})
	}
}

func (al *adaptiveLimiter) tryAccept(ctx context.Context, w http.ResponseWriter) bool {
	atomic.AddInt64(&adaptiveSeen, 1)

	// Try to acquire the hard semaphore
	select {
	case al.hardSem <- struct{}{}:
		// Got in
	case <-ctx.Done():
		<-al.hardSem
		http.Error(w, "request canceled", http.StatusRequestTimeout)
		return false
	default:
		// Hard limit hit - reject
		metrics.IncRejected()
		http.Error(w, "server busy (adaptive limit)", http.StatusServiceUnavailable)
		return false
	}

	// Check soft limit - reject early if we're overloaded
	for {
		inFlight := atomic.LoadInt64(&adaptiveInFlight)
		soft := atomic.LoadInt64(&adaptiveSoft)

		if inFlight <= soft || inFlight <= 0 {
			return true
		}

		// Load is high - reduce soft limit further
		newSoft := int64(float64(inFlight) * 0.6)
		if newSoft < int64(al.maxLimit)/4 {
			newSoft = int64(al.maxLimit) / 4
		}
		if newSoft < atomic.SwapInt64(&adaptiveSoft, newSoft) || atomic.LoadInt64(&adaptiveSoft) >= newSoft {
			// We won the CAS or no one else changed it
		}

		inFlight = atomic.LoadInt64(&adaptiveInFlight)
		soft = atomic.LoadInt64(&adaptiveSoft)
		if inFlight <= soft {
			return true
		}

		// Still over - release and reject (load shedding)
		<-al.hardSem
		atomic.AddInt64(&adaptiveRejected, 1)
		metrics.IncRejected()
		http.Error(w, "server busy (adaptive limit)", http.StatusServiceUnavailable)
		return false
	}
}

// AdaptiveStats returns current adaptive limiter statistics.
func AdaptiveStats() (inFlight, softLimit, rejected, seen int64) {
	return atomic.LoadInt64(&adaptiveInFlight),
		atomic.LoadInt64(&adaptiveSoft),
		atomic.LoadInt64(&adaptiveRejected),
		atomic.LoadInt64(&adaptiveSeen)
}
