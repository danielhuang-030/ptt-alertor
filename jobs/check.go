package jobs

import (
	"context" // Added back as it's used by Checker.Run and Checker.checkOffPeak
	"fmt"     // Needed for taskPseudoID Sprintf
	"os"      // Used by highBoardNames
	"strings"
	"sync" // Used by ckerOnce
	"time" // Used by various functions for durations and tickers

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/channels/discord"
	"github.com/Ptt-Alertor/ptt-alertor/channels/line"
	"github.com/Ptt-Alertor/ptt-alertor/channels/mail"
	"github.com/Ptt-Alertor/ptt-alertor/channels/messenger"
	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram"
	"github.com/Ptt-Alertor/ptt-alertor/models" // Used by init, Checker.Run, sendMessage, etc.
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/author"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/counter"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
	// "github.com/Ptt-Alertor/ptt-alertor/myutil" // Ensure this is removed or commented
)

const checkHighBoardDuration = 1 * time.Second // Assuming this was from original, kept it.
const workers = 300

var boardCh = make(chan *board.Board, 700) // Assuming this was from original
var highBoards []*board.Board             // Assuming this was from original
var highBoardNames = strings.Split(os.Getenv("BOARD_HIGH"), ",") // Assuming this was from original
var normalBoards []*board.Board // Declared for use in Checker.Run

var ckCh = make(chan check)

func init() {
	// Populate highBoards (assuming this logic is from the original version provided in turn 105 or earlier)
	for _, name := range highBoardNames {
		if name != "" { // Add check for empty name
			bd := models.Board()
			bd.Name = name
			highBoards = append(highBoards, bd)
		}
	}

	// Initialize worker pool
	for i := 0; i < workers; i++ {
		go messageWorker(ckCh)
	}
}

func messageWorker(ckCh chan check) {
	for {
		ck := <-ckCh // Read from channel ONCE per loop iteration

		var taskPseudoID string
		checkerInstance := ck.Self() 

		if len(checkerInstance.articles) > 0 && checkerInstance.articles[0] != nil {
			taskPseudoID = fmt.Sprintf("%s-%s-%s-%s", checkerInstance.Profile.Account, checkerInstance.board, checkerInstance.word, checkerInstance.articles[0].Link)
		} else {
			taskPseudoID = fmt.Sprintf("%s-%s-%s-no_articles_or_empty_link", checkerInstance.Profile.Account, checkerInstance.board, checkerInstance.word)
		}

		log.WithFields(log.Fields{
			"taskPseudoID":      taskPseudoID,
			"targetUserAccount": checkerInstance.Profile.Account,
			"board":             checkerInstance.board,
			"type":              checkerInstance.subType,
			"word":              checkerInstance.word,
			"articleCount":      len(checkerInstance.articles),
			"action":            "received_task_in_messageWorker",
		}).Info("messageWorker: Received notification task from ckCh, preparing to call sendMessage.")
		
		sendMessage(ck) 
	}
}

type check interface {
	String() string
	Self() Checker
	Stop()
	Run()
}

var cker *Checker       // Assuming this was from original
var ckerOnce sync.Once // Assuming this was from original

type Checker struct {
	board    string
	keyword  string
	author   string
	articles article.Articles
	subType  string
	word     string
	Profile  user.Profile
	done     chan struct{}
	ch       chan Checker
	duration time.Duration
}

// NewChecker gets a Checker instance (assuming from original)
func NewChecker() *Checker {
	ckerOnce.Do(func() {
		cker = &Checker{
			duration: 250 * time.Millisecond,
		}
		cker.done = make(chan struct{})
		cker.ch = make(chan Checker)
	})
	return cker
}

func (c Checker) String() string {
	subType := "關鍵字"
	if c.author != "" {
		subType = "作者"
	}
	return fmt.Sprintf(
		`%s@%s
看板：%s; %s%s%s
`,
		c.word, c.board,
		c.board, subType, c.word, c.articles.String())
}

// Self return Checker itself
func (c Checker) Self() Checker {
	return c
}

// Run is main in Job (structure from original, normalBoards logic adapted)
func (c Checker) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				checkBoards(highBoards, checkHighBoardDuration)
			}
		}
	}()

	offPeakCh := make(chan bool)
	go c.checkOffPeak(ctx, offPeakCh)

	go func() {
		var offPeak bool
		duration := c.duration

		// Prepare the list of normal boards by excluding highBoards
		allBoards := models.Board().All()
		highBoardNameSet := make(map[string]bool)
		for _, hb := range highBoards {
			if hb != nil {
				highBoardNameSet[hb.Name] = true
			}
		}

		// Filter allBoards to create the package-level normalBoards
		// Clear it first in case Run is called multiple times (though unlikely for this structure)
		normalBoards = []*board.Board{} 
		for _, b := range allBoards {
			if b != nil { 
				if _, isHigh := highBoardNameSet[b.Name]; !isHigh {
					normalBoards = append(normalBoards, b)
				}
			}
		}
		log.Infof("Initialized board checks: %d high-frequency boards, %d normal-frequency boards.", len(highBoards), len(normalBoards))

		for {
			select {
			case <-ctx.Done():
				for len(offPeakCh) > 0 {
					<-offPeakCh
				}
				return
			case op := <-offPeakCh:
				if offPeak != op {
					if op {
						log.Info("Switch to Slow Mode for normal boards") // Updated log
						duration = c.duration * 2
					} else {
						log.Info("Switch to Normal Mode for normal boards") // Updated log
						duration = c.duration
					}
					offPeak = op
				}
			default:
				checkBoards(normalBoards, duration)
			}
		}
	}()

	for {
		select {
		case bd := <-boardCh:
			go checkKeywordSubscriber(bd, c)
			go checkAuthorSubscriber(bd, c)
		case receivedCker := <-c.ch: // Renamed from cker to receivedCker
			log.WithFields(log.Fields{
				"targetUserAccount": receivedCker.Profile.Account,
				"board":             receivedCker.board,
				"type":              receivedCker.subType,
				"word":              receivedCker.word,
				"articleCount":      len(receivedCker.articles),
				"action":            "relaying_task_to_ckCh",
			}).Info("Received notification task from c.ch, relaying to ckCh for actual sending.")
			ckCh <- receivedCker
		case <-c.done:
			cancel()
			for len(boardCh) > 0 {
				<-boardCh
			}
			for len(c.ch) > 0 {
				<-c.ch
			}
			return
		}
	}
}

