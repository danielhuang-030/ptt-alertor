# 重寫背景排程 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 以中央 Scheduler + 有界佇列 + 小 worker pool 取代各 job 的 busy loop，讓無訂閱時接近空轉，有訂閱時約 5–15 秒內通知，且不改 `jobs/check.go`／`channels/*` 發送邏輯。

**Architecture:** 在既有 `package jobs` 新增 Scheduler（Idle／Active）、WorkQueue、固定 worker。抓板／比對後透過既有 `ckCh` 通知路徑送出（抽出小函式 `enqueueCheck`，行為等同今日 `cker.ch`→`ckCh`）。`main` 以 env `SCHED_LEGACY` 可切回舊 `Checker`／`PushSumChecker`／`CommentChecker`；預設走新排程。一次性 CacheCleaner／MigrateDB／Fetcher 改為 env 閘門，不再無條件 `init()`。

**Tech Stack:** Go 1.21+、既有 Redis models、Docker Compose 本機驗證、`testing` 標準庫。

**Spec:** `docs/superpowers/specs/2026-09-20-scheduler-rewrite-design.md`

## Global Constraints

- 無訂閱時不抓 PTT；只留 HTTP（與極低頻訂閱探針）。
- 有訂閱時通知延遲目標約 5–15 秒；`SCHED_ACTIVE_TICK` 預設 5–10 秒；`BOARD_HIGH` 最短不低於約 5 秒。
- 不重寫 `jobs/check.go` 的 `sendMessage`／`messageWorker`，不改 `channels/*` 對外發送。
- 同一看板最多一個 in-flight refresh；佇列有界，禁止無界 goroutine 扇出。
- 熱路徑禁止 Redis `KEYS`。
- 本機開發用既有 `docker-compose.dev.yml`（勿提交）；產品 compose 維持原檔。
- 提交前不 `git push`（除非使用者另指示）。

## Review Focus

- 空庫啟動後 1–2 分鐘仍不該對 PTT 發請求（除可觀測的訂閱探針）。
- Active→Idle 必須在訂閱歸零後停止抓板，不能卡在 Active。
- 佇列滿時必須丟棄／跳過並打 log，不可阻塞死或開爆 goroutine。
- 單板連續失敗只退避該板，其他板與 notify 路徑仍運作。
- `SCHED_LEGACY=1` 時行為應回到舊 checkers 啟動路徑，避免半新半舊雙跑。

## 檔案配置（先鎖定）

| 檔案 | 職責 |
|------|------|
| `jobs/scheduler_config.go` | 讀 `SCHED_*`／`BOARD_HIGH` 預設與解析 |
| `jobs/scheduler_queue.go` | 有界佇列、per-board in-flight |
| `jobs/scheduler.go` | Idle／Active、tick、worker pool、Stop |
| `jobs/scheduler_work.go` | `RefreshBoard`／後續推數留言工作；呼叫既有抓板與比對；`enqueueCheck` |
| `jobs/scheduler_subs.go` | 是否有訂閱、有訂閱看板集合（避免 `KEYS`） |
| `jobs/scheduler_*_test.go` | 上述單元測試 |
| `jobs/check.go` | **僅**抽出／匯出 `enqueueCheck(Checker)`（或同等）供 Scheduler 寫入 `ckCh`；不改 `sendMessage` |
| `jobs/oneshot.go` | 將原 `init()` 一次性工作改 env 閘門 |
| `main.go` | 啟動 Scheduler（或 legacy）；保留 HTTP／頻道 bot |
| `README.md` | 記載新 env 與啟動行為 |
| `.env.example` | 補上 `SCHED_*`、`SCHED_LEGACY` |

---

### Task 1: Scheduler 設定解析

**Files:**
- Create: `jobs/scheduler_config.go`
- Test: `jobs/scheduler_config_test.go`
- Modify: `.env.example`

**Interfaces:**
- Consumes: `os.Getenv`
- Produces: `type SchedulerConfig struct { IdlePoll, ActiveTick time.Duration; Workers, QueueSize int; HighBoards []string; HighTick time.Duration; Legacy bool }`；`func LoadSchedulerConfig() SchedulerConfig`

