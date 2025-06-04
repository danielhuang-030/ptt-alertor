package discord

import (
	"fmt"
	"os"

	"strings" // Added import

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/models" // Added import
	"github.com/bwmarrin/discordgo"
)

const discordAccountPrefix = "discord_channel:"

var (
	discordBotToken  string
	defaultChannelID string
	discordSession   *discordgo.Session
)

func init() {
	discordBotToken = os.Getenv("DISCORD_BOT_TOKEN")
	defaultChannelID = os.Getenv("DISCORD_CHANNEL_ID")

	if discordBotToken == "" {
		log.Warn("Discord Bot Token (DISCORD_BOT_TOKEN) is not set. Discord notifications will be disabled.")
		discordSession = nil
		return
	}

	var err error
	discordSession, err = discordgo.New("Bot " + discordBotToken)
	if err != nil {
		log.Warnf("Failed to create Discord session: %v. Discord notifications will be disabled.", err)
		discordSession = nil
		return
	}

	// Optionally, you can open a websocket connection here if you plan to receive events
	// err = discordSession.Open()
	// if err != nil {
	// 	log.Warnf("Failed to open Discord session: %v. Discord notifications might not work as expected.", err)
	//  // Decide if this is critical enough to set discordSession = nil
	// 	return
	// }

	// Set necessary intents
	discordSession.Identify.Intents = discordgo.IntentsGuildMessages

	// Register the messageCreate func as a callback for MessageCreate events.
	discordSession.AddHandler(messageCreate)

	// Open a websocket connection to Discord and begin listening.
	err = discordSession.Open()
	if err != nil {
		log.Warnf("Error opening Discord session: %v. Discord notifications and commands will be disabled.", err)
		discordSession = nil
		return
	}

	log.Info("Discord session initialized and listening successfully.")
}

