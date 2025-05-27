package jobs

import (
	"os" // Added import
	"strings" // Added import
	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/channels/discord" // Added import
	"github.com/Ptt-Alertor/ptt-alertor/channels/line"
	"github.com/Ptt-Alertor/ptt-alertor/channels/mail"
	"github.com/Ptt-Alertor/ptt-alertor/channels/messenger"
	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram"
	"github.com/Ptt-Alertor/ptt-alertor/models/counter"
)

const workers = 300

var ckCh = make(chan check)

func init() {
	for i := 0; i < workers; i++ {
		go messageWorker(ckCh)
	}
}

func messageWorker(ckCh chan check) {
	for {
		ck := <-ckCh
		sendMessage(ck)
	}
}

type check interface {
	String() string
	Self() Checker
	Stop()
	Run()
}

// Helper function to check if a slice contains a string (if not already present globally or in utils)
func containsString(slice []string, str string) bool {
	for _, v := range slice {
		if v == str {
			return true
		}
	}
	return false
}

func sendMessage(c check) {
	cr := c.Self()
	account := cr.Profile.Account
	
	finalSentPlatforms := []string{} // 記錄最終成功發送的平台

	discordWebhookURL := os.Getenv("DISCORD_WEBHOOK")
	discordAttempted := false
	discordSentSuccessfully := false

	if discordWebhookURL != "" {
		discordAttempted = true
		// Modify the function call here
		err := discord.SendTextMessage(discordWebhookURL, c.String()) // <--- Modification point
		if err == nil {
			discordSentSuccessfully = true
			finalSentPlatforms = append(finalSentPlatforms, "discord")
		} else {
			log.WithError(err).WithFields(log.Fields{
				"account": account, "platform": "discord", "board": cr.board, "type": cr.subType, "word": cr.word,
			}).Warn("Failed to send Discord notification")
		}
	}

	// Line Notify: 只有在 Discord 未嘗試或嘗試但未成功發送，且使用者有 Line Access Token 時才嘗試
	if (!discordAttempted || !discordSentSuccessfully) && cr.Profile.LineAccessToken != "" {
		sendLineNotify(c) 
        if !containsString(finalSentPlatforms, "line_notify") {
		    finalSentPlatforms = append(finalSentPlatforms, "line_notify")
        }
	} else if discordSentSuccessfully && cr.Profile.LineAccessToken != "" {
        log.WithFields(log.Fields{
            "account":  account, "platform": "line_notify", "board": cr.board, "type": cr.subType, "word": cr.word,
        }).Info("Skipping Line Notify because Discord notification was sent successfully.")
    }

	// 其他渠道 (Email, Messenger, Telegram) 總是嘗試 (如果已設定)
	if cr.Profile.Email != "" {
		sendMail(c)
		if !containsString(finalSentPlatforms, "mail") { finalSentPlatforms = append(finalSentPlatforms, "mail") }
	}
	// 舊的 Line (非 Notify) 邏輯
	if cr.Profile.Line != "" && cr.Profile.LineAccessToken == "" {
		// 只有在 Discord 未嘗試或失敗，且新 Line Notify 也沒設定或失敗時，才考慮這個舊 Line
		// (Discord 沒發送成功 AND Line Notify Token是空的)
		if (!discordAttempted || !discordSentSuccessfully) && cr.Profile.LineAccessToken == "" {
			log.WithFields(log.Fields{
				"account":  account, "platform": "line_legacy", "board": cr.board, "type": cr.subType, "word": cr.word,
			}).Warn("Attempting legacy Line push.")
			sendLine(c) // sendLine 內部是發送訊息的邏輯
            if !containsString(finalSentPlatforms, "line_legacy") { finalSentPlatforms = append(finalSentPlatforms, "line_legacy") }
		}
	}
	if cr.Profile.Messenger != "" {
		sendMessenger(c)
		if !containsString(finalSentPlatforms, "messenger") { finalSentPlatforms = append(finalSentPlatforms, "messenger") }
	}
	if cr.Profile.Telegram != "" {
		sendTelegram(c)
		if !containsString(finalSentPlatforms, "telegram") { finalSentPlatforms = append(finalSentPlatforms, "telegram") }
	}

	// 統一日誌記錄
	if len(finalSentPlatforms) > 0 {
		counter.IncrAlert()
		log.WithFields(log.Fields{
			"account":  account,
			"platform": strings.Join(finalSentPlatforms, ","),
			"board":    cr.board,
			"type":     cr.subType,
			"word":     cr.word,
		}).Info("Message Sent")
	} else {
        log.WithFields(log.Fields{
			"account":  account, "board": cr.board, "type": cr.subType, "word": cr.word,
		}).Info("No notification platform was successfully notified or configured for user.")
    }
}

func sendMail(c check) {
	cr := c.Self()
	m := new(mail.Mail)
	m.Title.BoardName = cr.board
	m.Title.Keyword = cr.keyword
	m.Body.Articles = cr.articles
	m.Receiver = cr.Profile.Email
	m.Send()
}

func sendLine(c check) {
	cr := c.Self()
	line.PushTextMessage(cr.Profile.Line, c.String())
}

func sendLineNotify(c check) {
	cr := c.Self()
	line.Notify(cr.Profile.LineAccessToken, c.String())
}

func sendMessenger(c check) {
	cr := c.Self()
	m := messenger.New()
	m.SendTextMessage(cr.Profile.Messenger, c.String())
}

func sendTelegram(c check) {
	cr := c.Self()
	telegram.SendTextMessage(cr.Profile.TelegramChat, c.String())
}
