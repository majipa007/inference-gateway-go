package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	baseURL   string
	httpClient *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func LoadConfig() (string, time.Duration) {
	baseURL := strings.TrimSpace(os.Getenv("LLAMA_CPP_URL"))
	if baseURL == "" {
		baseURL = "http://localhost:8081"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	timeoutStr := strings.TrimSpace(os.Getenv("LLAMA_CPP_TIMEOUT"))
	if timeoutStr == "" {
		timeoutStr = "30s"
	}
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil || timeout <= 0 {
		log.Printf("WARN: invalid LLAMA_CPP_TIMEOUT=%q, using default %s", timeoutStr, "30s")
		timeout = 30 * time.Second
	}

	return baseURL, timeout
}

var defaultClient *Client

func init() {
	baseURL, timeout := LoadConfig()
	defaultClient = NewClient(baseURL, timeout)
}

func SetDefaultClient(c *Client) {
	defaultClient = c
}

func (c *Client) Completion(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal completion request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"completion", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create completion request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("completion request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read completion response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("llama.cpp returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var completionResp CompletionResponse
	if err := json.Unmarshal(respBody, &completionResp); err != nil {
		return nil, fmt.Errorf("unmarshal completion response: %w", err)
	}

	return &completionResp, nil
}

func (c *Client) CompletionStream(ctx context.Context, req *CompletionRequest, fn func(text string)) error {
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal completion request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"completion", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create completion request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("completion request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("llama.cpp returned status %d: %s", resp.StatusCode, string(respBody))
	}

	buf := make([]byte, 4096)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			data := string(buf[:n])
			lines := strings.Split(data, "\n")
			for _, line := range lines {
				line = strings.TrimPrefix(line, "data:")
				line = strings.TrimSpace(line)
				if line == "" || line == "[DONE]" {
					continue
				}
				var s StreamResponse
				if err := json.Unmarshal([]byte(line), &s); err != nil {
					continue
				}
				if s.Content != "" {
					fn(s.Content)
				}
				if s.Stop {
					return nil
				}
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("stream read error: %w", err)
		}
	}
}
