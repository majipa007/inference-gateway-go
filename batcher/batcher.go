package batcher

import (
	"sync"
	"sync/atomic"
	"time"
)

// Request represents a single request in the batch queue.
type Request struct {
	ID       string
	Payload  []byte
	Received time.Time
	RespCh   chan<- Response
	ErrCh    chan<- error
}

// Response is the result of processing a batched request.
type Response struct {
	ID      string
	Payload []byte
	Err     error
}

// Batcher collects incoming requests and processes them in groups.
type Batcher struct {
	maxSize      int
	maxWait      time.Duration
	handler      func(batch []*Request) []Response
	entries      []*entry
	mu           sync.Mutex
	cond         *sync.Cond
	running      int32
	started      chan struct{}
	stopped      chan struct{}
	batchCount   uint64
	entryCount   uint64
}

type entry struct {
	req *Request
	dl  time.Time
}

type Option func(*Batcher)

// WithMaxSize sets the maximum batch size.
func WithMaxSize(n int) Option {
	return func(b *Batcher) { b.maxSize = n }
}

// WithMaxWait sets the maximum wait time before flushing.
func WithMaxWait(d time.Duration) Option {
	return func(b *Batcher) { b.maxWait = d }
}

// New creates a new batcher with the given options.
func New(handler func([]*Request) []Response, opts ...Option) *Batcher {
	b := &Batcher{
		maxSize:   32,
		maxWait:   50 * time.Millisecond,
		handler:   handler,
		started:   make(chan struct{}),
		stopped:   make(chan struct{}),
	}
	for _, opt := range opts {
		opt(b)
	}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// Start launches the batcher's processing goroutine.
func (b *Batcher) Start() {
	if !atomic.CompareAndSwapInt32(&b.running, 0, 1) {
		return // already running
	}
	b.mu.Lock()
	b.entries = make([]*entry, 0, b.maxSize)
	b.mu.Unlock()
	close(b.started)
	go b.loop()
}

// Stop gracefully shuts down the batcher, waiting for in-flight requests.
func (b *Batcher) Stop() {
	atomic.StoreInt32(&b.running, 0)
	b.cond.Broadcast()
	<-b.stopped
}

// Submit adds a request to the batch. Blocks if the queue is full.
func (b *Batcher) Submit(req *Request) {
	<-b.started

	respCh := make(chan Response, 1)
	req.RespCh = respCh

	b.mu.Lock()
	b.entries = append(b.entries, &entry{
		req: req,
		dl:  time.Now().Add(b.maxWait),
	})

	if len(b.entries) >= b.maxSize {
		b.cond.Broadcast()
	}
	b.mu.Unlock()

	// Wait for this entry to be processed by the batch handler
	<-respCh
}

func (b *Batcher) loop() {
	defer close(b.stopped)

	for atomic.LoadInt32(&b.running) == 1 {
		b.mu.Lock()

		// Wait until we have entries or stop is requested
		for len(b.entries) == 0 {
			if atomic.LoadInt32(&b.running) == 0 {
				b.mu.Unlock()
				return
			}
			b.mu.Unlock()
			time.Sleep(time.Millisecond)
			b.mu.Lock()
		}

		// Batching window - wait for more entries to arrive (up to maxWait)
		if len(b.entries) > 0 && len(b.entries) < b.maxSize {
			b.mu.Unlock()
			time.Sleep(b.maxWait / 2)
			b.mu.Lock()
		}

		// Collect entries for this batch
		batch := make([]*Request, len(b.entries))
		for i, e := range b.entries {
			batch[i] = e.req
		}
		b.entries = b.entries[:0]
		b.cond.Broadcast()
		b.mu.Unlock()

		// Process the batch
		results := b.handler(batch)
		atomic.AddUint64(&b.batchCount, 1)
		atomic.AddUint64(&b.entryCount, uint64(len(batch)))

		// Send results back to each request
		for i, result := range results {
			if i < len(batch) {
				batch[i].RespCh <- result
				close(batch[i].RespCh)
			}
		}
	}
}

// Stats holds batcher runtime statistics.
type Stats struct {
	TotalBatches uint64
	TotalEntries uint64
	AvgBatchSize float64
}

// Stats returns current batcher statistics.
func (b *Batcher) Stats() Stats {
	batches := atomic.LoadUint64(&b.batchCount)
	entries := atomic.LoadUint64(&b.entryCount)
	var avg float64
	if batches > 0 {
		avg = float64(entries) / float64(batches)
	}
	return Stats{
		TotalBatches: batches,
		TotalEntries: entries,
		AvgBatchSize: avg,
	}
}
