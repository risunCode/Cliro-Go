# Live Validation Checklist

## Basic Text

- Send `POST /v1/chat/completions` with a Kiro-routed model and plain text user content
- Send `POST /v1/messages` with a Kiro-routed Claude model and plain text content
- Verify non-empty assistant text and stable usage fields

## Thinking

- Send an OpenAI request with reasoning fields
- Send an Anthropic request with thinking budget fields
- Verify parsed thinking content is surfaced back in both protocol responses

## Tools

- Send a request with tool definitions and confirm Kiro emits tool calls
- Send a continuation with tool results and verify they are preserved in payload history
- Verify returned tool arguments are valid JSON and stop reason becomes tool-related

## Images

- Send OpenAI `image_url` data URL content
- Send Anthropic `image` base64 content blocks
- Verify Kiro payload includes encoded image sources and request still completes

## Runtime Fallback

- Simulate primary Kiro runtime failure
- Verify CLIRO attempts the fallback host
- Verify logs include attempted runtime host and fallback behavior

## Failure Handling

- Send malformed tool payloads and verify explicit client errors
- Simulate malformed or truncated Kiro stream content and verify provider failure instead of silent empty success
- Simulate relogin/quota-style failures and verify account-state transitions are applied
