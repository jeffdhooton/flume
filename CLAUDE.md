# flume — instructions for the building agent

This file is loaded automatically by Claude Code in this directory. Read it first.

## What this is

A local runtime visibility daemon for AI agents. Passively captures HTTP requests/responses, SQL queries, and exceptions from a running dev server and exposes them as millisecond-latency MCP queries. Replaces the guess→log→reproduce→read debugging cycle that eats 30-50% of tokens in every debugging session.

Think: local Datadog/Sentry, but free, local-only, and built for AI agent consumption — not dashboards.

## The problem

When Claude Code debugs a runtime issue today, the cycle is:
1. Read code and guess what's happening
2. Add a log/print statement
3. Ask the user to reproduce
4. Read the output
5. Repeat 3-5 times

This burns 5-10 tool calls per debugging cycle. If CC could just query "show me the last request to /api/orders with its SQL queries and response" — that's 1 call.

## How it should work

flume sits between the dev server and the outside world (reverse proxy, or middleware injection, or log tailing — the approach is a design decision for you to make and document). It captures structured data:

- **HTTP requests/responses** — method, URL, headers, body, status, timing
- **SQL queries** — raw query, params, timing, rows affected
- **Exceptions/errors** — stack trace, message, context
- **Application logs** — structured log lines from the running app

Data is stored locally in a ring buffer (recent N minutes / N requests — not unbounded). Queryable via MCP tools like:

- `flume_requests` — "show me the last 5 requests to /api/orders"
- `flume_queries` — "show me SQL queries from the last request"
- `flume_errors` — "show me recent exceptions"
- `flume_request` — "show me full detail for request <id>"

## Hard constraints (inherited from the sibling projects)

- **Language: Go 1.23+.** Same stack as `~/workspace/scry` and `~/workspace/trawl`.
- **No CGO. Ever.** Single static binary, cross-compile freely.
- **No telemetry, no network calls** (except proxied app traffic obviously).
- **JSON output by default.** Primary user is an AI agent. Add `--pretty` for humans.
- **Local-only.** No cloud, no shared state.
- **One binary, one daemon per user.** Auto-spawn on first CLI call (same pattern as scry).
- **MCP stdio server** for Claude Code integration. Same pattern as `scry mcp`.

## Sibling projects — borrow decisions wholesale

- **`~/workspace/scry`** — code intelligence daemon. Same architecture: Go, cobra CLI, BadgerDB, daemon over Unix socket, JSON-RPC 2.0, MCP stdio, GoReleaser releases, `doctor` command, `setup` command for CC integration. **Read scry's CLAUDE.md, README.md, and docs/DECISIONS.md for patterns to copy.**
- **`~/workspace/trawl`** — web scraping daemon. Same stack.
- **`~/workspace/tome`** — schema awareness (sibling being built in parallel). flume may feed data to tome (sniffed response shapes from real traffic become API schema entries).
- **`~/workspace/lore`** — git intelligence (sibling being built in parallel).

Use the same CLI framework (cobra), logger (zerolog), storage approach, daemon model, and release pipeline. Don't re-evaluate these choices.

## Language/framework support priority

1. **PHP/Laravel** — primary user stack. Laravel has great structured logging, Telescope exists as prior art for what data to capture. Middleware injection or log tailing are both viable.
2. **Node.js/Express** — common stack. HTTP middleware or proxy approach.
3. **Go** — net/http middleware or reverse proxy.

## Key design decisions to make (and document in docs/DECISIONS.md)

1. **Capture strategy**: reverse proxy (sits in front of the app) vs. middleware/SDK injection (code in the app) vs. log tailing (parse structured logs). Each has tradeoffs around setup friction, data richness, and language-agnosticism.
2. **Storage**: ring buffer in memory? BadgerDB with TTL? SQLite? Needs to be fast to write (on hot path) and fast to query.
3. **SQL capture**: how to intercept database queries varies wildly by language/framework. May need per-framework adapters.
4. **Data retention**: how much to keep, how to expire. Agent needs recent context, not a full history.

## What "done" looks like for P0

- `flume start` runs a daemon that captures HTTP request/response pairs from a local dev server
- `flume status` shows the daemon state and what it's watching
- `flume mcp` exposes MCP tools for querying captured data
- `flume setup` registers the MCP server with Claude Code
- Works with at least one capture strategy on one framework (Laravel or Express)
- CC can query "what happened on the last request to /foo" and get a useful answer

## When you make decisions

Document every architectural choice in `docs/DECISIONS.md`. Include reasoning, not just the verdict. Same format as scry's DECISIONS.md.
