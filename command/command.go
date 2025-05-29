package command

import (
	"bytes"
	"errors"
	"flag"
	"regexp"
	"strconv"
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	"github.com/Ptt-Alertor/ptt-alertor/ptt/web"

	"fmt"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/subscription"
	"github.com/Ptt-Alertor/ptt-alertor/models/top"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

const subArticlesLimit int = 50
const updateFailedMsg string = "失敗，請嘗試封鎖再解封鎖，並重新執行註冊步驟。\n若問題未解決，請至粉絲團或 LINE 首頁留言。"

var inputErrorTips = []string{
	"指令格式錯誤。",
	"1. 需以空白分隔動作、板名、參數",
	"2. 板名欄位開頭與結尾不可有逗號",
	"3. 板名欄位間不允許空白字元。",
}

// Commands is commands documents
var Commands = map[string]map[string]string{
	"一般": {
		"指令": "可使用的指令清單",
		"清單": "設定的看板、關鍵字、作者",
		"排行": "前五名追蹤的關鍵字、作者",
		"listen (或 監聽)": "啟用 PTT Alertor 於目前頻道",
		"unlisten (或 取消監聽)": "停用 PTT Alertor 於目前頻道",
	},
	"關鍵字相關": {
		"新增 看板 關鍵字": "新增追蹤關鍵字",
		"刪除 看板 關鍵字": "取消追蹤關鍵字",
		"範例":        "新增 gossiping,movie 金城武,結衣",
	},
	"作者相關": {
		"新增作者 看板 作者": "新增追蹤作者",
		"刪除作者 看板 作者": "取消追蹤作者",
		"範例":         "新增作者 gossiping ffaarr,obov",
	},
	"推噓文數相關": {
		"新增(推/噓)文數 看板 總數": "通知推或噓文數",
		"範例":              "新增推文數 joke,beauty 10",
		"歸零即刪除":           "新增噓文數 joke 0",
	},
	"推文相關": {
		"新增推文 網址": "新增推文追蹤",
		"刪除推文 網址": "刪除推文追蹤",
		"範例":      "新增推文 https://www.ptt.cc/bbs/EZsoft/M.1497363598.A.74E.html",
	},
	"進階應用": {
		"參考連結": "https://line-notify.sating.cc/docs",
	},
}

var commandActionMap = map[string]updateAction{
	"新增":    addKeywords,
	"刪除":    removeKeywords,
	"新增作者":  addAuthors,
	"刪除作者":  removeAuthors,
	"新增推文":  addArticles,
	"刪除推文":  removeArticles,
	"新增推文數": updatePushUp,
	"新增噓文數": updatePushDown,
}

// HandleCommand handles command from chatbot
func HandleCommand(text string, userID string, isUser bool) string {
	command := strings.ToLower(strings.Fields(strings.TrimSpace(text))[0])
	if isUser {
		log.WithFields(log.Fields{
			"account": userID,
			"command": command,
		}).Info("Command Request")
	}
	switch command {
	case "debug":
		return handleDebug(userID)
	case "清單", "list":
		return handleList(userID)
	case "指令", "help":
		return stringCommands()
	case "排行", "ranking":
		return listTop()
	case "新增", "刪除":
		re := regexp.MustCompile("^(新增|刪除)\\s+([^,，][\\w-_,，\\.]*[^,，:\\s]):?\\s+(\\*|.*[^\\s])")
		if matched := re.MatchString(text); !matched {
			errorTips := inputErrorTips
			additionalTips := []string{
				"正確範例：",
				command + " gossiping,lol 問卦,爆卦",
			}
			errorTips = append(errorTips, additionalTips...)
			return strings.Join(errorTips, "\n")
		}
		args := re.FindStringSubmatch(text)
		result, err := handleKeyword(command, userID, args[2], args[3])
		if err != nil {
			return err.Error()
		}
		return result
	case "新增作者", "刪除作者":
		re := regexp.MustCompile("^(新增作者|刪除作者)\\s+([^,，][\\w-_,，\\.]*[^,，:\\s]):?\\s+(\\*|[\\s,\\w]+)$")
		matched := re.MatchString(text)
		if !matched {
			errorTips := inputErrorTips
			additionalTips := []string{
				"4. 作者為半形英文與數字組成。",
				"正確範例：",
				command + " gossiping,lol ffaarr,obov",
			}
			errorTips = append(errorTips, additionalTips...)
			return strings.Join(errorTips, "\n")
		}
		args := re.FindStringSubmatch(text)
		result, err := handleAuthor(command, userID, args[2], args[3])
		if err != nil {
			return err.Error()
		}
		return result
	case "新增推文數", "新增噓文數":
		re := regexp.MustCompile("^(新增推文數|新增噓文數)\\s+([^,，][\\w-_,，\\.]*[^,，:\\s]):?\\s+(100|[1-9][0-9]|[0-9])$")
		matched := re.MatchString(text)
		if !matched {
			errorTips := inputErrorTips
			additionalTips := []string{
				"4. 推噓文數需為介於 0-100 的數字",
				"正確範例：",
				command + " gossiping,beauty 100",
			}
			errorTips = append(errorTips, additionalTips...)
			return strings.Join(errorTips, "\n")
		}
		args := re.FindStringSubmatch(text)
		result, err := handlePushSum(command, userID, args[2], args[3])
		if err != nil {
			return err.Error()
		}
		return result
	case "新增推文", "刪除推文":
		re := regexp.MustCompile("^(新增推文|刪除推文)\\s+https?://www.ptt.cc/bbs/([\\w-_]*)/(M\\.\\d+.A.\\w*)\\.html$")
		matched := re.MatchString(text)
		if !matched {
			errorTips := []string{
				"指令格式錯誤。",
				"1. 網址與指令需至少一個空白。",
				"2. 網址錯誤格式。",
				"正確範例：",
				command + " https://www.ptt.cc/bbs/EZsoft/M.1497363598.A.74E.html",
			}
			return strings.Join(errorTips, "\n")
		}
		args := re.FindStringSubmatch(text)
		result, err := handleComment(command, userID, args[2], args[3])
		if err != nil {
			return err.Error()
		}
		return result
	case "清理推文":
		return cleanCommentList(userID)
	case "推文清單":
		return handleCommentList(userID)
	case "add", "del":
		return handleCommandLine(userID, command, text)
	case "listen", "監聽":
		return handleListen(userID) // userID is actually channelID here
	case "unlisten", "取消監聽":
		return handleUnlisten(userID) // userID is actually channelID here
	}
	if !isUser {
		return ""
	}
	return "無此指令，請打「指令」查看指令清單"
}

// handleListen enables PTT Alertor service for the given channelID.
func handleListen(channelID string) string {
	u := models.User().Find(channelID)
	// If u.Profile.Account is empty, it implies the user (channel) was not found or is not properly initialized.
	// For Discord, HandleDiscordFollow should have already created/updated the user.
	// So, an empty Account here means something went wrong, or the initial follow hasn't happened.
	if u.Profile.Account == "" {
		// This message assumes that a follow/setup command should have been issued first if the account is new.
		// Or, it implies that an operation was attempted on a channelID not yet registered.
		return "啟用服務失敗，無法識別頻道帳號。請嘗試再次發送指令。"
	}
	u.Enable = true
	if err := u.Update(); err != nil {
		log.WithError(err).WithField("channelID", channelID).Error("Enable service failed in handleListen")
		return "啟用服務失敗，請稍後再試或聯繫開發者。"
	}
	log.WithField("channelID", channelID).Info("Service enabled for channel")
	return "已成功啟用 PTT Alertor 服務於此頻道。"
}

// handleUnlisten disables PTT Alertor service for the given channelID.
func handleUnlisten(channelID string) string {
	u := models.User().Find(channelID)
	if u.Profile.Account == "" {
		// Similar to handleListen, an empty Account suggests the channel isn't recognized.
		return "停用服務失敗，無法識別頻道帳號。請確認此頻道是否已啟用服務。"
	}
	u.Enable = false
	if err := u.Update(); err != nil {
		log.WithError(err).WithField("channelID", channelID).Error("Disable service failed in handleUnlisten")
		return "停用服務失敗，請稍後再試或聯繫開發者。"
	}
	log.WithField("channelID", channelID).Info("Service disabled for channel")
	return "已成功停用 PTT Alertor 服務於此頻道。"
}

func handleCommandLine(userID, command, text string) string {
	var keywordStr, authorStr, push, boo string
	cl := flag.NewFlagSet("Ptt Alertor: <add|del> <-flag <argument>> <board> [board...]\nexample: add -k ptt -a chodino -p 10 ezsoft", flag.ContinueOnError)
	bf := new(bytes.Buffer)
	cl.SetOutput(bf)
	cl.StringVar(&keywordStr, "keyword", "", "keywords: <keyword>[,keyword...]")
	cl.StringVar(&keywordStr, "k", "", "abbr. of keyword")
	cl.StringVar(&authorStr, "author", "", "authors: <author>[,author...]")
	cl.StringVar(&authorStr, "a", "", "abbr. of author")
	cl.StringVar(&push, "push", "", "number of push's sum: <sum>")
	cl.StringVar(&push, "p", "", "abbr. of push")
	cl.StringVar(&boo, "boo", "", "number of boo's sum: <sum>")
	cl.StringVar(&boo, "b", "", "abbr. of boo")

	args := strings.Fields(text)
	err := cl.Parse(args[1:])
	boardStrs := cl.Args()
	for i := 0; i < len(boardStrs); i++ {
		boardStrs[i] = strings.TrimSpace(strings.Trim(boardStrs[i], ","))
	}
	boardStr := strings.Join(boardStrs, ",")
	if bf.Len() != 0 {
		return bf.String()
	}
	if cl.NFlag() == 0 {
		return "未指定參數。輸入 " + command + " -h 查看參數列表。"
	}
	if boardStr == "" {
		errorTips := []string{
			"未指定板名。",
			"範例：add -k ptt -a chodino -p 10 ezsoft",
			"輸入 " + command + " -h 查看提示訊息。",
		}
		return strings.Join(errorTips, "\n")
	}

	log.WithField("command", text).Info("Command Line Request")

	var commandPrefix string
	switch command {
	case "add":
		commandPrefix = "新增"
	case "del":
		commandPrefix = "刪除"
	}

	var errMsgs myutil.StringSlice
	if keywordStr != "" {
		command = commandPrefix
		_, err = handleKeyword(command, userID, boardStr, keywordStr)
		if err != nil {
			errMsgs.AppendNonRepeatElement(err.Error(), false)
		}
	}
	if authorStr != "" {
		command = commandPrefix + "作者"
		_, err = handleAuthor(command, userID, boardStr, authorStr)
		if err != nil {
			errMsgs.AppendNonRepeatElement(err.Error(), false)
		}
	}
	if push != "" {
		if commandPrefix == "刪除" {
			push = "0"
		}
		command = "新增推文數"
		_, err = handlePushSum(command, userID, boardStr, push)
		if err != nil {
			errMsgs.AppendNonRepeatElement(err.Error(), false)
		}
	}
	if boo != "" {
		if commandPrefix == "刪除" {
			boo = "0"
		}
		command = "新增噓文數"
		_, err = handlePushSum(command, userID, boardStr, boo)
		if err != nil {
			errMsgs.AppendNonRepeatElement(err.Error(), false)
		}
	}
	if len(errMsgs) != 0 {
		return strings.Join([]string(errMsgs), "\n")
	}
	return commandPrefix + "成功。"
}

func handleDebug(account string) string {
	return models.User().Find(account).Profile.Account
}

func handleList(account string) string {
	subs := models.User().Find(account).Subscribes
	if len(subs) == 0 {
		return "尚未建立清單。請打「指令」查看新增方法。"
	}
	return subs.String()
}

func cleanCommentList(account string) string {
	var i int
	for _, sub := range models.User().Find(account).Subscribes {
		for _, code := range sub.Articles {
			article := models.Article()
			article.Code = code
			bl, err := article.Exist()
			if err != nil {
				return "清理推文失敗，請洽至粉絲團或 LINE 首頁留言。"
			}
			if !bl {
				update(removeArticles, account, []string{sub.Board}, code)
				i++
			}
		}
	}
	return fmt.Sprintf("清理 %d 則推文", i)
}

func handleCommentList(account string) string {
	subs := models.User().Find(account).Subscribes
	if len(subs) == 0 {
		return "尚未建立清單。請打「指令」查看新增方法。"
	}
	return "推文追蹤清單，上限 50 篇：\n" + subs.StringCommentList() + "\n輸入「清理推文」，可刪除無效連結。"
}

func stringCommands() string {
	str := ""
	for cat, cmds := range Commands {
		str += "[" + cat + "]\n"
		for cmd, doc := range cmds {
			str += cmd
			if doc != "" {
				str += "：" + doc
			}
			str += "\n"
		}
		str += "\n"
	}
	return strings.TrimSpace(str)
}

func listTop() string {
	content := "關鍵字"
	for i, keyword := range top.ListKeywords(5) {
		content += fmt.Sprintf("\n%d. %s", i+1, keyword)
	}
	content += "\n----\n作者"
	for i, author := range top.ListAuthors(5) {
		content += fmt.Sprintf("\n%d. %s", i+1, author)
	}
	content += "\n----\n推噓文"
	for i, pushSum := range top.ListPushSum(5) {
		content += fmt.Sprintf("\n%d. %s", i+1, pushSum)
	}
	content += "\n\nTOP 100:\nhttps://line-notify.sating.cc/top"
	return content
}

func handleKeyword(command, userID, board, keywordStr string) (string, error) {
	boardNames := splitParamString(board)
	input := keywordStr
	var inputs []string
	if strings.HasPrefix(input, "regexp:") {
		if !checkRegexp(input) {
			return "", errors.New("正規表示式錯誤，請檢查規則。")
		}
		inputs = []string{keywordStr}
	} else {
		inputs = splitParamString(keywordStr)
	}
	log.WithFields(log.Fields{
		"id":      userID,
		"command": command,
		"boards":  boardNames,
		"words":   inputs,
	}).Info("Keyword Command")
	err := update(commandActionMap[command], userID, boardNames, inputs...)
	if msg, ok := checkBoardError(err); ok {
		return "", errors.New(msg)
	}
	if err != nil {
		log.WithError(err).Error("Keyword Command Failed")
		return "", errors.New(command + updateFailedMsg)
	}
	return command + "成功", nil
}

func handleAuthor(command, userID, board, authorStr string) (string, error) {
	if ok, _ := regexp.MatchString("^(\\*|[\\s,\\w]+)$", authorStr); !ok {
		return "", errors.New("作者為半形英文與數字組成。")
	}
	boardNames := splitParamString(board)
	authors := splitParamString(authorStr)
	log.WithFields(log.Fields{
		"id":      userID,
		"command": command,
		"boards":  boardNames,
		"words":   authors,
	}).Info("Author Command")
	err := update(commandActionMap[command], userID, boardNames, authors...)
	if msg, ok := checkBoardError(err); ok {
		return "", errors.New(msg)
	}
	if err != nil {
		log.WithError(err).Error("Author Command Failed")
		return "", errors.New(command + updateFailedMsg)
	}
	return command + "成功", nil
}

func handlePushSum(command, account, board, sumStr string) (string, error) {
	if sum, err := strconv.Atoi(sumStr); err != nil || sum < 0 || sum > 100 {
		return "", errors.New("推噓文數需為介於 0-100 的數字")
	}
	boardNames := splitParamString(board)
	log.WithFields(log.Fields{
		"id":      account,
		"command": command,
		"boards":  boardNames,
		"words":   sumStr,
	}).Info("PushSum Command")
	for _, boardName := range boardNames {
		if strings.EqualFold(boardName, "allpost") {
			return "", errors.New("推文數通知不支持 ALLPOST 板。")
		}
	}
	err := update(commandActionMap[command], account, boardNames, sumStr)
	if msg, ok := checkBoardError(err); ok {
		return "", errors.New(msg)
	}
	if err != nil {
		log.WithError(err).Error("PushSum Command Failed")
		return "", errors.New(command + updateFailedMsg)
	}
	return command + "成功", nil
}

func handleComment(command, userID, boardName, articleCode string) (string, error) {
	log.WithFields(log.Fields{
		"id":      userID,
		"command": command,
		"boards":  boardName,
		"words":   articleCode,
	}).Info("Comment Command")
	if strings.EqualFold(command, "新增推文") {
		if !checkArticleExist(boardName, articleCode) {
			return "", errors.New("文章不存在")
		}
		if countUserArticles(userID) >= subArticlesLimit {
			return "", errors.New("推文追蹤最多 50 篇，輸入「推文清單」，整理追蹤列表。")
		}
	}
	err := update(commandActionMap[command], userID, []string{boardName}, articleCode)
	if err != nil {
		log.WithError(err).Error("Comment Command Failed")
		return "", errors.New(command + updateFailedMsg)
	}
	return command + "成功", nil
}

func countUserArticles(account string) (cnt int) {
	for _, sub := range models.User().Find(account).Subscribes {
		cnt += len(sub.Articles)
	}
	return cnt
}

func checkArticleExist(boardName, articleCode string) bool {
	a := models.Article()
	a.Code = articleCode
	if bl, _ := a.Exist(); bl {
		return true
	}
	if web.CheckArticleExist(boardName, articleCode) {
		a.Board = boardName
		initialArticle(a)
		return true
	}
	return false
}

func initialArticle(a *article.Article) error {
	atcl, err := web.FetchArticle(a.Board, a.Code)
	if err != nil {
		return err
	}
	a.Link = atcl.Link
	a.Title = atcl.Title
	a.ID = atcl.ID
	a.LastPushDateTime = atcl.LastPushDateTime
	a.Comments = atcl.Comments
	return a.Save()
}

func checkBoardError(err error) (string, bool) {
	if bErr, ok := err.(board.BoardNotExistError); ok {
		return "板名錯誤，請確認拼字。可能板名：\n" + bErr.Suggestion, true
	}
	return "", false
}

func checkRegexp(input string) bool {
	pattern := strings.Replace(strings.TrimPrefix(input, "regexp:"), "//", "////", -1)
	_, err := regexp.Compile(pattern)
	if err != nil {
		return false
	}
	return true
}

func splitParamString(paramString string) (params []string) {
	paramString = strings.Trim(paramString, ",，")
	if !strings.ContainsAny(paramString, ",，") {
		return []string{paramString}
	}

	if strings.Contains(paramString, ",") {
		params = strings.Split(paramString, ",")
	} else {
		params = []string{paramString}
	}

	for i := 0; i < len(params); i++ {
		if strings.Contains(params[i], "，") {
			params = append(params[:i], append(strings.Split(params[i], "，"), params[i+1:]...)...)
			i--
		}
	}

	for i, param := range params {
		params[i] = strings.TrimSpace(param)
	}

	return params
}

func update(action updateAction, account string, boardNames []string, inputs ...string) error {
	u := models.User().Find(account)
	if boardNames[0] == "**" {
		boardNames = nil
		for _, uSub := range u.Subscribes {
			boardNames = append(boardNames, uSub.Board)
		}
	}
	for _, boardName := range boardNames {
		sub := subscription.Subscription{
			Board: strings.ToLower(boardName),
		}
		err := action(&u, sub, inputs...)
		if err != nil {
			return err
		}
		err = u.Update()
		if err != nil {
			log.WithError(err).Error("Subscription Update Error")
			return err
		}
	}
	return nil
}

func HandleLineFollow(id, accountType string) error {
	u := models.User().Find(id)
	isNewUser := u.Profile.Messenger == "" && u.Profile.Telegram == "" && u.Profile.DiscordChannelID == "" && u.Profile.Line == ""
	
	u.Profile.Line, u.Profile.Type = id, accountType
	if u.Profile.Account == "" {
		u.Profile.Account = id
	}

	logFields := log.Fields{
		"id":       id,
		"type":     accountType,
		"platform": "line",
		"isNew":    isNewUser,
	}
	if isNewUser {
		log.WithFields(logFields).Info("New user joining via Line")
	} else {
		log.WithFields(logFields).Info("Existing user updated/re-joined via Line")
	}
	return handleFollow(u, isNewUser)
}

func HandleMessengerFollow(id string) error {
	u := models.User().Find(id)
	isNewUser := u.Profile.Line == "" && u.Profile.Telegram == "" && u.Profile.DiscordChannelID == "" && u.Profile.Messenger == ""
	
	u.Profile.Messenger = id
	if u.Profile.Account == "" {
		u.Profile.Account = id
	}

	logFields := log.Fields{
		"id":       id,
		"platform": "messenger",
		"isNew":    isNewUser,
	}
	if isNewUser {
		log.WithFields(logFields).Info("New user joining via Messenger")
	} else {
		log.WithFields(logFields).Info("Existing user updated/re-joined via Messenger")
	}
	return handleFollow(u, isNewUser)
}

func HandleTelegramFollow(id string, chatID int64) error {
	u := models.User().Find(id)
	isNewUser := u.Profile.Line == "" && u.Profile.Messenger == "" && u.Profile.DiscordChannelID == "" && u.Profile.Telegram == ""

	u.Profile.Telegram = id
	u.Profile.TelegramChat = chatID
	if u.Profile.Account == "" {
		u.Profile.Account = id
	}
	
	logFields := log.Fields{
		"id":       id,
		"chatID":   chatID,
		"platform": "telegram",
		"isNew":    isNewUser,
	}
	if isNewUser {
		log.WithFields(logFields).Info("New user joining via Telegram")
	} else {
		log.WithFields(logFields).Info("Existing user updated/re-joined via Telegram")
	}
	return handleFollow(u, isNewUser)
}

// HandleDiscordFollow handles user follow event from Discord.
// The 'userID' parameter now receives the channelID, which will be used as the accountKey.
// channelID is the Discord Channel ID where the command was initiated or for DMs.
// guildID is the Discord Guild ID (server ID) if applicable.
func HandleDiscordFollow(guildID, channelID, userIDAsAccountKey string) error {
	accountKey := userIDAsAccountKey // userIDAsAccountKey is the m.ChannelID passed from discord.go
	u := models.User().Find(accountKey)

	// Determine if this is a new user.
	// A reliable way is to check if u.CreateTime is zero, assuming it's set upon creation and not otherwise.
	// Or, if u.Profile.Account is empty after Find if Find doesn't auto-populate it.
	// Let's assume u.CreateTime.IsZero() is the correct check for a new user record.
	// Another way: check if Profile.Account is empty if Find doesn't set it for new users.
	// For this implementation, we'll check if the user existed before this call.
	// This requires checking a field that is only populated upon actual user creation/saving.
	// If u.Profile.Account is not set by Find if the user doesn't exist, that's a good check.
	// Let's assume u.Profile.Account would be empty if the user is truly new before any assignment.
	// However, models.User().Find(accountKey) might initialize u.Profile.Account = accountKey
	// A more robust check for "newness" is often `u.CreateTime.IsZero()` if CreateTime is only set upon actual creation.
	// Or, if `Find` returns a user with an empty `Account` field when it's a new record.
	// Given the existing structure, let's capture the state *before* we modify `u.Profile`.
	
	isNewUser := u.CreateTime.IsZero() // Assuming CreateTime is zero for a new user object from Find.
                                      // Or more accurately, if Find returns a user that doesn't exist yet, 
                                      // its CreateTime (or equivalent persistent field) would be the zero value.

	// Set or update the user's profile
	u.Profile.Account = accountKey
	u.Profile.DiscordChannelID = accountKey // Now using accountKey (channelID) here as well
	u.Profile.Type = "discord_channel"     // Set type to "discord_channel"

	// Clear other platform identifiers if this is primarily a Discord channel user
	// This depends on desired behavior: should a Discord channel "take over" an existing user?
	// For now, we won't clear them, allowing a user to be multi-platform.
	// u.Profile.Line = ""
	// u.Profile.Messenger = ""
	// u.Profile.Telegram = ""
	// u.Profile.Email = ""


	logFields := log.Fields{
		"accountKey": accountKey,
		"channelID":  channelID, // This is the same as accountKey in this context
		"guildID":    guildID,
		"isNewUser":  isNewUser,
		"platform":   "discord",
	}

	if isNewUser {
		log.WithFields(logFields).Info("New Discord channel user record to be created")
	} else {
		log.WithFields(logFields).Info("Existing Discord channel user record to be updated")
	}

	return handleFollow(u, isNewUser)
}

func handleFollow(u user.User, isNewUser bool) error {
	u.Enable = true
	if u.Profile.Account == "" {
		// This should ideally not happen if callers ensure Profile.Account is set.
		log.Error("handleFollow called with empty u.Profile.Account. This indicates an issue in the calling function.")
		return errors.New("user account identifier is missing in handleFollow")
	}

	if isNewUser {
		log.Infof("Attempting to save new user %s", u.Profile.Account)
		if err := u.Save(); err != nil {
			log.Errorf("Failed to save new user %s: %v", u.Profile.Account, err)
			return err
		}
		log.Infof("Successfully saved new user %s", u.Profile.Account)
		return nil
	}

	log.Infof("Attempting to update existing user %s", u.Profile.Account)
	if err := u.Update(); err != nil {
		log.Errorf("Failed to update user %s: %v", u.Profile.Account, err)
		return err
	}
	log.Infof("Successfully updated user %s", u.Profile.Account)
	return nil
}
