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

type engineResult struct {
	name        string
	latencies   []time.Duration
	okCount     uint64
	rejectedCount uint64
	timedOutCount uint64
	otherCount  uint64
	mu          sync.Mutex
}

func main() {
	addr := flag.String("addr", "http://localhost:8080/predict", "predict endpoint URL")
	engine := flag.String("engine", "ollama", "inference engine: ollama or llamacpp")
	engineAB := flag.Bool("ab", false, "run A/B test comparing both engines")
	model := flag.String("model", "llama3.2", "model name")
	prompt := flag.String("prompt", "Explain semaphores simply", "prompt text")
	requests := flag.Int("n", 1000, "total number of requests to send")
	concurrency := flag.Int("c", 100, "number of concurrent workers")
	timeout := flag.Duration("timeout", 35*time.Second, "per-request client timeout")
	flag.Parse()

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

	if *engineAB {
		// A/B test mode - send requests to both Ollama and llama.cpp
		ollamaAddr := strings.Replace(*addr, "/llamacpp/predict", "/predict", 1)
		llamacppAddr := *addr
		if !strings.HasSuffix(llamacppAddr, "/llamacpp/predict") {
			llamacppAddr = strings.Replace(*addr, "/predict", "/llamacpp/predict", 1)
		}

		fmt.Printf("=== A/B Test Mode ===\n")
		fmt.Printf("Ollama endpoint : %s\n", ollamaAddr)
		fmt.Printf("llama.cpp endpoint : %s\n", llamacppAddr)
		fmt.Printf("Total requests per engine : %d\n\n", *requests)

		var ollamaResult, llamacppResult engineResult
		ollamaResult.name = "Ollama"
		llamacppResult.name = "llama.cpp"

		ollamaJobs := make(chan int, *requests)
		llamacppJobs := make(chan int, *requests)

		// Send half to Ollama, half to llama.cpp
		for i := 0; i < *requests; i++ {
			ollamaJobs <- i
			llamacppJobs <- i
		}
		close(ollamaJobs)
		close(llamacppJobs)

		var ollamaWg, llamacppWg sync.WaitGroup

		// Ollama workers
		ollamaWg.Add(1)
		go func() {
			defer ollamaWg.Done()
			for range ollamaJobs {
				runRequest(ctx, client, ollamaAddr, body, &ollamaResult)
			}
		}()

		// llama.cpp workers
		llamacppWg.Add(1)
		go func() {
			defer llamacppWg.Done()
			for range llamacppJobs {
				runRequest(ctx, client, llamacppAddr, body, &llamacppResult)
			}
		}()

		// Wait for both
		done := make(chan struct{})
		go func() {
			ollamaWg.Wait()
			llamacppWg.Wait()
			close(done)
		}()

		<-done

		// Print results
		printResult(&ollamaResult)
		fmt.Println("\n---\n")
		printResult(&llamacppResult)
		fmt.Println("\n---\n")
		printComparison(&ollamaResult, &llamacppResult)

	} else {
		// Single engine mode
		engineAddr := *addr
		if *engine == "llamacpp" && !strings.HasSuffix(engineAddr, "/llamacpp/predict") {
			engineAddr = strings.Replace(engineAddr, "/predict", "/llamacpp/predict", 1)
		}

		var result engineResult
		result.name = *engine

		jobs := make(chan int, *requests)
		var wg sync.WaitGroup
		for range *concurrency {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for range jobs {
					runRequest(ctx, client, engineAddr, body, &result)
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
		printResult(&result)
	}
}

func runRequest(ctx context.Context, client *http.Client, addr string, body []byte, result *engineResult) {
	status, latency, err := sendRequest(ctx, client, addr, body)
	result.mu.Lock()
	result.latencies = append(result.latencies, latency)
	switch {
	case err == nil && status == http.StatusOK:
		atomic.AddUint64(&result.okCount, 1)
	case status == http.StatusServiceUnavailable:
		atomic.AddUint64(&result.rejectedCount, 1)
	case status == http.StatusGatewayTimeout:
		atomic.AddUint64(&result.timedOutCount, 1)
	default:
		atomic.AddUint64(&result.otherCount, 1)
	}
	result.mu.Unlock()
}

func printResult(result *engineResult) {
	fmt.Printf("=== %s Results ===\n", result.name)

	latencies := result.latencies
	sent := atomic.LoadUint64(&result.okCount) +
		atomic.LoadUint64(&result.rejectedCount) +
		atomic.LoadUint64(&result.timedOutCount) +
		atomic.LoadUint64(&result.otherCount)

	if len(latencies) > 0 {
		elapsed := latencies[len(latencies)-1]
		fmt.Printf("duration     : %s\n", elapsed.Round(time.Millisecond))
		if elapsed > 0 {
			fmt.Printf("throughput   : %.2f req/s\n", float64(sent)/elapsed.Seconds())
		}
		fmt.Printf("p1 latency   : %s\n", percentileLatency(latencies, 1))
		fmt.Printf("p50 latency  : %s\n", percentileLatency(latencies, 50))
		fmt.Printf("p90 latency  : %s\n", percentileLatency(latencies, 90))
		fmt.Printf("p99 latency  : %s\n", percentileLatency(latencies, 99))
	}
	fmt.Printf("ok           : %d\n", atomic.LoadUint64(&result.okCount))
	fmt.Printf("rejected     : %d\n", atomic.LoadUint64(&result.rejectedCount))
	fmt.Printf("timed out    : %d\n", atomic.LoadUint64(&result.timedOutCount))
	fmt.Printf("other        : %d\n", atomic.LoadUint64(&result.otherCount))
}

func printComparison(ollama, llamacpp *engineResult) {
	fmt.Println("=== Comparison ===")

	ollamaLatencies := ollama.latencies
	llamacppLatencies := llamacpp.latencies

	fmt.Printf("\n%20s | %20s | %20s\n", "Metric", "Ollama", "llama.cpp")
	fmt.Printf("%20s | %20s | %20s\n", "--------------------", "--------------------", "--------------------")

	// Success rate
	ollamaOk := float64(ollama.okCount)
	llamacppOk := float64(llamacpp.okCount)
	ollamaTotal := float64(len(ollamaLatencies))
	llamacppTotal := float64(len(llamacppLatencies))

	ollamaRate := 0.0
	if ollamaTotal > 0 {
		ollamaRate = (ollamaOk / ollamaTotal) * 100
	}
	llamacppRate := 0.0
	if llamacppTotal > 0 {
		llamacppRate = (llamacppOk / llamacppTotal) * 100
	}
	fmt.Printf("%20s | %18.1f%%  | %18.1f%%\n", "Success Rate", ollamaRate, llamacppRate)

	// P50 latency
	if len(ollamaLatencies) > 0 && len(llamacppLatencies) > 0 {
		p50Ollama := percentileLatency(ollamaLatencies, 50)
		p50Llama := percentileLatency(llamacppLatencies, 50)
		fmt.Printf("%20s | %10s  | %10s\n", "P50 Latency", p50Ollama.Round(time.Millisecond).String(), p50Llama.Round(time.Millisecond).String())

		// P99 latency
		p99Ollama := percentileLatency(ollamaLatencies, 99)
		p99Llama := percentileLatency(llamacppLatencies, 99)
		fmt.Printf("%20s | %10s  | %10s\n", "P99 Latency", p99Ollama.Round(time.Millisecond).String(), p99Llama.Round(time.Millisecond).String())

		// Throughput comparison
		ollamaThroughput := float64(len(ollamaLatencies)) / ollamaLatencies[len(ollamaLatencies)-1].Seconds()
		llamacppThroughput := float64(len(llamacppLatencies)) / llamacppLatencies[len(llamacppLatencies)-1].Seconds()

		var faster string
		var ratio float64
		if llamacppThroughput > ollamaThroughput {
			faster = "Ollama"
			ratio = ollamaThroughput / llamacppThroughput
		} else {
			faster = "llama.cpp"
			ratio = llamacppThroughput / ollamaThroughput
		}
		fmt.Printf("\n%20s : %s by %.2fx\n", "Faster Throughput", faster, 1.0/ratio)
	}
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