// messageCreate will be called (by the Discordgo library) every time a new
// message is created on any channel that the authenticated bot has access to.
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	guildID := m.GuildID
	channelID := m.ChannelID
	authorID := m.Author.ID // Original Discord User ID

	log.WithFields(log.Fields{
		"guildID":   guildID,
		"channelID": channelID,
		"authorID":  authorID,
		"message":   m.Content,
	}).Debug("Discord message received")

	// Get current channel's user/enable state
	accountKeyForDB := discordAccountPrefix + m.ChannelID // Use prefixed ID
	channelUser := models.User().Find(accountKeyForDB)
	// A channel is listening if its specific, prefixed account record exists, is enabled, and type matches.
	isChannelListening := channelUser.Enable &&
		channelUser.Profile.Account == accountKeyForDB &&
		channelUser.Profile.Type == "discord_channel"

	log.WithFields(log.Fields{
		"channelID":             m.ChannelID,
		"accountKeyInDB":        accountKeyForDB,
		"retrievedRawAccount":   channelUser.Profile.Account, // Log the Account field from DB
		"retrievedEnableFlag":   channelUser.Enable,          // Log the Enable flag from DB
		"retrievedProfileType":  channelUser.Profile.Type,    // Log the Type from DB
		"isConsideredListening": isChannelListening,          // Log the derived listening state
	}).Debug("Checked channel initial listening state")

	var textToHandle string
	var executeCommand bool = false
	var botMentioned bool = false

	for _, mentionedUser := range m.Mentions {
		if mentionedUser.ID == s.State.User.ID {
			botMentioned = true
			break
		}
	}

	parsedMentionCommand := ""
	if botMentioned {
		botMentionString := "<@" + s.State.User.ID + ">"
		botMentionStringWithNick := "<@!" + s.State.User.ID + ">"
		// Check for nickname mention first as it's more specific
		if strings.HasPrefix(m.Content, botMentionStringWithNick) {
			parsedMentionCommand = strings.TrimSpace(strings.Replace(m.Content, botMentionStringWithNick, "", 1))
		} else if strings.HasPrefix(m.Content, botMentionString) {
			parsedMentionCommand = strings.TrimSpace(strings.Replace(m.Content, botMentionString, "", 1))
		}
	}

	// --- Special handling for listen/unlisten commands (these always require a mention) ---
	if botMentioned && (strings.EqualFold(parsedMentionCommand, "監聽") || strings.EqualFold(parsedMentionCommand, "listen")) {
		// 步驟1: 確保使用者記錄存在並獲取/更新初始狀態 (Enable 可能被設為 true)
		if err := command.HandleDiscordFollow(m.GuildID, m.ChannelID, m.ChannelID); err != nil {
			log.WithError(err).WithFields(log.Fields{"channelID": m.ChannelID, "command": parsedMentionCommand}).Error("Error in HandleDiscordFollow for 'listen' command.")
			s.ChannelMessageSend(m.ChannelID, "處理監聽指令時發生內部錯誤，請稍後再試。(正體中文)")
			return // Stop processing
		}

		// 步驟2: 再次獲取最新的使用者物件，以確保我們操作的是 HandleDiscordFollow 更新後的狀態
		currentChannelUser := models.User().Find(accountKeyForDB) // accountKeyForDB 應已在此函式作用域內定義 (discordAccountPrefix + m.ChannelID)
		if currentChannelUser.Profile.Account != accountKeyForDB || currentChannelUser.Profile.Type != "discord_channel" {
			log.WithFields(log.Fields{
				"accountKeyInDB":      accountKeyForDB,
				"foundProfileAccount": currentChannelUser.Profile.Account,
				"foundProfileType":    currentChannelUser.Profile.Type,
			}).Error("Failed to verify channel user after HandleDiscordFollow for 'listen'.")
			s.ChannelMessageSend(m.ChannelID, "啟用服務時未能正確驗證頻道資訊，請再試一次。(正體中文)")
			return
		}

		// 步驟3: 明確設定 Enable = true 並更新
		currentChannelUser.Enable = true
		if err := currentChannelUser.Update(); err != nil {
			log.WithError(err).WithField("accountKeyInDB", accountKeyForDB).Error("Enable service failed on user.Update() for 'listen' in messageCreate.")
			s.ChannelMessageSend(m.ChannelID, "啟用服務失敗，資料更新時發生錯誤，請稍後再試。(正體中文)")
			return
		}

		// 步驟4: 發送成功回應
		s.ChannelMessageSend(m.ChannelID, "已啟用 PTT Alertor 服務於此頻道。我將會開始監聽並通知新訊息。(正體中文)")

		// 步驟5: 更新此函式範圍內的 isChannelListening 狀態
		isChannelListening = true
		log.WithFields(log.Fields{"channelID": m.ChannelID, "parsedInput": parsedMentionCommand, "newState": isChannelListening}).Info("Successfully processed 'listen' command directly in messageCreate.")

		// 監聽指令處理完畢，不再交給後續的 executeCommand 流程
		// executeCommand 保持/設為 false, textToHandle 也不用設
		executeCommand = false // Explicitly set to false
	} else if botMentioned && (strings.EqualFold(parsedMentionCommand, "取消監聽") || strings.EqualFold(parsedMentionCommand, "unlisten")) {
		// 步驟1: 確保使用者記錄存在 (主要目的是獲取 User 物件，Enable 狀態會被後續明確設定)
		if err := command.HandleDiscordFollow(m.GuildID, m.ChannelID, m.ChannelID); err != nil {
			log.WithError(err).WithFields(log.Fields{"channelID": m.ChannelID, "command": parsedMentionCommand}).Error("Error in HandleDiscordFollow for 'unlisten' command.")
			s.ChannelMessageSend(m.ChannelID, "處理取消監聽指令時發生內部錯誤，請稍後再試。(正體中文)")
			return // Stop processing
		}

		// 步驟2: 再次獲取最新的使用者物件
		currentChannelUser := models.User().Find(accountKeyForDB) // accountKeyForDB 應已在此函式作用域內定義
		if currentChannelUser.Profile.Account != accountKeyForDB || currentChannelUser.Profile.Type != "discord_channel" {
			log.WithFields(log.Fields{
				"accountKeyInDB":      accountKeyForDB,
				"foundProfileAccount": currentChannelUser.Profile.Account,
				"foundProfileType":    currentChannelUser.Profile.Type,
			}).Error("Failed to verify channel user after HandleDiscordFollow for 'unlisten'.")
			s.ChannelMessageSend(m.ChannelID, "停用服務時未能正確驗證頻道資訊，請再試一次。(正體中文)")
			return
		}

		// 步驟3: 明確設定 Enable = false 並更新
		currentChannelUser.Enable = false
		if err := currentChannelUser.Update(); err != nil {
			log.WithError(err).WithField("accountKeyInDB", accountKeyForDB).Error("Disable service failed on user.Update() for 'unlisten' in messageCreate.")
			s.ChannelMessageSend(m.ChannelID, "停用服務失敗，資料更新時發生錯誤，請稍後再試。(正體中文)")
			return
		}

		// 步驟4: 發送成功回應
		s.ChannelMessageSend(m.ChannelID, "已停用 PTT Alertor 服務於此頻道。我將不再監聽此頻道。(正體中文)")

		// 步驟5: 更新此函式範圍內的 isChannelListening 狀態
		isChannelListening = false
		log.WithFields(log.Fields{"channelID": m.ChannelID, "parsedInput": parsedMentionCommand, "newState": isChannelListening}).Info("Successfully processed 'unlisten' command directly in messageCreate.")

		// 取消監聽指令處理完畢
		executeCommand = false // Explicitly set to false
	} else { // Not a listen/unlisten command, proceed with state-based logic
		if isChannelListening {
			potentialDirectCommand := strings.TrimSpace(m.Content)
			// Use command.IsKnownCommand to check if it's a command users can type directly
			if command.IsKnownCommand(potentialDirectCommand) {
				textToHandle = potentialDirectCommand
				executeCommand = true
				log.WithFields(log.Fields{"channelID": m.ChannelID, "directCommand": textToHandle}).Debug("Processing direct command in listening channel.")
			} else if botMentioned { // Bot was mentioned, but not for listen/unlisten
				if parsedMentionCommand != "" { // A command followed the mention
					textToHandle = parsedMentionCommand
					executeCommand = true
					log.WithFields(log.Fields{"channelID": m.ChannelID, "mentionedCommand": textToHandle}).Debug("Processing mentioned command in listening channel.")
				} else { // Bot was mentioned, but no specific command followed (e.g., just "@Bot")
					s.ChannelMessageSend(m.ChannelID, "您好，我正在監聽此頻道。可以直接輸入指令，或用 `@PTT通知 指令` 的方式互動。(正體中文)")
					log.WithFields(log.Fields{"channelID": m.ChannelID}).Debug("Bot mentioned with no command in listening channel. Sent help hint.")
					// executeCommand remains false, no further command processing
				}
			} else { // Not a known direct command and bot was not mentioned in a listening channel. Ignore.
				log.WithFields(log.Fields{"channelID": m.ChannelID, "message": m.Content}).Debug("Ignoring non-command message in listening channel.")
				// executeCommand remains false
			}
		} else { // Channel is NOT listening (isChannelListening == false)
			if botMentioned { // Bot was mentioned (and it's not listen/unlisten, which were handled above)
				if parsedMentionCommand != "" { // A command followed the mention
					// Respond that the channel is not listening, instruct how to listen.
					s.ChannelMessageSend(m.ChannelID, "此頻道尚未啟用監聽模式。請先使用 `@PTT通知 監聽` 來啟用服務。(正體中文)")
					log.WithFields(log.Fields{"channelID": m.ChannelID, "mentionedCommand": parsedMentionCommand}).Info("Bot mentioned with command in non-listening channel. Replied with 'please listen first'.")
				} else { // User just mentioned the bot with no command text
					s.ChannelMessageSend(m.ChannelID, "您好！如需啟用 PTT 通知服務，請輸入 `@PTT通知 監聽`。(正體中文)")
					log.WithFields(log.Fields{"channelID": m.ChannelID}).Info("Bot mentioned with no command in non-listening channel. Replied with 'how to listen'.")
				}
				// executeCommand remains false, no command processed unless it was 'listen'
			} else { // Not listening AND bot not mentioned. Ignore.
				log.WithFields(log.Fields{"channelID": m.ChannelID}).Debug("Ignoring message in non-listening, non-mentioned channel.")
				// executeCommand remains false
			}
		}
	}

	// --- Actual command execution ---
	if executeCommand && textToHandle != "" {
		log.WithFields(log.Fields{
			"channelID":             m.ChannelID,
			"accountKeyForDB":       accountKeyForDB, // Log the ID being sent to HandleCommand
			"textToHandle":          textToHandle,
			"isChannelListeningNow": isChannelListening, // Log listening state at the time of execution
		}).Info("Preparing to execute command.")

		// IMPORTANT: Pass accountKeyForDB (prefixed ID) to HandleCommand as the userID argument
		responseText := command.HandleCommand(textToHandle, accountKeyForDB, true)

		if responseText != "" {
			_, err := s.ChannelMessageSend(m.ChannelID, responseText)
			if err != nil {
				log.WithError(err).WithFields(log.Fields{"channelID": m.ChannelID}).Error("Failed to send Discord message response")
			}
		}
	} else {
		log.WithFields(log.Fields{
			"channelID":          m.ChannelID,
			"executeCommandFlag": executeCommand,
			"textToHandle":       textToHandle,
			"originalContent":    m.Content,
		}).Debug("No command executed or no text to handle for final processing.")
	}
}

