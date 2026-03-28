# AGENTS.md - Guide for AI Agents Working in CLIro-Go

This document provides essential information for AI agents working in the CLIro-Go codebase.

## Project Overview

**CLIro-Go** is a desktop application built with Wails (Go + Svelte) that provides an OpenAI-compatible proxy server for ChatGPT Codex API access. It manages multiple ChatGPT accounts, handles OAuth authentication, quota tracking, and exposes a local HTTP proxy that translates OpenAI API calls to ChatGPT's Codex backend.

- **Language**: Go 1.23+ (backend), TypeScript + Svelte 3 (frontend)
- **Framework**: Wails v2.11.0, Vite, Tailwind CSS
- **Current Release**: v0.1.0 (Initial Release)
- **License**: Not specified
- **Main Entry Point**: [main.go](main.go)
- **Data Directory**: `~/.cliro-go/` with multiple JSON files:
  - `config.json` - Application settings (proxy port, LAN access, auto-start)
  - `accounts.json` - All ChatGPT account data
  - `stats.json` - Proxy usage statistics

## Essential Commands

### Development

```bash
# Install frontend dependencies
cd frontend && npm install && cd ..

# Run dev mode (hot reload for both Go and frontend)
wails dev

# Type check frontend
cd frontend && npm run check
```

### Building

```bash
# Production build (creates executable in build/bin/)
wails build

# Frontend only
cd frontend && npm run build
```

### Testing

```bash
# Test proxy health
curl http://localhost:8095/health

# Test chat completions
curl -X POST http://localhost:8095/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.2-codex","messages":[{"role":"user","content":"Hello"}]}'
```

## Architecture

### Wails Bridge Pattern

**Go Backend** exposes methods to frontend via Wails bindings:
- [app.go](app.go) - Main `App` struct with methods like `GetState()`, `StartProxy()`, `StartCodexAuth()`
- [main.go](main.go) - Binds `App` to Wails runtime
- Auto-generates TypeScript bindings in `frontend/wailsjs/go/main/App.ts`

**Svelte Frontend** calls Go methods:
- [frontend/src/services/wails-api.ts](frontend/src/services/wails-api.ts) - Wraps generated bindings
- Components import and call `appService.getState()`, `appService.startProxy()`, etc.

**Real-time Events**:
- Backend emits: `wruntime.EventsEmit(ctx, "log:entry", entry)`
- Frontend subscribes: `EventsOn("log:entry", callback)`

### Core Modules

#### 1. Proxy Service ([internal/proxy/service.go](internal/proxy/service.go))
OpenAI-compatible HTTP server with routes:
- `POST /v1/chat/completions` - Main chat endpoint
- `POST /v1/completions` - Legacy completions
- `GET /v1/models` - List available models
- `GET /health`, `GET /v1/stats` - Monitoring

**Request Flow**:
1. Receives OpenAI-format request
2. Gets available account from pool
3. Ensures fresh token via `auth.EnsureFreshAccount()`
4. Translates to ChatGPT Codex API format
5. Streams or collects response
6. Updates account stats and cooldowns

**Retry Logic**: Tries all available accounts on failure, handles quota exhaustion and auth errors.

#### 2. Auth System ([internal/auth/codex.go](internal/auth/codex.go))
- **OAuth 2.0 PKCE flow** for ChatGPT authentication
- **Token management**: Stores access/refresh/id tokens, auto-refreshes before expiry (5min skew)
- **Callback server**: Runs on `localhost:1455` to receive OAuth redirects
- **Session tracking**: Polls auth status, emits events to frontend

#### 3. Config Management ([internal/config/config.go](internal/config/config.go))
- **Multi-file storage**: Separates concerns into `config.json`, `accounts.json`, `stats.json`
- **Storage layer** ([internal/config/storage.go](internal/config/storage.go)): Handles file I/O for each data type
- **Manager**: In-memory cache with thread-safe access via `sync.RWMutex`
- **Atomic updates**: `UpdateAccount(id, func(*Account))` pattern
- **Snapshot pattern**: `Snapshot()` returns immutable copy for UI
- **Auto-save**: Writes to disk on every mutation (only affected file)

