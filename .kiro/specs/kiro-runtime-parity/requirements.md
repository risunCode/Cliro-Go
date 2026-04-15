# Requirements Document

## Introduction

This feature hardens CLIRO's Kiro runtime path so OpenAI and Anthropic requests can be translated to Kiro and back with high reliability. The goal is protocol parity strong enough for live usage, with special focus on thinking, tool calls, tool results, images, streaming, stop reasons, usage, metadata continuity, and provider/runtime fallback behavior.

## Requirements

### Requirement 1

**User Story:** As a proxy client, I want OpenAI-format requests to be translated to Kiro reliably, so that chat, reasoning, tools, and images work without silent degradation.

#### Acceptance Criteria

1. WHEN an OpenAI chat or responses request is received THEN the system SHALL normalize it into a canonical internal request shape before Kiro payload generation.
2. WHEN an OpenAI request contains text, mixed content blocks, or image blocks THEN the system SHALL preserve those inputs in the canonical request and Kiro payload.
3. WHEN an OpenAI request contains tool definitions or tool-call-related metadata THEN the system SHALL preserve tool schema and execution context in the Kiro payload.
4. IF an OpenAI request includes reasoning or thinking-related parameters THEN the system SHALL enable Kiro thinking behavior without requiring extra user configuration.
5. WHEN an OpenAI request contains metadata such as conversation identifiers or profile ARN THEN the system SHALL preserve relevant values through Kiro execution.

### Requirement 2

**User Story:** As a proxy client, I want Anthropic-format requests to be translated to Kiro reliably, so that Claude-compatible applications work against CLIRO without protocol surprises.

#### Acceptance Criteria

1. WHEN an Anthropic messages request is received THEN the system SHALL normalize it into the same canonical internal request shape used by the OpenAI path.
2. WHEN an Anthropic request contains text blocks, image blocks, tool_use blocks, or tool_result blocks THEN the system SHALL preserve those blocks in canonical form and Kiro payload generation.
3. WHEN an Anthropic request contains thinking blocks or thinking budget parameters THEN the system SHALL preserve the request intent during Kiro execution.
4. IF an Anthropic request contains system content THEN the system SHALL preserve that content while composing Kiro-compatible history and system prompt additions.

### Requirement 3

**User Story:** As a proxy client, I want Kiro responses to be converted back into OpenAI-format responses reliably, so that OpenAI-compatible clients receive stable text, reasoning, tool calls, usage, and stop reasons.

#### Acceptance Criteria

1. WHEN a Kiro response contains assistant text THEN the system SHALL expose it as OpenAI-compatible response text in both non-streaming and streaming flows.
2. WHEN a Kiro response contains parsed thinking content THEN the system SHALL expose it as OpenAI-compatible reasoning or thinking output with a stable signature.
3. WHEN a Kiro response contains tool call events THEN the system SHALL expose them as OpenAI-compatible tool calls with normalized JSON arguments.
4. WHEN a Kiro response completes THEN the system SHALL emit OpenAI-compatible usage and stop reason values.
5. IF a Kiro response contains no visible output THEN the system SHALL classify the failure explicitly instead of silently returning an empty successful response.

### Requirement 4

**User Story:** As a proxy client, I want Kiro responses to be converted back into Anthropic-format responses reliably, so that Anthropic-compatible clients receive stable content blocks, thinking blocks, tool use blocks, usage, and stop reasons.

#### Acceptance Criteria

1. WHEN a Kiro response contains assistant text THEN the system SHALL expose it as Anthropic text content.
2. WHEN a Kiro response contains parsed thinking content THEN the system SHALL expose it as Anthropic thinking content with a signature compatible with CLIRO's response model.
3. WHEN a Kiro response contains tool call events THEN the system SHALL expose them as Anthropic tool_use content blocks with normalized input.
4. WHEN a Kiro response completes THEN the system SHALL emit Anthropic-compatible usage and stop reason values.

### Requirement 5

**User Story:** As a proxy operator, I want canonical request and response shapes to be explicit and validated, so that protocol conversion bugs are caught early and ownership stays clear.

#### Acceptance Criteria

