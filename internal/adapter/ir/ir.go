package ir

type Protocol string

const (
	ProtocolOpenAI    Protocol = "openai"
	ProtocolAnthropic Protocol = "anthropic"
)

type Endpoint string

const (
	EndpointOpenAIResponses      Endpoint = "openai_responses"
	EndpointOpenAIChat           Endpoint = "openai_chat"
	EndpointOpenAICompletions    Endpoint = "openai_completions"
	EndpointAnthropicMessages    Endpoint = "anthropic_messages"
	EndpointAnthropicCountTokens Endpoint = "anthropic_count_tokens"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleDeveloper Role = "developer"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Request struct {
	Protocol    Protocol
	Endpoint    Endpoint
	Model       string
	Messages    []Message
	Stream      bool
	Temperature *float64
	TopP        *float64
	MaxTokens   *int
	Tools       []Tool
	ToolChoice  any
	User        string
	Metadata    map[string]any
}

type Message struct {
	Role       Role
	Content    any
	Name       string
	ToolCalls  []ToolCall
	ToolCallID string
}

type Tool struct {
	Name        string
	Description string
	Schema      any
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Response struct {
	ID         string
	Model      string
	Text       string
	Thinking   string
	ToolCalls  []ToolCall
	Usage      Usage
	StopReason string
}

type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	InputTokens      int
	OutputTokens     int
}

type Event struct {
	Type       string
	TextDelta  string
	ThinkDelta string
	ToolDelta  any
	Done       bool
}