#### 4. Connection Pooling ([internal/pool/pool.go](internal/pool/pool.go))
- **Round-robin selection** with atomic counter
- **Availability filtering**: Skips disabled, cooldown, or quota-exhausted accounts
- **No pre-warming**: Accounts validated on-demand by proxy service

#### 5. Quota System ([internal/auth/quota.go](internal/auth/quota.go))
- **Multi-endpoint fallback**: Tries `/quota`, `/limits`, `/me` endpoints
- **Bucket-based tracking**: Per-model usage limits with reset times
- **Refresh strategies**: Manual, batch, or automatic on auth errors

#### 6. Logger ([internal/logger/logger.go](internal/logger/logger.go))
- **Ring buffer** with configurable capacity (default 1000 entries)
- **Event emission**: Broadcasts log entries to frontend via Wails events
- **Levels**: Info, Error, Debug
- **Context attachment**: Binds to Wails context for event emission

## Key Patterns & Conventions

### Go Code Style

Follow idiomatic Go conventions from the `go-naming` and `golang-patterns` skills:

**Naming**:
- Packages: lowercase, single word (e.g., `proxy`, `auth`, `config`)
- Exported types: PascalCase (e.g., `Manager`, `Service`, `Account`)
- Unexported: camelCase (e.g., `proxyBindHost`, `resolveDataDir`)
- Receivers: 1-2 letter abbreviations (e.g., `a *App`, `m *Manager`)

**Error Handling**:
- Return errors, don't panic (except in `main()` or `startup()`)
- Wrap errors with context: `fmt.Errorf("failed to start proxy: %w", err)`
- Check errors immediately after function calls

**Concurrency**:
- Use `sync.RWMutex` for shared state (see [config.go](internal/config/config.go))
- Atomic operations for counters (see [pool.go](internal/pool/pool.go))
- Context propagation for cancellation

### Frontend Code Style

**Svelte Components**:
- Use `<script lang="ts">` for TypeScript
- Reactive statements: `$: derivedValue = sourceValue * 2`
- Store subscriptions: `$storeName` auto-subscribes

**Tailwind CSS**:
- Utility-first approach
- Custom colors defined in [tailwind.config.cjs](frontend/tailwind.config.cjs)
- Responsive: `md:`, `lg:` prefixes

**Type Safety**:
- Import generated types from `wailsjs/go/models.ts`
- Define component props with TypeScript interfaces
- Use `svelte-check` to catch type errors

## Common Tasks

### Adding a New Go Method for Frontend

1. Add method to `App` struct in [app.go](app.go):
```go
func (a *App) MyNewMethod(param string) (string, error) {
    // Implementation
    return result, nil
}
```

2. Rebuild or run `wails dev` - bindings auto-generate

3. Use in frontend:
```typescript
import { MyNewMethod } from '@/wailsjs/go/main/App';
const result = await MyNewMethod("value");
```

### Adding a New Proxy Endpoint

1. Add route handler in [internal/proxy/service.go](internal/proxy/service.go)
2. Register route in `Start()` method
3. Update OpenAPI docs if needed
4. Test with curl or Postman

### Adding a New Config Field

1. Add field to `Config` struct in [internal/config/config.go](internal/config/config.go)
2. Add getter/setter methods to `Manager`
3. Update `Snapshot()` to include new field
4. Update frontend types if exposed via `GetState()`

### Emitting Events to Frontend

```go
import wruntime "github.com/wailsapp/wails/v2/pkg/runtime"

// In any method with access to a.ctx
wruntime.EventsEmit(a.ctx, "my:event", data)
```

