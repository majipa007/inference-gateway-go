# ML Inference Gateway

A production-style Go gateway for ML/LLM inference focused on admission control, request batching, scheduling, deadlines, and observability.

## Current Status

Early build stage.

Implemented so far:
- Go project initialized
- basic HTTP server
- `/health` endpoint
- `/predict` endpoint
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
