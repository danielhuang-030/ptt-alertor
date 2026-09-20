# Design: Rewrite background scheduler for extreme idle savings

Date: 2026-09-20  
Branch: `refactor/reduce-resource-usage`  
Status: draft — awaiting human review before implementation plan

## Intent

Reduce CPU and Redis load when Ptt-Alertor has little or no subscription work, without breaking notification semantics or the public HTTP API.

### Success criteria

- **Idle (no subscriptions):** app + Redis CPU near idle; no PTT board fetching; HTTP API still serves.
- **Active (has subscriptions):** most notifications within about **5–15 seconds** of a matching new article / push / comment event (poll cadence about **5–10 seconds**).
- Existing Discord / Line / Telegram / Messenger / mail send path behavior and message shape stay compatible.
- Idle and active load are observable (state, queue depth, timings, tracked board count).

### Expected load reduction (estimate, verify after implement)

Baseline (empty local Docker, before change): app ~80% CPU, Redis ~60% CPU, Redis ~60k `instantaneous_ops_per_sec`, near-zero keyspace hits.

| Mode | Expected direction | Suggested acceptance checks |
|------|--------------------|-----------------------------|
| Idle (no subs) | Order-of-magnitude drop; busy-loop work should disappear | After 1–2 minutes steady: app+Redis CPU low (often single-digit % or less); Redis ops/sec near 0 (at most a light subscription check every 30–60s) |
| Active (has subs) | Polling ~1/20–1/40 of former ~250ms loops if tick is 5–10s; scales with subscribed board count and worker count | Spot-check notification delay ~5–15s; load above idle but far below “empty stack at 80% CPU” |
| Unchanged cost | Legitimate HTTP, real PTT fetch/Redis writes, outbound channel I/O | Still expected when work is real |

These are **order-of-magnitude targets**, not SLAs. Ship only after measured `docker stats` + Redis `INFO` confirm idle collapse.

### Non-goals

- Changing channel protocols or public API contracts.
- Introducing new external brokers (Kafka, etc.).
- Perfect sub-second latency.

## Current problems (observed)

On an empty local Docker stack:

- App ~80% CPU; Redis ~60% CPU and ~60k ops/sec with near-zero keyspace hits.
- Background jobs (`Checker`, `PushSumChecker`, `CommentChecker`, etc.) use sub-second sleep loops and spawn many goroutines.
- `main` `init()` runs CacheCleaner / Fetcher / MigrateDB / CategoryCleaner on every process start.
- Redis `KEYS` appears on hot/maintenance paths.

## Approach

**C — Rewrite the scheduler** (chosen over dual-mode patch-only or full event-bus).

Replace per-job busy loops with one central **Scheduler** + in-memory work queue + small worker pool. Keep **notification sending** (`jobs/check.go`, `channels/*`) unchanged: the scheduler only feeds the existing notify entry points with compatible results.

## Architecture

### Components

1. **Scheduler** — owns Idle / Active state; decides when to enqueue work.
2. **Work queue** — in-process queue of work items.
3. **Worker pool** — fixed concurrency (default 2–4); executes fetch/match; hands off to existing notify path.
4. **HTTP server** — unchanged surface; starts with Scheduler (not with migration jobs).
5. **Notify sink (unchanged)** — existing `check` / channel send pipeline.

### Work item types

- `RefreshBoard` — fetch board articles, detect new articles, run keyword/author matching (logic currently inside Checker pipeline).
- `CheckPushSum` — push-sum related checks for subscribed boards/articles.
- `CheckComment` — comment checks for subscribed articles.

Exact Go types may group push/comment under a shared “article follow-up” item if that matches existing code more cleanly; behavior must preserve today’s notify outcomes.

### States

