# Changelog

All notable changes to this project are documented in this file.

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
