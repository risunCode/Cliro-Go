# Implementation Plan

- [x] 1. Add startup warning domain model and manager exposure
  - Add a structured startup warning type in config domain and wire warning storage in manager state.
  - Expose immutable warning snapshot getter for app state serialization.
  - _Requirements: 3.1, 3.3, 3.4_

- [x] 2. Implement shared corruption recovery utility in storage layer
  - Add helper(s) that back up corrupted bytes to timestamped files and rewrite canonical fallback JSON.
  - Ensure helper supports settings/accounts/stats recovery scenarios.
  - _Requirements: 1.4, 1.5, 3.1, 3.2_

- [x] 3. Harden settings load path against malformed JSON
  - Update `LoadSettings` to recover from decode failures using defaults without panic propagation.
  - Emit structured warning metadata for recovered settings files.
  - _Requirements: 1.1, 1.4, 1.5, 3.1_

- [x] 4. Harden accounts load path with strict root format and corruption recovery
  - Keep canonical parser strict to JSON array roots only.
  - Treat non-array roots (including legacy envelope) as corruption and recover to empty account list.
  - _Requirements: 1.2, 2.1, 2.2, 2.3, 4.4_

- [x] 5. Harden stats load path against malformed JSON
  - Recover malformed stats file to zero-value stats and persist canonical JSON.
  - Emit startup warning metadata for recovered stats files.
  - _Requirements: 1.3, 1.4, 1.5, 3.1_

- [x] 6. Update app startup/state contract for resilience notices
  - Ensure startup flow logs recovery warnings and includes modal-ready warning data in `GetState` response payload.
  - Keep startup alive for recoverable corruption cases without panic.
  - _Requirements: 1.1, 1.2, 1.3, 3.3, 3.4, 3.5_

- [x] 7. Implement second-instance launch handling in backend
  - Add `OnSecondInstanceLaunch` callback wiring in `main.go` and handler in `app.go`.
  - Restore/show/foreground existing window and emit `app:second-instance` event payload.
  - _Requirements: 5.1, 5.2, 5.4_

- [x] 8. Implement banned account classification and persistence
  - Detect token-invalidated/auth-revoked provider responses and normalize them into banned account state.
  - Persist banned status/reason and ensure pool/account selection excludes banned accounts.
  - _Requirements: 6.1, 6.2, 6.3, 6.5_

- [x] 9. Add structured request/account usage logging across gateway flow
  - Emit request lifecycle logs for account/provider selection, retries, success, and failure.
  - Include safe request context and token usage fields while redacting secrets.
  - Keep emitted usage values aligned with persisted account/global stats updates.
  - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_

- [x] 10. Correct API router path behavior, security/header handling, and `/v1/responses` support
  - Keep displayed default local base URL as `127.0.0.1:<port>` without `/v1` suffix.
  - Add normalization logic that prevents doubled `/v1/v1` paths when users override base paths.
  - Implement `v1/responses` route support and tighten security/header validation behavior.
  - _Requirements: 8.1, 8.2, 8.3, 8.4, 8.5_

- [x] 11. Add frontend notice handling for startup warnings, second launch, banned status, and router defaults
  - Add a configuration error prompt/modal that appears after initial state load when startup warnings exist.
  - Aggregate multiple startup warnings into a single modal list with file + backup details.
  - Subscribe to `app:second-instance` event and show in-app reopen notice when app is launched again.
  - Surface banned account status/reason clearly in account UI.
  - Update API router UI/help text to use `127.0.0.1:<port>` as the default base URL without auto-appending `/v1`.
  - _Requirements: 3.3, 3.4, 3.5, 3.6, 5.3, 6.4, 8.1, 8.2_

- [x] 12. Replace brittle config tests with deterministic table-driven coverage
  - Add malformed JSON test matrix for settings/accounts/stats and strict non-array account root behavior.
  - Assert backup creation, canonical rewrite, fallback output, and non-panic startup path.
  - _Requirements: 4.1, 4.2, 4.3, 4.4_

- [x] 13. Add focused tests for second-instance, warning serialization, banned detection, logging, and router correctness
  - Add unit tests for second-instance notice payload construction and state warning propagation.
  - Add tests that verify warning aggregation/modal payload completeness for multiple recovered files.
  - Add tests for token-invalidated banned classification and pool exclusion behavior.
  - Add tests for structured request logging, `/v1/responses`, base-path normalization, and security/header validation.
  - Keep tests deterministic and independent of live Wails window context.
  - _Requirements: 5.3, 5.4, 4.1, 3.4, 3.5, 6.1, 6.3, 6.6, 7.1, 7.2, 7.3, 8.3, 8.6_

- [x] 14. Run validation gates and fix regressions
  - Run `go test ./internal/config`, `go test ./internal/...`, and `go test .`.
  - Run final `wails build` after all code and test updates are complete.
  - _Requirements: 4.1, 4.2, 5.1, 6.6, 8.6_
