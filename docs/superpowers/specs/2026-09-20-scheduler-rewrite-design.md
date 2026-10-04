# 設計：重寫背景排程以極大化空轉省資源

日期：2026-09-20  
分支：`refactor/reduce-resource-usage`  
狀態：草稿 — 等人審過後再寫實作計劃

## 意圖

在幾乎沒有訂閱工作時，大幅降低 Ptt-Alertor 的 CPU 與 Redis 負載，且不破壞通知語意與對外 HTTP API。

### 成功標準

- **Idle（無訂閱）**：app + Redis CPU 接近閒置；不抓 PTT 看板；HTTP API 仍可服務。
- **Active（有訂閱）**：多數通知在相符新文／推文／留言後約 **5–15 秒內**送出（輪詢週期約 **5–10 秒**）。
- Discord／Line／Telegram／Messenger／mail 既有發送行為與訊息形狀保持相容。
- Idle／Active 負載可觀測（狀態、佇列深度、耗時、追蹤看板數）。

### 預期負載降幅（估算，實作後驗證）

基準（改前、空的本機 Docker）：app 約 80% CPU、Redis 約 60% CPU、Redis 約 6 萬 `instantaneous_ops_per_sec`、keyspace hits 近乎 0。

| 模式 | 預期方向 | 建議驗收 |
|------|----------|----------|
| Idle（無訂閱） | 數量級下降；busy loop 應消失 | 穩定跑 1–2 分鐘後：app+Redis CPU 低（常見個位數％以下）；Redis ops/sec 近 0（頂多每 30–60 秒一次輕量「有沒有訂閱」） |
| Active（有訂閱） | 若 tick 為 5–10 秒，輪詢約為昔日 ~250ms 的 1/20～1/40；隨訂閱看板數與 worker 數放大 | 抽測通知延遲約 5–15 秒；負載高於 Idle，但遠低於「空庫就 80% CPU」 |
| 不會憑空消失的成本 | 正當 HTTP、真實抓 PTT／寫 Redis、對外頻道 I/O | 有真實工作时仍會發生 |

以上是**數量級目標**，不是 SLA。需以實測 `docker stats` + Redis `INFO` 確認空轉崩落後再視為完成。

### 非目標

- 更改頻道協定或公開 API 契約。
- 引入新的外部 broker（Kafka 等）。
- 追求亞秒級延遲。

## 現況問題（觀測）

空的本機 Docker 堆疊上：

- App 約 80% CPU；Redis 約 60% CPU、約 6 萬 ops/s，keyspace hits 近乎 0。
- 背景 job（`Checker`、`PushSumChecker`、`CommentChecker` 等）用亞秒級 sleep 迴圈並開大量 goroutine。
- `main` 的 `init()` 每次行程啟動都跑 CacheCleaner／Fetcher／MigrateDB／CategoryCleaner。
- 熱路徑／維護路徑出現 Redis `KEYS`。

## 做法

**C — 重寫排程**（相對「只打雙模式補丁」或「完整事件匯流排」）。

用中央 **Scheduler** + 行程內 work queue + 小型 worker pool，取代各 job 的 busy loop。**通知發送**（`jobs/check.go`、`channels/*`）維持不變：Scheduler 只把相容結果餵進既有 notify 入口。

## 架構

### 元件

1. **Scheduler** — 擁有 Idle／Active 狀態；決定何時入队。
2. **Work queue** — 行程內工作佇列。
3. **Worker pool** — 固定併發（預設 2–4）；執行抓取／比對；交接給既有 notify 路徑。
4. **HTTP server** — 對外表面不變；與 Scheduler 一併啟動（不與遷移 job 綁死）。
5. **Notify sink（不變）** — 既有 `check`／頻道發送管線。

### 工作項類型

- `RefreshBoard` — 抓看板文章、偵測新文、跑關鍵字／作者比對（目前在 Checker 管線內）。
- `CheckPushSum` — 有訂閱之看板／文章的推數相關檢查。
- `CheckComment` — 有訂閱文章的留言檢查。

實作時 Go 型別可把推數／留言收成共用「文章後續」工作項（若更貼近現碼）；行為須保留今日 notify 結果。

### 狀態

