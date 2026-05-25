package llamacpp

// PredictRequest is the payload the gateway accepts from the client.
type PredictRequest struct {
	Model  string  `json:"model"`
	Prompt string  `json:"prompt"`
	Tokens int     `json:"n_predict"`
	Temp   float32 `json:"temperature"`
}

// CompletionRequest is the payload sent to the llama.cpp REST server.
type CompletionRequest struct {
	Prompt               string   `json:"prompt"`
	Model                string   `json:"model,omitempty"`
	NPredict             int      `json:"n_predict"`
	Temperature          float32  `json:"temperature"`
	Stream               bool     `json:"stream"`
	TopK                 int      `json:"top_k,omitempty"`
	TopP                 float32  `json:"top_p,omitempty"`
	MaxTokens            int      `json:"max_tokens,omitempty"`
	RepeatPenalty        float32  `json:"repeat_penalty,omitempty"`
	Stop                 []string `json:"stop,omitempty"`
}

// CompletionResponse is the response returned by the llama.cpp REST server.
type CompletionResponse struct {
	Model            string `json:"model"`
	TokensRead       int    `json:"tokens_read"`
	TokensEvaluated  int    `json:"tokens_evaluated"`
	CompletionText   string `json:"completion_text"`
	Stop             bool   `json:"stop"`
	StoppedOutOfTurn bool   `json:"stopped_out_of_turn"`
	Content          string `json:"content"`
}

// StreamResponse represents a single SSE event from llama.cpp streaming.
type StreamResponse struct {
	Content        string `json:"content"`
	Stop           bool   `json:"stop"`
	StoppedOutOfTurn bool   `json:"stopped_out_of_turn"`
	TokensEvaluated int    `json:"tokens_evaluated"`
	TokensRead      int    `json:"tokens_read"`
	CompletionText  string `json:"completion_text"`
}
