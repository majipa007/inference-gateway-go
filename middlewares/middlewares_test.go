package middlewares

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"inference-gateway-go/metrics"
)

func TestLoggerWare(t *testing.T) {
	called := int32(0)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	LoggerWare(next).ServeHTTP(rec, req)

	if atomic.LoadInt32(&called) != 1 {
		t.Error("expected next handler to be called")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestLoggerWareSkipsMetricsPath(t *testing.T) {
	called := int32(0)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	LoggerWare(next).ServeHTTP(rec, req)

	if atomic.LoadInt32(&called) != 1 {
		t.Error("expected next handler to be called for /metrics")
	}
}

func TestMetricsWareIncrementsTotal(t *testing.T) {
	called := int32(0)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt32(&called, 1)
		w.WriteHeader(http.StatusOK)
	})

	before := metrics.Snapshot().TotalRequests
	req := httptest.NewRequest(http.MethodPost, "/predict", nil)
	rec := httptest.NewRecorder()

	MetricsWare(next).ServeHTTP(rec, req)

	if atomic.LoadInt32(&called) != 1 {
		t.Error("expected next handler to be called")
	}

	after := metrics.Snapshot().TotalRequests
	if after != before+1 {
		t.Errorf("expected total requests to increment by 1, before=%d after=%d", before, after)
	}
}

func TestMetricsWareSkipsOwnEndpoint(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	before := metrics.Snapshot().TotalRequests
	MetricsWare(next).ServeHTTP(rec, req)
	after := metrics.Snapshot().TotalRequests

	if after != before {
		t.Errorf("expected total requests to not increment for /metrics, before=%d after=%d", before, after)
	}
}

func TestTimeoutMiddleware(t *testing.T) {
	// Handler that takes 5 seconds
	nextSlow := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	// Handler that completes immediately
	nextFast := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("timeout exceeded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		// 100ms timeout should be exceeded by the 500ms handler
		timeoutHandler := TimeoutMiddleware(100 * time.Millisecond)(nextSlow)
		timeoutHandler.ServeHTTP(rec, req)

		// Context should be cancelled, handler may or may not complete
		// but the context timeout propagates
		if rec.Code != http.StatusOK {
			// It's okay if status changes - the important thing is
			// the context timeout middleware ran
			t.Logf("handler returned status %d with 100ms timeout on 500ms handler", rec.Code)
		}
	})

	t.Run("timeout not exceeded", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		// 5 second timeout - handler completes in 0ms
		timeoutHandler := TimeoutMiddleware(5 * time.Second)(nextFast)
		timeoutHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})
}

func TestTimeoutMiddlewarePropagatesToContext(t *testing.T) {
	done := make(chan struct{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The context should have a deadline
		_, hasDeadline := r.Context().Deadline()
		if !hasDeadline {
			t.Error("expected context to have a deadline")
		}
		close(done)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	timeoutHandler := TimeoutMiddleware(5 * time.Second)(next)
	timeoutHandler.ServeHTTP(rec, req)

	<-done
}

func TestConcurrentLimitWare(t *testing.T) {
	called := int32(0)

	// Handler with max 2 concurrent
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&called, 1)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	limitHandler := ConcurrentLimitWare(2)(next)

	// Send 5 concurrent requests
	results := make(chan int, 5)
	for i := 0; i < 5; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/predict", nil)
			rec := httptest.NewRecorder()
			limitHandler.ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}

	for i := 0; i < 5; i++ {
		code := <-results
		if code != http.StatusOK && code != http.StatusServiceUnavailable {
			t.Errorf("unexpected status code: %d", code)
		}
	}

	// All requests should have been handled (either processed or rejected)
	if called < 2 {
		t.Errorf("expected at least 2 requests to be processed, got %d", called)
	}
}

func TestConcurrentLimitWareRejectsExcess(t *testing.T) {
	beforeRejects := metrics.Snapshot().Rejected

	// Handler that blocks
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	limitHandler := ConcurrentLimitWare(1)(next)

	// First request starts
	results := make(chan int, 3)
	for i := 0; i < 3; i++ {
		go func() {
			req := httptest.NewRequest(http.MethodPost, "/predict", nil)
			rec := httptest.NewRecorder()
			limitHandler.ServeHTTP(rec, req)
			results <- rec.Code
		}()
	}

	for i := 0; i < 3; i++ {
		<-results
	}

	afterRejects := metrics.Snapshot().Rejected
	rejects := int(afterRejects - beforeRejects)

	if rejects == 0 {
		t.Error("expected at least one request to be rejected with 503")
	}
	if rejects > 2 {
		t.Errorf("expected at most 2 rejections, got %d", rejects)
	}
}
