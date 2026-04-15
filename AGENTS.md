# AGENTS.md - CLIRO Agent Guide

This guide gives AI coding agents the current project scope, architecture, and workflows for CLIRO.

## Project Overview

**CLIRO** is a Wails desktop control plane for a local OpenAI-compatible + Anthropic-compatible proxy.

- **Backend**: Go 1.23+
- **Frontend**: Svelte + TypeScript + Vite
- **Desktop shell**: Wails v2.11+
- **Current release**: **v0.4.0**
- **Main entry point**: `main.go`
- **Wails app bridge**: `app.go`

CLIRO currently focuses on:

- Multi-account routing across **Codex** and **Kiro** providers.
- OAuth/device/social auth flows and token lifecycle handling.
- Quota-aware account health + cooldown + scheduling.
- API Router controls (proxy runtime, model aliasing, Cloudflared, endpoint tester, one-click CLI config sync).
- Local desktop-first UX with persisted JSON state in `~/.cliro/`.

## Current Scope Snapshot

- `GET /v1/models` exposes canonical model IDs only (no published `-thinking` aliases).
- Requests with `-thinking` suffix are still normalized during model resolution for compatibility.
- **Model effort suffixes** enable automatic reasoning parameter injection:
  - `-low` / `-minimal` → 4096 budget tokens (OpenAI: `effort: "low"`)
  - `-medium` → 10000 budget tokens (OpenAI: `effort: "medium"`)
  - `-high` → 16384 budget tokens (OpenAI: `effort: "high"`)
  - `-xhigh` → 32768 budget tokens (OpenAI: `effort: "xhigh"`)
- Example: `gpt-5.4-high` auto-injects `reasoning: {effort: "high"}` for Codex, `thinking: {budget_tokens: 16384}` for Anthropic
- **Cross-protocol reasoning/thinking conversion**:
  - OpenAI `reasoning.effort` ↔ Anthropic `thinking.budget_tokens` bidirectional mapping
  - Automatic parameter filtering to prevent "Unknown parameter" errors
  - Response format: `reasoning_content` field in OpenAI endpoints, thinking blocks in Anthropic endpoints
- Kiro runtime uses `q.us-east-1.amazonaws.com/generateAssistantResponse` first and falls back to `codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse` on runtime failure.
- Authorization mode (when enabled) requires the configured proxy API key on all proxy routes.
- Smart quota refresh skips accounts that are disabled/banned/not-yet-reset exhausted; force refresh bypasses smart skip.
- One-click CLI config sync targets in API Router:
  - `claude-code`
  - `opencode-cli`
  - `kilo-cli`
  - `codex-ai`

## User Agent Spoofing

CLIRO mimics official client user agents to ensure compatibility with upstream provider APIs.

### Codex Provider (OpenAI)

**Implementation**: hardcoded in `internal/provider/codex/service.go`

**Current Version**: `codex-tui/0.118.0`

**Format**: `codex-tui/{version} ({os}; {arch}) {app} (codex-tui; {version})`

**Example**:
- `codex-tui/0.118.0 (Mac OS 26.3.1; arm64) iTerm.app/3.6.9 (codex-tui; 0.118.0)`

**Headers Sent on Inference Requests**:
- `User-Agent`: `codex-tui` format above
- `Originator`: `codex-tui`
- `Version`: `0.118.0`
- `Origin`: `https://chatgpt.com`
- `Referer`: `https://chatgpt.com/`
- `Session_id`: Random UUID per request

### Kiro Provider (AWS)

**Implementation**: `internal/auth/kiro/types.go`, `internal/provider/kiro/service.go`

**Current Version**: `KiroIDE-0.11.107`

**AWS SDK Version**: `aws-sdk-js/1.2.15`

#### Social Auth Mode (Default)

**Headers Sent**:
- `User-Agent`: `aws-sdk-js/1.2.15 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.2.15 m/E KiroIDE-0.11.107-{machineID}`
- `x-amz-user-agent`: `aws-sdk-js/1.2.15 KiroIDE 0.11.107`
- `x-amzn-kiro-agent-mode`: `spec`

**Social Auth Specific**:
- `User-Agent`: `KiroIDE-0.11.107-{uuid}` (for auth endpoints)

#### IDC Auth Mode (IAM Identity Center)

