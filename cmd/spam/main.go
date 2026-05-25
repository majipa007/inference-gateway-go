package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type predictRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

func main() {
	addr := flag.String("addr", "http://localhost:8080/predict", "predict endpoint URL")
	engine := flag.String("engine", "ollama", "inference engine: ollama or llamacpp")
	model := flag.String("model", "llama3.2", "model name")
	prompt := flag.String("prompt", "Explain semaphores simply", "prompt text")
	requests := flag.Int("n", 1000, "total number of requests to send")
	concurrency := flag.Int("c", 100, "number of concurrent workers")
	timeout := flag.Duration("timeout", 35*time.Second, "per-request client timeout")
	flag.Parse()

	if *engine == "llamacpp" && !strings.HasSuffix(*addr, "/llamacpp/predict") {
		*addr = strings.Replace(*addr, "/predict", "/llamacpp/predict", 1)
	}

	if *requests <= 0 {
		fmt.Fprintln(os.Stderr, "-n must be greater than 0")
		os.Exit(1)
	}
	if *concurrency <= 0 {
		fmt.Fprintln(os.Stderr, "-c must be greater than 0")
		os.Exit(1)
	}

	if *requests <= 0 {
		fmt.Fprintln(os.Stderr, "-n must be greater than 0")
		os.Exit(1)
	}
	if *concurrency <= 0 {
		fmt.Fprintln(os.Stderr, "-c must be greater than 0")
		os.Exit(1)
	}

	body, err := json.Marshal(predictRequest{
		Model:  *model,
		Prompt: *prompt,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to encode request body: %v\n", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := &http.Client{Timeout: *timeout}
	jobs := make(chan int)
	latencies := make([]time.Duration, 0, *requests)
	var latenciesMu sync.Mutex

	var okCount uint64
	var rejectedCount uint64
	var timedOutCount uint64
	var otherCount uint64

	start := time.Now()
	var wg sync.WaitGroup
	for range *concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				status, latency, err := sendRequest(ctx, client, *addr, body)
				latenciesMu.Lock()
				latencies = append(latencies, latency)
				latenciesMu.Unlock()
				switch {
				case err == nil && status == http.StatusOK:
					atomic.AddUint64(&okCount, 1)
				case status == http.StatusServiceUnavailable:
					atomic.AddUint64(&rejectedCount, 1)
				case status == http.StatusGatewayTimeout:
					atomic.AddUint64(&timedOutCount, 1)
				default:
					atomic.AddUint64(&otherCount, 1)
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for i := 0; i < *requests; i++ {
			select {
			case <-ctx.Done():
				return
			case jobs <- i:
			}
		}
	}()

	wg.Wait()
	elapsed := time.Since(start)

	sent := atomic.LoadUint64(&okCount) +
		atomic.LoadUint64(&rejectedCount) +
		atomic.LoadUint64(&timedOutCount) +
		atomic.LoadUint64(&otherCount)

	fmt.Printf("target       : %s\n", *addr)
	fmt.Printf("requests     : %d\n", *requests)
	fmt.Printf("concurrency  : %d\n", *concurrency)
	fmt.Printf("duration     : %s\n", elapsed.Round(time.Millisecond))
	if elapsed > 0 {
		fmt.Printf("throughput   : %.2f req/s\n", float64(sent)/elapsed.Seconds())
	}
	if len(latencies) > 0 {
		fmt.Printf("p1 latency   : %s\n", percentileLatency(latencies, 1))
		fmt.Printf("p50 latency  : %s\n", percentileLatency(latencies, 50))
		fmt.Printf("p90 latency  : %s\n", percentileLatency(latencies, 90))
		fmt.Printf("p99 latency  : %s\n", percentileLatency(latencies, 99))
	}
	fmt.Printf("ok           : %d\n", atomic.LoadUint64(&okCount))
	fmt.Printf("rejected     : %d\n", atomic.LoadUint64(&rejectedCount))
	fmt.Printf("timed out    : %d\n", atomic.LoadUint64(&timedOutCount))
	fmt.Printf("other        : %d\n", atomic.LoadUint64(&otherCount))
}

func sendRequest(ctx context.Context, client *http.Client, addr string, body []byte) (int, time.Duration, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, addr, bytes.NewReader(body))
	if err != nil {
		return 0, time.Since(start), err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return 0, time.Since(start), err
	}
	defer resp.Body.Close()

	return resp.StatusCode, time.Since(start), nil
}

func percentileLatency(latencies []time.Duration, percentile float64) time.Duration {
	sorted := append([]time.Duration(nil), latencies...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	rank := int(math.Ceil((percentile / 100) * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}

	return sorted[rank-1].Round(time.Millisecond)
}
