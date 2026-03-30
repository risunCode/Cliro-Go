# CLIro-Go

CLIro-Go is a Wails desktop control plane for running a local OpenAI-compatible proxy across ChatGPT Codex and Kiro accounts. CLIro stands for CLIrouter.

Current release: **v0.2.0**

## Highlights

- OpenAI-compatible and Anthropic-compatible local proxy endpoints:
  - `POST /v1/responses`
  - `POST /v1/chat/completions`
  - `POST /v1/completions`
  - `POST /v1/messages`
  - `GET /v1/models`
  - `GET /health`
  - `GET /v1/stats`
- Multi-account routing for Codex and Kiro with availability-aware scheduling, circuit breaker steps, and cooldown handling.
- OAuth flows for Codex plus Kiro device auth and Kiro social auth.
- Token refresh, quota refresh, smart batch quota refresh, and force-refresh-all quota actions.
- API Router controls for proxy runtime, security, routing policy, endpoint testing, and Cloudflared public access.
- Local CLI auth sync for Kilo, Opencode, and Codex CLI.

## Supported Models

- Codex models are listed directly in `GET /v1/models`.
- Kiro models are listed directly and also publish `-thinking` aliases.
- `-thinking` is a Kiro-only convention. Example:
  - `claude-sonnet-4.5`
  - `claude-sonnet-4.5-thinking`

## Local Data

CLIro-Go stores runtime state in `~/.cliro-go/`:

- `config.json` - proxy, auth, scheduling, Cloudflared, and UI-facing settings
- `accounts.json` - connected account records with token/quota metadata
- `stats.json` - usage counters for the local proxy
- `app.log` - persistent application log file
- `bin/cloudflared(.exe)` - downloaded Cloudflared binary when public access is installed

## Development

Prerequisites:

- Go 1.23+
- Node.js 18+ and npm
- Wails CLI v2.11+

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

Run validation:

```bash
cd frontend
npm run check
cd ..
go test . ./internal/...
```

## Build

Build desktop app:

```bash
wails build
```

Windows output:

`build/bin/Cliro-Go.exe`

## Proxy Base URL

Default base URL:

`http://localhost:8095`

Example:

```bash
curl -X POST http://localhost:8095/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.3-codex","messages":[{"role":"user","content":"Hello"}]}'
```

## Notes

- Authorization mode requires the configured API key for all proxy routes.
- Cloudflared public access is managed from the API Router tab and depends on the local proxy being online.
- Smart `Refresh All Quotas` skips accounts still waiting for quota reset; `Force Refresh All Quotas` checks every configured account.

## Attribution

- Codex and Kiro icons/marks remain the property of their respective owners.
- The CLIRO route app icon uses Icons8 artwork: `https://icons8.com/icons/set/route`

## Release Notes

See [`CHANGELOG.md`](CHANGELOG.md) for the full `v0.2.0` change history.