1. WHEN OpenAI or Anthropic requests are parsed THEN the system SHALL convert them into typed canonical request structures rather than relying on loosely typed intermediate payloads wherever avoidable.
2. WHEN canonical requests are validated THEN the system SHALL reject incompatible message, image, tool, tool_result, and thinking combinations explicitly.
3. WHEN provider responses are normalized THEN the system SHALL convert them into a canonical response shape before protocol-specific encoding.

### Requirement 6

**User Story:** As a proxy operator, I want Kiro payload generation to preserve system, history, tools, tool results, images, and metadata, so that Kiro execution reflects the original client request accurately.

#### Acceptance Criteria

1. WHEN canonical requests are converted to Kiro payloads THEN the system SHALL preserve conversation IDs, continuation IDs, and profile ARN when available.
2. WHEN canonical requests contain prior assistant or user history THEN the system SHALL preserve conversation order and role semantics in Kiro-compatible history.
3. WHEN canonical requests contain tools or tool results THEN the system SHALL encode them into Kiro-compatible tool definitions and tool result entries.
4. WHEN canonical requests contain images THEN the system SHALL encode supported images into Kiro-compatible image sources.
5. IF canonical requests require thinking mode THEN the system SHALL inject Kiro-compatible thinking instructions into the active user input without requiring manual feature flags.

### Requirement 7

**User Story:** As a proxy operator, I want Kiro stream parsing to be resilient, so that partial events, tool fragments, thinking fragments, and usage updates are reconstructed reliably.

#### Acceptance Criteria

1. WHEN AWS event-stream frames are received from Kiro THEN the system SHALL parse frame headers and payloads safely.
2. WHEN Kiro emits fragmented content, thinking, or tool-call input events THEN the system SHALL reconstruct complete outputs in order.
3. WHEN Kiro emits duplicate or malformed tool fragments THEN the system SHALL deduplicate or sanitize them rather than returning malformed client-visible tool calls.
4. WHEN Kiro emits usage data in varying key formats THEN the system SHALL normalize those values into canonical usage fields.
5. IF Kiro stream data is truncated or malformed THEN the system SHALL surface an explicit provider failure rather than silently succeeding with corrupted output.

### Requirement 8

**User Story:** As a proxy operator, I want Kiro model routing to be reliable and user-friendly, so that supported Kiro models resolve predictably and out-of-the-box.

#### Acceptance Criteria

1. WHEN requests target supported Claude, Qwen, DeepSeek, Minimax, or auto Kiro model names THEN the system SHALL resolve them to Kiro provider routing successfully.
2. WHEN requests use normalized or alias-like Claude version variants THEN the system SHALL normalize them consistently.
3. WHEN `/v1/models` is requested THEN the system SHALL expose Kiro-supported catalog entries alongside other supported provider models.

### Requirement 9

**User Story:** As a proxy operator, I want Kiro runtime fallback behavior to be explicit and resilient, so that runtime host instability does not break live usage unnecessarily.

#### Acceptance Criteria

1. WHEN CLIRO sends a Kiro runtime request THEN the system SHALL try the Q CLI runtime host first.
2. IF the primary Kiro runtime host fails due to transport, parse, or upstream status failure THEN the system SHALL retry against the fallback Kiro runtime host.
3. WHEN all Kiro runtime hosts fail THEN the system SHALL surface the most useful failure reason to the client and account health systems.

### Requirement 10

**User Story:** As a proxy operator, I want provider failures to be mapped consistently, so that auth, quota, malformed request, empty output, and runtime failures are visible and actionable.

#### Acceptance Criteria

1. WHEN Kiro runtime or conversion failures occur THEN the system SHALL classify them into explicit provider failure categories.
2. WHEN a Kiro failure implies relogin, quota cooldown, or account disablement THEN the system SHALL apply the correct account-state transition.
3. WHEN a failure is caused by unsupported request shape or malformed payload THEN the system SHALL return a client-visible request error instead of a generic provider error.

### Requirement 11

**User Story:** As a proxy operator, I want request/response logging around Kiro execution to be rich and structured, so that live debugging is possible without reintroducing architecture sprawl.

#### Acceptance Criteria

1. WHEN Kiro requests are prepared and executed THEN the system SHALL log provider, model, endpoint, thinking state, and account routing decisions.
2. WHEN Kiro fallback hosts are attempted THEN the system SHALL log which runtime host was used and whether fallback occurred.
3. WHEN Kiro responses complete or fail THEN the system SHALL log usage, stop reason, tool activity, and failure class in structured form.