```typescript
// In Svelte component
import { EventsOn } from '@/wailsjs/runtime/runtime';

onMount(() => {
    EventsOn("my:event", (data) => {
        console.log(data);
    });
});
```

## Gotchas & Important Notes

### Authentication Flow
- User clicks "Add Account" → `StartCodexAuth()` called
- Opens browser to ChatGPT OAuth page
- User logs in, redirects to `localhost:1455/auth/callback`
- Backend exchanges code for tokens, saves account
- Frontend polls `GetCodexAuthSession(sessionId)` until success/error

### Port Requirements
- **Proxy**: Default 8095 (configurable in settings)
- **OAuth Callback**: Port 1455 (hardcoded, must be free)

### Data Persistence
- Config stored in `~/.cliro-go/` directory with multiple JSON files:
  - `config.json` - Application settings (proxy port, LAN access, auto-start)
  - `accounts.json` - All ChatGPT account data with tokens and stats
  - `stats.json` - Proxy usage statistics
- No database - file-based storage
- **Security**: Tokens stored in plaintext - file permissions are critical (0600)

### Token Refresh
- Tokens auto-refresh when < 5min until expiry
- Manual refresh available per account
- Refresh failures trigger cooldown and error logging

### Account Availability
- Accounts filtered by: enabled flag, cooldown status, quota exhaustion
- Pool returns error if no accounts available
- Proxy retries with next account on failure

### CORS & Security
- Proxy allows all origins by default (for local development)
- No authentication on proxy endpoints
- **Warning**: Exposing to LAN (0.0.0.0) makes proxy accessible to network

### Development Hot Reload
- `wails dev` watches both Go and frontend files
- Go changes trigger backend rebuild
- Frontend changes trigger Vite HMR
- Bindings regenerate automatically on Go method changes
 

## Testing Strategy

### Manual Testing
1. Run `wails dev`
2. Add account via OAuth flow
3. Start proxy server
4. Test endpoints with curl
5. Monitor logs in UI

### Integration Testing
- Test full OAuth flow with real ChatGPT account
- Verify token refresh logic
- Test quota exhaustion handling
- Verify account rotation in pool

### Frontend Testing
- Use `npm run check` for type checking
- Manual UI testing in dev mode
- Test event subscriptions and real-time updates

## Debugging Tips

### Backend Debugging
- Check logs in UI or via `GetLogs()` method
- Add `a.log.Info("module", "message")` for tracing
- Use `fmt.Printf()` in dev mode (not in production)
- Check `~/.cliro-go/config.json` for state inspection

### Frontend Debugging
- Open DevTools in Wails window (F12 or Cmd+Option+I)
- Console logs visible in browser DevTools
- Check Network tab for Wails IPC calls
- Use Svelte DevTools extension

### Common Issues
- **"Port already in use"**: Change proxy port in settings or kill process
- **"No accounts available"**: Check account enabled status and cooldowns
- **"OAuth callback failed"**: Ensure port 1455 is free
- **"Token expired"**: Manual refresh or wait for auto-refresh

## API Reference

### App Struct Methods (Exposed to Frontend)

All methods in [app.go](app.go) are automatically bound to frontend via Wails:

```go
// State Management
GetState() State                              // Get full app state snapshot
GetAccounts() []config.Account                // Get all accounts
GetProxyStatus() map[string]any               // Get proxy server status
GetLogs(limit int) []logger.Entry             // Get recent log entries
GetHostName() string                           // Get local machine host name

// Proxy Control
StartProxy() error                            // Start proxy server
StopProxy() error                             // Stop proxy server
SetProxyPort(port int) error                  // Change proxy port
SetAllowLAN(allow bool) error                 // Toggle LAN access
SetAutoStartProxy(autoStart bool) error       // Toggle auto-start

// Account Management
StartCodexAuth() (*auth.CodexAuthStart, error)           // Initiate OAuth flow
GetCodexAuthSession(sessionID string) auth.CodexAuthSessionView  // Poll auth status
CancelCodexAuth(sessionID string)                        // Cancel OAuth flow
RefreshAccount(accountID string) error                   // Refresh account tokens
RefreshQuota(accountID string) error                     // Refresh quota info
RefreshAllQuotas() error                                 // Refresh all account quotas
DeleteAccount(accountID string) error                    // Delete account
ToggleAccount(accountID string, enabled bool) error      // Enable/disable account
ClearCooldown(accountID string) error                    // Clear cooldown timer
ImportAccounts(accounts []config.Account) (int, error)   // Import accounts from JSON payload
SyncCodexAccountToKiloAuth(accountID string) (auth.KiloAuthSyncResult, error)         // Sync account to Kilo CLI auth
SyncCodexAccountToOpencodeAuth(accountID string) (auth.OpencodeAuthSyncResult, error)  // Sync account to Opencode auth
SyncCodexAccountToCodexCLI(accountID string) (auth.CodexAuthSyncResult, error)         // Sync account to Codex CLI auth

// Utilities
ClearLogs()                                             // Clear in-memory log buffer
OpenExternalURL(url string) error                       // Open URL in default browser
OpenDataDir() error                                     // Open config folder in explorer
```

### Proxy Endpoints

**Base URL**: `http://localhost:8095` (default)

```bash
# Health Check
GET /health
Response: {"status":"ok","running":true,"started_at":1234567890}

# Statistics
GET /v1/stats
Response: {
  "status":"ok",
  "accounts":3,
  "enabledAccounts":2,
  "available":1,
  "stats":{
    "totalRequests":150,
    "successRequests":145,
    "failedRequests":5,
    "promptTokens":12000,
    "completionTokens":8000,
    "totalTokens":20000
  }
}

# List Models
GET /v1/models
Response: {
  "object":"list",
  "data":[
    {"id":"gpt-5.2-codex","object":"model","owned_by":"codex"},
    {"id":"gpt-5.3-codex","object":"model","owned_by":"codex"}
  ]
}

# Chat Completions (OpenAI-compatible)
POST /v1/chat/completions
Content-Type: application/json
{
  "model": "gpt-5.2-codex",
  "messages": [
    {"role":"user","content":"Hello"}
  ],
  "stream": false,
  "temperature": 0.7,
  "max_tokens": 2000
}

# Legacy Completions
POST /v1/completions
Content-Type: application/json
{
  "model": "gpt-5.2-codex",
  "prompt": "Hello",
  "stream": false
}
```

## Frontend Component Guide

### Component Structure

```
frontend/src/
├── components/
│   ├── common/              # Shared reusable UI across tabs
│   │   ├── AppHeader.svelte
│   │   ├── AppFooter.svelte
│   │   ├── Button.svelte
│   │   ├── ModalBackdrop.svelte
│   │   ├── StatusBadge.svelte
│   │   ├── SurfaceCard.svelte
│   │   ├── ToastViewport.svelte
│   │   └── ToggleSwitch.svelte
│   └── accounts/            # Accounts-specific UI components
│       ├── AccountCard.svelte
│       ├── AccountDetailModal.svelte
│       ├── AccountRow.svelte
│       ├── AccountsGrid.svelte
│       ├── AccountsTable.svelte
│       ├── AccountsToolbar.svelte
│       └── ConnectPromptModal.svelte
├── tabs/                    # Tab pages and tab-local modules
│   ├── DashboardTab.svelte
│   ├── AccountsTab.svelte
│   ├── AccountsTab.css
│   ├── ApiRouterTab.svelte
│   ├── SystemLogsTab.svelte
│   └── SettingsTab.svelte
├── services/
│   ├── wails-api.ts
│   ├── error.ts
│   ├── bootstrap.ts
│   ├── proxy-actions.ts
│   ├── auth-session.ts
│   └── logs-subscription.ts
├── stores/
│   ├── theme.ts
│   └── toast.ts
└── utils/
    ├── accounts/
    │   ├── filters.ts
    │   ├── provider.ts
    │   ├── quota.ts
    │   └── selection.ts
    ├── formatters.ts
    ├── tabs.ts
    └── cn.ts
```