- [ ] **Step 1: 寫失敗測試**

```go
func TestLoadSchedulerConfigDefaults(t *testing.T) {
    t.Setenv("SCHED_IDLE_POLL", "")
    t.Setenv("SCHED_ACTIVE_TICK", "")
    t.Setenv("SCHED_WORKERS", "")
    t.Setenv("SCHED_QUEUE_SIZE", "")
    t.Setenv("SCHED_LEGACY", "")
    t.Setenv("BOARD_HIGH", "")
    cfg := LoadSchedulerConfig()
    if cfg.IdlePoll < 30*time.Second || cfg.IdlePoll > 60*time.Second {
        t.Fatalf("IdlePoll default out of range: %v", cfg.IdlePoll)
    }
    if cfg.ActiveTick < 5*time.Second || cfg.ActiveTick > 10*time.Second {
        t.Fatalf("ActiveTick default out of range: %v", cfg.ActiveTick)
    }
    if cfg.Workers < 2 || cfg.Workers > 4 {
        t.Fatalf("Workers default out of range: %d", cfg.Workers)
    }
    if cfg.Legacy {
        t.Fatal("Legacy should default false")
    }
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./jobs -run TestLoadSchedulerConfigDefaults -count=1`  
Expected: FAIL（未定義 `LoadSchedulerConfig`）

- [ ] **Step 3: 最小實作**

在 `scheduler_config.go` 實作預設：`IdlePoll=30s`、`ActiveTick=5s`、`Workers=2`、`QueueSize=256`、`HighTick=max(5s, min(ActiveTick, 5s))`；解析 duration（支援 `30s`）與 int；`SCHED_LEGACY` 為 `1`/`true` 時 Legacy=true；`BOARD_HIGH` 逗號分隔小寫 trim。

- [ ] **Step 4: 測試通過**

Run: `go test ./jobs -run TestLoadSchedulerConfig -count=1`  
Expected: PASS（含 override 測試：自行再加 `TestLoadSchedulerConfigOverrides`）

- [ ] **Step 5: Commit**

```bash
git add jobs/scheduler_config.go jobs/scheduler_config_test.go .env.example
git commit -m "feat(jobs): add scheduler config defaults and env parsing"
```

---

### Task 2: 有界佇列與 per-board in-flight

**Files:**
- Create: `jobs/scheduler_queue.go`
- Test: `jobs/scheduler_queue_test.go`

**Interfaces:**
- Consumes: Task 1 無硬相依
- Produces:
  - `type WorkKind int`（至少 `WorkRefreshBoard`）
  - `type WorkItem struct { Kind WorkKind; Board string }`
  - `type WorkQueue struct { ... }`
  - `func NewWorkQueue(size int) *WorkQueue`
  - `func (q *WorkQueue) TryEnqueue(item WorkItem) bool` — 滿或同板 in-flight／已在佇列則 false
  - `func (q *WorkQueue) MarkDone(board string)`
  - `func (q *WorkQueue) Pop(ctx context.Context) (WorkItem, bool)`
  - `func (q *WorkQueue) Len() int`

- [ ] **Step 1: 寫失敗測試**

```go
func TestWorkQueueRejectsDuplicateBoard(t *testing.T) {
    q := NewWorkQueue(8)
    ok1 := q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "gossiping"})
    ok2 := q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "gossiping"})
    if !ok1 || ok2 {
        t.Fatalf("first=%v second=%v want true,false", ok1, ok2)
    }
}

func TestWorkQueueBounded(t *testing.T) {
    q := NewWorkQueue(1)
    if !q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "a"}) {
        t.Fatal("first should enqueue")
    }
    if q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "b"}) {
        t.Fatal("second should fail when full")
    }
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./jobs -run 'TestWorkQueue' -count=1`  
Expected: FAIL

- [ ] **Step 3: 最小實作**

用 buffered channel + `map[string]struct{}` 追蹤 queued／in-flight；`Pop` 時標 in-flight；`MarkDone` 清除。