| State | Condition | Behavior |
|-------|-----------|----------|
| Idle | Subscription / tracked-board count is 0 | No PTT fetch. Optionally re-check “any subscriptions?” every `SCHED_IDLE_POLL` (default 30–60s). HTTP only. |
| Active | At least one subscription / tracked board | Enqueue refresh work for subscribed boards on `SCHED_ACTIVE_TICK` (default ~5–10s). `BOARD_HIGH` may use a shorter tick but not below ~5s. |

Transition Idle → Active when subscription count becomes non-zero (idle poll or optional ping after subscription API/Bot write). Active → Idle when count returns to zero.

### Queue and backpressure

- At most one in-flight refresh per board.
- Bounded queue; when full, drop/skip oldest or current cycle and log (never unbounded goroutine fan-out).
- Workers call into the **existing notify path** after match (equivalent to today’s post-`cker.ch` behavior). They must not call Discord/Line APIs directly.

### Subscription discovery

- Idle: periodic light check for any subscriptions.
- Active: board set derived from current subscribers (recomputed each tick or every N seconds).
- Optional: after successful subscribe via API/Bot, ping Scheduler (nice-to-have, not required for v1).

### Redis / storage

- Keep existing board/user models for persistence.
- Remove `KEYS` from hot paths; use `SCAN` or maintain index sets (project already uses `SMEMBERS` in places).
- One-shot cleaners/migrations must not run on every process start.

### Startup and one-shot jobs

- `main` starts: logging, HTTP, Scheduler (and existing channel bots as today).
- Move `CacheCleaner`, `MigrateDB`, bulk `Fetcher`, etc. out of unconditional `init()` into CLI subcommands or explicit env-gated one-shot runs.
- Document the new startup expectations in README.

### Errors and site outage

- Per-board fetch failure: log + short backoff for that board; do not stop the scheduler.
- Site-wide PTT failure: pause enqueue (reuse/adapt `PttMonitor` idea); resume when PTT is healthy again.

### Observability

Log or simple metrics at least:

- Scheduler state (Idle / Active)
- Queue length
- Tick / job duration
- Success / failure counts
- Number of tracked boards

### Configuration (env)

| Variable | Purpose | Suggested default |
|----------|---------|-------------------|
| `SCHED_IDLE_POLL` | How often Idle checks for subscriptions | 30s–60s |
| `SCHED_ACTIVE_TICK` | Active board refresh cadence | 5s–10s |
| `SCHED_WORKERS` | Worker pool size | 2–4 |
| `BOARD_HIGH` | High-activity boards (existing) | unchanged list; min interval ≥ ~5s |

### Testing

- Unit: queue semantics, Idle ↔ Active transitions, per-board in-flight limit.
- Integration: no subscriptions ⇒ no PTT fetch; with subscriptions ⇒ existing notify entry is invoked (spy/fake; no real outbound channel calls required).
- Manual: Docker idle `docker stats` + Redis `INFO` ops/sec much lower than ~60k; `/boards` still 200.

### Optional risk control

Feature flag to fall back to legacy checkers for v1 rollback if needed (decide during planning whether to keep).

### Migration / rollout

1. Implement Scheduler behind clear package boundaries.
2. Wire `main` to Scheduler; leave notify sink untouched.
3. Verify idle metrics and active notify with local Docker.
4. Push/PR later when credentials/quota allow (out of scope for this design doc).

## Explicit constraints from product discussion

- Prefer **extreme idle savings** over minimum latency.
- With subscriptions, target **~5–15s** notification delay.
- With zero subscriptions: **do not fetch PTT**; HTTP API only.
- **Do not rewrite** `jobs/check.go` / `channels/*` send logic in this effort.

## Open points for the implementation plan

- Exact package layout (`jobs/scheduler` vs new `scheduler` package).
- Whether push-sum and comment share one worker type or separate queues.
- Whether to keep a legacy feature flag in v1.
- Concrete CLI shape for one-shot migrations.

## Approval

Human approved chat design sections §1–§3 on 2026-09-20, including estimated load-reduction acceptance checks.  
Next gate: human reviews **this written spec**, then `writing-plans` produces the implementation plan. No product code until the plan is approved.