**Headers Sent**:
- `User-Agent`: `aws-sdk-rust/1.3.9 os/macos lang/rust/1.87.0`
- `x-amz-user-agent`: `aws-sdk-rust/1.3.9 ua/2.1 api/ssooidc/1.88.0 os/macos lang/rust/1.87.0 m/E app/AmazonQ-For-CLI`
- `x-amzn-kiro-agent-mode`: `vibe`

#### Device/OIDC Auth

**Headers Sent**:
- `User-Agent`: `aws-sdk-js/1.2.15 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/sso-oidc#1.2.15 m/E KiroIDE`
- `x-amz-user-agent`: `aws-sdk-js/1.2.15 KiroIDE`

### Version Update Guidelines

When updating user agent versions:

1. **Codex TUI**: Update Codex version/user-agent constants in `internal/provider/codex/service.go`
2. **Kiro IDE**: Update version strings in:
   - `internal/provider/kiro/service.go` (runtime constants)
   - `internal/auth/kiro/types.go` (auth constants)
   - `internal/auth/kiro/device.go` (OIDC headers)
3. **Test**: Verify auth flows and API requests still work after version bump
4. **Reference**: Check latest versions at:
   - Codex CLI: `https://www.npmjs.com/package/@openai/codex`
   - Kiro CLI: `https://kiro.dev/changelog`

### Machine ID Generation

Kiro requests include a random machine ID suffix to simulate unique client instances:
- Generated via `uuid.NewString()` with hyphens removed
- Appended to user agent string (e.g., `KiroIDE-0.11.107-a1b2c3d4e5f6...`)
- Regenerated per request for social auth mode

## Data Directory

CLIRO persists runtime data under `~/.cliro/`:

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

Output (Windows): `build/bin/CLIRO.exe`

## Architecture

### Wails Bridge

- `app.go` exposes methods used by frontend.
- `main.go` binds `App` into Wails runtime.
- Generated bindings live in `frontend/wailsjs/go/main/`.
- Frontend calls into backend exclusively through `frontend/src/backend/` layer.

### Frontend Module Boundaries

Current frontend layout:

```text
frontend/src/
  App.svelte                        # app entry — mounts AppFrame
  main.ts                           # Svelte bootstrap

  app/                              # app-level orchestration
    bootstrap/
      app-bootstrap.ts              # initializeAppBootstrap() — startup sequence
      app-events.ts                 # bindAppRuntimeEvents / bindAppActivityEvents
    modals/
      AppCloseModal.svelte          # close-to-tray / confirm-quit modal
      ConfigurationRecoveryModal.svelte
      UpdateRequiredModal.svelte
    overlays/
      AppOverlayHost.svelte         # stacks all app-level modals
    routes/
      app-routes.ts                 # APP_ROUTES registry — static + lazy per tab
      RouteOutlet.svelte            # renders the active route component
    services/
      app-controller.ts             # central controller — AppShellState + all action namespaces
      logs-subscription.ts          # Wails event → ring-log bridge
      startup-warnings.ts           # maps startup warnings to display entries
    shell/
      AppFrame.svelte               # top-level shell: header + tabs + route outlet + footer
      AppHeader.svelte
      AppFooter.svelte
    types/
      index.ts                      # AppState, LogEntry, UpdateInfo
    utils/
      backup.ts
      tabs.ts                       # APP_TABS, AppTabId

  backend/                          # all backend access — ONLY place that imports wailsjs
    client/
      browser.ts                    # browser-mode stubs
      runtime-events.ts             # typed Wails event subscription helpers
      wails-client.ts               # raw Wails JS bindings wrapper
    gateways/
      accounts-gateway.ts           # account CRUD calls
      auth-gateway.ts               # codex + kiro auth calls
      logs-gateway.ts               # getLogs / clearLogs
      router-gateway.ts             # proxy / cloudflared / alias / cli-sync calls
      system-gateway.ts             # getState / openDataDir / confirmQuit / hideToTray etc.
    models/
      wails.ts                      # raw Wails DTO types (WailsAccount, WailsAppState, …)
      system.ts

  features/                         # domain features
    accounts/
      api/
        accounts-api.ts
        auth-api.ts
      components/
        AccountsScreen.svelte       # container: wires store + actions → AccountsWorkspace
        AccountsWorkspace.svelte
        connect/
          AccountsConnectSection.svelte
          ConnectPromptModal.svelte
          KiroConnectModal.svelte
        list/
          AccountActions.svelte / AccountCard.svelte / AccountRow.svelte
          AccountsGrid.svelte / AccountsListSection.svelte
          AccountsTable.svelte / AccountsToolbar.svelte / ProviderAvatar.svelte
        modals/
          AccountDetailModal.svelte / AccountSyncModal.svelte
          AccountsWorkspaceModals.svelte / BatchDeleteModal.svelte / CredentialField.svelte
      store/
        accounts-actions.ts         # createAccountsScreenActions()
        accounts-store.ts           # createAccountsStoreState()
      utils/
        account.ts / account-quota.ts / auth-session.ts
        preferences.ts / presenter.ts / sync.ts
        workspace.ts / workspace-controller.ts
      index.ts / types.ts

    router/
      components/
        cli-sync/
          CliSyncInfoModal.svelte / CliSyncPanel.svelte
        cloudflared/
          CloudflaredPanel.svelte
        endpoint-tester/
          EndpointTesterPanel.svelte
        model-alias/
          ModelAliasPanel.svelte
        proxy/
          ProxyControlsPanel.svelte / ProxyInlineSwitch.svelte
          ProxyRuntimeCard.svelte / ProxySecurityCard.svelte
        scheduling/
          SchedulingPanel.svelte
      store/
        router-actions.ts
        router-store.ts
      utils/
        alias-form.ts / cli-sync.ts / cloudflared.ts
        endpoint-tester.ts / scheduling.ts
      index.ts / types.ts

    logs/
      components/
        SystemLogsWorkspace.svelte
      utils/
        logs-view.ts                # maps LogEntry → display columns (Level/Source/Account/Detail/Time)
      index.ts

    usage/
      components/
        UsageWorkspace.svelte
      utils/
        request-log.ts
      index.ts

    settings/                       # settings feature
      components/
        BackupToolsCard.svelte      # export / import / restore backup
        DataFolderCard.svelte       # open data folder
        SettingsScreen.svelte       # top-level settings layout
      store/
        task-state.ts               # deriveSettingsViewState / createAsyncTaskState
      utils/
        backup.ts                   # validateBackupPayload / assertBackupPayloadRestorable
      index.ts / types.ts

  components/common/                # reusable UI primitives
    BaseModal.svelte / Button.svelte / CollapsibleSurfaceSection.svelte
    ControlWorkspaceCard.svelte / ModalBackdrop.svelte / ModalWindowHeader.svelte
    OpsPanelSection.svelte / StatusBadge.svelte / SurfaceCard.svelte
    ToastViewport.svelte / ToggleSwitch.svelte

  shared/                           # cross-feature utilities
    stores/
      theme.ts / toast.ts
    utils/
      async.ts / browser.ts / cn.ts / copy.ts
      error.ts / formatters.ts / storage.ts

  tabs/                             # thin tab wrappers (compose features)
    AccountsTab.svelte
    ApiRouterTab.svelte
    DashboardTab.svelte
    SystemLogsTab.svelte
    UsageTab.svelte
    # SettingsTab.svelte removed — settings routes directly to SettingsScreen

  styles/
    index.css                       # imports all partials
    base/base.css
    tokens/theme.css
    primitives/components.css
    features/
      accounts.css / logs.css / router.css / settings.css / usage.css
```

Key frontend files:

- App bootstrap and shell:
  - `frontend/src/app/bootstrap/app-bootstrap.ts` — `initializeAppBootstrap()` startup sequence
  - `frontend/src/app/bootstrap/app-events.ts` — runtime + activity event binding
  - `frontend/src/app/services/app-controller.ts` — `AppShellState`, `AppActions`, `AccountsActions`, `RouterActions`, `LogsActions`, `SettingsActions`
  - `frontend/src/app/routes/app-routes.ts` — `APP_ROUTES` static + lazy route registry
  - `frontend/src/app/routes/RouteOutlet.svelte` — renders active tab route
  - `frontend/src/app/shell/AppFrame.svelte`
  - `frontend/src/app/utils/tabs.ts` — `APP_TABS`, `AppTabId`
