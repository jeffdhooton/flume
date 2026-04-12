# flume — architectural decisions

One entry per decision, newest at top. Each entry answers: what, why, what would change our minds. Cross-reference with CLAUDE.md for the original design brief.

---

## 2026-04-12 — Capture strategy: reverse proxy

**Decision:** flume captures HTTP traffic via a reverse proxy (`net/http/httputil.ReverseProxy`). The daemon listens on a configurable port (default 8089), forwards all traffic to the upstream dev server, and captures complete request/response pairs in transit.

**Why:** The CLAUDE.md identified three viable approaches: reverse proxy, per-framework middleware/SDK injection, and log tailing. We chose the reverse proxy because:

1. **Language-agnostic immediately.** Works with Laravel, Express, Go, Rails, Django — anything that speaks HTTP. No per-framework adapter needed for P0.
2. **Zero code changes in the app.** The developer just points their browser/client at flume's port instead of the dev server's port.
3. **Richest HTTP data at the network layer.** Complete request headers, request body, response headers, response body, and accurate timing. No framework-level filtering or transformation.
4. **Clear separation of concerns.** The capture layer doesn't need to understand the app's internals. SQL queries and exceptions can be added later via a separate ingestion endpoint without changing the proxy architecture.

Alternatives considered:
- **Middleware/SDK injection**: richest data (sees SQL, exceptions, app context) but requires per-framework packages and code changes. Planned as a future layer via `POST /ingest` on the daemon, not the P0 capture mechanism.
- **Log tailing**: non-invasive but depends on the app having structured logging, is less real-time, and produces lossy data (logs rarely capture full request/response bodies).

**What would change our minds:** If the most common debugging scenarios require SQL query visibility and the ingestion API proves too friction-heavy for users, we'd prioritize middleware SDKs over additional proxy features.

---

## 2026-04-12 — Storage: BadgerDB with TTL

**Decision:** Captured requests are stored in BadgerDB with per-entry TTL (default 30 minutes) and a soft max entry count (default 1000). Data directory: `~/.flume/data/`.

**Why:** Matching scry's proven pattern. BadgerDB is:
- Pure Go (no CGO), which is a hard constraint.
- Fast for write-heavy workloads (LSM-tree, write-ahead log).
- Supports TTL natively (`Entry.WithTTL`), so expiry requires zero application-level bookkeeping.
- Battle-tested in the sibling project.

Alternatives considered:
- **In-memory ring buffer**: simpler but loses data on daemon restart. For a debugging tool, surviving a daemon restart matters — the user might restart flume to change flags, and losing the last 30 minutes of captured data is a bad experience.
- **SQLite (via modernc.org/sqlite)**: richer query semantics but heavier dependency, and the query patterns here (recent-N, filter by path/method/status) are simple enough that BadgerDB prefix iteration handles them well.

Key schema:
- `req:<timestamp_ns>:<ulid>` → JSON-serialized request record. Timestamp prefix enables efficient reverse-chronological iteration.

**What would change our minds:** If query patterns become complex enough to need joins or aggregation (e.g., "show me the average response time per endpoint over the last hour"), SQLite would be worth the migration. Current needs are simple list/filter/detail lookups.

---

## 2026-04-12 — Body size cap: 512 KB

**Decision:** Request and response bodies are captured up to 512 KB. Beyond that, the body is truncated and metadata fields (`request_body_truncated`, `request_body_orig_size`) are set.

**Why:** An AI agent's context window is the bottleneck. A 5 MB response body dumped into a tool result is useless — it would consume the entire context and still be truncated by the host. 512 KB is generous enough to capture most JSON API responses, HTML pages, and error messages, while preventing a single large file download from bloating the store.

**What would change our minds:** If common debugging scenarios involve large payloads where the relevant data is past the 512 KB mark (e.g., paginated API responses where the error is on page 50), we'd add a `--max-body-size` flag rather than raising the default.

---

## 2026-04-12 — ULID generation: hand-rolled

**Decision:** Request IDs are ULIDs (26-character Crockford base32, 48-bit millisecond timestamp + 80-bit random). Generated in `internal/daemon/ulid.go` (~50 lines) rather than pulling in `github.com/oklog/ulid/v2`.

**Why:** The implementation is trivial (timestamp + crypto/rand + base32 encode), and adding a dependency for 50 lines of code goes against the single-binary, minimal-deps philosophy shared across all sibling projects. ULIDs sort lexicographically by time, which makes BadgerDB prefix scans return results in chronological order naturally.

**What would change our minds:** If we needed monotonic ULID sequences within the same millisecond (for correctness guarantees under high concurrency), oklog/ulid's entropy pool would be worth the dependency. Current usage — one ULID per HTTP request — has no ordering constraints within a millisecond.

---

## 2026-04-12 — Daemon architecture: clone scry's pattern

**Decision:** Single daemon per user, Unix domain socket at `~/.flume/flumed.sock`, PID file at `~/.flume/flumed.pid`, hand-rolled JSON-RPC 2.0 over newline-delimited JSON, auto-spawn on first CLI call, MCP stdio server for Claude Code integration.

**Why:** This is the proven pattern from scry. No reason to diverge. The daemon model allows:
- Persistent state (captured requests survive across CLI invocations).
- Background operation (proxy runs continuously, captures traffic while the agent works).
- MCP integration (Claude Code launches `flume mcp` which dials the daemon).

The auto-spawn pattern is critical for UX: the user never needs to manually start the daemon. First `flume requests` or first MCP tool call auto-spawns it.

**What would change our minds:** If the daemon model proves problematic (e.g., orphaned daemons, port conflicts), we'd consider a per-invocation model where `flume mcp` runs an embedded proxy directly. But the scry daemon has been stable in production, so this is unlikely.

---

## 2026-04-12 — No zerolog: use fmt.Fprintf to stderr

**Decision:** Daemon logging uses `fmt.Fprintf(os.Stderr, ...)` rather than a structured logging library like zerolog.

**Why:** Despite CLAUDE.md mentioning zerolog as the standard logger, scry — the sibling project we're cloning patterns from — uses `fmt.Fprintf(os.Stderr, ...)` throughout. The daemon produces very few log lines (startup, shutdown, proxy errors), so a structured logging library adds dependency weight without meaningful benefit. Stdout is reserved for JSON data output; stderr is for human-readable status messages.

**What would change our minds:** If flume grows to need structured log parsing (e.g., a `flume logs` command that queries daemon logs), zerolog would be the right choice. Currently, the daemon log is a simple text file users can `tail -f`.
