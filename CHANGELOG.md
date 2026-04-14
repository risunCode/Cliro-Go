# Changelog

All notable changes to this project are documented in this file.

## [0.3.3] - 2026-04-14

### Fixed

- Fixed `collapseBlankLines` function in Kiro provider that was causing response text to merge without proper spacing between lines and paragraphs.
- Added comprehensive test coverage for `collapseBlankLines` to ensure proper line spacing, blank line collapsing, and paragraph preservation.

### Tests & Validation

- Added 8 test cases for `collapseBlankLines` covering single line, multiple lines, blank line handling, and real-world response scenarios.
- Validation passed for:
  - `go test ./...`
  - `npm run check`
  - `wails build`

## [0.3.2] - 2026-04-14

### Changed

- Removed dead code across `internal/provider/kiro/normalize.go` and `stream.go`: 4 unreachable normalization functions, 2 dead stream helpers, 4 unused `UsageSnapshot` fields.
- Extracted shared auth helpers `TokenExpired` and `DefaultHTTPClient` to `internal/auth/shared` to eliminate duplication between `auth/codex` and `auth/kiro`.
- Replaced per-request `make([]byte, 0, 64*1024)` scanner buffer in Codex streaming path with a `sync.Pool` to reduce GC pressure under concurrent load.
- Stack-allocated 12-byte prelude buffer in `readEventFrame` (Kiro EventStream parser) to eliminate two small heap allocations per streaming frame.
- Hoisted Kiro request payload build out of the per-attempt inner retry loop — payload is now built once per account candidate.
- Added atomic lock-free model alias snapshot (`atomic.Pointer`) on `config.Manager` so the hot proxy request path avoids a mutex + map clone per request.
- Added targeted `ThinkingSettings()` read method on `config.Manager` to avoid a full `Snapshot()` clone on every Kiro request.
- Maintained account sort invariant at write time (`UpsertAccount`, `load`) so `Accounts()` no longer re-sorts on every read.
- Normalized `account.Provider` to lowercase at write time; removed runtime `strings.ToLower` from hot pool comparison path.
- Pre-computed Kiro origin byte patterns (`originAIEditor`, `originCLI`) and machine ID as package-level vars to eliminate per-request allocations.
- Single-trimmed `req.Model` and `req.RouteFamily` at top of `completePrepared` in both providers instead of re-trimming on each use.
- Added fast-path in `extractUsage` to skip allocation when no usage keys are present in the stream payload.
- Collapsed `upstreamErrorMessage` to a single `json.Unmarshal` call probing all error keys in one pass.
- Rewrote `collapseBlankLines` to a single-pass `strings.Builder` with a fast-path for inputs containing no newlines.
- Added fast-path in `stripInternalMetadataBlocks` to skip regex when `<environment_details>` tag is absent.
- Simplified `switch level` in logging helpers across gateway and provider packages by removing redundant `strings.ToLower(strings.TrimSpace(...))` wrapping.
- Switched gateway hot path from `ModelAliases()` (lock + clone) to `ModelAliasesSnapshot()` (atomic load, no clone).
- Frontend: added `healthState ?? 'ready'` fallback in account presenter to correctly handle accounts where backend omits the field via `omitempty`.
- Frontend: removed redundant `.toLowerCase()` in `normalizeProviderID` — backend now guarantees lowercase provider values at write time.

### Tests & Validation

- Validation passed for:
  - `go test ./...`
  - `wails build`



### Changed

- Renamed project/module identity from `cliro-go` to `cliro` and updated app branding to `CLIRO`.
- Moved local app data directory from `~/.cliro-go/` to `~/.cliro` with no backward-compat retention.
- Simplified frontend backend access by removing obsolete compat layers, barrel exports, and redundant wrappers.
- Simplified close modal behavior so the first close shows the modal and the next close exits directly.

### Fixed

- Removed stale compatibility cleanup paths that were still targeting old `cliro-go` frontend/backend shims.

### Tests & Validation

- Validation passed for:
  - `go test . ./internal/...`
  - `npm run check`
  - `wails build`

## [0.3.0] - 2026-04-01

### Added

- Added shared utility package `internal/util` with `FirstNonEmpty()` helper to eliminate code duplication across 11 files.

### Changed