- Backend access layer:
  - `frontend/src/backend/client/wails-client.ts` — grouped raw Wails JS binding adapter
  - `frontend/src/backend/gateways/` — final domain-oriented frontend backend surface
- Accounts feature:
  - `frontend/src/features/accounts/components/AccountsScreen.svelte` — container
  - `frontend/src/features/accounts/store/accounts-actions.ts`
  - `frontend/src/features/accounts/store/accounts-store.ts`
- Router feature:
  - `frontend/src/features/router/store/router-actions.ts`
  - `frontend/src/features/router/store/router-store.ts`
- Settings feature:
  - `frontend/src/features/settings/components/SettingsScreen.svelte`
  - `frontend/src/features/settings/utils/backup.ts`
- Logs and usage:
  - `frontend/src/features/logs/utils/logs-view.ts`
  - `frontend/src/features/usage/utils/request-log.ts`

### Backend Core Modules

- **Proxy Shell**: `internal/proxy/http/`
  - server lifecycle, mux wiring, common request context, shared route shell
- **OpenAI/Codex Proxy Face**: `internal/proxy/codex/`
  - OpenAI-style request/response DTOs
  - OpenAI-style request normalization and response encoding
  - OpenAI-style streaming helpers and route-specific execution helpers
- **Anthropic Proxy Face**: `internal/proxy/anthropic/`
  - Anthropic request/response DTOs
  - Anthropic request normalization and response encoding
  - Anthropic streaming helpers and route-specific execution helpers
- **Canonical Proxy Model**: `internal/proxy/models/`
  - shared typed request/response/content/tool/image/thinking model
  - validation rules, tool arg remapping, model/provider resolution, model catalog exposure
- **Proxy Shared Helpers**: `internal/proxy/shared/`
  - small response/error helpers shared by protocol-facing proxy packages
- **Auth**: `internal/auth/`
  - Codex OAuth flow + Kiro auth flows
- **Provider Services**: `internal/provider/`
  - root package keeps shared health/failure classification only
  - `internal/provider/codex/` owns Codex runtime execution
  - `internal/provider/kiro/` owns Kiro runtime payload building, runtime fallback, stream parsing, and quota fetch
- **Config Storage**: `internal/config/`
  - snapshot + atomic updates over JSON files
- **Structured Logging**: `internal/logger/`
  - in-memory + persistent JSONL log storage
  - structured entries with `level`, `scope`, `event`, `requestId`, `message`, and `fields`
- **Sync Services**: `internal/sync/`
  - `internal/sync/cliconfig/` for one-click CLI config patch/read/write
  - `internal/sync/authtoken/` for account auth token sync into supported CLIs
- **Cloudflared**: `internal/cloudflared/manager.go`
  - install, start/stop tunnel, parse URL/status

### Backend Notes

- `internal/contract/`, `internal/gateway/`, `internal/route/`, and `internal/protocol/*` are gone. Do not recreate them.
- Shared request/response ownership now lives in `internal/proxy/models/`.
- Protocol-specific behavior belongs in `internal/proxy/codex/` or `internal/proxy/anthropic/`, not in generic proxy glue.
- Provider-specific runtime behavior belongs in `internal/provider/codex/` or `internal/provider/kiro/`.
- Kiro path is live and usable, but edge-case hardening is still most likely needed around images, tool-result fidelity, and streaming parity.

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
- Structured log entries include `level`, `scope`, `event`, `requestId`, `message`, and optional `fields`.
- The system logs table is optimized around `Level / Source / Account / Detail / Time`; update `frontend/src/features/logs/utils/logs-view.ts` when `logger.Entry` shape changes.

## Key App Methods (Wails → Frontend)

Important methods exposed in `app.go`:

### State & Logs

- `GetState()`
- `GetAccounts()`
- `GetProxyStatus()`
- `RefreshCloudflaredStatus()`
- `GetLogs(limit int)`
- `ClearLogs()`
- `GetHostName()`

### Lifecycle & Window

- `ConfirmQuit()`
- `HideToTray()`
- `RestoreWindow()`

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