- [ ] **Step 4: 測試通過**

Run: `go test ./jobs -run 'TestWorkQueue' -count=1`  
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add jobs/scheduler_queue.go jobs/scheduler_queue_test.go
git commit -m "feat(jobs): add bounded work queue with per-board inflight"
```

---

### Task 3: Idle／Active 狀態機（不含真實抓板）

**Files:**
- Create: `jobs/scheduler.go`（狀態與 tick 骨架）
- Test: `jobs/scheduler_state_test.go`

**Interfaces:**
- Consumes: `SchedulerConfig`、`WorkQueue`
- Produces:
  - `type SchedulerState int`（`StateIdle`、`StateActive`）
  - `type Scheduler struct`
  - `func NewScheduler(cfg SchedulerConfig, hasSubs func() bool, boards func() []string) *Scheduler`
  - `func (s *Scheduler) State() SchedulerState`
  - `func (s *Scheduler) TickOnce(now time.Time)` — 可測：依 hasSubs 切換狀態；Active 時對 boards 做 TryEnqueue
  - `func (s *Scheduler) QueueLen() int`

- [ ] **Step 1: 寫失敗測試**

```go
func TestSchedulerIdleSkipsEnqueue(t *testing.T) {
    s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
        func() bool { return false },
        func() []string { return []string{"gossiping"} },
    )
    s.TickOnce(time.Now())
    if s.State() != StateIdle {
        t.Fatalf("state=%v", s.State())
    }
    if s.QueueLen() != 0 {
        t.Fatalf("queue=%d want 0", s.QueueLen())
    }
}

func TestSchedulerActiveEnqueuesBoards(t *testing.T) {
    s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
        func() bool { return true },
        func() []string { return []string{"gossiping", "lol"} },
    )
    s.TickOnce(time.Now())
    if s.State() != StateActive {
        t.Fatalf("state=%v", s.State())
    }
    if s.QueueLen() != 2 {
        t.Fatalf("queue=%d want 2", s.QueueLen())
    }
}
```

- [ ] **Step 2: 跑測試確認失敗**

Run: `go test ./jobs -run 'TestScheduler' -count=1`  
Expected: FAIL

- [ ] **Step 3: 最小實作**

`TickOnce`：若 `!hasSubs` → Idle 且不入队；否則 Active，對每個 board `TryEnqueue(RefreshBoard)`。尚不開 goroutine worker。

- [ ] **Step 4: 測試通過 + 補 Active→Idle**

再加測試：先有訂閱入队，再 `hasSubs=false` 的下一次 Tick 狀態變 Idle 且不再新增工作。

- [ ] **Step 5: Commit**

```bash
git add jobs/scheduler.go jobs/scheduler_state_test.go
git commit -m "feat(jobs): add scheduler idle/active tick without fetch"
```

---

### Task 4: 訂閱探測（避免 KEYS）

**Files:**
- Create: `jobs/scheduler_subs.go`
- Test: `jobs/scheduler_subs_test.go`（能 mock 就 mock；若強依賴 Redis，以介面注入）

**Interfaces:**
- Consumes: 既有 `models`／keyword／author／user 查詢（只讀）
- Produces:
  - `type SubIndex interface { HasAny() bool; SubscribedBoards() []string }`
  - `func NewRedisSubIndex() SubIndex`（實作須用 SMEMBERS／既有 index，**禁止 KEYS**）
  - 單元測試可用 fake：`type fakeSubIndex struct{ any bool; boards []string }`

- [ ] **Step 1: 寫失敗測試（fake 接上 Scheduler）**

```go
func TestSchedulerUsesSubIndex(t *testing.T) {
    idx := &fakeSubIndex{any: true, boards: []string{"soft_job"}}
    s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
        idx.HasAny, idx.SubscribedBoards)
    s.TickOnce(time.Now())
    if s.QueueLen() != 1 {
        t.Fatalf("queue=%d", s.QueueLen())
    }
}
```

- [ ] **Step 2–4: 實作介面與 fake；Redis 實作若需整合測可標 `//go:build integration` 或延後 Task 7**

