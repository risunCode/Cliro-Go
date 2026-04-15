# Implementation Plan

- [x] 1. Strengthen canonical proxy models for typed content and validation
  - Expand `internal/proxy/models/resolve.go` to introduce explicit canonical content/image/tool_result/thinking structures while preserving current ownership in the same package
  - Add validation helpers for incompatible message, image, tool, tool_result, and thinking combinations before provider execution
  - Ensure canonical request/response shapes remain the single shared model across proxy and provider layers
  - _Requirements: 5.1, 5.2, 5.3, 6.1, 6.4, 7.4_

- [x] 2. Harden OpenAI codec conversion into and out of canonical models
  - [x] 2.1 Normalize OpenAI inbound content blocks, reasoning fields, images, and tool metadata
    - Update `internal/proxy/openai/codec.go` so chat/responses/completions requests consistently map into canonical request structures
    - Preserve text, image, tool, and metadata fields without silent degradation
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 5.1_

  - [x] 2.2 Stabilize OpenAI outbound response and streaming encoding
    - Update `internal/proxy/openai/codec.go` to encode canonical responses with reliable IDs, usage, tool calls, thinking output, and stop reasons
    - Keep OpenAI-compatible response semantics stable across non-streaming and streaming paths
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 5.3_

- [x] 3. Harden Anthropic codec conversion into and out of canonical models
  - [x] 3.1 Normalize Anthropic inbound system/messages content, images, tool_use, tool_result, and thinking blocks
    - Update `internal/proxy/anthropic/codec.go` so Anthropic requests map into canonical request structures with preserved role/content semantics
    - _Requirements: 2.1, 2.2, 2.3, 2.4, 5.1_

  - [x] 3.2 Stabilize Anthropic outbound response and streaming encoding
    - Update `internal/proxy/anthropic/codec.go` to encode canonical responses into Anthropic text, thinking, tool_use, usage, and stop reason output consistently
    - _Requirements: 4.1, 4.2, 4.3, 4.4, 5.3_

- [x] 4. Make Kiro payload generation deterministic and parity-focused
  - [x] 4.1 Rework history and system composition for Kiro payloads
    - Update `internal/provider/kiro/payload.go` so system/developer content, user history, assistant history, and metadata continuity are encoded in a stable order
    - Preserve conversationId, continuationId, and profileArn when present
    - _Requirements: 1.5, 2.4, 6.1, 6.2_

  - [x] 4.2 Encode tools, tool results, and images into Kiro-compatible payload structures
    - Update `internal/provider/kiro/payload.go` to preserve tool definitions, tool results, and supported image blocks without best-effort degradation
    - _Requirements: 1.2, 1.3, 2.2, 6.3, 6.4_

  - [x] 4.3 Stabilize Kiro thinking-mode injection and canonical metadata usage
    - Keep default-enabled thinking behavior while ensuring the active user input and system prompt additions remain deterministic and traceable
    - _Requirements: 1.4, 2.3, 6.5_

- [x] 5. Harden Kiro stream parsing and completion reconstruction
  - [x] 5.1 Parse AWS event-stream frames into deterministic internal state transitions
    - Refine `internal/provider/kiro/stream.go` to safely parse frames, normalize event variants, and reconstruct text, thinking, tool calls, and usage
    - _Requirements: 7.1, 7.2, 7.4_

  - [x] 5.2 Handle malformed, duplicate, and truncated tool or content fragments safely
    - Deduplicate tool events, normalize JSON arguments, and fail explicitly on malformed or corrupted stream outcomes rather than returning silent success
    - _Requirements: 3.5, 7.3, 7.5, 10.1, 10.3_

- [x] 6. Tighten Kiro runtime execution and provider failure behavior
  - [x] 6.1 Improve Kiro runtime host fallback and logging behavior
    - Update `internal/provider/kiro/execute.go` and `internal/provider/kiro/service.go` so Q CLI runtime is tried first, codewhisperer host is fallback, and host choice/fallback are logged explicitly
    - _Requirements: 9.1, 9.2, 9.3, 11.1, 11.2_

  - [x] 6.2 Map Kiro runtime failures into provider/account-state transitions consistently
    - Update `internal/provider/kiro/service.go` and related failure classification usage so auth, quota, malformed request, empty output, and upstream failures trigger the right account-state actions
    - _Requirements: 3.5, 7.5, 10.1, 10.2, 10.3, 11.3_

- [x] 7. Complete proxy execution parity for Codex and Kiro outcomes
  - Update `internal/proxy/http/execute.go` and `internal/proxy/http/routes.go` so provider outcomes are converted into canonical responses consistently and logged with protocol/provider/model/thinking/tool/usage details
  - Ensure Kiro and Codex both fit the same outward response path without provider-specific leaks
  - _Requirements: 3.1, 3.4, 4.4, 5.3, 11.1, 11.3_

- [x] 8. Expand and stabilize Kiro model routing and catalog behavior
  - Update `internal/proxy/models/resolve.go` so supported Claude/Qwen/DeepSeek/Minimax/auto Kiro models resolve predictably and are exposed through `/v1/models`
  - Keep normalization and routing deterministic for version variants and alias-like forms
  - _Requirements: 8.1, 8.2, 8.3_

- [x] 9. Reconcile Codex adapter with the hardened canonical model
  - Update `internal/provider/codex/payload.go` and `internal/provider/codex/execute.go` so Codex continues to operate correctly against the stronger canonical request/response structures introduced for parity work
  - _Requirements: 5.1, 5.3_

- [x] 10. Add focused validation and live-check scaffolding once parity work is complete
  - Reintroduce only the minimal automated validation needed for canonical model conversion, Kiro payload generation, stream parsing, and protocol round-trips
  - Document a live validation checklist for text, thinking, tool calls, tool results, images, fallback host behavior, and malformed-response handling
  - _Requirements: 3.1, 3.2, 3.3, 4.1, 4.2, 4.3, 6.3, 6.4, 7.1, 7.2, 7.3, 7.4, 7.5, 9.2, 9.3, 10.1, 10.3, 11.3_
