---
name: zone-integrations
description: Zone Coder (Tier B) skill for GO_server external integrations in internal/telegram/ and internal/ai/. Enforces interface isolation, context timeouts, FLOOD_WAIT/429 handling, graceful degradation, and secret safety.
---

# Zone Coder: External Integrations (`GO_server`)

Implement scoped Telegram MTProto and LLM/AI client adapters strictly inside `internal/telegram/` and `internal/ai/`, following the active RFC and Research Brief.

## Zone Boundaries & Hard Prohibitions

- **Owned Paths:** only explicitly assigned files under `internal/telegram/` or `internal/ai/`, including `*_test.go`.
- **Forbidden:**
  - Never modify `internal/config/config.go` or `cmd/`; configuration and composition needs are integration blockers for the Orchestrator and human.
  - Never modify `internal/domain/`, `internal/storage/postgres/`, `internal/handler/`, `migrations/`, `go.mod`, or `Dockerfile`. Escalate cross-zone needs instead of expanding the zone.
  - **Never declare the task "Done" / "Готово"** — always hand off as `Status: HANDOFF`; final readiness requires independent Reviewer and Tester.
  - Never leak `gotd/td` or LLM HTTP types into `internal/usecase/` — implement interfaces declared in `internal/usecase/interfaces.go`.
  - Never log API keys, auth tokens, 2FA passwords, raw MTProto `accounts.session` bytes, or client phone numbers.

## Pre-Flight Check

1. Confirm you have the active RFC and Research Brief.
2. Check transport code and config patterns confirmed by Research. Reading `internal/config/config.go` is allowed for context, but editing it remains an integration blocker.

## Zone Rules & Repository Standards

### General Integration Invariants
- **Graceful Degradation by Default:** If optional credentials (`TELEGRAM_API_ID`/`TELEGRAM_API_HASH` or `AI_API_KEY`) are unset, `config.Must()` and server startup **must not crash**. Log clearly that the integration is disabled and return a typed error so handlers return `503 Service Unavailable`.
- **Timeouts & Cancellation:** Every outbound RPC or HTTP call must derive from the caller's `context.Context` with an explicit timeout (`AI_TIMEOUT`, per-RPC timeout). Always close response bodies (`defer resp.Body.Close()`).
- **No Unbounded Goroutines:** Background workers/queues must respect `ctx.Done()` and support graceful shutdown.

### Telegram MTProto (`internal/telegram/`)
- **Reuse Existing Proxy Transport:** Always reuse `createProxyDialer` and `bufferedConn` from `internal/telegram/client.go` (`TELEGRAM_PROXY`, `USE_PROXY` / `TELEGRAM_NO_PROXY`) — never rewrite proxy dialing from scratch. Keep `cmd/telegram-dev` compatibility intact.
- **Scope & Pagination:** Sync only private user dialogs (`PeerUser`), skipping groups, channels, and bots. Paginate `MessagesGetDialogs` (`OffsetDate`, `OffsetID`, `OffsetPeer`) and fetch history incrementally via `MinID`.
- **MTProto Error Handling:**
  - On `FLOOD_WAIT`, wait for the requested duration using `select` on `time.After(...)` and `ctx.Done()`.
  - Surface revoked/invalid sessions (to mark account `disconnected`) and account bans (to mark `banned`) cleanly without panicking.

### LLM / AI Adapter (`internal/ai/`)
- **Standard Library HTTP:** Implement OpenAI-compatible clients using Go's `net/http` with configured timeout — no extra third-party SDKs in `go.mod`.
- **Context Trimming & Privacy:** Format messages chronologically with explicit role labels (`Client` / `Manager`) and timestamps, trim oldest messages to `AI_MAX_MESSAGES` and `AI_MAX_CHARS`, and strip phone numbers.
- **Strict JSON & Validation:** Prompt for raw JSON only, safely strip accidental ```` ```json ... ``` ```` fences, unmarshal strictly, and validate all enum outputs (`Risk`: `low|medium|high`, `SuggestedStatus`, `AlertTypes`) before returning. On `429`, timeout, network failure, or malformed JSON, return an error without persisting partial/garbage data.

### Mandatory Unit Tests
- For `internal/ai/`: use `net/http/httptest.Server` to test valid JSON, fenced JSON, malformed JSON, `429 Too Many Requests`, and context timeout.
- For `internal/telegram/`: unit-test dialog/message mapping, `FLOOD_WAIT` cancellation, and session adapter behavior without requiring live Telegram credentials.

## Coder Handoff Format

Use the complete handoff from `docs/agents/roles/coder.md`; the fields below are zone-specific additions, not a replacement.

- **Changed Files:** exact list of created/modified files.
- **RFC Compliance:** mapping of each adapter/config field to RFC requirements.
- **Tests Added/Updated:** exact test names and failure modes verified (`httptest`, timeout, `429`/`FLOOD_WAIT`).
- **Status:** `HANDOFF` (or `BLOCKED` with the unmet precondition)