// EmbedFooter is a part of an Embed
type EmbedFooter struct {
	Text    string `json:"text,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

// EmbedImage is a part of an Embed
type EmbedImage struct {
	URL string `json:"url,omitempty"`
}

// EmbedThumbnail is a part of an Embed
type EmbedThumbnail struct {
	URL string `json:"url,omitempty"`
}

// EmbedAuthor is a part of an Embed
type EmbedAuthor struct {
	Name    string `json:"name,omitempty"`
	URL     string `json:"url,omitempty"`
	IconURL string `json:"icon_url,omitempty"`
}

// EmbedField is a part of an Embed
type EmbedField struct {
	Name   string `json:"name,omitempty"`
	Value  string `json:"value,omitempty"`
	Inline bool   `json:"inline,omitempty"`
}

// Embed is the main embed object
type Embed struct {
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	URL         string          `json:"url,omitempty"`
	Timestamp   string          `json:"timestamp,omitempty"` // ISO8601 timestamp
	Color       int             `json:"color,omitempty"`     // Integer representation of a hex color code
	Footer      *EmbedFooter    `json:"footer,omitempty"`
	Image       *EmbedImage     `json:"image,omitempty"`
	Thumbnail   *EmbedThumbnail `json:"thumbnail,omitempty"`
	Author      *EmbedAuthor    `json:"author,omitempty"`
	Fields      []*EmbedField   `json:"fields,omitempty"`
}

// toDiscordgoEmbed converts our custom Embed struct to discordgo.MessageEmbed
func toDiscordgoEmbed(embed *Embed) *discordgo.MessageEmbed {
	if embed == nil {
		return nil
	}

	dgEmbed := &discordgo.MessageEmbed{
		URL:         embed.URL,
		Title:       embed.Title,
		Description: embed.Description,
		Timestamp:   embed.Timestamp,
		Color:       embed.Color,
	}

	if embed.Footer != nil {
		dgEmbed.Footer = &discordgo.MessageEmbedFooter{
			Text:    embed.Footer.Text,
			IconURL: embed.Footer.IconURL,
		}
	}

	if embed.Image != nil {
		dgEmbed.Image = &discordgo.MessageEmbedImage{
			URL: embed.Image.URL,
		}
	}

	if embed.Thumbnail != nil {
		dgEmbed.Thumbnail = &discordgo.MessageEmbedThumbnail{
			URL: embed.Thumbnail.URL,
		}
	}

	if embed.Author != nil {
		dgEmbed.Author = &discordgo.MessageEmbedAuthor{
			Name:    embed.Author.Name,
			URL:     embed.Author.URL,
			IconURL: embed.Author.IconURL,
		}
	}

	if len(embed.Fields) > 0 {
		dgEmbed.Fields = make([]*discordgo.MessageEmbedField, len(embed.Fields))
		for i, field := range embed.Fields {
			if field != nil {
				dgEmbed.Fields[i] = &discordgo.MessageEmbedField{
					Name:   field.Name,
					Value:  field.Value,
					Inline: field.Inline,
				}
			}
		}
	}

	return dgEmbed
}

// PushMessage sends a message with an optional embed to a specific Discord channel using the initialized bot session.
func PushMessage(channelID string, message string, embed *Embed) error {
	if discordSession == nil {
		log.Warn("Attempted to send message via Discord, but session is not initialized. Discord notifications might be disabled.")
		return fmt.Errorf("Discord session not initialized")
	}

	if channelID == "" {
		log.Warn("Attempted to send message via Discord, but channelID is empty. Using default channel ID.")
		channelID = defaultChannelID
		if channelID == "" {
			log.Error("Discord default channel ID is not set. Cannot send message.")
			return fmt.Errorf("Discord default channel ID is not configured")
		}
	}

	msgSend := &discordgo.MessageSend{
		Content: message,
		Embed:   toDiscordgoEmbed(embed),
	}

	// If embed is nil, msgSend.Embed will be nil, and ChannelMessageSendComplex will just send content.
	// If embed is provided, it's converted and included.

	_, err := discordSession.ChannelMessageSendComplex(channelID, msgSend)
	if err != nil {
		log.Errorf("Failed to send complex message to Discord channel %s: %v", channelID, err)
		return fmt.Errorf("failed to send complex Discord message to channel %s: %w", channelID, err)
	}
	return nil
}
