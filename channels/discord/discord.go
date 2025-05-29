package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"strings" // Added import

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/bwmarrin/discordgo"
)

var (
	// discordWebhookURL is no longer the primary method. Kept for now if direct webhook sending is still needed somewhere.
	discordWebhookURL string
	discordBotToken   string
	defaultChannelID  string
	discordSession    *discordgo.Session
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

	var actualCommand string
	botMentioned := false

	// 檢查 Bot 是否在訊息的提及列表中
	for _, mentionedUser := range m.Mentions {
		if mentionedUser.ID == s.State.User.ID {
			botMentioned = true
			break
		}
	}

	if !botMentioned {
		return // 如果 Bot 沒有被提及，則忽略此訊息
	}

	// 解析指令：移除 Bot 的提及部分，並去除前後空白
	botMentionString := "<@" + s.State.User.ID + ">"
	botMentionStringWithNick := "<@!" + s.State.User.ID + ">"

	if strings.HasPrefix(m.Content, botMentionStringWithNick) {
		actualCommand = strings.TrimSpace(strings.Replace(m.Content, botMentionStringWithNick, "", 1))
	} else if strings.HasPrefix(m.Content, botMentionString) {
		actualCommand = strings.TrimSpace(strings.Replace(m.Content, botMentionString, "", 1))
	} else {
		log.Warnf("Bot was mentioned by %s in channel %s, but command format was not recognized: %s", m.Author.Username, m.ChannelID, m.Content)
		return
	}

	if actualCommand == "" {
		// Optionally send help message or just return
		// s.ChannelMessageSend(m.ChannelID, "您好！請在提及我之後輸入指令，例如 `@PttAlertor 指令`")
		return
	}

	// Ensure user continuity for Discord interactions using the channel ID as the account key
	err := command.HandleDiscordFollow(m.GuildID, m.ChannelID, m.ChannelID) // Use m.ChannelID as the third argument
	if err != nil {
		log.WithError(err).WithFields(log.Fields{
			"channelID": m.ChannelID,
			"guildID":   m.GuildID,
		}).Error("Failed to ensure Discord user continuity in messageCreate")
		// Decide if this error is critical enough to stop further processing.
	} else {
		log.WithFields(log.Fields{
			"channelID": m.ChannelID,
			"guildID":   m.GuildID,
		}).Info("Discord user continuity ensured in messageCreate")
	}

	// Use the channel ID directly as the account identifier for command handling
	accountForCommands := m.ChannelID

	// Call HandleCommand to process the extracted actualCommand with the channel ID.
	responseText := command.HandleCommand(actualCommand, accountForCommands, true)

	if responseText != "" {
		_, err := s.ChannelMessageSend(m.ChannelID, responseText) // Send response back to the original channel
		if err != nil {
			log.WithError(err).WithFields(log.Fields{
				"channelID":          m.ChannelID,
				"accountForCommands": accountForCommands,
			}).Error("Failed to send Discord message response")
		}
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

// SendWebhookMessage sends a message with an optional embed to a Discord webhook URL.
// It directly uses the custom Embed struct for JSON serialization.
func SendWebhookMessage(webhookURL string, message string, embed *Embed) error {
	if webhookURL == "" {
		return fmt.Errorf("Discord webhook URL is empty")
	}

	payload := make(map[string]interface{})
	payload["content"] = message
	if embed != nil {
		// Discord webhooks expect an array of embeds.
		payload["embeds"] = []*Embed{embed}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal Discord webhook payload: %w", err)
	}

	resp, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to send Discord webhook message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var responseBody bytes.Buffer
		// Attempt to read the response body for more detailed error information.
		_, readErr := responseBody.ReadFrom(resp.Body)
		if readErr != nil {
			// If reading the response body fails, return the original status code error along with the read error.
			return fmt.Errorf("Discord webhook returned status %d (and failed to read response body: %v)", resp.StatusCode, readErr)
		}
		// Return the status code error along with the response body.
		return fmt.Errorf("Discord webhook returned status %d. Response: %s", resp.StatusCode, responseBody.String())
	}

	return nil
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
