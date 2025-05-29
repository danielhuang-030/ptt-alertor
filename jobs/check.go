package jobs

import (
	"strings" // Added import
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

func checkAuthor(author string, bd *board.Board, cker Checker) {
	if bd == nil { // Nil check
		return
	}
	log.WithFields(log.Fields{
		"board":                cker.board,
		"author":               author,
		"articlesToCheckCount": len(bd.NewArticles),
		"targetUserAccount":    cker.Profile.Account,
	}).Debug("Checking author match for user.")
	authorArticles := make(article.Articles, 0)
	for _, newAtcl := range bd.NewArticles {
		if newAtcl != nil && strings.EqualFold(newAtcl.Author, author) { // Nil check for newAtcl
			authorArticles = append(authorArticles, newAtcl)
		}
	}
	if len(authorArticles) != 0 {
		cker.author = author
		cker.articles = authorArticles
		cker.subType = "author"
		cker.word = author
		log.WithFields(log.Fields{
			"board":                cker.board,
			"author":               author,
			"matchedArticlesCount": len(authorArticles),
			"targetUserAccount":    cker.Profile.Account,
			"action":               "sending_to_cker_ch_for_author",
		}).Info("Author matched, sending notification task to cker.ch.")
		cker.ch <- cker
	}
}

// Helper function to check if a slice contains a string
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

	finalSentPlatforms := []string{}

	discordAttempted := false
	discordSentSuccessfully := false
	if cr.Profile.DiscordChannelID != "" {
		discordAttempted = true

		attemptLogFields := log.Fields{
			"platform":                  "discord_bot",
			"board":                     cr.board,
			"type":                      cr.subType,
			"word":                      cr.word,
			"targetDiscordChannelID":    cr.Profile.DiscordChannelID,
			"accountKeyInDB":            account,
		}
		// Removed ParseDiscordInternalID logic here
		log.WithFields(attemptLogFields).Info("Attempting to send Discord notification via Bot")

		var embed *discord.Embed
		if len(cr.articles) > 0 && cr.articles[0] != nil {
			article := cr.articles[0]
			embed = &discord.Embed{
				Title: article.Title,
				URL:   article.Link,
				Color: 0x0099FF,
				Footer: &discord.EmbedFooter{
					Text: "PTT Alertor",
				},
			}
		}

		messageContent := "您有新的 PTT 通知！"
		if embed != nil && len(cr.articles) == 1 && cr.articles[0] != nil {
			messageContent = cr.board + " 板有新文章符合您的訂閱：" + cr.articles[0].Title
		}

		if len(messageContent) > 2000 {
			messageContent = messageContent[:1997] + "..."
		}

		if embed == nil {
		    if len(c.String()) > 0 && len(c.String()) <= 2000 {
		        messageContent = c.String()
		    } else if len(c.String()) > 2000 {
		        messageContent = c.String()[:1997] + "..."
		    }
		}

		err := discord.PushMessage(cr.Profile.DiscordChannelID, messageContent, embed)
		if err == nil {
			discordSentSuccessfully = true
			if !containsString(finalSentPlatforms, "discord_bot") {
				finalSentPlatforms = append(finalSentPlatforms, "discord_bot")
			}
		} else {
			failureLogFields := log.Fields{
				"platform":                  "discord_bot",
				"board":                     cr.board,
				"type":                      cr.subType,
				"word":                      cr.word,
				"targetDiscordChannelID":    cr.Profile.DiscordChannelID,
				"accountKeyInDB":            account,
			}
			log.WithError(err).WithFields(failureLogFields).Warn("Failed to send Discord notification via Bot")
		}
	}

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

	if cr.Profile.Email != "" {
		sendMail(c)
		if !containsString(finalSentPlatforms, "mail") { finalSentPlatforms = append(finalSentPlatforms, "mail") }
	}

	if cr.Profile.Line != "" && cr.Profile.LineAccessToken == "" {
		if (!discordAttempted || !discordSentSuccessfully) {
			log.WithFields(log.Fields{
				"account":  account, "platform": "line_legacy", "board": cr.board, "type": cr.subType, "word": cr.word,
			}).Warn("Attempting legacy Line push.")
			sendLine(c)
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
	// Assuming cr.word is the keyword for mail title.
	// If cr.keyword is a specific field, use that. For now, using cr.word.
	m.Title.Keyword = cr.word
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
