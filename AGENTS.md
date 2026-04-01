# AGENTS.md - CLIro-Go Agent Guide

This guide gives AI coding agents the current project scope, architecture, and workflows for CLIro-Go.

## Project Overview

**CLIro-Go** is a Wails desktop control plane for a local OpenAI-compatible + Anthropic-compatible proxy.

- **Backend**: Go 1.23+
- **Frontend**: Svelte + TypeScript + Vite
- **Desktop shell**: Wails v2.11+
- **Current release**: **v0.3.0**
- **Main entry point**: `main.go`
- **Wails app bridge**: `app.go`

CLIro-Go currently focuses on:

- Multi-account routing across **Codex** and **Kiro** providers.
- OAuth/device/social auth flows and token lifecycle handling.
- Quota-aware account health + cooldown + scheduling.
- API Router controls (proxy runtime, model aliasing, Cloudflared, endpoint tester, one-click CLI config sync).
- Local desktop-first UX with persisted JSON state in `~/.cliro-go/`.

## Current Scope Snapshot

- `GET /v1/models` exposes canonical model IDs only (no published `-thinking` aliases).
- Requests with `-thinking` suffix are still normalized during model resolution for compatibility.
- Kiro runtime endpoints remain fixed to `q.us-east-1.amazonaws.com` and `codewhisperer.us-east-1.amazonaws.com`.
- Authorization mode (when enabled) requires the configured proxy API key on all proxy routes.
- Smart quota refresh skips accounts that are disabled/banned/not-yet-reset exhausted; force refresh bypasses smart skip.
- One-click CLI config sync targets in API Router:
  - `claude-code`
  - `opencode-cli`
  - `kilo-cli`
  - `codex-ai`

## Data Directory

CLIro-Go persists runtime data under `~/.cliro-go/`:

- `config.json` - proxy, scheduling, cloudflared, model aliases, etc.
- `accounts.json` - account and token/quota state.
- `stats.json` - proxy usage counters.
- `app.log` - persistent app log.
- `bin/cloudflared(.exe)` - local Cloudflared binary.

## Essential Commands

### Development

```bash
cd frontend && npm install && cd ..
wails dev
```

### Validation

```bash
cd frontend && npm run check && cd ..
go test . ./internal/...
```

### Production Build

```bash
wails build
```

Output (Windows): `build/bin/Cliro-Go.exe`

## Architecture

### Wails Bridge

- `app.go` exposes methods used by frontend.
- `main.go` binds `App` into Wails runtime.
- Generated bindings live in `frontend/wailsjs/go/main/`.
- Frontend Wails wrapper is in `frontend/src/shared/api/wails/client.ts`.

### Frontend Module Boundaries

Current frontend layout:

```text
frontend/src/
  App.svelte           # app bootstrap and shell wiring
  app/                 # app shell orchestration, overlays, top-level services, shared app contracts
    api/
    lib/
    modals/
    providers/
    services/
      app-controller.ts
      logs-subscription.ts
      startup-warnings.ts
    shell/
    types.ts
  features/            # domain features and feature-local UI/helpers
    accounts/
      components/
        connect/
        list/
        modals/
    router/
      components/
        cli-sync/
        cloudflared/
        endpoint-tester/
        model-alias/
        proxy/
        scheduling/
    logs/
      components/
      lib/
    usage/
      components/
      lib/
  shared/              # cross-feature utilities and stores
    api/wails/
    lib/
    stores/
  components/common/   # reusable primitives
  tabs/                # tab wrappers that compose app/features
  styles/              # base/theme/component stylesheets
```

Key frontend files:

- Root shell and orchestration:
  - `frontend/src/App.svelte`
  - `frontend/src/app/services/app-controller.ts`
  - `frontend/src/app/providers/AppOverlayStack.svelte`
  - `frontend/src/app/shell/AppFrame.svelte`
- Accounts feature:
  - `frontend/src/features/accounts/components/AccountsWorkspace.svelte`
- Router feature:
  - `frontend/src/features/router/components/proxy/ProxyControlsPanel.svelte`
  - `frontend/src/features/router/components/proxy/ProxyRuntimeCard.svelte`
  - `frontend/src/features/router/components/proxy/ProxySecurityCard.svelte`
  - `frontend/src/features/router/components/scheduling/SchedulingPanel.svelte`
  - `frontend/src/features/router/components/cloudflared/CloudflaredPanel.svelte`
  - `frontend/src/features/router/components/endpoint-tester/EndpointTesterPanel.svelte`
  - `frontend/src/features/router/components/model-alias/ModelAliasPanel.svelte`
  - `frontend/src/features/router/components/cli-sync/CliSyncPanel.svelte`
