# Ptt-Alertor

<img align="right" src="https://raw.githubusercontent.com/Ptt-Alertor/ptt-alertor/master/logo.jpg">

[![Build Status](https://github.com/Ptt-Alertor/ptt-alertor/actions/workflows/main.yml/badge.svg)](https://github.com/Ptt-Alertor/ptt-alertor/actions/workflows/main.yml)
[![codecov](https://codecov.io/gh/Ptt-Alertor/ptt-alertor/branch/master/graph/badge.svg)](https://codecov.io/gh/Ptt-Alertor/ptt-alertor)
[![Go Report Card](https://goreportcard.com/badge/github.com/Ptt-Alertor/ptt-alertor)](https://goreportcard.com/report/github.com/Ptt-Alertor/ptt-alertor)
[![Code Climate](https://api.codeclimate.com/v1/badges/f7047295fce56a0465dc/maintainability)](https://codeclimate.com/github/Ptt-Alertor/ptt-alertor/maintainability)
[![StackShare](https://img.shields.io/badge/tech-stack-0690fa.svg?style=flat)](https://stackshare.io/ptt-alertor/ptt-alertor)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

## API

### Board

* GET /boards

* GET /boards/[board name]/articles

* GET /boards/[board name]/articles/[article code]

### Keyword

* GET /keyword/boards

### Author

* GET /author/boards

### PushSum

* GET /pushsum/boards

### Articles

* GET /articles

### User (Auth)

* GET /users

* GET /users/[account]

* POST /users

```json
{
    "profile":{
        "account": "sample",
        "email":"sample@mail.com"
    },
    "subscribes":[
        {
            "board":"gossiping",
            "keywords":["問卦","爆卦","公告"]
        },
        {
            "board":"lol",
            "keywords":["閒聊"]
        }
    ]
}
```

* PUT /users/[account]

```json
{
    "profile":{
        "account": "sample",
        "email":"sample@mail.com"
    },
    "subscribes":[]
}
```

## 環境變數 (Environment Variables)

Ptt Alertor 的許多功能可以透過環境變數進行配置。以下是一些主要的設定選項：

(此處未來可以補充其他服務相關的環境變數，如 `PTT_ALERTOR_HOST_PORT`, `AUTH_USER`, `AUTH_PW` 等。)

### 看板資料儲存後端設定 (Board Data Storage Backend)

Ptt Alertor 看板資料（如文章列表）的**預設儲存方式是使用 Redis (`redis`)**。當您選擇使用 Redis 作為儲存後端（包括預設情況）或快取時，**請務必確保 `REDIS_HOST_PORT` 環境變數已正確設定**，否則服務可能無法啟動或正常運作。您可以根據您的部署環境和需求，透過以下環境變數選擇不同的儲存後端。

*   **`BOARD_DRIVER_TYPE`**:
    *   **說明**: 設定看板主要資料（文章列表等）的儲存驅動程式。**如果此環境變數未設定，系統將預設採用 `redis` 驅動**。
    *   **可選值**:
        *   `redis`: (預設選項) 使用 Redis 儲存。文章資料將以 JSON 字串格式儲存在 Redis 中。選擇此選項時（或在預設情況下），**必須確保 Redis 服務 (`REDIS_HOST_PORT`) 已正確設定且 Ptt Alertor 可以連線**。
        *   `file`: 使用伺服器本地檔案系統儲存。資料將以 JSON 格式儲存在 `BOARD_FILE_STORAGE_PATH` 指定的路徑下（若未指定 `BOARD_FILE_STORAGE_PATH`，則使用預設路徑）。
        *   `dynamodb`: 使用 AWS DynamoDB 作為儲存後端。選擇此選項時，**必須確保 AWS 相關環境變數 (如 `AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) 已正確設定**，否則可能導致服務啟動失敗或運行時錯誤。
    *   **範例**: `BOARD_DRIVER_TYPE=file` (若要改用檔案儲存)

*   **`BOARD_FILE_STORAGE_PATH`**:
    *   **說明**: 此變數僅在 `BOARD_DRIVER_TYPE` 明確設定為 `file` 時使用，用以指定看板資料檔案的儲存目錄路徑。
    *   如果此變數未設定（且 `BOARD_DRIVER_TYPE` 設定為 `file`），系統將使用一個內建的預設路徑 (例如：`./storage/board_articles/`)。
    *   **範例**: `BOARD_FILE_STORAGE_PATH=/app/data/board_data`

*   **`BOARD_CACHER_TYPE`**:
    *   **說明**: 設定看板列表及看板是否存在等快取資訊的儲存驅動程式。
    *   **可選值**:
        *   `redis` (預設且目前唯一選項): 使用 Redis 進行快取。選擇此選項時，**必須確保 Redis 服務 (`REDIS_HOST_PORT`) 已正確設定且 Ptt Alertor 可以連線**。
    *   **範例**: `BOARD_CACHER_TYPE=redis` (通常保持預設即可)

**重要提示**:
*   **Redis 設定**: 當使用 `redis` 作為 `BOARD_DRIVER_TYPE` (預設情況) 或 `BOARD_CACHER_TYPE` (預設情況) 時，**`REDIS_HOST_PORT` 環境變數的正確設定至關重要**。請確認 Redis 服務可正常連線。
*   **DynamoDB 設定**: 若選擇 `dynamodb` 作為 `BOARD_DRIVER_TYPE`，請務必檢查您的 AWS 環境變數設定 (如 `AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`)。
*   **檔案儲存設定**: 若選擇 `file` 作為 `BOARD_DRIVER_TYPE`，請考慮設定 `BOARD_FILE_STORAGE_PATH`。

## 頻道支援

Ptt-Alertor 目前支援透過以下頻道接收 PTT 文章通知：

*   Email
*   Line
*   Facebook Messenger
*   Telegram
*   Discord

### Discord 通知設定

若要啟用 Ptt-Alertor 的 Discord 通知功能，您需要先建立一個 Discord 應用程式及 Bot，然後獲取必要的憑證並設定 Ptt-Alertor。

#### 步驟 1：建立 Discord 應用程式與 Bot

1.  **前往 Discord Developer Portal**:
    *   開啟瀏覽器，進入 [Discord Developer Portal](https://discord.com/developers/applications)。
    *   使用您的 Discord 帳號登入。

2.  **建立新應用程式 (New Application)**:
    *   點擊右上角的 "New Application" 按鈕。
    *   為您的應用程式命名（例如 "PttAlertorBot" 或您喜歡的任何名稱），並同意 Discord 的開發者服務條款。
    *   點擊 "Create"。

3.  **建立 Bot 使用者**:
    *   在應用程式頁面，點擊左側導覽列中的 "Bot" 標籤。
    *   點擊 "Add Bot" 按鈕，並在彈出的確認視窗中選擇 "Yes, do it!"。

4.  **獲取 Bot Token (`DISCORD_BOT_TOKEN`)**:
    *   在 "Bot" 頁面，您會看到一個名為 "TOKEN" 的區塊（通常在 Bot 使用者名稱下方）。
    *   您可以點擊 "Reset Token" 來產生一個新的 Token (如果您之前已產生過)，或者點擊 "Copy" 直接複製現有的 Token。
    *   **此 Token 非常重要且應視為機密資訊**。將此 Token 複製下來，它將用於設定 Ptt-Alertor 的 `DISCORD_BOT_TOKEN` 環境變數。

#### 步驟 2：啟用特權閘道意圖 (Privileged Gateway Intents)

為了讓 Bot 能夠接收並處理頻道中的訊息內容 (非斜線指令或 `@mention` 的一般訊息)，您需要啟用特權閘道意圖。

1.  **前往 Bot 設定頁面**:
    *   在您的應用程式的 "Bot" 標籤頁面中。

2.  **啟用 Message Content Intent**:
    *   向下捲動到 "Privileged Gateway Intents" 區塊。
    *   找到並啟用 (開啟) **"MESSAGE CONTENT INTENT"**。
    *   **注意**: Discord 要求啟用此意圖的 Bot 可能需要通過驗證（如果 Bot 加入超過 100 個伺服器）。對於個人使用或小型社群，通常不會立即遇到此限制。

3.  **(可選) 其他意圖**:
    *   "PRESENCE INTENT" (顯示 Bot 的線上狀態) 和 "SERVER MEMBERS INTENT" (獲取伺服器成員列表) 目前對於 Ptt-Alertor 的核心功能不是必需的，但未來若有進階功能可能會用到。您可以暫時不啟用它們。

#### 步驟 3：邀請 Bot 到您的伺服器 (OAuth2)

1.  **前往 OAuth2 URL Generator**:
    *   在您的應用程式頁面，點擊左側導覽列中的 "OAuth2" 標籤，然後選擇其下的 "URL Generator" 子標籤。

2.  **設定邀請範圍與權限**:
    *   在 "SCOPES" 區塊，勾選 `bot`。
    *   勾選 `bot` 後，下方會出現 "BOT PERMISSIONS" 區塊。在此區塊中，勾選 Ptt-Alertor Bot 所需的權限。建議至少包含：
        *   `View Channels` (讀取訊息/查看頻道)：允許 Bot 讀取頻道中的指令。
        *   `Send Messages` (傳送訊息)：允許 Bot 發送通知和指令回覆。
        *   `Embed Links` (嵌入連結)：允許 Bot 發送的訊息中包含預覽效果更佳的嵌入式連結。
        *   `Attach Files` (附加檔案)：(可選) 未來若支援透過 Bot 發送圖片或其他檔案時可能需要。

3.  **產生並使用邀請連結**:
    *   設定好權限後，複製頁面底部 "GENERATED URL" 區塊中的連結。
    *   將此連結提供給擁有您目標 Discord 伺服器管理權限的使用者（或者如果您自己就是管理員，則自行開啟）。
    *   在瀏覽器中開啟此 OAuth2 URL，選擇要將 Bot 加入的伺服器，然後點擊 "Authorize" (授權) 並完成相關步驟。

#### 步驟 4：設定 Ptt-Alertor 環境變數

完成上述步驟後，您需要在 Ptt-Alertor 的執行環境中設定以下變數：

1.  **`DISCORD_BOT_TOKEN`**:
    *   **說明**: 貼上您在「步驟 1.4」中從 Discord Developer Portal 複製的 Bot Token。
    *   **範例**: `DISCORD_BOT_TOKEN=您的BotToken字串`

2.  **`DISCORD_CHANNEL_ID`**:
    *   **說明**: 這是您希望 Bot 主要在其中接收指令並發送通知的 Discord 文字頻道的 ID。Bot 只會回應在此特定頻道中發出的指令。
    *   **取得方式**:
        1.  在您的 Discord 用戶端中，前往「使用者設定」(通常是您頭像旁邊的齒輪圖示)。
        2.  在「應用程式設定」分類下，選擇「進階設定」。
        3.  啟用「開發者模式」。
        4.  關閉設定，然後在您的 Discord 伺服器中，找到您希望 Bot 互動的文字頻道。
        5.  在該頻道上按右鍵，選擇「複製 ID」(Copy ID)。此 ID 即為 `DISCORD_CHANNEL_ID`。
    *   **範例**: `DISCORD_CHANNEL_ID=您的頻道ID字串`

3.  **(可選) `DISCORD_WEBHOOK_URL`**:
    *   **說明**: 如果您希望透過特定的 Discord Webhook URL 發送某些通知（這是一種較簡單、單向的訊息推送方式，獨立於 Bot Token 的運作），可以設定此變數。如果主要使用 Bot 的雙向互動功能，則此變數通常不需要設定。
    *   **範例**: `DISCORD_WEBHOOK_URL=您的Webhook網址`

#### Bot 互動與指令

一旦 Ptt-Alertor 啟動並成功使用上述 Token 連線到 Discord，Bot 將會開始監聽您在 `DISCORD_CHANNEL_ID` 所指定的頻道中發出的指令。

例如，您可以在該頻道中輸入：

*   `指令`：Bot 會回覆可用的指令清單。
*   `清單`：Bot 會回覆您目前設定的看板、關鍵字及作者追蹤清單。

## Credits

### Real Life

Rose Li, Aries Huang, Scott Kao, Amy Li

### Ptt

DMM, oas, bestpika, Zero0910, lucky0509, wbreeze, chang0206, lindo0130, hungys, gyman7788, tooilxui, myamyakoko, whkuo, papago89, timeline, Kamikiri

### Facebook

Mr.clu, Woqeker