func (c Checker) checkOffPeak(ctx context.Context, offPeakCh chan<- bool) {
	loc := time.FixedZone("CST", 8*60*60)
	ticker := time.NewTicker(10 * time.Minute)
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			t := now.In(loc)
			if t.Hour() >= 3 && t.Hour() < 7 {
				offPeakCh <- true
			} else {
				offPeakCh <- false
			}
		}
	}
}

func (c Checker) Stop() {
	c.done <- struct{}{}
	log.Info("Checker Stop")
}

func checkBoards(bds []*board.Board, duration time.Duration) {
	for _, bd := range bds {
		if bd == nil { // Nil check for safety
			continue
		}
		time.Sleep(duration)
		go checkNewArticle(bd, boardCh)
	}
}

func checkNewArticle(bd *board.Board, boardCh chan *board.Board) {
	if bd == nil { // Nil check
		log.Warn("checkNewArticle called with nil board")
		return
	}
	log.WithFields(log.Fields{"board": bd.Name, "source": "checkNewArticle"}).Debug("Preparing to call bd.WithNewArticles()")
	bd.WithNewArticles()
	log.WithFields(log.Fields{"board": bd.Name, "newArticlesCount": len(bd.NewArticles), "source": "checkNewArticle"}).Debug("Returned from bd.WithNewArticles()")
	if bd.NewArticles == nil && len(bd.OnlineArticles) > 0 {
		bd.Articles = bd.OnlineArticles
		log.WithField("board", bd.Name).Info("Created Articles")
		bd.Save()
	}
	if len(bd.NewArticles) != 0 {
		bd.Articles = bd.OnlineArticles
		log.WithField("board", bd.Name).Info("Updated Articles")
		if err := bd.Save(); err == nil {
			log.WithFields(log.Fields{"board": bd.Name, "newArticlesCount": len(bd.NewArticles), "action": "sending_to_boardCh"}).Info("Board has new articles, sending to boardCh for processing.")
			boardCh <- bd
		}
	}
}

func checkKeywordSubscriber(bd *board.Board, cker Checker) {
	if bd == nil { // Nil check
		return
	}
	u := models.User()
	accounts := keyword.Subscribers(bd.Name)
	for _, account := range accounts {
		user := u.Find(account)
		if user.Enable {
			cker.Profile = user.Profile
			go checkKeywordSubscription(user, bd, cker)
		}
	}
}

func checkKeywordSubscription(user user.User, bd *board.Board, cker Checker) {
	if bd == nil { // Nil check
		return
	}
	for _, sub := range user.Subscribes {
		if bd.Name == sub.Board {
			cker.board = sub.Board
			for _, keyword := range sub.Keywords {
				go checkKeyword(keyword, bd, cker)
			}
		}
	}
}

func checkKeyword(keyword string, bd *board.Board, cker Checker) {
	if bd == nil { // Nil check
		return
	}
	log.WithFields(log.Fields{
		"board":                cker.board,
		"keyword":              keyword,
		"articlesToCheckCount": len(bd.NewArticles),
		"targetUserAccount":    cker.Profile.Account,
	}).Debug("Checking keyword match for user.")
	keywordArticles := make(article.Articles, 0)
	for _, newAtcl := range bd.NewArticles {
		if newAtcl != nil && newAtcl.MatchKeyword(keyword) { // Nil check for newAtcl
			newAtcl.Author = ""
			keywordArticles = append(keywordArticles, newAtcl)
		}
	}
	if len(keywordArticles) != 0 {
		cker.keyword = keyword
		cker.articles = keywordArticles
		cker.subType = "keyword"
		cker.word = keyword
		log.WithFields(log.Fields{
			"board":                cker.board,
			"keyword":              keyword,
			"matchedArticlesCount": len(keywordArticles),
			"targetUserAccount":    cker.Profile.Account,
			"action":               "sending_to_cker_ch_for_keyword",
		}).Info("Keyword matched, sending notification task to cker.ch.")
		cker.ch <- cker
	}
}

func checkAuthorSubscriber(bd *board.Board, cker Checker) {
	if bd == nil { // Nil check
		return
	}
	u := models.User()
	accounts := author.Subscribers(bd.Name)
	for _, account := range accounts {
		user := u.Find(account)
		if user.Enable {
			cker.Profile = user.Profile
			go checkAuthorSubscription(user, bd, cker)
		}
	}
}

func checkAuthorSubscription(user user.User, bd *board.Board, cker Checker) {
	if bd == nil { // Nil check
		return
	}
	for _, sub := range user.Subscribes {
		if bd.Name == sub.Board {
			cker.board = sub.Board
			for _, author := range sub.Authors {
				go checkAuthor(author, bd, cker)
			}
		}
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

	log.WithFields(log.Fields{
		"targetUserAccount": cr.Profile.Account, 
		"board": cr.board,
		"type": cr.subType,
		"word": cr.word,
		"articleCount": len(cr.articles),
		"action": "sendMessage_entry",
	}).Info("sendMessage: Entered function to process notification task.")

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
