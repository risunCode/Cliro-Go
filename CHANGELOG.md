# Changelog

All notable changes to this project are documented in this file.

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
  - Codex CLI backup creation before overwrite (`.bak.cliro-go`)
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
- Local data isolation in `~/.cliro-go/` (`config.json`, `accounts.json`, `stats.json`).

### Developer Experience

- Wails binding-based frontend service layer (`frontend/src/services/wails-api.ts`).
- Centralized tab/page architecture under `frontend/src/tabs`.
- Shared component system under `frontend/src/components`.
- Build and validation workflow:
  - `npm run check`
  - `npm run build`
  - `go test ./...` (targeted package checks)
  - `wails build`
