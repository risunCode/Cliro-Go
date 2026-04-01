package openai

type ChatResponse struct {
	ID      string       `json:"id"`
	Object  string       `json:"object"`
	Created int64        `json:"created"`
	Model   string       `json:"model"`
	Choices []ChatChoice `json:"choices"`
	Usage   Usage        `json:"usage"`
}

type ChatChoice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type ChatMessage struct {
	Role             string `json:"role"`
	Content          any    `json:"content"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	ToolCalls        any    `json:"tool_calls,omitempty"`
}

type CompletionsResponse struct {
	ID      string              `json:"id"`
	Object  string              `json:"object"`
	Created int64               `json:"created"`
	Model   string              `json:"model"`
	Choices []CompletionsChoice `json:"choices"`
	Usage   Usage               `json:"usage"`
}

type CompletionsChoice struct {
	Index            int    `json:"index"`
	Text             string `json:"text"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	FinishReason     string `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type ResponsesResponse struct {
	ID         string                `json:"id"`
	Object     string                `json:"object"`
	CreatedAt  int64                 `json:"created_at"`
	Status     string                `json:"status"`
	Model      string                `json:"model"`
	Output     []ResponsesOutputItem `json:"output,omitempty"`
	OutputText string                `json:"output_text,omitempty"`
	Usage      ResponsesUsage        `json:"usage"`
}

type ResponsesOutputItem struct {
	ID        string                 `json:"id,omitempty"`
	Type      string                 `json:"type"`
	Role      string                 `json:"role,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Content   []ResponsesContentPart `json:"content,omitempty"`
	CallID    string                 `json:"call_id,omitempty"`
	Name      string                 `json:"name,omitempty"`
	Arguments string                 `json:"arguments,omitempty"`
}

type ResponsesContentPart struct {
	Type             string `json:"type"`
	Text             string `json:"text,omitempty"`
	ReasoningContent string `json:"reasoning_content,omitempty"`
	Annotations      []any  `json:"annotations,omitempty"`
}

type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}
