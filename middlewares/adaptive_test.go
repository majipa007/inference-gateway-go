package middlewares

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAdaptiveConcurrentLimit_RejectsAtCapacity(t *testing.T) {
	var callCount int64

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	mw := NewAdaptiveConcurrentLimit(3)
	wrapped := mw(handler)

	// Send 5 concurrent requests (limit is 3)
	results := make(chan int, 5)
	for i := 0; i < 5; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	// Collect results
	rejected := 0
	for i := 0; i < 5; i++ {
		code := <-results
		if code == http.StatusServiceUnavailable {
			rejected++
		}
	}

	if rejected == 0 {
		t.Error("expected at least one rejection at capacity")
	}

	// Some requests should have succeeded
	succeeded := 5 - rejected
	if succeeded == 0 {
		t.Error("expected some requests to succeed")
	}
}

func TestAdaptiveConcurrentLimit_AllowUnderLimit(t *testing.T) {
	var callCount int64

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&callCount, 1)
		w.WriteHeader(http.StatusOK)
	})

	mw := NewAdaptiveConcurrentLimit(5)
	wrapped := mw(handler)

	// Send 3 concurrent requests (under limit of 5)
	results := make(chan int, 3)
	for i := 0; i < 3; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	for i := 0; i < 3; i++ {
		code := <-results
		if code != http.StatusOK {
			t.Errorf("expected OK, got %d", code)
		}
	}

	if atomic.LoadInt64(&callCount) != 3 {
		t.Errorf("expected 3 calls, got %d", atomic.LoadInt64(&callCount))
	}
}

func TestAdaptiveConcurrentLimit_MultipleSequential(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := NewAdaptiveConcurrentLimit(2)
	wrapped := mw(handler)

	// Send requests sequentially
	results := make(chan int, 5)
	for i := 0; i < 5; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			wrapped.ServeHTTP(w, req)
			results <- w.Code
		}()
	}

	for i := 0; i < 5; i++ {
		code := <-results
		if code != http.StatusOK && code != http.StatusServiceUnavailable {
			t.Errorf("expected OK or 503, got %d", code)
		}
	}
}

func TestAdaptiveStats(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := NewAdaptiveConcurrentLimit(5)
	wrapped := mw(handler)

	// Send some requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)
	}

	inFlight, softLimit, _, _ := AdaptiveStats()
	if inFlight < 0 {
		t.Errorf("negative in-flight: %d", inFlight)
	}
	if softLimit <= 0 {
		t.Errorf("expected positive soft limit, got %d", softLimit)
	}
}
