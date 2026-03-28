# CLIro-Go

CLIro-Go is a desktop control plane for running a local OpenAI-compatible proxy powered by ChatGPT Codex accounts. CLIro stands for CLIrouter.

It is built with Wails (Go backend + Svelte frontend), supports multiple accounts, handles OAuth callback login, refreshes tokens and quota, and exposes a local API endpoint compatible with common OpenAI SDK workflows.

Current release: **v0.1.0** (Initial Release)

## Highlights

- OpenAI-compatible local proxy (`/v1/chat/completions`, `/v1/completions`, `/v1/models`)
- Multi-account pool with round-robin selection and availability filtering
- OAuth callback flow for Codex account connection
- Token refresh + quota refresh with multi-endpoint fallback
- Cooldown and auto-disable handling when quota is exhausted or account is deactivated
- Desktop dashboard for proxy status, traffic, logs, and account operations
- Grid/list account views with search, filter, per-account actions, and bulk actions
- Account import/export (single + selected bulk export)
- Sync account credentials to local CLI auth files:
  - Kilo CLI (`~/.local/share/kilo/auth.json`)
  - Codex CLI (`~/.codex/auth.json`)

## Tech Stack

- Backend: Go 1.23+
- Desktop runtime: Wails v2
- Frontend: Svelte 3 + TypeScript + Vite + Tailwind
 
## Local Data

CLIro-Go stores local state in `~/.cliro-go/`:

- `config.json`: proxy settings (port, LAN access, auto-start)
- `accounts.json`: connected account records and token/quota metadata
- `stats.json`: runtime usage counters

## Development

Prerequisites:

- Go 1.23+
- Node.js 18+ and npm
- Wails CLI v2

Install frontend dependencies:

```bash
cd frontend
npm install
cd ..
```

Run desktop dev mode:

```bash
wails dev
```

Run frontend type checks:

```bash
cd frontend
npm run check
```

## Build

Build desktop app:

```bash
wails build
```

Output binary (Windows):

`build/bin/Cliro-Go.exe`

Build frontend only:

```bash
cd frontend
npm run build
```

## Proxy API

Default base URL:

`http://localhost:8095`

Key endpoints:

- `GET /health`
- `GET /v1/stats`
- `GET /v1/models`
- `POST /v1/chat/completions`
- `POST /v1/completions`

Example:

```bash
curl -X POST http://localhost:8095/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.2-codex","messages":[{"role":"user","content":"Hello"}]}'
```

## UI Features

- **Dashboard**: compact KPI cards, traffic/token grids, host-based greeting hero
- **Accounts**:
  - provider filters (`All`, `Codex`, etc.)
  - grid/list view toggle
  - selection mode + bulk actions (`power`, `export`, `delete`)
  - import from JSON
  - per-account sync to Kilo CLI / Codex CLI auth
- **System Logs**:
  - structured table-like log stream
  - search + level/scope filters
  - sortable order (newest/oldest)
  - copy visible logs

## Security Notes

- Tokens are stored locally in JSON files; keep OS file permissions restricted.
- Enabling LAN mode binds proxy to `0.0.0.0` and exposes it on local network.
- Use trusted networks only if LAN mode is enabled.

## Release Notes

See full release details in [`CHANGELOG.md`](CHANGELOG.md).
