package config

type ProxyStats struct {
	TotalRequests    int   `json:"totalRequests"`
	SuccessRequests  int   `json:"successRequests"`
	FailedRequests   int   `json:"failedRequests"`
	PromptTokens     int   `json:"promptTokens"`
	CompletionTokens int   `json:"completionTokens"`
	TotalTokens      int   `json:"totalTokens"`
	LastRequestAt    int64 `json:"lastRequestAt,omitempty"`
}