| 狀態 | 條件 | 行為 |
|------|------|------|
| Idle | 訂閱／追蹤看板數 = 0 | 不抓 PTT。可每 `SCHED_IDLE_POLL`（預設 30–60 秒）再確認「還有沒有訂閱」。只留 HTTP。 |
| Active | 至少一個訂閱／追蹤看板 | 依 `SCHED_ACTIVE_TICK`（預設約 5–10 秒）對有訂閱看板入队刷新。`BOARD_HIGH` 可用較短 tick，但不低於約 5 秒。 |

訂閱數由 0→非 0：Idle→Active（idle 輪詢，或訂閱 API／Bot 寫入後可選 ping）。回到 0：Active→Idle。

### 佇列與背壓

- 同一看板同時最多一個 in-flight 刷新。
- 佇列有界；滿了則丟棄／跳過最舊或本輪並打 log（禁止無界 goroutine 擴散）。
- Worker 比對後走**既有 notify 路徑**（等同今日 `cker.ch` 之後）；不可直接打 Discord／Line API。

### 訂閱發現

- Idle：定期輕量檢查是否有任何訂閱。
- Active：看板集合來自目前訂閱者（每 tick 或每 N 秒重算）。
- 可選：API／Bot 訂閱成功後 ping Scheduler（nice-to-have，v1 非必須）。

### Redis／儲存

- 看板／使用者持久化沿用現有 model。
- 熱路徑去掉 `KEYS`；改 `SCAN` 或維護 index set（專案已有 `SMEMBERS` 用法）。
- 一次性清理／遷移不可每次行程啟動都跑。

### 啟動與一次性工作

- `main` 啟動：logging、HTTP、Scheduler（以及今日既有的頻道 bot）。
- 將 `CacheCleaner`、`MigrateDB`、大批 `Fetcher` 等移出無條件 `init()`，改 CLI 子命令或明確 env 才跑。
- README 記載新的啟動預期。

### 錯誤與整站故障

- 單板抓取失敗：log + 該板短暫退避；不停止 Scheduler。
- PTT 整站異常：暫停入队（沿用／收斂 `PttMonitor`）；恢復後再 Active。

### 觀測

至少 log 或簡單 metrics：

- Scheduler 狀態（Idle／Active）
- 佇列長度
- Tick／工作耗時
- 成功／失敗次數
- 追蹤看板數

### 設定（env）

| 變數 | 用途 | 建議預設 |
|------|------|----------|
| `SCHED_IDLE_POLL` | Idle 檢查訂閱的間隔 | 30s–60s |
| `SCHED_ACTIVE_TICK` | Active 看板刷新週期 | 5s–10s |
| `SCHED_WORKERS` | Worker 池大小 | 2–4 |
| `BOARD_HIGH` | 高活躍看板（既有） | 名單不變；最短間隔 ≥ 約 5s |

### 測試

- 單元：佇列語意、Idle↔Active、單板 in-flight 上限。
- 整合：無訂閱 ⇒ 不抓 PTT；有訂閱 ⇒ 會呼叫既有 notify 入口（spy／fake；不必真打外網頻道）。
- 手動：Docker 空轉 `docker stats` + Redis `INFO` ops/sec 遠低於約 6 萬；`/boards` 仍 200。

### 可選風險控制

v1 可用 feature flag 切回舊 checkers（實作計劃時再決定要不要留）。

### 遷移／上線

1. 以清楚套件邊界實作 Scheduler。
2. `main` 接到 Scheduler；notify sink 不動。
3. 本機 Docker 驗證空轉指標與有訂閱通知。
4. 之後有憑證／額度再 push／PR（本設計文件範圍外）。

## 產品討論中的明確約束

- 優先**極大化空轉省資源**，而非最低延遲。
- 有訂閱時，通知延遲目標約 **5–15 秒**。
- 零訂閱：**不抓 PTT**；只留 HTTP API。
- 這次**不重寫** `jobs/check.go`／`channels/*` 發送邏輯。

## 留給實作計劃的未決點

- 套件配置（`jobs/scheduler` 或新的 `scheduler`）。
- 推數與留言是否共用一種 worker／佇列。
- v1 是否保留舊 checkers feature flag。
- 一次性遷移的具體 CLI 形狀。

## 核准

已於 2026-09-20 在對話中核准設計 §1–§3（含負載降幅驗收預估）。  
下一關：人審**本 written spec**，再以 `writing-plans` 產出實作計劃。計劃核准前不寫產品碼。
