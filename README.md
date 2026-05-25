# ML Inference Gateway

A production-style Go gateway for ML/LLM inference focused on admission control, request batching, scheduling, deadlines, and observability.

## Current Status

Implemented features:
- Go project initialized
- basic HTTP server
- `/health` endpoint
- `/metrics` endpoint with latency histograms, cache counters, batch counters
- `/predict` endpoint (Ollama backend)
- `/llamacpp/predict` endpoint (llama.cpp REST API)
- `/llamacpp/stream` endpoint (llama.cpp SSE streaming)
- `/llamacpp/health` endpoint
- terminal dashboard for live metrics graphs with braille area charts
- spam/load generator with A/B test mode for engine comparison
- request validation
- concurrent request limiting (static and adaptive)
- LRU response caching with configurable TTL and capacity
- request batching for throughput optimization
- adaptive concurrency limiter with dynamic soft limit
- comprehensive test suite (40+ tests across all packages)

## Project Goal

Build a gateway that sits in front of model backends and handles:
- safe admission control
- bounded concurrency
- timeout and cancellation propagation
- batching
- scheduling
- observability under load
- multi-backend support (Ollama vs llama.cpp)

This project is focused on **systems behavior under load**, not model accuracy.

## Tech Stack

- Go
- `net/http`
- Ollama for local model serving
- llama.cpp REST API for comparison

## Endpoints

### `GET /health`
Health check that verifies the gateway can reach Ollama.
Returns `200` only when Ollama responds successfully.

### `POST /predict`
Ollama endpoint. Accepts a prediction request and forwards it to the Ollama model backend.

Example request:
```json
{
  "model": "llama3.2",
  "prompt": "Explain semaphores simply"
}
```

### `POST /llamacpp/predict`
llama.cpp endpoint. Accepts a prediction request and forwards it to the llama.cpp REST server.

Example request:
```json
{
  "model": "llama3.2",
  "prompt": "Explain semaphores simply",
  "n_predict": 256,
  "temperature": 0.8
}
```

### `POST /llamacpp/stream`
llama.cpp streaming endpoint. Returns an SSE stream of completion tokens.

Example request:
```json
{
  "model": "llama3.2",
  "prompt": "Explain semaphores simply",
  "n_predict": 256,
  "temperature": 0.8
}
```

### `GET /metrics`
Returns gateway metrics in JSON format:
- `in_flight` - current concurrent requests
- `rejected` - requests shed due to concurrency limits
- `timed_out` - requests that exceeded timeout
- `total_requests` - total requests received
- `batched` - requests processed through batcher
- `total_batches` - total batch operations performed
- `cache_hits` - cache lookup hits
- `cache_misses` - cache lookup misses
- `latency_p50` / `latency_p90` / `latency_p99` - latency percentiles (seconds)
- `adaptive_soft_limit` - current adaptive concurrency threshold

## Configuration

### Ollama
```bash
# Default: http://localhost:11434
export OLLAMA_URL=http://localhost:11434
```

### llama.cpp
```bash
# Default: http://localhost:8081
export LLAMA_CPP_URL=http://localhost:8081

# Default: 30s
export LLAMA_CPP_TIMEOUT=30s
```

### Gateway
```bash
# Default: 30s
export GATEWAY_TIMEOUT=200s
```

## Running

### Start the gateway
```bash
go run .
```

### Dashboard
In another terminal, launch the live dashboard:
```bash
GOCACHE=/tmp/gocache go run ./cmd/dashboard
```

Optional flags:
```bash
GOCACHE=/tmp/gocache go run ./cmd/dashboard -addr http://localhost:8080/metrics -interval 1s -width 64
```

## Load Testing

Current gateway defaults:
- max concurrent `/predict` requests: `2000`
- request timeout: `30s`

### Ollama endpoint
```bash
GOCACHE=/tmp/gocache go run ./cmd/spam -n 1000 -c 100
```

### llama.cpp endpoint
```bash
GOCACHE=/tmp/gocache go run ./cmd/spam -n 1000 -c 100 -engine llamacpp
```

### Custom flags
```bash
GOCACHE=/tmp/gocache go run ./cmd/spam \
  -addr http://localhost:8080/predict \
  -model llama3.2 \
  -prompt "Explain semaphores simply" \
  -n 1000 -c 100 -timeout 35s
```