- **All backend access goes through `frontend/src/backend/`** — never import from `wailsjs` directly in features or UI components.
- Use the domain gateway modules in `backend/gateways/` as the import point for backend calls in feature API modules.
- Keep data access inside feature `api/` or `store/` modules, not ad-hoc in UI components.
- Reuse `components/common/` primitives and `shared/` stores/utils.
- Keep tab files thin; place business logic in `features/*` and `app/*` modules.
- Feature screens use a container component (e.g., `AccountsScreen.svelte`, `SettingsScreen.svelte`) that wires store state + actions, then passes them down to workspace components.
- Route registration lives in `app/routes/app-routes.ts` — add new tabs there using static or lazy `load:` routes.
- Keep router sub-surfaces under feature-owned folders (`proxy/`, `cloudflared/`, `cli-sync/`, `endpoint-tester/`, `model-alias/`, `scheduling/`).
- When system log structure changes, update both `frontend/src/features/logs/utils/logs-view.ts` and `frontend/src/features/logs/components/SystemLogsWorkspace.svelte` together.
- Styles split by feature under `frontend/src/styles/features/` — add a new CSS file per new feature.
- Use `npm run check` before finalizing changes.

## Common Tasks

### Add a New Wails Method

1. Add exported method to `app.go`.
2. Run `wails dev` or `wails build` to regenerate JS/TS bindings in `frontend/wailsjs/`.
3. Expose via the appropriate gateway in `frontend/src/backend/gateways/`.
4. Wire into a feature-level action or store.

### Add a New Tab / Route

1. Create the tab component under `frontend/src/tabs/` (thin wrapper) or a screen under `features/<name>/components/`.
2. Register a new route entry in `frontend/src/app/routes/app-routes.ts` — use `load:` for lazy routes.
3. Add the tab ID to `APP_TABS` in `frontend/src/app/utils/tabs.ts`.
4. Add the tab button to `AppHeader.svelte`.

### Add a New Proxy Capability

1. Update protocol-facing codec logic in `internal/proxy/codex/` and/or `internal/proxy/anthropic/` if protocol mapping is needed.
2. Update route shell wiring in `internal/proxy/http/` if a new endpoint or handler path is needed.
3. Update validation and model resolution in `internal/proxy/models/`.
4. Add tests for both OpenAI and Anthropic request paths.

### Add/Change CLI Sync Target

1. Add `App` constant + `appDefinition` in `internal/sync/cliconfig/service.go`.
2. Implement read/patch logic for status + sync.
3. Extend frontend type union (`CliSyncAppID`) and router card metadata.
4. Add tests in `internal/sync/cliconfig/service_test.go`.

## Testing Checklist

- `go test . ./internal/...` passes.
- `cd frontend && npm run check` passes.
- Proxy reliability is currently strongest on Codex/OpenAI paths. Kiro is live and routed, but agents should expect the remaining risk to concentrate in image handling, tool-result fidelity, and stream edge cases.
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

- **Proxy won't start**: port conflict; change port or free process.
- **No available accounts**: check enabled flag, cooldown, quota status, and auth validity.
- **Cloudflared URL missing**: ensure proxy is running and Cloudflared status refreshed.
- **CLI sync says unsupported target/model**: verify target ID union and local model catalog membership.

## System Tray (Windows)

- **Library**: `fyne.io/systray v1.12.0` (replaces `github.com/getlantern/systray`)
- **Implementation**: `internal/tray/controller_windows.go`
- **Entry point**: `systray.Register(onReady, onExit)` — non-blocking, hooks into the existing Wails/WebView2 message pump on the main thread. Do **not** switch back to `go systray.Run(...)` — that pattern breaks tray menu delivery on Windows because it creates the tray HWND on a goroutine without a proper shell-accessible message loop.
- **Why**: Wails owns the OS main thread. `systray.Run()` from `getlantern/systray` tried to own it too (via a goroutine), causing Windows to silently drop tray shell messages (`WM_RBUTTONUP`, `WM_LBUTTONUP`). `fyne.io/systray`'s `Register()` is specifically designed for embedding alongside webview/toolkit event loops.
- **Controller interface**: unchanged — `Start()`, `SetProxyRunning()`, `Close()`, `Supported()`, `Available()` all remain stable.
- **Non-Windows**: `controller_other.go` is a noop stub; no changes needed there.

## References

- Wails docs: `https://wails.io/docs/introduction`
- Svelte docs: `https://svelte.dev/docs`
- Project overview: `README.md`
- Release notes: `CHANGELOG.md`
- Frontend package notes: `frontend/README.md`