- Removed built-in Kiro model aliases from `internal/route/catalog_kiro.go` for cleaner model resolution.
- Moved `internal/thinking/` package into `internal/provider/thinking/` to reduce root-level package clutter.

### Fixed

- Fixed Kiro live Anthropic streaming regression where tool-use blocks were not emitted after thinking completion.
- Fixed redundant condition in `isQuotaCooldownState` that checked `AccountHealthCooldownQuota` twice.
- Fixed potential integer overflow in account pool `Next()` by applying modulo before int conversion.
- Removed unused `suffix` parameter from `CatalogModels()` function.

### Tests & Validation

- Added regression test for Kiro Anthropic live streaming tool-use emission.
- Updated route tests to reflect removal of built-in Kiro aliases.
- Validation passed for:
  - `go test . ./internal/...`
  - `npm run check`
  - `wails build`

## [0.2.1] - 2026-03-31

### Added

- Added one-click **Kilo CLI** config sync target in API Router, writing to `~/.config/kilo/opencode.json`.
- Added Kilo install detection based on `~/.config/kilo` presence so sync remains available even without PATH-resolved CLI binaries.
- Added richer OpenCode/Kilo one-click config model metadata (`name`, `limit`, `modalities`, `reasoning`) for selected local model targets.
- Added docs for cross-protocol adapter audit and implemented model-alias feature behavior:
  - `docs/audit-adapter-cross-protocol.md`
  - `docs/feature-model-aliasing.md`

### Changed

- Refactored frontend into modular boundaries under `app/`, `features/`, `shared/`, and `styles/` while keeping existing tab routes intact.
- Updated API Router one-click CLI sync UX with compact cards/modals, expanded install-path visibility, and refresh-on-expand detection.
- Updated default theme initialization to `solarized` in shared store + CSS token defaults.
- Updated footer status UX with clearer online/offline pill state and bind/base URL visibility when proxy is running.
- Updated OpenCode and Kilo one-click config output to use `provider.CLIRO` + `permission.bash = "allow"` schema format.

### Fixed
-  Claude Code CLI, Kilo CLI and OpenCode CLI now working properly*
- Fixed Cloudflared install/status detection by broadening binary discovery under `~/.cliro/bin` and refreshing status at startup.
- Fixed CLI install-path detection and stale detection behavior in one-click sync workflows.
- Fixed one-click CLI sync result ID mapping to avoid silent fallback to `claude-code` for unknown IDs.
- Removed stale frontend auto-refresh settings/card logic that no longer matched current runtime behavior.

### Tests & Validation

- Expanded `internal/clisync` tests for Kilo target support, config writing, and status/install-path behavior.
- Validation passed for:
  - `go test . ./internal/...`
  - `npm run check`

## [0.2.0] - 2026-03-30

### Added

- Added first-class Kiro provider support across account lifecycle, routing, and protocol translation.
- Added Kiro auth flows:
  - AWS Builder ID device authorization
  - Google/GitHub social login via localhost callback
- Added Anthropic-compatible Kiro endpoint support via `POST /v1/messages`.
- Added OpenAI Responses API support via `POST /v1/responses`.
- Added provider health classification, availability breakdowns, and richer provider-unavailable diagnostics.
- Added account scheduling controls with `cache_first`, `balance`, and `performance` modes.
- Added staged circuit breaker settings for repeated transient failures.
- Added Cloudflared public access management from the API Router tab, including install, quick tunnel, and named tunnel modes.
- Added force-refresh-all quota action alongside smarter batch quota refresh behavior.
- Added Kiro `-thinking` model aliases in `GET /v1/models`.

### Changed

- Migrated Kiro connect UX from import-only flow to native auth flows in the Accounts tab and modal.
- Updated frontend auth session orchestration to support Codex and Kiro session polling.
- Reworked API Router controls around proxy runtime, security, public access, endpoint testing, and routing policy.
- Changed smart quota refresh to skip accounts still in future quota cooldown, while keeping manual/per-account refresh available.
- Updated `/v1/models` so `-thinking` aliases are published only for Kiro models.
- Updated app and docs release versioning to `v0.2.0`.

### Security

- Enforced proxy API key authentication on all proxy routes when authorization mode is enabled.
- Added support for both `Authorization: Bearer <key>` and `X-API-Key` request headers.