### A/B Test (Ollama vs llama.cpp)
Runs equal load on both engines simultaneously and outputs a comparison table:
```bash
GOCACHE=/tmp/gocache go run ./cmd/spam -ab -n 1000 -c 100
```

Example output:
```
=== A/B Test Mode ===
Ollama endpoint : http://localhost:8080/predict
llama.cpp endpoint : http://localhost:8080/llamacpp/predict
Total requests per engine : 1000

              Metric |               Ollama |            llama.cpp
-------------------- | -------------------- | --------------------
        Success Rate |               98.5%  |               99.2%
         P50 Latency |       45ms   |       32ms
         P99 Latency |       120ms  |       89ms

   Faster Throughput : llama.cpp by 1.41x
```

## Architecture

```
Client
  → LoggerWare (request logging)
  → MetricsWare (request counting)
  → TimeoutMiddleware (deadline propagation)
  → AdaptiveLimiter (concurrency control)
  → CachingHandler (LRU cache: hit → return / miss → next handler)
  → Handler (Ollama or llama.cpp)
  → response returned to client

Caching:
  Key: model + prompt
  Default TTL: 10 minutes
  Default capacity: 10,000 entries
  Cache key: model:prompt (e.g., "llama3.2:Explain semaphores simply")

Adaptive Concurrency:
  - Hard limit: 2000 concurrent requests
  - Soft limit: starts at hard limit, reduces to 60% of actual load under pressure
  - Minimum soft limit: hard_limit / 4
  - Load shedding: rejects requests when load exceeds soft limit
  - Prevents backend saturation during traffic spikes
```

## Performance Features

### Response Caching
Identical requests (same model + prompt) return cached responses without hitting the backend. Second request includes `X-Cache: HIT` header.

### Adaptive Concurrency
Unlike static limits, the adaptive limiter monitors in-flight requests and dynamically adjusts the acceptance threshold. During high load, it proactively sheds traffic before backends saturate.

### Request Batching
The batcher collects requests over a configurable window (default 50ms) or max batch size (default 32), then processes them together for throughput optimization.

### A/B Testing
Run side-by-side comparisons between Ollama and llama.cpp:
```bash
go run ./cmd/spam -ab -n 1000 -c 100
```
Outputs a comparison table with success rate, P50/P99 latency, and throughput ratio for each engine.

## Testing

Run all tests:
```bash
go test ./...
```

Run with coverage:
```bash
go test ./... -cover
```

Run specific package:
```bash
go test ./gateway/... -v
go test ./cache/... -v
go test ./middlewares/... -v
```

Test suite covers:
- **cache**: LRU eviction, TTL expiry, concurrent access, capacity bounds (12 tests)
- **batcher**: Batch collection, flush timing, stats tracking (9 tests)
- **middlewares**: Adaptive limiter rejection, context cancellation, concurrent stats (4 tests)
- **gateway**: Cache hit/miss, TTL expiry, capacity eviction, concurrent access, integration flows (14 tests)
- **metrics**: Histogram recording, percentile calculation, counter operations, batch/cache tracking

## Comparison: Ollama vs llama.cpp

This project supports both backends for comparison:

| Feature | Ollama | llama.cpp |
|---------|--------|-----------|
| Endpoint | `/predict` | `/llamacpp/predict` |
| Streaming | No | `/llamacpp/stream` (SSE) |
| Config | `OLLAMA_URL` | `LLAMA_CPP_URL`, `LLAMA_CPP_TIMEOUT` |
| Model loading | Automatic | Pre-loaded server |
| API style | `/api/generate` | `/completion` |
| Token metrics | No | Yes (tokens_read, tokens_evaluated) |
| Health check | `/health` | `/llamacpp/health` |

## Gateway Features (Shared by Both Backends)

| Feature | Implementation |
|---------|---------------|
| Response caching | LRU cache, configurable TTL (default 10m), capacity (default 10000) |
| Adaptive concurrency | Dynamic soft limit, load shedding, configurable hard limit (default 2000) |
| Latency tracking | Histogram with P50/P90/P99 percentiles |
| Request logging | Structured logging with method, path, status, duration |
| Timeout propagation | Context-based with configurable gateway timeout |
| Metrics | JSON endpoint with counters, histograms, and percentiles |
| Dashboard | Terminal UI with braille area charts, rolling history, real-time updates |
| Load testing | Concurrent workers, configurable duration, A/B comparison mode |
