# CLIRO

## its end here, i have no time to maintain this
-- goodbye!.

CLIRO is a Wails desktop control plane for running a local OpenAI-compatible proxy across ChatGPT Codex and Kiro accounts. CLIRO stands for CLIrouter.

Current release: **v0.4.0**

## Screenshot

| Dashboard | Accounts |
| --- | --- |
| <img width="1186" height="693" alt="Dashboard" src="https://github.com/user-attachments/assets/868fa06f-8fdd-4b01-938d-0e928db025ca" /> | <img width="1186" height="693" alt="Accounts" src="https://github.com/user-attachments/assets/dc3daaf7-5679-40e2-8af0-6f5c6bf844ce" /> |

| API Route | Usage |
| --- | --- |
| <img width="1186" height="693" alt="API Route" src="https://github.com/user-attachments/assets/d3f6455f-2a1c-4eab-a952-0a59939ab180" /> | <img width="1186" height="693" alt="Usage" src="https://github.com/user-attachments/assets/fe2f0516-097c-4297-ba62-68690de00052" /> |

## Highlights

- **v0.4.0 architecture refresh**:
  - proxy runtime restructured around `internal/proxy/codex`, `internal/proxy/anthropic`, `internal/proxy/models`, and `internal/proxy/shared`
  - old `internal/contract`, `internal/gateway`, `internal/route`, and `internal/protocol/*` layers removed
  - Codex runtime simplified into focused execution/payload/runtime files
  - Kiro runtime promoted from partial path into a live routed provider with payload building, event-stream parsing, host fallback, and catalog support
- OpenAI-compatible and Anthropic-compatible local proxy endpoints:
  - `POST /v1/responses`
  - `POST /v1/chat/completions`
  - `POST /v1/completions`
  - `POST /v1/messages`
  - `GET /v1/models`
  - `GET /health`
  - `GET /v1/stats`
- **Automatic reasoning/thinking parameter injection** via model name suffixes:
  - `-low` / `-minimal` → 4096 budget tokens (OpenAI: `effort: "low"`)
  - `-medium` → 10000 budget tokens (OpenAI: `effort: "medium"`)
  - `-high` → 16384 budget tokens (OpenAI: `effort: "high"`)
  - `-xhigh` → 32768 budget tokens (OpenAI: `effort: "xhigh"`)
  - Example: `gpt-5.4-high` automatically enables extended thinking
- **Cross-protocol reasoning/thinking conversion**:
  - OpenAI `reasoning.effort` ↔ Anthropic `thinking.budget_tokens` bidirectional mapping
  - Automatic parameter filtering to prevent "Unknown parameter" errors
  - Universal `reasoning_content` field in responses for client compatibility
- Multi-account routing for Codex and Kiro with availability-aware scheduling, circuit breaker steps, and cooldown handling.
- OAuth flows for Codex plus Kiro device auth and Kiro social auth.
- Token refresh, quota refresh, smart batch quota refresh, and force-refresh-all quota actions.
- API Router controls for proxy runtime, security, routing policy, endpoint testing, and Cloudflared public access.
- One-click CLI config sync in API Router for Claude Code, OpenCode, Kilo CLI, and Codex AI, including model selection from local `/v1/models` catalog.
- Local CLI auth sync for Kilo, Opencode, and Codex CLI.

## Supported Models

- All models are listed directly in `GET /v1/models`.
- **Model name suffixes** enable automatic reasoning/thinking:
  - Add `-low`, `-medium`, `-high`, or `-xhigh` to any model name
  - Example: `gpt-5.4-high`, `claude-sonnet-4.5-high`
- Suffix is stripped during routing and converted to appropriate reasoning parameters
- Current Kiro catalog includes examples such as:
  - `auto`
  - `claude-opus-4.5`
  - `claude-opus-4.6`
  - `claude-sonnet-4`
  - `claude-sonnet-4.5`
  - `claude-sonnet-4.6`
  - `claude-haiku-4.5`
  - `claude-haiku-4.6`
  - `minimax-m2.5`
  - `qwen3-coder-next`
  - `deepseek-3.2`

## Local Data

CLIRO stores runtime state in `~/.cliro/`:

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

`build/bin/CLIRO.exe`

## Proxy Base URL

Default base URL:

`http://localhost:8095/v1`

Example without reasoning:

```bash
curl -X POST http://localhost:8095/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.3-codex","messages":[{"role":"user","content":"Hello"}]}'
```

Example with automatic reasoning (using `-high` suffix):

```bash
curl -X POST http://localhost:8095/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-5.4-high","messages":[{"role":"user","content":"Explain quantum entanglement"}]}'
```

The response will include `reasoning_content` field with extended thinking.

## Notes

- Authorization mode requires the configured API key for all proxy routes.
- Cloudflared public access is managed from the API Router tab and depends on the local proxy being online.
- Smart `Refresh All Quotas` skips accounts still waiting for quota reset; `Force Refresh All Quotas` checks every configured account.
- Kiro runtime uses `q.us-east-1.amazonaws.com/generateAssistantResponse` first and falls back to `codewhisperer.us-east-1.amazonaws.com/generateAssistantResponse` on runtime failure.
- Reliability is now strongest on Codex/OpenAI paths; Kiro is live and routed, with the remaining risk concentrated around image handling, tool-result fidelity, and streaming edge cases.
- Cross-protocol adapter audit and compatibility coverage are documented in `docs/audit-adapter-cross-protocol.md`.
- Model aliasing behavior and examples are documented in `docs/feature-model-aliasing.md`.

## Attribution

- Codex and Kiro icons/marks remain the property of their respective owners.
- The CLIRO route app icon uses Icons8 artwork: `https://icons8.com/icons/set/route`

## Release Notes

See [`CHANGELOG.md`](CHANGELOG.md) for the full release history.