### Component Patterns

**Props & Events**:
```svelte
<script lang="ts">
  import type { Account } from '@/services/wails-api'
  
  // Props
  export let account: Account
  export let busy: boolean = false
  
  // Events
  import { createEventDispatcher } from 'svelte'
  const dispatch = createEventDispatcher<{
    refresh: string  // accountId
    delete: string
  }>()
  
  function handleRefresh() {
    dispatch('refresh', account.id)
  }
</script>

<button on:click={handleRefresh} disabled={busy}>
  Refresh
</button>
```

**Reactive Statements**:
```svelte
<script lang="ts">
  export let accounts: Account[]
  
  // Auto-recomputes when accounts changes
  $: enabledCount = accounts.filter(a => a.enabled).length
  $: availableCount = accounts.filter(a => 
    a.enabled && a.cooldownUntil <= Date.now()/1000
  ).length
</script>

<p>Enabled: {enabledCount}, Available: {availableCount}</p>
```

**Store Subscriptions**:
```svelte
<script lang="ts">
  import { theme } from '@/stores/theme'
  import { toastStore } from '@/stores/toast'
  
  // Auto-subscribe with $prefix
  $: isDark = $theme === 'dark'
  
  function notify() {
    toastStore.push('success', 'Title', 'Message')
  }
</script>

<div class:dark={isDark}>
  <button on:click={notify}>Notify</button>
</div>
```

## Config Schema (config.json)

**Location**: `~/.cliro-go/config.json`

```json
{
  "proxyPort": 8095,
  "allowLan": false,
  "autoStartProxy": true
}
```

**Location**: `~/.cliro-go/accounts.json`

```json
[
  {
    "id": "uuid-here",
    "provider": "chatgpt",
    "email": "user@example.com",
    "accountId": "chatgpt-account-id",
    "planType": "plus",
    "quota": {
      "status": "ok",
      "summary": "2000/5000 requests remaining",
      "source": "/quota",
      "lastCheckedAt": 1234567890,
      "buckets": [
        {
          "name": "gpt-5.2-codex",
          "used": 3000,
          "total": 5000,
          "remaining": 2000,
          "percent": 60,
          "resetAt": 1234567890,
          "status": "ok"
        }
      ]
    },
    "accessToken": "ey...",
    "refreshToken": "ey...",
    "idToken": "ey...",
    "expiresAt": 1234567890,
    "enabled": true,
    "cooldownUntil": 0,
    "lastError": "",
    "requestCount": 150,
    "errorCount": 5,
    "promptTokens": 12000,
    "completionTokens": 8000,
    "totalTokens": 20000,
    "lastUsed": 1234567890,
    "lastRefresh": 1234567890,
    "createdAt": 1234567890,
    "updatedAt": 1234567890
  }
]
```

**Location**: `~/.cliro-go/stats.json`

```json
{
  "totalRequests": 150,
  "successRequests": 145,
  "failedRequests": 5,
  "promptTokens": 12000,
  "completionTokens": 8000,
  "totalTokens": 20000,
  "lastRequestAt": 1234567890
}
```

**Field Descriptions**:
- `cooldownUntil`: Unix timestamp when account exits cooldown (0 = no cooldown)
- `expiresAt`: Unix timestamp when access token expires
- `quota.buckets`: Per-model usage limits with reset times
- `enabled`: Whether account is active in pool rotation
- `lastError`: Most recent error message (empty if no error)

## Development Workflow

### Git Workflow

