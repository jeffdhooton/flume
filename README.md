# flume

Runtime visibility daemon for AI agents. Captures HTTP requests and responses from your dev server via a reverse proxy and exposes them as millisecond-latency MCP queries.

Think: local Datadog, but free, local-only, and built for AI agent consumption — not dashboards.

## The problem

When an AI agent debugs a runtime issue, the cycle is:

1. Read code and guess what's happening
2. Add a log/print statement
3. Ask the user to reproduce
4. Read the output
5. Repeat 3–5 times

This burns 5–10 tool calls per debugging cycle. With flume, the agent queries "show me the last request to `/api/orders` with its response" — that's 1 call.

## How it works

flume runs a reverse proxy between your browser and your dev server. Every HTTP request/response pair is captured with full headers, bodies, and timing, stored in a local ring buffer, and queryable via MCP tools or the CLI.

```
Browser/Client → flume (:8089) → Dev Server (:8000)
                    ↓
              BadgerDB store
                    ↓
         MCP tools / CLI queries
```

Language-agnostic. Works with Laravel, Express, Rails, Django, Go — anything that speaks HTTP. Zero code changes in your app.

## Install

```bash
go install github.com/jeffdhooton/flume/cmd/flume@latest
```

### Register with Claude Code

```bash
flume setup
```

This registers flume as an MCP server. Claude Code will auto-start the daemon on first tool call — no manual setup needed after this.

### Verify

```bash
flume doctor
```

## Usage

### Quick start

```bash
# Start your dev server on port 8000 (or whatever port it uses)
php artisan serve --port=8000

# Start flume proxy (auto-starts on first MCP call, but you can start manually)
flume start --port 8089 --target localhost:8000

# Point your browser at localhost:8089 instead of localhost:8000
# Every request is now captured

# Query from CLI
flume requests --pretty
flume request <id> --pretty
```

### MCP tools (for AI agents)

Once registered via `flume setup`, Claude Code can call these tools directly:

| Tool | Description |
|---|---|
| `flume_requests` | List recent HTTP requests. Filter by path, method, status code. |
| `flume_request` | Full detail for one request — headers, bodies, timing. |
| `flume_status` | Daemon state, proxy config, request count. |

### CLI commands

| Command | Description |
|---|---|
| `flume start [--port 8089] [--target localhost:8000]` | Start the daemon and reverse proxy |
| `flume stop` | Stop the daemon |
| `flume status` | Show daemon state |
| `flume requests [--path X] [--method X] [--limit N]` | List captured requests |
| `flume request <id>` | Full request/response detail |
| `flume mcp` | Run as MCP stdio server (used by Claude Code) |
| `flume setup` | Register with Claude Code |
| `flume doctor` | Health check |
| `flume version` | Print version |

### Flags

- `--pretty` — Pretty-print JSON output (all commands)
- `--port` — Proxy listen port (default: 8089)
- `--target` — Upstream dev server address (default: localhost:8000)
- `--foreground` — Run daemon in foreground (for debugging)

## Architecture

- **Capture**: Reverse proxy via `net/http/httputil.ReverseProxy`. Language-agnostic, zero app changes.
- **Storage**: BadgerDB with 30-minute TTL and 1000-entry soft cap. Data lives in `~/.flume/data/`.
- **Daemon**: Unix domain socket at `~/.flume/flumed.sock`, auto-spawns on first CLI/MCP call.
- **RPC**: JSON-RPC 2.0 over the Unix socket.
- **MCP**: stdio server speaking the Model Context Protocol.
- **Body cap**: 512 KB per request/response body. Truncated bodies are flagged in metadata.
- **IDs**: ULIDs (time-sortable, globally unique).

Single static binary. No CGO. No telemetry. No network calls (except proxied app traffic). Local-only.

See [`docs/DECISIONS.md`](docs/DECISIONS.md) for detailed architectural rationale.

## Sibling projects

flume is part of a suite of local-first AI agent tools:

- **[scry](https://github.com/jeffdhooton/scry)** — Code intelligence daemon. Pre-computes symbols, references, and call graphs for millisecond-latency lookups.
- **trawl** — Web scraping daemon.
- **tome** — Schema awareness.
- **lore** — Git intelligence.

All share the same architecture: Go, single binary, no CGO, BadgerDB, daemon over Unix socket, MCP stdio, cobra CLI.

## License

MIT
