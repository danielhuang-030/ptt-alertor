package telegram

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"

	log "github.com/Ptt-Alertor/logrus"

	"strconv"

	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	"github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/julienschmidt/httprouter"
)

var (
	bot   *tgbotapi.BotAPI
	err   error
	token = os.Getenv("TELEGRAM_TOKEN")
	host  = os.Getenv("APP_HOST")
)

func init() {
	if token == "" {
		log.Warnf("TELEGRAM_TOKEN is empty. Telegram channel will be disabled.")
		bot = nil
		return
	}

	var initErr error
	bot, initErr = tgbotapi.NewBotAPI(token)
	if initErr != nil {
		log.Warnf("Telegram Bot initialization failed: %v. Telegram channel will be disabled.", initErr)
		bot = nil
		return
	}
	log.Info("Telegram Authorized on " + bot.Self.UserName) // Moved here

	if host == "" {
		log.Warnf("APP_HOST is empty. Cannot set Telegram webhook. Telegram channel functionality might be limited or disabled.")
		bot = nil // 如果 Webhook 是必要的，則禁用
		return
	}
	
	webhookConfig := tgbotapi.NewWebhook(host + "/telegram/" + token)
	webhookConfig.MaxConnections = 100
	var whErr error // Use local error variable, matching prompt's whErr
	_, whErr = bot.SetWebhook(webhookConfig) 
	if whErr != nil {
		log.Warnf("Telegram Bot set webhook failed: %v. Telegram channel will be disabled.", whErr)
		bot = nil 
		return
	}
	log.Info("Telegram Bot Sets Webhook Success for " + bot.Self.UserName)
}

// HandleRequest handles request from webhook
func HandleRequest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if bot == nil {
		log.Warn("Telegram channel is disabled, cannot handle request.")
		// Optionally, write an HTTP error response to the client
		// http.Error(w, "Telegram service is not configured or is currently unavailable.", http.StatusServiceUnavailable)
		return
	}
	bytes, err := ioutil.ReadAll(r.Body) // Use local err
	if err != nil {
		log.WithError(err).Error("Telegram Read Request Body Failed")
	}

	var update tgbotapi.Update
	json.Unmarshal(bytes, &update)

	if update.CallbackQuery != nil {
		handleCallbackQuery(update)
		return
	}

	if update.Message != nil {
		if update.Message.IsCommand() {
			handleCommand(update)
			return
		}
		if update.Message.Text != "" {
			handleText(update)
			return
		}
	}
}

func handleCallbackQuery(update tgbotapi.Update) {
	var responseText string
	userID := strconv.Itoa(update.CallbackQuery.From.ID)
	switch update.CallbackQuery.Data {
	case "CANCEL":
		responseText = "取消"
	default:
		responseText = command.HandleCommand(update.CallbackQuery.Data, userID, true)
	}
	SendTextMessage(update.CallbackQuery.Message.Chat.ID, responseText)
}

// help - 所有指令清單
// list - 設定清單
// ranking - 熱門關鍵字、作者、推文數
// add - 新增看板關鍵字、作者、推文數
// del - 刪除看板關鍵字、作者、推文數
// showkeyboard - 顯示快捷小鍵盤
// hidekeyboard - 隱藏快捷小鍵盤
func handleCommand(update tgbotapi.Update) {
	var responseText string
	userID := strconv.Itoa(update.Message.From.ID)
	chatID := update.Message.Chat.ID

	switch update.Message.Command() {
	case "add", "del":
		text := update.Message.Command() + " " + update.Message.CommandArguments()
		responseText = command.HandleCommand(text, userID, true)
	case "start":
		command.HandleTelegramFollow(userID, chatID)
		responseText = "歡迎使用 Ptt Alertor\n輸入「指令」查看相關功能。\n\n觀看Demo:\nhttps://media.giphy.com/media/3ohzdF6vidM6I49lQs/giphy.gif"
	case "help":
		responseText = command.HandleCommand("help", userID, true)
	case "list":
		responseText = command.HandleCommand("list", userID, true)
	case "ranking":
		responseText = command.HandleCommand("ranking", userID, true)
	case "showkeyboard":
		showReplyKeyboard(chatID)
		return
	case "hidekeyboard":
		hideReplyKeyboard(chatID)
		return
	default:
		responseText = "I don't know the command"
	}
	SendTextMessage(chatID, responseText)
}

func handleText(update tgbotapi.Update) {
	var responseText string
	userID := strconv.Itoa(update.Message.From.ID)
	chatID := update.Message.Chat.ID
	text := update.Message.Text
	if match, _ := regexp.MatchString("^(刪除|刪除作者)+\\s.*\\*+", text); match {
		sendConfirmation(chatID, text)
		return
	}
	responseText = command.HandleCommand(text, userID, true)
	SendTextMessage(chatID, responseText)
}

func sendConfirmation(chatID int64, cmd string) {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("是", cmd),
			tgbotapi.NewInlineKeyboardButtonData("否", "CANCEL"),
		))
	msg := tgbotapi.NewMessage(chatID, "確定"+cmd+"？")
	msg.ReplyMarkup = markup
	if bot == nil {
		log.Warn("Telegram channel is disabled, cannot send confirmation.")
		return
	}
	var sendErr error // Use local err
	_, sendErr = bot.Send(msg)
	if sendErr != nil {
		log.WithError(sendErr).Error("Telegram Send Confirmation Failed")
	}
}

const maxCharacters = 4096

// SendTextMessage sends text message to chatID
func SendTextMessage(chatID int64, text string) {
	if bot == nil { // Added check for exported function
		log.Warn("Telegram channel is disabled, cannot send message (exported function).")
		return
	}
	for _, msg := range myutil.SplitTextByLineBreak(text, maxCharacters) {
		sendTextMessage(chatID, msg)
	}
}

func sendTextMessage(chatID int64, text string) {
	if bot == nil {
		log.Warn("Telegram channel is disabled, cannot send message (internal function).")
		return
	}
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	var sendErr error // Use local err
	_, sendErr = bot.Send(msg)
	if sendErr != nil {
		log.WithError(sendErr).Error("Telegram Send Message Failed")
	}
}

func showReplyKeyboard(chatID int64) {
	if bot == nil {
		log.Warn("Telegram channel is disabled, cannot show reply keyboard.")
		return
	}
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("清單"),
			tgbotapi.NewKeyboardButton("推文清單"),
			tgbotapi.NewKeyboardButton("排行"),
			tgbotapi.NewKeyboardButton("指令"),
		))
	msg := tgbotapi.NewMessage(chatID, "顯示小鍵盤")
	msg.ReplyMarkup = keyboard
	var sendErr error // Use local err
	_, sendErr = bot.Send(msg)
	if sendErr != nil {
		log.WithError(sendErr).Error("Telegram Show Reply Keyboard Failed")
	}
}

func hideReplyKeyboard(chatID int64) {
	if bot == nil {
		log.Warn("Telegram channel is disabled, cannot hide reply keyboard.")
		return
	}
	msg := tgbotapi.NewMessage(chatID, "隱藏小鍵盤")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	var sendErr error // Use local err
	_, sendErr = bot.Send(msg)
	if sendErr != nil {
		log.WithError(sendErr).Error("Telegram Hide Reply Keyboard Failed")
	}
}
