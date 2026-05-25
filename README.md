# ML Inference Gateway

A production-style Go gateway for ML/LLM inference focused on admission control, request batching, scheduling, deadlines, and observability.

## Current Status

Early build stage.

Implemented so far:
- Go project initialized
- basic HTTP server
- `/health` endpoint
- `/metrics` endpoint with basic counters
- `/predict` endpoint (Ollama backend)
- `/llamacpp/predict` endpoint (llama.cpp REST API)
- `/llamacpp/stream` endpoint (llama.cpp streaming)
- terminal dashboard for live metrics graphs
- spam/load generator for both endpoints
- request validation
- concurrent request limiting

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
Returns basic gateway counters:
- `in_flight`
- `rejected`
- `timed_out`
- `total_requests`

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

## Architecture

```
Client
  → Go gateway
  → model backend (Ollama OR llama.cpp)
  → response returned to client

Middleware chain:
  LoggerWare → MetricsWare → TimeoutMiddleware → ConcurrentLimitWare → handler
```

## Testing

Run all tests:
```bash
go test ./...
```

Run with coverage:
```bash
go test ./... -cover
```

## Comparison: Ollama vs llama.cpp

This project supports both backends for comparison:

| Feature | Ollama | llama.cpp |
|---------|--------|-----------|
| Endpoint | `/predict` | `/llamacpp/predict` |
| Streaming | No | `/llamacpp/stream` |
| Config | `OLLAMA_URL` | `LLAMA_CPP_URL` |
| Model loading | Automatic | Pre-loaded server |
| API style | `/api/generate` | `/completion` |
| Token metrics | No | Yes (tokens_read, tokens_evaluated) |
