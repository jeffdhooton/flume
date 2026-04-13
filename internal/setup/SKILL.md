---
name: flume
description: |
  Route HTTP traffic inspection through flume (a local reverse-proxy daemon) instead
  of adding log statements or curl commands. flume captures every request/response
  hitting your dev server with full headers, bodies, and timing, stored in a local
  ring buffer, queryable via MCP tools in <10ms. Use this skill whenever debugging
  API behavior, checking what payload caused an error, or verifying endpoint responses.

  TRIGGER when: user asks "what requests hit the server", "show me the last request
  to /api/X", "what was the response body", "why did that endpoint return 500",
  "show me recent errors", "what's hitting the API"; or when debugging runtime HTTP
  behavior that would otherwise require adding log/print statements and reproducing.

  DO NOT use for: reading source code, searching for symbols, database schema
  questions, git history — use Read/Grep/scry/tome/lore for those. flume only
  knows about HTTP traffic that has flowed through its reverse proxy.
allowed-tools:
  - Bash
  - Read
  - Grep
  - Glob
---

# /flume — HTTP traffic inspection through a local reverse proxy

flume is a single static Go binary that runs a reverse proxy between your
browser/client and your dev server. Every HTTP request/response pair is captured
with full headers, bodies, and timing, stored in BadgerDB with a 30-minute TTL,
and queryable via MCP tools or the CLI.

**Language-agnostic. Zero code changes in your app.** Works with Laravel, Express,
Rails, Django, Go — anything that speaks HTTP.

```
Browser/Client → flume (:8089) → Dev Server (:8000)
                    ↓
              BadgerDB store
                    ↓
         MCP tools / CLI queries
```

## Routing table

| Query shape | Tool | Why |
|---|---|---|
| "What requests hit /api/orders recently?" | **flume_requests** | List with filters |
| "Show me the full request/response for ID X" | **flume_request** | Complete detail |
| "Is the proxy running?" | **flume_status** | Daemon health |
| "Where is function X defined?" | **scry** / Grep | Not HTTP traffic |
| "What columns does the users table have?" | **tome** | Not HTTP traffic |
| "Who changed this file?" | **lore** | Not HTTP traffic |

## Golden path

1. **Check if flume is running** before querying:
   ```bash
   flume status
   ```
   If the daemon is running and proxy is active, proceed. If not, see
   "Starting the proxy" below.

2. **List recent requests**:
   ```bash
   flume requests --pretty
   flume requests --path /api/orders --pretty
   flume requests --method POST --pretty
   flume requests --status-min 400 --status-max 599 --pretty
   flume requests --limit 5 --pretty
   ```

3. **Drill into a specific request**:
   ```bash
   flume request <id> --pretty
   ```
   Returns full request and response headers, bodies, status code, and timing.

4. **Interpret results**:
   - Empty list → no traffic has flowed through the proxy yet. Make sure the
     client is hitting flume's port (default 8089), not the dev server directly.
   - Request body shows `[binary: N bytes]` → binary content (images, PDFs).
     The raw bytes are stored but displayed as a placeholder.
   - Bodies are capped at 512 KB. Truncated bodies are flagged in metadata.

## Starting the proxy

```bash
# Start flume proxying to your dev server on port 8000
flume start --port 8089 --target localhost:8000

# For a different dev server port
flume start --target localhost:3000
```

The daemon auto-starts on first MCP tool call, but the proxy needs to know
where to forward traffic. If the proxy isn't configured yet:

1. Ask the user what port their dev server runs on.
2. Start flume with `--target localhost:<port>`.
3. Tell the user to point their browser at `localhost:8089`.

## MCP tools

### flume_requests

List recent captured requests. Returns ID, method, path, status, duration, timestamp.

Parameters:
- `path` (string) — filter by URL path substring, e.g. `/api/orders`
- `method` (string) — filter by HTTP method, e.g. `POST`
- `status_min` (int) — minimum status code inclusive, e.g. `400`
- `status_max` (int) — maximum status code inclusive, e.g. `499`
- `limit` (int) — max results, default 20

### flume_request

Full detail for one request by ID. Returns complete request/response headers,
bodies, timing, and status code.

Parameters:
- `id` (string, required) — the request ID from flume_requests

### flume_status

Daemon state: whether the proxy is running, listen port, target, request count,
retention window.

No parameters.

## CLI command reference

```bash
flume start [--port 8089] [--target localhost:8000] [--foreground]
flume stop
flume status
flume requests [--path X] [--method X] [--status-min N] [--status-max N] [--limit N]
flume request <id>
flume mcp              # MCP stdio server (used by Claude Code)
flume setup            # Register with Claude Code
flume doctor           # Health check
flume version
```

Global flags:
- `--pretty` — pretty-print JSON output (all commands)

## When flume returns nothing

Decision tree:

1. **No requests listed** → proxy hasn't seen traffic yet. Verify the client
   is hitting flume's port, not the dev server directly.
2. **Daemon not running** → `flume status` returns an error. Run `flume start`.
3. **Requests expired** → data has a 30-minute TTL. Reproduce the request.
4. **Wrong filters** → relax path/method/status filters and try again.

## Data retention

- 30-minute TTL on all captured data
- 1000-entry soft cap (oldest evicted first)
- IDs are ULIDs (time-sortable, globally unique)
- Data lives in `~/.flume/data/`