- Logs and usage feature workspaces:
  - `frontend/src/features/logs/components/SystemLogsWorkspace.svelte`
  - `frontend/src/features/logs/lib/logs-view.ts`
  - `frontend/src/features/usage/components/UsageWorkspace.svelte`
  - `frontend/src/features/usage/lib/request-log.ts`

### Backend Core Modules

- **Gateway**: `internal/gateway/`
  - OpenAI + Anthropic endpoint handlers
  - Provider routing + retry + availability diagnostics
- **Routing & Model Resolution**: `internal/route/`
  - Provider resolution, model catalogs, alias-aware selection
- **Auth**: `internal/auth/`
  - Codex OAuth flow + Kiro auth flows
- **Provider Services**: `internal/provider/`
  - Codex and Kiro request execution
  - Quota service orchestration (`internal/provider/quota/service.go`)
  - Thinking parsing/arbitration under `internal/provider/thinking/`
- **Config Storage**: `internal/config/`
  - Snapshot + atomic updates over JSON files
- **Structured Logging**: `internal/logger/`
  - In-memory + persistent JSONL log storage
  - Structured entries with `level`, `scope`, `event`, `requestId`, `message`, and `fields`
- **Sync Services**: `internal/sync/`
  - `internal/sync/cliconfig/` for one-click CLI config patch/read/write
  - `internal/sync/authtoken/` for account auth token sync into supported CLIs
- **Cloudflared**: `internal/cloudflared/manager.go`
  - Install, start/stop tunnel, parse URL/status
- **Contracts & Protocol Codecs**:
  - `internal/contract/` holds protocol-neutral request/response types and validation rules
  - `internal/protocol/openai/` and `internal/protocol/anthropic/` hold protocol types plus decode/encode pipelines

## One-Click CLI Sync Details

CLI sync lives under API Router and is implemented in `internal/sync/cliconfig/`.

Account auth-sync is separate and implemented in `internal/sync/authtoken/`.

Targets and config files:

- `claude-code` -> `~/.claude/settings.json` + `~/.claude.json`
- `opencode-cli` -> `~/.config/opencode/opencode.json`
- `kilo-cli` -> `~/.config/kilo/opencode.json`
- `codex-ai` -> `~/.codex/config.toml` + `~/.codex/auth.json`

For OpenCode/Kilo JSON config generation:

- `$schema` is set to `https://opencode.ai/config.json`
- provider key is `CLIRO`
- `permission.bash = "allow"`
- selected model is injected from local catalog (`GetLocalModelCatalog()`)

Kilo install detection:

- Treated as installed when `~/.config/kilo` exists (even if binary is not in PATH).

## Real-Time Events

- Backend emits log events via Wails runtime events.
- Frontend subscribes via `frontend/src/app/services/logs-subscription.ts` and renders in system logs UI.
- Structured log entries now include `level`, `scope`, `event`, `requestId`, `message`, and optional `fields`.
- The system logs table is optimized around `Level / Source / Account / Detail / Time`; update `frontend/src/features/logs/lib/logs-view.ts` when `logger.Entry` shape changes.

## Key App Methods (Wails -> Frontend)

Important methods exposed in `app.go`:

### State & Logs

- `GetState()`
- `GetAccounts()`
- `GetProxyStatus()`
- `RefreshCloudflaredStatus()`
- `GetLogs(limit int)`
- `ClearLogs()`
- `GetHostName()`

### Proxy & Router Controls

- `StartProxy()` / `StopProxy()`
- `SetProxyPort(port int)`
- `SetAllowLAN(enabled bool)`
- `SetAutoStartProxy(enabled bool)`
- `SetProxyAPIKey(apiKey string)`
- `RegenerateProxyAPIKey()`
- `SetAuthorizationMode(enabled bool)`
- `SetSchedulingMode(mode string)`
- `GetModelAliases()` / `SetModelAliases(aliases map[string]string)`

### Cloudflared

- `InstallCloudflared()`
- `StartCloudflared()`
- `StopCloudflared()`
- `SetCloudflaredConfig(mode, token string, useHTTP2 bool)`

### Account Auth & Quota

