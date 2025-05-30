package jobs

import (
	"fmt"
	"strings" // Added import
	"sync"
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/channels/discord" // Added import
	"github.com/Ptt-Alertor/ptt-alertor/channels/line"
	"github.com/Ptt-Alertor/ptt-alertor/channels/mail"
	"github.com/Ptt-Alertor/ptt-alertor/channels/messenger"
	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram"
	"github.com/Ptt-Alertor/ptt-alertor/models/counter"
	"github.com/Ptt-Alertor/ptt-alertor/myutil" // Added import for ParseDiscordInternalID
)

const workers = 300

var ckCh = make(chan check)

var (
	recentlyNotifiedEvents = make(map[string]time.Time)
	recentlyNotifiedMutex  = &sync.Mutex{}
	notificationCoolDown   = 1 * time.Minute // Notification cool-down time, e.g., 1 minute
)

func init() {
	for i := 0; i < workers; i++ {
		go messageWorker(ckCh)
	}
}

func messageWorker(ckCh chan check) {
	for {
		ck := <-ckCh
		cr := ck.Self() // Get Checker instance early for logging
		log.WithFields(log.Fields{
			"board":           cr.board,
			"type":            cr.subType,
			"word":            cr.word,
			"articles_count":  len(cr.articles),
			"profile_account": cr.Profile.Account,
			"discord_ch_id":   cr.Profile.DiscordChannelID,
		}).Info("Message worker received Checker from ckCh")
		sendMessage(ck) // Pass original 'ck'
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
	log.WithFields(log.Fields{
		"account":           cr.Profile.Account,
		"board":             cr.board,
		"type":              cr.subType,
		"word":              cr.word,
		"articles_count":    len(cr.articles),
		"target_discord_ch": cr.Profile.DiscordChannelID,
	}).Info("sendMessage BEGIN")

	// --- New Notification Cool-down Logic ---
	notificationKey := cr.board // Base key is board name
	if cr.subType == "keyword" || cr.subType == "author" {
		// For specific and common subscription types, use a more precise key
		notificationKey = fmt.Sprintf("%s:%s:%s", cr.board, cr.subType, strings.ToLower(cr.word)) // Ensure word is also standardized for the key
	} else if cr.subType != "" && cr.word != "" {
		// Handle other possible subType + word combinations similarly
		notificationKey = fmt.Sprintf("%s:%s:%s", cr.board, cr.subType, strings.ToLower(cr.word))
	} else if cr.subType != "" {
		// If only subType is present (e.g., special notification types)
		notificationKey = fmt.Sprintf("%s:%s", cr.board, cr.subType)
	}
	// If the key remains just the board name, cool-down is board-level.
	// The goal is for the key to identify "the same batch of content" as precisely as possible.
	log.WithFields(log.Fields{ // Log point 1
		"calculated_notification_key": notificationKey,
		"board":                       cr.board,
		"sub_type":                    cr.subType,
		"word":                        cr.word,
	}).Debug("Notification key calculated for cool-down check.")

	log.WithField("notification_key_to_check", notificationKey).Debug("About to check cool-down status for notification key.")
	
	recentlyNotifiedMutex.Lock() // Lock before read-check-write sequence
	
	lastNotifiedTime, found := recentlyNotifiedEvents[notificationKey]
	currentTime := time.Now()
	shouldSkipNotification := false

	if found && currentTime.Sub(lastNotifiedTime) < notificationCoolDown {
		// Still in cool-down period
		log.WithFields(log.Fields{
			"notification_key":        notificationKey,
			"was_found_in_map":        true, // Must be true to be in this block
			"time_since_last_notify":  currentTime.Sub(lastNotifiedTime).String(),
			"last_notified_time_raw":  lastNotifiedTime.Format(time.RFC3339),
			"cool_down_duration":      notificationCoolDown.String(),
			"cool_down_active_for_key":true, // Condition for this block
		}).Info("Notification for this key is in cool-down period, skipping all platform sends for this event.")
		shouldSkipNotification = true
	} else {
		// Not in cool-down (or first time), allow send and update timestamp immediately
		recentlyNotifiedEvents[notificationKey] = currentTime 
		log.WithFields(log.Fields{
			"notification_key":        notificationKey,
			"was_found_in_map":        found, // Log if it was found (i.e., cool-down expired) or not (first time)
			"previous_last_notified_time_raw": lastNotifiedTime.Format(time.RFC3339), // Log previous time, will be zero if !found
			"cool_down_duration":      notificationCoolDown.String(),
			"new_timestamp_set":       currentTime.Format(time.RFC3339), // Log the new timestamp being set
			"cool_down_active_for_key":false, // Condition for this block (not actively cooling down)
		}).Debug("Notification key NOT in cool-down. Updated timestamp to current time, proceeding with send.")
		// shouldSkipNotification remains false
	}
	recentlyNotifiedMutex.Unlock() // Unlock after read-check-write sequence

	if shouldSkipNotification {
		return // Skip all sending if decided above
	}
	// --- End of New Notification Cool-down Logic ---

	account := cr.Profile.Account
	finalSentPlatforms := []string{} // 記錄最終成功發送的平台

	// Declare messageContent and embed here to be populated by the new logic
	var messageContent string
	var embed *discord.Embed

	// Standardize displayBoard and displayWord
	displayBoard := strings.ToLower(strings.TrimSpace(cr.board))
	displayWord := ""
	if cr.word != "" {
		displayWord = strings.ToLower(strings.TrimSpace(cr.word))
	}

	if len(cr.articles) > 0 {
		if len(cr.articles) == 1 {
			article := cr.articles[0]
			title := strings.TrimSpace(article.Title)
			if len(title) > 250 { // Discord Embed Title limit is 256
				title = title[:250] + "..."
			}
			messageContent = fmt.Sprintf("看板 [%s] 有新文章符合您的訂閱 '%s': %s", displayBoard, displayWord, article.Title)
			embed = &discord.Embed{
				Title:       title,
				Description: "點擊查看文章：\n" + article.Link,
				URL:         article.Link,
				Color:       0x0099FF,
				Footer:      &discord.EmbedFooter{Text: "PTT Alertor"},
			}
			// Optional: Add author and date if available
			// if article.Author != "" { embed.Author = &discord.EmbedAuthor{Name: article.Author} }
			// if !article.Date.IsZero() { embed.Timestamp = article.Date.Format(time.RFC3339) }
		} else { // Multiple articles
			messageContent = fmt.Sprintf("看板 [%s] 有 %d 篇新文章符合您的訂閱 '%s'。", displayBoard, len(cr.articles), displayWord)
			
			embedDescription := "偵測到多篇相關文章：\n"
			var fields []*discord.EmbedField

			for i, article := range cr.articles {
				fieldTitle := strings.TrimSpace(article.Title)
				if len(fieldTitle) > 250 { // Discord Embed Field Name limit is 256
					fieldTitle = fieldTitle[:250] + "..."
				}
				fieldValue := article.Link
				if len(fieldValue) > 1018 { // Discord Embed Field Value limit is 1024
					 fieldValue = fieldValue[:1018] + "..."
				}

				if i < 5 { // Max 5 articles in fields
					fields = append(fields, &discord.EmbedField{
						Name:   fmt.Sprintf("%d. %s", i+1, fieldTitle),
						Value:  fieldValue,
						Inline: false,
					})
				} else {
					embedDescription += fmt.Sprintf("...還有 %d 篇更多文章未在下方列表中完全顯示。\n", len(cr.articles)-i)
					break 
				}
			}

			embed = &discord.Embed{
				Title:       fmt.Sprintf("[%s] %d 篇關於 '%s' 的新文章", displayBoard, len(cr.articles), displayWord),
				Description: embedDescription,
				Color:       0x0099FF,
				Fields:      fields,
				Footer:      &discord.EmbedFooter{Text: "PTT Alertor"},
			}
		}
	} else {
		log.WithFields(log.Fields{
			"notification_key": notificationKey, 
			"board": cr.board, "type": cr.subType, "word": cr.word,
		}).Warn("sendMessage called with zero articles in Checker object, no Discord message will be sent.")
		return // No articles, nothing to send for Discord (and likely other platforms)
	}
	
	if len(messageContent) > 2000 { // Discord message content limit
		messageContent = messageContent[:1997] + "..."
	}

	discordAttempted := false
	discordSentSuccessfully := false
	if cr.Profile.DiscordChannelID != "" {
		discordAttempted = true

		// Prepare base log fields for Discord notification attempt
		attemptLogFields := log.Fields{
			"platform":                  "discord_bot",
			"board":                     cr.board, // Use original cr.board for logging consistency if needed
			"type":                      cr.subType,
			"word":                      cr.word,
			"notificationTargetChannelID": cr.Profile.DiscordChannelID,
		}
		parsedUserID, parsedChannelIDFromAccount, parseErr := myutil.ParseDiscordInternalID(account)
		if parseErr == nil {
			attemptLogFields["internalAccountID"] = account
			attemptLogFields["discordUserID"] = parsedUserID
			attemptLogFields["channelIDFromAccount"] = parsedChannelIDFromAccount
		} else {
			attemptLogFields["account"] = account
		}
		log.WithFields(attemptLogFields).Info("Attempting to send Discord notification via Bot")
		
		// Debug log before PushMessage (this part was already present and correct)
		var msgSummary string
		if len(messageContent) > 50 {
			msgSummary = messageContent[:50] + "..."
		} else {
			msgSummary = messageContent
		}

		var embTitle string
		if embed != nil { // Ensure embed is not nil before accessing Title
			embTitle = embed.Title
		} else {
			embTitle = "<nil_embed>" // Or some other placeholder if embed is nil
		}

		log.WithFields(log.Fields{
			"target_discord_ch": cr.Profile.DiscordChannelID,
			"message_summary":   msgSummary,
			"embed_title":       embTitle,
		}).Debug("Details before calling discord.PushMessage")

		err := discord.PushMessage(cr.Profile.DiscordChannelID, messageContent, embed)
		if err == nil {
			discordSentSuccessfully = true
			if !containsString(finalSentPlatforms, "discord_bot") { //避免重複添加
				finalSentPlatforms = append(finalSentPlatforms, "discord_bot")
			}
			// Successful send log can be part of the unified log at the end,
			// or a specific one here using similar logFields if needed.
		} else {
			// Prepare detailed log fields for failure
			failureLogFields := log.Fields{
				"platform":                  "discord_bot",
				"board":                     cr.board,
				"type":                      cr.subType,
				"word":                      cr.word,
				"notificationTargetChannelID": cr.Profile.DiscordChannelID,
			}
			if parseErr == nil { // Use result from above parsing
				failureLogFields["internalAccountID"] = account
				failureLogFields["discordUserID"] = parsedUserID
				failureLogFields["channelIDFromAccount"] = parsedChannelIDFromAccount
			} else {
				failureLogFields["account"] = account
			}
			log.WithError(err).WithFields(failureLogFields).Warn("Failed to send Discord notification via Bot")
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
		}).Info("Skipping Line Notify because Discord notification via Bot was sent successfully.")
	}

	// 其他渠道 (Email, Messenger, Telegram) 總是嘗試 (如果已設定)
	// 但如果 Discord Bot 已成功發送，且使用者主要透過 Discord 接收，則可以考慮是否跳過其他平台
	// 目前邏輯：如果 Discord Bot 成功，Line Notify 會跳過。其他平台則繼續嘗試。
	// 這裡可以根據需求調整，例如，如果 Discord Bot 成功，則完全不再嘗試其他平台。
	// For now, keeping original logic for other platforms to always try if configured.

	if cr.Profile.Email != "" {
		sendMail(c)
		if !containsString(finalSentPlatforms, "mail") { finalSentPlatforms = append(finalSentPlatforms, "mail") }
	}
	// 舊的 Line (非 Notify) 邏輯
	if cr.Profile.Line != "" && cr.Profile.LineAccessToken == "" {
		// 只有在 Discord Bot 未嘗試或失敗，且 Line Notify 也沒設定或失敗時，才考慮這個舊 Line
		if (!discordAttempted || !discordSentSuccessfully) && cr.Profile.LineAccessToken == "" {
			log.WithFields(log.Fields{
				"account":  account, "platform": "line_legacy", "board": cr.board, "type": cr.subType, "word": cr.word,
			}).Warn("Attempting legacy Line push because Discord Bot and Line Notify were not successful/configured.")
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

		// Timestamp was already updated IF this notification was allowed to proceed.
		// No further timestamp update needed here. We just log that it was processed.
		log.WithField("processed_notification_key", notificationKey).Debug("Notification sent (or attempted); cool-down was set at decision time.")
	} else {
        log.WithFields(log.Fields{ // This log is fine, indicates no send attempt was successful.
			"account":  account, "board": cr.board, "type": cr.subType, "word": cr.word,
			"processed_notification_key": notificationKey, // Add key for context
		}).Info("No notification platform was successfully notified or configured for user (cool-down may have been set if this was first attempt).")
    }
}

func sendMail(c check) {
	cr := c.Self()
	log.WithFields(log.Fields{"platform": "mail", "account": cr.Profile.Account, "target": cr.Profile.Email, "board": cr.board, "word": cr.word}).Debug("Attempting to send Mail")
	m := new(mail.Mail)
	m.Title.BoardName = cr.board
	m.Title.Keyword = cr.keyword
	m.Body.Articles = cr.articles
	m.Receiver = cr.Profile.Email
	m.Send()
}

func sendLine(c check) {
	cr := c.Self()
	log.WithFields(log.Fields{"platform": "line_legacy", "account": cr.Profile.Account, "target": cr.Profile.Line, "board": cr.board, "word": cr.word}).Debug("Attempting to send Line (Legacy)")
	line.PushTextMessage(cr.Profile.Line, c.String())
}

func sendLineNotify(c check) {
	cr := c.Self()
	log.WithFields(log.Fields{"platform": "line_notify", "account": cr.Profile.Account, "target": "Line Token Present", "board": cr.board, "word": cr.word}).Debug("Attempting to send Line Notify")
	line.Notify(cr.Profile.LineAccessToken, c.String())
}

func sendMessenger(c check) {
	cr := c.Self()
	log.WithFields(log.Fields{"platform": "messenger", "account": cr.Profile.Account, "target": cr.Profile.Messenger, "board": cr.board, "word": cr.word}).Debug("Attempting to send Messenger")
	m := messenger.New()
	m.SendTextMessage(cr.Profile.Messenger, c.String())
}

func sendTelegram(c check) {
	cr := c.Self()
	log.WithFields(log.Fields{"platform": "telegram", "account": cr.Profile.Account, "target": cr.Profile.TelegramChat, "board": cr.board, "word": cr.word}).Debug("Attempting to send Telegram")
	telegram.SendTextMessage(cr.Profile.TelegramChat, c.String())
}