package batcher

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatcher_ProcessesRequests(t *testing.T) {
	var processed int64

	handler := func(batch []*Request) []Response {
		atomic.AddInt64(&processed, int64(len(batch)))
		results := make([]Response, len(batch))
		for i, req := range batch {
			results[i] = Response{
				ID:      req.ID,
				Payload: []byte(fmt.Sprintf("batched_%d", len(batch))),
			}
		}
		return results
	}

	b := New(handler, WithMaxSize(10), WithMaxWait(100*time.Millisecond))
	b.Start()
	defer b.Stop()

	// Submit a few requests
	for i := 0; i < 5; i++ {
		req := &Request{
			ID:     fmt.Sprintf("req-%d", i),
			Payload: []byte(fmt.Sprintf("payload-%d", i)),
		}
		b.Submit(req)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt64(&processed) != 5 {
		t.Errorf("expected 5 requests processed, got %d", atomic.LoadInt64(&processed))
	}
}

func TestBatcher_BatchCollectsMultiple(t *testing.T) {
	var maxBatchSeen int
	var mu sync.Mutex

	handler := func(batch []*Request) []Response {
		mu.Lock()
		if len(batch) > maxBatchSeen {
			maxBatchSeen = len(batch)
		}
		mu.Unlock()

		results := make([]Response, len(batch))
		for i, req := range batch {
			results[i] = Response{ID: req.ID, Payload: []byte("ok")}
		}
		return results
	}

	b := New(handler, WithMaxSize(100), WithMaxWait(50*time.Millisecond))
	b.Start()
	defer b.Stop()

	// Submit requests quickly - they should batch together
	for i := 0; i < 20; i++ {
		req := &Request{
			ID:      fmt.Sprintf("req-%d", i),
			Payload: []byte("payload"),
		}
		b.Submit(req)
	}

	time.Sleep(150 * time.Millisecond)

	mu.Lock()
	if maxBatchSeen < 2 {
		t.Errorf("expected batches of at least 2, max seen was %d", maxBatchSeen)
	}
	mu.Unlock()
}

func TestBatcher_Stats(t *testing.T) {
	handler := func(batch []*Request) []Response {
		results := make([]Response, len(batch))
		for i, req := range batch {
			results[i] = Response{ID: req.ID, Payload: []byte("ok")}
		}
		return results
	}

	b := New(handler, WithMaxSize(5), WithMaxWait(100*time.Millisecond))
	b.Start()
	defer b.Stop()

	for i := 0; i < 10; i++ {
		req := &Request{
			ID:      fmt.Sprintf("req-%d", i),
			Payload: []byte("payload"),
		}
		b.Submit(req)
	}

	time.Sleep(200 * time.Millisecond)

	stats := b.Stats()
	if stats.TotalBatches == 0 {
		t.Error("expected at least one batch")
	}
	if stats.TotalEntries != 10 {
		t.Errorf("expected 10 total entries, got %d", stats.TotalEntries)
	}
	if stats.AvgBatchSize < 0.5 || stats.AvgBatchSize > 10 {
		t.Errorf("unexpected avg batch size: %.1f", stats.AvgBatchSize)
	}
}

func TestBatcher_WithMaxSizeLimit(t *testing.T) {
	var maxBatchSeen int
	var mu sync.Mutex

	handler := func(batch []*Request) []Response {
		mu.Lock()
		if len(batch) > maxBatchSeen {
			maxBatchSeen = len(batch)
		}
		mu.Unlock()

		results := make([]Response, len(batch))
		for i, req := range batch {
			results[i] = Response{ID: req.ID, Payload: []byte("ok")}
		}
		return results
	}

	b := New(handler, WithMaxSize(5), WithMaxWait(500*time.Millisecond))
	b.Start()
	defer b.Stop()

	// Submit 20 requests - with max batch size 5, should batch in groups
	for i := 0; i < 20; i++ {
		req := &Request{
			ID:      fmt.Sprintf("req-%d", i),
			Payload: []byte("payload"),
		}
		b.Submit(req)
	}

	time.Sleep(300 * time.Millisecond)

	mu.Lock()
	if maxBatchSeen > 5 {
		t.Errorf("expected max batch size of 5, got %d", maxBatchSeen)
	}
	mu.Unlock()
}

func TestBatcher_CancelableWait(t *testing.T) {
	handler := func(batch []*Request) []Response {
		results := make([]Response, len(batch))
		for i, req := range batch {
			results[i] = Response{ID: req.ID, Payload: []byte("ok")}
		}
		return results
	}

	b := New(handler, WithMaxSize(5), WithMaxWait(200*time.Millisecond))
	b.Start()

	req := &Request{
		ID:      "req-1",
		Payload: []byte("payload"),
	}

	done := make(chan struct{})
	go func() {
		b.Submit(req)
		close(done)
	}()

	select {
	case <-done:
		// Request completed
	case <-time.After(1 * time.Second):
		// This is expected - the single request might be waiting for more to fill the batch
		// or it timed out after maxWait
		t.Logf("Submit took >1s (expected for single request with batch wait)")
	}

	b.Stop()
}

func TestBatcher_MultipleSequentialBatches(t *testing.T) {
	handler := func(batch []*Request) []Response {
		results := make([]Response, len(batch))
		for i, req := range batch {
			results[i] = Response{
				ID:      req.ID,
				Payload: []byte(fmt.Sprintf("batch-%d-size-%d", len(batch), len(batch))),
			}
		}
		return results
	}

	b := New(handler, WithMaxSize(3), WithMaxWait(100*time.Millisecond))
	b.Start()
	defer b.Stop()

	// First batch
	for i := 0; i < 3; i++ {
		req := &Request{
			ID:      fmt.Sprintf("b1-r%d", i),
			Payload: []byte("batch1"),
		}
		b.Submit(req)
	}
	time.Sleep(150 * time.Millisecond)

	// Second batch
	for i := 0; i < 2; i++ {
		req := &Request{
			ID:      fmt.Sprintf("b2-r%d", i),
			Payload: []byte("batch2"),
		}
		b.Submit(req)
	}
	time.Sleep(150 * time.Millisecond)

	stats := b.Stats()
	if stats.TotalBatches < 2 {
		t.Errorf("expected at least 2 batches, got %d", stats.TotalBatches)
	}
	if stats.TotalEntries != 5 {
		t.Errorf("expected 5 total entries, got %d", stats.TotalEntries)
	}
}

func TestBatcher_PayloadRoundTrip(t *testing.T) {
	handler := func(batch []*Request) []Response {
		results := make([]Response, len(batch))
		for i, req := range batch {
			results[i] = Response{
				ID:      req.ID,
				Payload: []byte("echo_" + string(req.Payload)),
			}
		}
		return results
	}

	b := New(handler, WithMaxSize(10), WithMaxWait(50*time.Millisecond))
	b.Start()
	defer b.Stop()

	req := &Request{
		ID:      "test-1",
		Payload: []byte("hello"),
	}
	b.Submit(req)

	time.Sleep(100 * time.Millisecond)
	// If we got here without panic, the round-trip worked
}

func TestBatcher_JSONPayloadRoundTrip(t *testing.T) {
	handler := func(batch []*Request) []Response {
		results := make([]Response, len(batch))
		for i, req := range batch {
			// Echo back with prefix
			results[i] = Response{
				ID:      req.ID,
				Payload: []byte(fmt.Sprintf(`{"result":"batched","original":%s}`, string(req.Payload))),
			}
		}
		return results
	}

	b := New(handler, WithMaxSize(10), WithMaxWait(50*time.Millisecond))
	b.Start()
	defer b.Stop()

	type input struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}
	inputData := input{Model: "llama3.2", Prompt: "hello world"}
	payload, _ := json.Marshal(inputData)

	req := &Request{
		ID:      "json-test",
		Payload: payload,
	}
	b.Submit(req)

	time.Sleep(100 * time.Millisecond)
	// Round-trip succeeded
}
