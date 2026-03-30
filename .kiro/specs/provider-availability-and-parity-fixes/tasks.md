# Implementation Plan

- [x] 1. Audit and align canonical translation boundaries with the reference architecture
  - Map current request parsing/conversion code to the reference roles of `translator_new/to_ir/*`, `translator_new/from_ir/*`, and `translator_new/sdk_adapter.go`.
  - Add or refactor minimal helpers so protocol normalization and metadata preservation happen at translation boundaries instead of ad-hoc late-stage fixes.
  - _Requirements: 2.3, 4.1, 4.2, 4.3, 5.1, 5.2, 6.1, 6.4_

- [x] 2. Add explicit account health state and availability snapshot models
  - Extend account/config state with health classification fields and add pool-level availability snapshot reporting.
  - Keep backward-compatible defaults so existing accounts load safely into `ready` state unless other fields indicate otherwise.
  - _Requirements: 1.5, 3.5, 6.5_

- [x] 3. Implement shared provider failure classification helpers
  - Add a reusable failure-classification helper for auth-refreshable, quota, retryable transport, durable disabled, banned, and request-shape failures.
  - Keep request-shape/parity errors separate from account-health mutations.
  - _Requirements: 1.2, 3.1, 3.2, 3.3_

- [x] 4. Update pool selection and diagnostics to avoid premature provider exhaustion
  - Make pool selection skip accounts according to the new health-state model and expose availability counters for logs/debugging.
  - Add richer reason summaries for pool-empty conditions instead of generic `no available ... accounts` only.
  - _Requirements: 1.1, 1.3, 1.4, 3.5, 6.3, 6.5_

- [x] 5. Adapt strict Codex request conversion and availability behavior from the reference
  - Align Codex request construction with the stricter reference conversion rules from `translator_new/from_ir/codex.go`.
  - Apply the new failure classification to Codex request, refresh, quota, and retry paths so transient/quota failures do not disable healthy Codex accounts prematurely.
  - _Requirements: 1.1, 1.2, 3.1, 3.3, 3.4, 6.1_

- [x] 6. Align Kiro request construction with reference endpoint/header parity
  - Split Kiro primary and fallback endpoint request builders so Q and CodeWhisperer use the correct header sets.
  - Remove or add `X-Amz-Target` only in the cases proven by the reference behavior.
  - _Requirements: 2.1, 2.2, 2.5, 6.1_

- [x] 7. Implement Kiro `profileArn`, continuation, and metadata parity handling
  - Add effective `profileArn` resolution rules and pass continuation/conversation metadata through the Kiro runtime path in the same translation/conversion stage used by the reference.
  - Preserve safe defaults for one-shot requests when metadata is absent.
  - _Requirements: 2.3, 5.1, 5.2, 5.3, 6.1_

- [x] 8. Harden Kiro auth retry, fallback, and availability classification flow
  - Add pre-request freshness checks, inline token-related retry behavior, and correct separation between transient cooldown and durable unavailable states.
  - Add primary/fallback endpoint retry behavior that mirrors the reference executor and prevent request-shape/header/profile mismatches from poisoning otherwise valid Kiro accounts.
  - _Requirements: 1.1, 1.2, 2.4, 2.5, 3.2, 3.3, 3.4, 6.2_

- [x] 9. Fix OpenAI Responses to Anthropic content normalization at the translation layer
  - Add explicit normalization for content part types so Anthropic-facing payloads never emit invalid `input_text` values in unsupported positions.
  - Preserve assistant text, refusal, and tool semantics during normalization, and keep the fix near request/response translation rather than executor fallback code.
  - _Requirements: 4.1, 4.2, 4.3, 4.4, 6.4_

- [x] 10. Improve gateway/provider diagnostics for pool and parity failures
  - Add structured logs that include provider, account, failure class, cooldown/disable outcome, and availability snapshot after state transitions.
  - Include actionable provider-unavailable summaries in request failure paths.
  - _Requirements: 1.4, 3.5, 6.5_

- [x] 11. Add regression tests for provider availability and parity behavior
  - Add tests covering Codex/Kiro pool-drain prevention, Kiro primary/fallback headers, `profileArn` logic, metadata preservation, auth retry behavior, and provider-unavailable thresholds.
  - Add regression tests for the Anthropic `input_text` compatibility failure and strict Codex request-shaping parity.
  - _Requirements: 6.1, 6.2, 6.3, 6.4, 6.5_

- [x] 12. Run validation gates and fix regressions
  - Run `go test ./internal/...`, `go test .`, `npm run check`, and `wails build` after implementation completes.
  - _Requirements: 6.1, 6.2, 6.3, 6.4_
