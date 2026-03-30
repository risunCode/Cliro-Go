# Implementation Plan

- [x] 1. Consolidate gateway files into normal handler grouping
  - Merge `routes.go`, `context.go`, and `errors.go` concerns into `server.go` while preserving exported behavior.
  - _Requirements: 1.1, 2.2, 3.1_

- [x] 2. Merge OpenAI gateway handlers into one cohesive file
  - [x] 2.1 Move logic from `handle_openai_chat.go` and `handle_openai_completions.go` into `openai_handlers.go`
    - Keep handler entrypoints and response behavior unchanged.
    - _Requirements: 1.1, 2.2, 3.1, 3.2_
  - [x] 2.2 Remove old split files and update references
    - Update any internal calls that referenced old file-local helpers.
    - _Requirements: 1.2, 3.2, 4.3_

- [x] 3. Merge Anthropic gateway handlers into one cohesive file
  - [x] 3.1 Move logic from `handle_anthropic_messages.go` and `handle_anthropic_count_tokens.go` into `anthropic_handlers.go`
    - Preserve request/response compatibility for `/v1/messages` and `/v1/messages/count_tokens`.
    - _Requirements: 1.1, 2.2, 3.1, 3.2_
  - [x] 3.2 Remove old split files and update references
    - Ensure no dead references remain.
    - _Requirements: 1.2, 3.2, 4.3_

- [x] 4. Consolidate protocol files by concern
  - [x] 4.1 Merge OpenAI request/response/stream definitions into `requests.go`, `responses.go`, and `stream.go`
    - Keep JSON tags and decoding behavior unchanged.
    - _Requirements: 1.1, 2.2, 3.1_
  - [x] 4.2 Merge Anthropic request/response/stream definitions into `requests.go`, `responses.go`, and `stream.go`
    - Keep decode/encode contract stable.
    - _Requirements: 1.1, 2.2, 3.1_

- [x] 5. Consolidate adapter files and simplify naming
  - [x] 5.1 Merge IR models into `adapter/ir/ir.go`
    - Move `types.go`, `request.go`, `response.go`, `event.go` into one cohesive IR file.
    - _Requirements: 1.1, 2.2, 3.2_
  - [x] 5.2 Merge decode files into `decode/openai.go`, `decode/anthropic.go`, and `decode/common.go`
    - Keep all current conversion paths intact.
    - _Requirements: 1.2, 2.2, 3.1, 3.2_
  - [x] 5.3 Merge encode files into `encode/openai.go`, `encode/anthropic.go`, `encode/openai_stream.go`, and `encode/anthropic_stream.go`
    - Maintain stream event semantics and response fields.
    - _Requirements: 1.2, 2.2, 3.1, 3.2_
  - [x] 5.4 Merge adapter rules into `rules/rules.go`
    - Preserve validation and capability checks.
    - _Requirements: 1.2, 2.2, 3.1_

- [x] 6. Normalize route package naming
  - Merge route definitions into `models.go` and `endpoints.go`.
  - Keep resolver behavior and tests passing.
  - _Requirements: 1.1, 2.3, 3.1_

- [x] 7. Consolidate provider runtime helpers
  - [x] 7.1 Merge codex runtime files (`map_ir`, `execute`, `stream_parser`) into `provider/codex/runtime.go`
    - Keep provider execution behavior and error mapping stable.
    - _Requirements: 1.2, 2.2, 3.1, 3.2_
  - [x] 7.2 Merge kiro runtime files (`map_ir`, `execute`, `event_parser`, `tool_map`, `thinking`) into `provider/kiro/runtime.go`
    - Keep tool parsing and thinking extraction behavior unchanged.
    - _Requirements: 1.2, 2.2, 3.1, 3.2_

- [x] 8. Consolidate account and platform utility files
  - [x] 8.1 Merge account helper files into `account/account.go` and keep `pool.go`
    - Preserve provider validation and account access helpers.
    - _Requirements: 1.1, 2.2, 3.1_
  - [x] 8.2 Merge platform helper files into `platform/platform.go`, `platform/http.go`, and `platform/clock.go`
    - Keep bind host/url/header utility behavior stable.
    - _Requirements: 1.1, 2.2, 3.1_

- [x] 9. Update imports, aliases, and remove obsolete files
  - Apply consistent import aliases (`provider`, `codexprovider`, `kiroprovider`) and remove stale references.
  - Delete replaced source files after successful symbol migration.
  - _Requirements: 1.2, 3.2, 4.1, 4.3, 5.1_

- [x] 10. Execute phased validation gates
  - Run `go test ./internal/...` and `go test .` after each major phase.
  - Run `wails build` after final consolidation.
  - Stop and fix failures before proceeding to the next phase.
  - _Requirements: 3.3, 4.2, 4.3, 5.1_