先保證介面 + fake 讓 Scheduler 可測。Redis 實作：優先讀既有「有訂閱的看板」集合；若今日只能 `KEYS user:*`，則**新增維護** `subscribed_boards` set（訂閱寫入時 SADD）——若改動訂閱寫入面太大，v1 可暫用「掃描使用者但快取結果 + Idle 才更新」並在計劃註記技術債；**熱路徑仍禁止每次 tick KEYS**。

- [ ] **Step 5: Commit**

```bash
git add jobs/scheduler_subs.go jobs/scheduler_subs_test.go
git commit -m "feat(jobs): add subscription index for scheduler"
```

---

### Task 5: 抽出 `enqueueCheck`（發送層不變）

**Files:**
- Modify: `jobs/check.go`（僅加匯出函式）
- Modify: `jobs/checker.go`（改呼叫 `enqueueCheck`，行為不變）
- Test: `jobs/enqueue_check_test.go`

**Interfaces:**
- Produces: `func enqueueCheck(c Checker)` — 非阻塞或與今日相同地送入 `ckCh`（今日是 `ckCh <- cker`）

- [ ] **Step 1: 寫測試** — 用短超時從測試用 channel 難直接讀未匯出 `ckCh`；改為：

```go
func TestEnqueueCheckDoesNotPanic(t *testing.T) {
    // messageWorker 已在 init 吃掉 ckCh；送一筆空 Checker 不應死鎖超過 1s
    done := make(chan struct{})
    go func() {
        enqueueCheck(Checker{})
        close(done)
    }()
    select {
    case <-done:
    case <-time.After(2 * time.Second):
        t.Fatal("enqueueCheck blocked")
    }
}
```

- [ ] **Step 2–4: 實作 `enqueueCheck`；Checker.Run 內 `ckCh <- cker` 改為 `enqueueCheck(cker)`**

- [ ] **Step 5: Commit**

```bash
git add jobs/check.go jobs/checker.go jobs/enqueue_check_test.go
git commit -m "refactor(jobs): export enqueueCheck for scheduler notify handoff"
```

---

### Task 6: Worker 執行 RefreshBoard → 既有比對 → enqueueCheck

**Files:**
- Create: `jobs/scheduler_work.go`
- Test: `jobs/scheduler_work_test.go`（注入 fetch／match stub）

**Interfaces:**
- Consumes: `WorkQueue`、`enqueueCheck`、既有 `board.WithNewArticles`／keyword／author 檢查函式（可先抽 thin wrapper）
- Produces:
  - `func (s *Scheduler) Start(ctx context.Context)`
  - `func (s *Scheduler) Stop()`
  - worker：Pop → 處理 → `MarkDone`；panic／error 時也要 MarkDone

- [ ] **Step 1: 寫失敗測試（stub board 處理）**

用介面：

```go
type BoardRefresher interface {
    Refresh(board string) error
}
```

測試：Enqueue 一板 → Start → 短等 → Refresh 被呼叫一次 → MarkDone 後可再入队同板。

- [ ] **Step 2–4: 實作 worker pool；真實 `Refresh` 呼叫現有抓板＋`checkKeywordSubscriber`／`checkAuthorSubscriber` 路徑中「產生 Checker 並 enqueueCheck」的邏輯（複製必要片段到 `scheduler_work.go` 或改成 package 內可呼叫函式，避免改 sendMessage）**

PushSum／Comment：本 Task 先做 RefreshBoard；下一 Task 再接。

- [ ] **Step 5: Commit**

```bash
git add jobs/scheduler_work.go jobs/scheduler_work_test.go jobs/scheduler.go
git commit -m "feat(jobs): run scheduler workers for board refresh"
```

---

### Task 7: 接上 PushSum／Comment 為後續工作項

**Files:**
- Modify: `jobs/scheduler_queue.go`（`WorkCheckPushSum`、`WorkCheckComment`）
- Modify: `jobs/scheduler_work.go`
- Modify: `jobs/scheduler.go`（Active tick 時一併為有訂閱目標入队，或 refresh 成功後衍生）
- Test: 延伸既有測試