- `StartCodexAuth()` / `GetCodexAuthSession()` / `CancelCodexAuth()` / `SubmitCodexAuthCode()`
- `StartKiroAuth()` / `StartKiroSocialAuth()` / `GetKiroAuthSession()` / `CancelKiroAuth()` / `SubmitKiroAuthCode()`
- `RefreshAccount(accountID string)`
- `RefreshAccountWithQuota(accountID string)`
- `RefreshQuota(accountID string)`
- `RefreshAllQuotas()`
- `ForceRefreshAllQuotas()`
- `ToggleAccount(accountID string, enabled bool)`
- `DeleteAccount(accountID string)`
- `ImportAccounts(accounts []config.Account)`
- `ClearCooldown(accountID string)`

### CLI Sync

- `GetLocalModelCatalog()`
- `GetCLISyncStatuses()`
- `SyncCLIConfig(appID, model string)`
- `GetCLISyncFileContent(appID, path string)`
- `SaveCLISyncFileContent(appID, path, content string)`

### Account Auth Sync

- `SyncCodexAccountToKiloAuth(accountID string)`
- `SyncCodexAccountToCodexCLI(accountID string)`
- `SyncCodexAccountToOpencodeAuth(accountID string)`

### Utilities

- `OpenExternalURL(rawURL string)`
- `OpenDataDir()`

## Proxy Endpoints

Default base URL: `http://localhost:8095`

- `POST /v1/responses`
- `POST /v1/chat/completions`
- `POST /v1/completions`
- `POST /v1/messages`
- `POST /v1/messages/count_tokens`
- `GET /v1/models`
- `GET /v1/stats`
- `GET /health`

## Coding Conventions

### Go

- Keep changes idiomatic and small per package boundary.
- Wrap errors with context (`fmt.Errorf("context: %w", err)`).
- Prefer immutable snapshot reads + atomic update closures for config/account mutations.
- Avoid panics in runtime paths.

### Frontend (Svelte + TS)

- Keep data access inside feature/shared API modules rather than ad-hoc Wails calls in UI.
- Reuse `components/common/` primitives and shared stores.
- Keep tab files thin; place business logic in `features/*` and `app/*` modules.
- Keep router sub-surfaces under feature-owned folders (`proxy/`, `cloudflared/`, `cli-sync/`, `endpoint-tester/`, `model-alias/`, `scheduling/`).
- When system log structure changes, update both `frontend/src/features/logs/lib/logs-view.ts` and `frontend/src/features/logs/components/SystemLogsWorkspace.svelte` together.
- Use `npm run check` before finalizing changes.

## Common Tasks

### Add a New Wails Method

1. Add exported method to `app.go`.
2. Run `wails dev` or `wails build` to regenerate JS/TS bindings.
3. Expose via `frontend/src/shared/api/wails/client.ts`.
4. Wire feature-level adapter/API module.

### Add a New Proxy Capability

1. Update protocol decode/encode logic in `internal/protocol/openai/` and/or `internal/protocol/anthropic/` if protocol mapping is needed.
2. Update gateway handlers in `internal/gateway/`.
3. Update route validation and model resolution in `internal/route/`.
4. Add tests for both OpenAI and Anthropic request paths.

### Add/Change CLI Sync Target

1. Add `App` constant + `appDefinition` in `internal/sync/cliconfig/service.go`.
2. Implement read/patch logic for status + sync.
3. Extend frontend type union (`CliSyncAppID`) and router card metadata.
4. Add tests in `internal/sync/cliconfig/service_test.go`.

## Testing Checklist

- `go test . ./internal/...` passes.
- `cd frontend && npm run check` passes.
- Manually verify API Router flows if router state/config behavior changed:
  - proxy start/stop
  - model alias save/apply
  - Cloudflared install/start/stop
  - one-click CLI sync statuses and write paths
- Manually verify System Logs if logger schema/UI changes:
  - level/source/account/detail/time columns render correctly
  - request/account-related fields are summarized into the expected columns
  - copy/export still includes the structured entry content

## Security Notes

- Tokens are local-file persisted; never print tokens in logs.
- When enabling LAN binding (`allowLan=true`), strongly pair with authorization mode.
- API key headers accepted in auth mode:
  - `Authorization: Bearer <key>`
  - `X-API-Key: <key>`

## Troubleshooting Hints

- **Proxy won’t start**: port conflict; change port or free process.
- **No available accounts**: check enabled flag, cooldown, quota status, and auth validity.
- **Cloudflared URL missing**: ensure proxy is running and Cloudflared status refreshed.
- **CLI sync says unsupported target/model**: verify target ID union and local model catalog membership.

## References

- Wails docs: `https://wails.io/docs/introduction`
- Svelte docs: `https://svelte.dev/docs`
- Project overview: `README.md`
- Release notes: `CHANGELOG.md`
- Frontend package notes: `frontend/README.md`