```bash
# Feature branch
git checkout -b feature/add-quota-alerts
git add .
git commit -m "feat: add quota alert notifications"
git push origin feature/add-quota-alerts

# Commit message format
# feat: new feature
# fix: bug fix
# refactor: code restructuring
# docs: documentation changes
# test: add/update tests
```

### PR Checklist

- [ ] Code follows Go naming conventions (`go-naming` skill)
- [ ] Error handling with context wrapping
- [ ] Frontend types match Go structs
- [ ] Wails bindings regenerated (`wails dev` or `wails build`)
- [ ] Manual testing completed
- [ ] No console errors in DevTools 

### Testing Checklist

**Backend**:
- [ ] Proxy endpoints respond correctly
- [ ] Account rotation works with multiple accounts\\
- [ ] Token refresh triggers before expiry
- [ ] Quota exhaustion triggers cooldown
- [ ] Error handling doesn't panic

**Frontend**:
- [ ] UI updates on state changes
- [ ] Event subscriptions work (logs, auth status)
- [ ] Loading states display correctly
- [ ] Error toasts show helpful messages
- [ ] Type checking passes (`npm run check`)

## Performance Tips

### Backend Optimization

**Avoid Blocking Operations**:
```go
// Bad: Blocks main thread
func (a *App) SlowOperation() error {
    time.Sleep(10 * time.Second)
    return nil
}

// Good: Run in goroutine, emit events
func (a *App) SlowOperation() error {
    go func() {
        time.Sleep(10 * time.Second)
        wruntime.EventsEmit(a.ctx, "operation:done", nil)
    }()
    return nil
}
```

**Minimize Config Writes**:
```go
// Bad: Multiple writes
store.SetProxyPort(8095)
store.SetAllowLAN(true)
store.SetAutoStartProxy(true)

// Good: Batch updates
store.UpdateAccount(id, func(a *config.Account) {
    a.Enabled = true
    a.CooldownUntil = 0
    a.LastError = ""
})
```

**Use Read Locks**:
```go
// Read-only operations
m.mu.RLock()
defer m.mu.RUnlock()
return m.cfg.ProxyPort

// Write operations
m.mu.Lock()
defer m.mu.Unlock()
m.cfg.ProxyPort = port
```

### Frontend Optimization

**Debounce Expensive Operations**:
```svelte
<script lang="ts">
  const debounce = <T extends (...args: any[]) => void>(fn: T, delay: number) => {
    let timer: ReturnType<typeof setTimeout> | null = null
    return (...args: Parameters<T>) => {
      if (timer) clearTimeout(timer)
      timer = setTimeout(() => fn(...args), delay)
    }
  }
   
  const refreshQuotas = debounce(async () => {
    await appService.refreshAllQuotas()
  }, 1000)
</script>
```

**Avoid Unnecessary Reactivity**:
```svelte
<script lang="ts">
  // Bad: Recomputes on every state change
  $: expensiveValue = accounts.map(a => computeExpensive(a))
  
  // Good: Only recomputes when accounts change
  $: expensiveValue = accounts.length > 0 
    ? accounts.map(a => computeExpensive(a)) 
    : []
</script>
```

## Security Best Practices

### Token Handling

**Never Log Tokens**:
```go
// Bad
a.log.Info("auth", "token: "+account.AccessToken)

// Good
a.log.Info("auth", "token refreshed for "+account.Email)
```

**Secure Config File**:
```go
// Set restrictive permissions on config file
os.Chmod(configPath, 0o600)  // Owner read/write only
```

### CORS Configuration

**Development** (current):
```go
w.Header().Set("Access-Control-Allow-Origin", "*")
```

**Production** (recommended):
```go
allowedOrigins := []string{"http://localhost:3000"}
origin := r.Header.Get("Origin")
if slices.Contains(allowedOrigins, origin) {
    w.Header().Set("Access-Control-Allow-Origin", origin)
}
```

### LAN Exposure