### Fixed

- Fixed proxy start success false-positive by validating bind/listen synchronously before reporting started state.
- Prevented data loss in import policy `replace` by replacing account datasets atomically instead of deleting existing accounts first.
- Bounded `cache_first` session affinity map with TTL and max-size eviction to avoid unbounded memory growth.
- Eliminated runtime `http.Client.Timeout` mutation race by swapping the auth manager HTTP client instance on timeout updates.
- Changed `ClearLogs` from optimistic fire-and-forget to structured status reporting for memory clear, file clear, and pending retry states.
- Improved account action consistency by disabling per-account controls while requests are in-flight.
- Added explicit state resync on settings/toggle failure paths to avoid stale UI state.
- Stabilized system log row identity keys to avoid expanded/copy state shifting when ordering or filters change.
- Added backup restore payload validation and step-level progress/error reporting.
- Improved Kiro token usage extraction by handling nested/variant usage key formats for more reliable input/output token counters.
- Fixed API Router toggles and selectors that were reverting to stale values because of incomplete Wails/config wiring.
- Fixed Refresh All Quotas latency by skipping disabled, banned, and not-yet-reset exhausted accounts during smart batch refresh.
- Fixed Cloudflared process lifecycle so it follows proxy startup, shutdown, and port/network restarts.

### Tests & Tooling

- Added Go tests for provider health classification, scheduling order, circuit cooldowns, smart quota refresh skipping, and Cloudflared URL parsing.
- Added Go tests for proxy API key middleware behavior and session binding eviction paths.
- Added Go tests for `replace` import no-data-loss behavior and logger clear result status paths.
- Git ignore hardening for `z_references/` to reduce accidental large reference commits.

## [0.1.0] - 2026-03-28

Initial release of **CLIro-Go**.

### Added

- Desktop control plane built with Wails (Go backend + Svelte frontend).
- OpenAI-compatible local proxy server with endpoints:
  - `POST /v1/chat/completions`
  - `POST /v1/completions`
  - `GET /v1/models`
  - `GET /health`
  - `GET /v1/stats`
- Multi-account runtime pool with round-robin selection and availability filtering.
- OAuth callback flow for Codex account onboarding.
- Token lifecycle features:
  - access/refresh token storage
  - proactive refresh with expiry skew handling
  - per-account refresh operation
- Quota lifecycle features:
  - quota fetch with endpoint fallback strategy
  - quota parsing + bucket aggregation
  - automatic cooldown and quota-aware disable handling
  - blocked/deactivated account detection and disable propagation
- Account management operations:
  - connect, refresh, refresh quota, refresh all quotas
  - enable/disable
  - delete single account
  - import account JSON payloads
  - export account JSON payloads
  - bulk enable/disable
  - bulk export selected
  - bulk delete selected (with confirmation modal)
- Sync integrations for local CLI auth files:
  - Sync Codex account to **Kilo CLI** (`~/.local/share/kilo/auth.json`)
  - Sync Codex account to **Opencode** (`~/.local/share/opencode/auth.json`)
  - Sync Codex account to **Codex CLI** (`~/.codex/auth.json`)
  - Codex CLI backup creation before overwrite (`.bak.cliro`)
- UI/UX modules:
  - compact dashboard with KPI grid + host-aware greeting hero
  - accounts grid/list views with provider tabs and search
  - account detail modal with copyable credentials
  - account sync modal with multi-target selection and sync result view
  - connect modal for OAuth link handoff flow
  - redesigned system logs tab with filters, search, copy-visible, and scrollable table viewport
  - global toast notifications and runtime status feedback
- Theme support and adaptive UI styling for dark/light/solarized modes.

### Security

- Restricted file permissions for persisted auth/token files where applicable (`0600`).
- Local data isolation in `~/.cliro/` (`config.json`, `accounts.json`, `stats.json`).

### Developer Experience

- Wails binding-based frontend service layer (`frontend/src/services/wails-api.ts`).
- Centralized tab/page architecture under `frontend/src/tabs`.
- Shared component system under `frontend/src/components`.
- Build and validation workflow:
  - `npm run check`
  - `npm run build`
  - `go test ./...` (targeted package checks)
  - `wails build`
