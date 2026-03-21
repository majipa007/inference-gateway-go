# ML Inference Gateway

A production-style Go gateway for ML/LLM inference focused on admission control, request batching, scheduling, deadlines, and observability.

## Current Status

Early build stage.

Implemented so far:
- Go project initialized
- basic HTTP server
- `/health` endpoint
- `/metrics` endpoint with basic counters
- `/predict` endpoint
- terminal dashboard for live metrics graphs
- request validation
- Ollama request/response wiring in progress

## Project Goal

Build a gateway that sits in front of model backends and handles:
- safe admission control
- bounded concurrency
- timeout and cancellation propagation
- batching
- scheduling
- observability under load

This project is focused on **systems behavior under load**, not model accuracy.

## Tech Stack

- Go
- `net/http`
- Ollama for local model serving

## Current Flow

Client  
→ Go gateway  
→ model backend (Ollama for now)  
→ response returned to client

## Endpoints

### `GET /health`
Basic health check.

### `POST /predict`
Accepts a prediction request and forwards it to the model backend.

Example request:
```json
{
  "model": "llama3.2",
  "prompt": "Explain semaphores simply"
}
```

### `GET /metrics`
Returns basic gateway counters:
- `in_flight`
- `rejected`
- `timed_out`
- `total_requests`

## Dashboard

Run the gateway:

```bash
go run .
```

In another terminal, launch the live dashboard:

```bash
GOCACHE=/tmp/gocache go run ./cmd/dashboard
```

Optional flags:

```bash
GOCACHE=/tmp/gocache go run ./cmd/dashboard -addr http://localhost:8080/metrics -interval 1s -width 64
```
