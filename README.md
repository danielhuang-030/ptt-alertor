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

Ptt Alertor 看板資料（如文章列表）的**預設儲存方式是使用本地檔案系統 (`file`)**。您可以根據您的部署環境和需求，透過以下環境變數選擇不同的儲存後端。

*   **`BOARD_DRIVER_TYPE`**:
    *   **說明**: 設定看板主要資料（文章列表等）的儲存驅動程式。**如果此環境變數未設定，系統將預設採用 `file` 驅動**。
    *   **可選值**:
        *   `file`: 使用伺服器本地檔案系統儲存。資料將以 JSON 格式儲存在 `BOARD_FILE_STORAGE_PATH` 指定的路徑下（若未指定 `BOARD_FILE_STORAGE_PATH`，則使用預設路徑）。這是系統的預設選項。
        *   `dynamodb`: 使用 AWS DynamoDB 作為儲存後端。選擇此選項時，**必須確保 AWS 相關環境變數 (如 `AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) 已正確設定**，否則可能導致服務啟動失敗或運行時錯誤。
        *   `redis`: 使用 Redis 儲存。文章資料將以 JSON 字串格式儲存在 Redis 中。選擇此選項時，**必須確保 Redis 服務 (`REDIS_HOST_PORT`) 已正確設定且 Ptt Alertor 可以連線**。
    *   **範例**: `BOARD_DRIVER_TYPE=redis` (若要使用 Redis 儲存)

*   **`BOARD_FILE_STORAGE_PATH`**:
    *   **說明**: 此變數在 `BOARD_DRIVER_TYPE` 設定為 `file` (包括未設定 `BOARD_DRIVER_TYPE` 而採用預設的 `file` 驅動時) 時使用，用以指定看板資料檔案的儲存目錄路徑。
    *   如果此變數未設定（且使用 `file` 驅動），系統將使用一個內建的預設路徑 (例如：`./storage/board_articles/`)。
    *   **範例**: `BOARD_FILE_STORAGE_PATH=/app/data/board_data`

*   **`BOARD_CACHER_TYPE`**:
    *   **說明**: 設定看板列表及看板是否存在等快取資訊的儲存驅動程式。
    *   **可選值**:
        *   `redis` (預設且目前唯一選項): 使用 Redis 進行快取。選擇此選項時，**必須確保 Redis 服務 (`REDIS_HOST_PORT`) 已正確設定且 Ptt Alertor 可以連線**。
    *   **範例**: `BOARD_CACHER_TYPE=redis` (通常保持預設即可)

**重要提示**:
*   若使用 `dynamodb` 作為 `BOARD_DRIVER_TYPE`，請務必檢查您的 AWS 環境變數設定。
*   若使用 `redis` (作為 `BOARD_DRIVER_TYPE` 或 `BOARD_CACHER_TYPE`)，請務必檢查 `REDIS_HOST_PORT` 設定並確認 Redis 服務可正常連線。

## 頻道支援

Ptt-Alertor 目前支援透過以下頻道接收 PTT 文章通知：

*   Email
*   Line
*   Facebook Messenger
*   Telegram
*   Discord

### Discord 通知設定

若要啟用 Discord 通知，您需要在您的環境設定中配置以下變數：

1.  **`DISCORD_BOT_TOKEN`**:
    *   **說明**: 這是您的 Discord 應用程式的 Bot Token。Ptt-Alertor 會使用此 Token 來登入並以 Bot 的身分運作。
    *   **取得方式**: 您需要在 [Discord Developer Portal](https://discord.com/developers/applications) 建立一個應用程式，並為該應用程式新增一個 Bot。Token 會在 Bot 設定頁面中提供。

2.  **`DISCORD_CHANNEL_ID`**:
    *   **說明**: 這是您希望 Bot 在其中接收指令並發送大部分通知的預設 Discord 頻道的 ID。Bot 只會回應在此頻道中發出的指令。
    *   **取得方式**: 在 Discord 中，開啟「使用者設定」 -> 「進階設定」 -> 開啟「開發者模式」。然後在您想要的頻道上按右鍵，選擇「複製 ID」。

3.  **(可選) `DISCORD_WEBHOOK_URL`**:
    *   **說明**: 如果您希望透過特定的 Discord Webhook URL 發送某些通知（獨立於 Bot Token 的運作方式），可以設定此變數。這通常用於較簡單的、單向的訊息推送。如果主要使用 Bot 功能，則此變數可能不需要設定。

#### Bot 互動與指令

設定完成並啟動 Ptt-Alertor 後，Bot 將會監聽您在 `DISCORD_CHANNEL_ID` 所指定頻道中發出的指令。

例如，您可以在該頻道中輸入：

*   `指令`：Bot 會回覆可用的指令清單。
*   `清單`：Bot 會回覆您目前設定的看板、關鍵字及作者追蹤清單。

#### 所需權限

請確保您的 Bot 在被邀請到您的 Discord 伺服器時，擁有以下基本權限：

*   **讀取訊息/查看頻道 (Read Messages/View Channels)**: Bot 需要此權限才能接收您在頻道中輸入的指令。
*   **傳送訊息 (Send Messages)**: Bot 需要此權限才能回覆指令以及發送 PTT 通知。
*   **(建議) 嵌入連結 (Embed Links)**: 如果希望通知訊息中的連結能以預覽方式顯示。
*   **(建議) 附加檔案 (Attach Files)**: 如果未來支援透過 Bot 發送圖片或其他檔案。

這些權限可以在您將 Bot 加入伺服器時的 OAuth2 URL 產生器中設定，或者之後在伺服器的「設定」 -> 「整合」或「角色」中調整 Bot 的權限。

## Credits

### Real Life

Rose Li, Aries Huang, Scott Kao, Amy Li

### Ptt

DMM, oas, bestpika, Zero0910, lucky0509, wbreeze, chang0206, lindo0130, hungys, gyman7788, tooilxui, myamyakoko, whkuo, papago89, timeline, Kamikiri

### Facebook

Mr.clu, Woqeker