**Warning**: Setting `allowLan: true` binds proxy to `0.0.0.0`, making it accessible to entire network.

**Mitigation**:
- Add API key authentication
- Use firewall rules to restrict access
- Only enable when necessary
- Consider VPN for remote access

## Deployment Guide

### Building for Production

**Windows**:
```bash
wails build
# Output: build/bin/Cliro-Go.exe
```

**Cross-compilation**:
```bash
# Build for Windows from Linux/Mac
wails build -platform windows/amd64

# Build for macOS
wails build -platform darwin/universal

# Build for Linux
wails build -platform linux/amd64
```

### Distribution

**Portable Mode**:
- Executable is self-contained
- Config created at `~/.cliro-go/` on first run
- No installer needed

**Installer** (optional):
- Use Inno Setup (Windows)
- Use DMG (macOS)
- Use AppImage/DEB (Linux)

### Updates

**Manual**:
1. Download new executable
2. Replace old executable
3. Config persists automatically

**Auto-update** (future):
- Implement version check endpoint
- Download and replace executable
- Restart application

## Troubleshooting Guide

### Common Errors

**"Port already in use"**:
```bash
# Windows: Find process using port
netstat -ano | findstr :8095
taskkill /PID <pid> /F

# Change port in settings or config.json
```

**"No accounts available"**:
- Check account enabled status
- Clear cooldowns if stuck
- Verify tokens not expired
- Refresh quota to check limits

**"OAuth callback failed"**:
- Ensure port 1455 is free
- Check firewall allows localhost:1455
- Try restarting auth flow
- Check browser console for errors

**"Token expired"**:
- Manual refresh via UI
- Wait for auto-refresh (triggers at <5min remaining)
- Re-authenticate if refresh token invalid

**"Quota exhausted"**:
- Account enters cooldown automatically
- Cooldown clears at reset time
- Add more accounts for rotation
- Check quota limits in account details

### Upstream API Errors

These errors come from ChatGPT Codex API, not from CLIro-Go:

**"Not supported when use chatgpt account"**:
- **Cause**: Feature/model not available for your ChatGPT plan type
- **Solution**: 
  - Check account `planType` in config.json (free, plus, team, enterprise)
  - Upgrade to ChatGPT Plus/Team for Codex model access
  - Some models (gpt-5.2-codex, gpt-5.3-codex) require paid plans
  - Free accounts have limited model access

**"Usage limit reached"**:
- **Cause**: Account hit quota limit for current period
- **Behavior**: CLIro-Go auto-cooldown until reset time
- **Solution**: Wait for quota reset or add more accounts

**"Unauthorized" / "Forbidden"**:
- **Cause**: Token invalid or expired
- **Behavior**: Account enters 1-minute transient cooldown
- **Solution**: Refresh token manually or re-authenticate

**Plan Type Limitations**:
```
Free:       Limited models, low quota
Plus:       Full Codex access, higher quota
Team:       Shared quota pool, admin controls
Enterprise: Custom limits, priority access
```

**Checking Plan Type**:
```bash
# Windows
notepad %USERPROFILE%\.cliro-go\config.json

# Look for "planType" field in accounts array
# Values: "free", "plus", "team", "enterprise"
```

### Debug Mode

**Enable verbose logging**:
```go
// In logger.go, add Debug level
a.log.Debug("module", "detailed message")
```

**Frontend DevTools**:
```javascript
// In browser console
localStorage.setItem('debug', 'true')
location.reload()
```

**Inspect Config**:
```bash
# Windows
notepad %USERPROFILE%\.cliro-go\config.json

# Linux/Mac
cat ~/.cliro-go/config.json | jq
```

## References

- **Wails Documentation**: https://wails.io/docs/introduction
- **Svelte Documentation**: https://svelte.dev/docs
- **Go Style Guide**: Use `go-naming` and `golang-patterns` skills
- **Related Projects**: See `z_references/` for reference implementations
