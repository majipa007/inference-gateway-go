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
}