**策略（鎖定）：** RefreshBoard 完成且有新文／既有追蹤需求時，再 `TryEnqueue` 對應 follow-up；不要用舊的 500ms／獨立 sleep 迴圈。

- [ ] 實作並測試：無訂閱時不入队 follow-up；有 stub 時 follow-up 會跑並 `enqueueCheck`。
- [ ] Commit: `feat(jobs): schedule pushsum/comment as follow-up work`

---

### Task 8: 一次性 job 離開無條件 init；main 接線

**Files:**
- Create: `jobs/oneshot.go`
- Modify: `main.go`
- Modify: `README.md`
- Modify: `.env.example`

**行為：**

- 將 `main.go` 的 `init()` 內 `NewCacheCleaner`／`NewFetcher`／`NewMigrateDB`／`NewCategoryCleaner`／`NewMigrateBoard`／`NewTop`／`NewPushSumKeyReplacer` 等，改為僅當 `RUN_ONESHOT_JOBS=1`（或個別旗標）時執行；預設啟動不跑。
- `startJobs()`：若 `LoadSchedulerConfig().Legacy` → 舊 `NewChecker`／`NewPushSumChecker`／`NewCommentChecker`／`NewPttMonitor`；否則 `NewScheduler(...).Start` + 保留 `PttMonitor`（站掛時暫停 Scheduler 入队——接上 `Pause`／`Resume` 方法）。
- **禁止** Legacy=false 時仍啟動舊 Checker（Review Focus：雙跑）。

- [ ] **測試：** `go test ./jobs -count=1`；手動：

```bash
sg docker -c 'docker compose -f docker-compose.dev.yml --env-file .env up -d --build'
# 空轉 1–2 分鐘
sg docker -c 'docker stats --no-stream'
sg docker -c 'docker exec pttalertor-redis redis-cli info stats' | rg instantaneous_ops
curl -sS -m 5 -w '\n%{http_code}\n' http://127.0.0.1:9090/boards
```

Expected: ops/sec 遠低於 ~60k；`/boards` 200；app CPU 明顯下降。

- [ ] Commit: `feat: wire scheduler by default and gate oneshot jobs`

---

### Task 9: 觀測 log 與文件收尾

**Files:**
- Modify: `jobs/scheduler.go`（定期 Info：state、queue len、tracked boards）
- Modify: `README.md`（正體中文或雙語：新 env、Idle／Active、驗收方式）

- [ ] Commit: `docs: document scheduler env and idle acceptance checks`

---

### Task 10: 全量驗證與分支整理

- [ ] `go test ./...`（允許既有失敗則記錄；新增測試必須過）
- [ ] 重複 Task 8 手動空轉驗收，把數字貼進 PR／交付說明
- [ ] 確認 `docker-compose.dev.yml` 仍 untracked
- [ ] 不 push，除非使用者要求

---

## Spec 對照自檢

| Spec 要求 | Task |
|-----------|------|
| Idle 不抓 PTT | 3, 4, 6, 8 |
| Active 5–10s／通知 5–15s | 1, 3, 6 |
| 發送層不變 | 5 |
| 有界佇列／per-board | 2 |
| 去掉啟動時 oneshot | 8 |
| 禁止熱路徑 KEYS | 4 |
| 觀測 | 9 |
| 可選 Legacy flag | 1, 8 |
| PttMonitor 暫停入队 | 8 |
| 負載驗收 | 8, 10 |

## 執行交接

Plan complete and saved to `docs/superpowers/plans/2026-09-20-scheduler-rewrite.md`. Please review the plan. Which execution approach would you prefer?

- **Subagent-driven（建議）** — 每個 task 用新的子代理實作，再經審查才進下一個；最穩，較花時間。
- **Native** — 我在這個對話裡依序實作全部 task，最後再做一次整支分支審查；较快，適合本機無 Cloud Agent 額度時。

**建議：Native**，因為 Cloud Agent 額度已用完，且我們本來就在本機分支開發。
