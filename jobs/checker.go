package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync" // Ensure sync is imported
	"time"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/author"
	"github.com/Ptt-Alertor/ptt-alertor/models/board"
	"github.com/Ptt-Alertor/ptt-alertor/models/keyword"
	"github.com/Ptt-Alertor/ptt-alertor/models/user"
)

const checkHighBoardDuration = 1 * time.Second

var boardCh = make(chan *board.Board, 700)
var highBoards []*board.Board // Populated in init
var highBoardNames = strings.Split(os.Getenv("BOARD_HIGH"), ",")

// Variables for instance-level lock in checkNewArticle
var (
	boardProcessingMutex      = &sync.Mutex{}
	boardsCurrentlyProcessing = make(map[string]bool)
)

func init() {
	for _, name := range highBoardNames {
		if name == "" { // Avoid creating boards with empty names
			continue
		}
		bd := models.Board()
		bd.Name = name
		highBoards = append(highBoards, bd)
	}
}

var cker *Checker
var ckerOnce sync.Once

type Checker struct {
	board    string
	keyword  string
	author   string
	articles article.Articles
	subType  string
	word     string
	Profile  user.Profile
	done     chan struct{}
	ch       chan Checker // Channel to send a prepared Checker instance to the main select loop
	duration time.Duration
}

func NewChecker() *Checker {
	ckerOnce.Do(func() {
		cker = &Checker{
			duration: 250 * time.Millisecond,
		}
		cker.done = make(chan struct{})
		// The channel 'ch' is for instances of Checker, not for 'check' interface type.
		// Its size should be considered based on expected throughput.
		cker.ch = make(chan Checker, 100) // Example size, adjust as needed
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

// Self return Checker itself. This is part of 'check' interface.
func (c Checker) Self() Checker {
	return c
}

func (c Checker) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Goroutine for high-frequency boards
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				if len(highBoards) > 0 {
					checkBoards(highBoards, checkHighBoardDuration)
				} else {
					time.Sleep(checkHighBoardDuration) 
				}
			}
		}
	}()

	offPeakCh := make(chan bool)
	go c.checkOffPeak(ctx, offPeakCh)

	// Goroutine for normal boards
	go func() {
		var offPeak bool
		currentDuration := c.duration 
		
		allBoards := models.Board().All()
		highBoardNameSet := make(map[string]bool)
		if len(highBoards) > 0 {
			for _, hb := range highBoards {
				if hb != nil && hb.Name != "" { 
					highBoardNameSet[hb.Name] = true
				}
			}
		}

		normalBoards := make([]*board.Board, 0)
		if len(allBoards) > 0 {
			for _, b := range allBoards {
				if b != nil && b.Name != "" { 
					if _, isHigh := highBoardNameSet[b.Name]; !isHigh {
						normalBoards = append(normalBoards, b)
					}
				}
			}
		}
		log.Infof("Initialized board checks: %d high-frequency boards, %d normal-frequency boards to be checked by this goroutine.", len(highBoards), len(normalBoards))

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
						log.Info("Switch to Slow Mode for normal boards")
						currentDuration = c.duration * 2
					} else {
						log.Info("Switch to Normal Mode for normal boards")
						currentDuration = c.duration
					}
					offPeak = op
				}
			default:
				if len(normalBoards) > 0 {
					checkBoards(normalBoards, currentDuration)
				} else {
					time.Sleep(currentDuration)
				}
			}
		}
	}()

	// Main select loop for processing boardCh (new articles found) and c.ch (notifications ready)
	for {
		select {
		case bd := <-boardCh:
			if bd == nil { 
				log.Warn("Received nil board from boardCh, skipping.")
				continue
			}
			checkerCopy := c 
			go checkKeywordSubscriber(bd, checkerCopy)
			go checkAuthorSubscriber(bd, checkerCopy)
		case taskToSend := <-c.ch: 
			log.WithFields(log.Fields{
				"targetUserAccount": taskToSend.Profile.Account,
				"board": taskToSend.board,
				"type": taskToSend.subType,
				"word": taskToSend.word,
				"articleCount": len(taskToSend.articles),
				"action": "relaying_task_to_ckCh",
			}).Info("Received notification task from c.ch, relaying to ckCh for actual sending.")
			ckCh <- taskToSend 
		case <-c.done:
			cancel()
			log.Info("Checker.Run: Draining boardCh...")
			for len(boardCh) > 0 { <-boardCh }
			log.Info("Checker.Run: Draining c.ch...")
			for len(c.ch) > 0 { <-c.ch }
			log.Info("Checker.Run: Channels drained, exiting.")
			return
		}
	}
}

func (c Checker) checkOffPeak(ctx context.Context, offPeakCh chan<- bool) {
	loc := time.FixedZone("CST", 8*60*60)
	ticker := time.NewTicker(10 * time.Minute) 
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			t := now.In(loc)
			currentOffPeakState := (t.Hour() >= 3 && t.Hour() < 7)
			offPeakCh <- currentOffPeakState
		}
	}
}

func (c Checker) Stop() {
	select {
	case c.done <- struct{}{}:
		log.Info("Checker Stop signal sent.")
	default:
		log.Warn("Checker Stop signal already sent or 'done' channel not ready.")
	}
}

func checkBoards(bds []*board.Board, duration time.Duration) {
	for _, bd := range bds {
		if bd == nil || bd.Name == "" { 
			log.Warn("checkBoards received a nil or empty-named board in the list, skipping.")
			continue
		}
		time.Sleep(duration) 
		go checkNewArticle(bd, boardCh)
	}
}

func checkNewArticle(bd *board.Board, boardCh chan *board.Board) {
	if bd == nil || bd.Name == "" {
		log.Warn("checkNewArticle received nil board or board with empty name.")
		return
	}

	boardProcessingMutex.Lock()
	if boardsCurrentlyProcessing[bd.Name] {
		boardProcessingMutex.Unlock()
		log.WithField("board", bd.Name).Debug("Board is already being processed by another goroutine, skipping this run.")
		return
	}
	boardsCurrentlyProcessing[bd.Name] = true
	boardProcessingMutex.Unlock()

	defer func() {
		boardProcessingMutex.Lock()
		delete(boardsCurrentlyProcessing, bd.Name)
		boardProcessingMutex.Unlock()
		log.WithField("board", bd.Name).Debug("Finished processing board, lock released.")
	}()

	log.WithFields(log.Fields{"board": bd.Name, "source": "checkNewArticle"}).Debug("Preparing to call bd.WithNewArticles()")
	bd.WithNewArticles() 
	log.WithFields(log.Fields{"board": bd.Name, "newArticlesCount": len(bd.NewArticles), "source": "checkNewArticle"}).Debug("Returned from bd.WithNewArticles()")

	if bd.NewArticles == nil && len(bd.OnlineArticles) > 0 {
		bd.Articles = bd.OnlineArticles 
		log.WithField("board", bd.Name).Info("Board has no prior saved articles, saving current online articles.")
		if err := bd.Save(); err != nil {
			log.WithError(err).WithField("board", bd.Name).Error("Failed to save initial articles for board.")
		}
	}

	if len(bd.NewArticles) > 0 {
		bd.Articles = bd.OnlineArticles 
		log.WithField("board", bd.Name).Info("Updated Articles (new articles found by comparison).") 
		if err := bd.Save(); err == nil { 
			log.WithFields(log.Fields{"board": bd.Name, "newArticlesCount": len(bd.NewArticles), "action": "sending_to_boardCh"}).Info("Board has new articles, sending to boardCh for processing.")
			boardCh <- bd 
		} else {
            log.WithError(err).WithField("board", bd.Name).Error("Failed to save board after finding new articles.")
        }
	}
}

// For checkKeywordSubscriber, checkKeywordSubscription, checkKeyword,
// and their author-based counterparts:
// It's crucial to handle the 'cker' (Checker instance) correctly to avoid race conditions
// if its fields (Profile, board, keyword, author, articles, subType, word) are modified.
// The original 'c' from Run() method receiver should be treated as a template or context.
// For each specific task/goroutine, a distinct Checker value should be used.

func checkKeywordSubscriber(bd *board.Board, cBase Checker) { // Pass cBase by value
	if bd == nil || bd.Name == "" { 
		log.Warn("checkKeywordSubscriber received nil or empty-named board.")
		return
	}
	u := models.User()
	accounts := keyword.Subscribers(bd.Name)
	for _, account := range accounts {
		user := u.Find(account)
		if user.Enable {
			taskCker := cBase             
			taskCker.Profile = user.Profile 
			taskCker.board = bd.Name     
			go checkKeywordSubscription(user, bd, taskCker)
		}
	}
}

func checkKeywordSubscription(user user.User, bd *board.Board, ckerBase Checker) { // Pass ckerBase by value
	if bd == nil || bd.Name == "" {
		log.Warn("checkKeywordSubscription received nil or empty-named board.")
		return
	}
	for _, sub := range user.Subscribes {
		if sub.Board == bd.Name { // Assuming board names are case-sensitive consistently
			for _, keyword := range sub.Keywords {
				taskCker := ckerBase       
				taskCker.board = bd.Name 
				go checkKeyword(keyword, bd, taskCker)
			}
		}
	}
}

func checkKeyword(keyword string, bd *board.Board, cker Checker) { // Pass cker by value
	if bd == nil || bd.Name == "" { 
		log.Warnf("checkKeyword received nil or empty-named board for keyword: %s.", keyword)
		return
	}
	log.WithFields(log.Fields{
		"board": cker.board, 
		"keyword": keyword,
		"articlesToCheckCount": len(bd.NewArticles), 
		"targetUserAccount": cker.Profile.Account,
	}).Debug("Checking keyword match for user.")
	
	keywordArticles := make(article.Articles, 0)
	for _, newAtcl := range bd.NewArticles {
		if newAtcl == nil { continue } 
		if newAtcl.MatchKeyword(keyword) {
			articleCopy := *newAtcl 
			articleCopy.Author = "" 
			keywordArticles = append(keywordArticles, &articleCopy)
		}
	}
	if len(keywordArticles) != 0 {
		taskCker := cker 
		taskCker.keyword = keyword
		taskCker.articles = keywordArticles 
		taskCker.subType = "keyword"
		taskCker.word = keyword
		log.WithFields(log.Fields{
			"board": taskCker.board,
			"keyword": keyword,
			"matchedArticlesCount": len(keywordArticles),
			"targetUserAccount": taskCker.Profile.Account,
			"action": "sending_to_cker_ch_for_keyword",
		}).Info("Keyword matched, sending notification task to cker.ch.")
		taskCker.ch <- taskCker 
	}
}

func checkAuthorSubscriber(bd *board.Board, cBase Checker) { // Pass cBase by value
	if bd == nil || bd.Name == "" {
		log.Warn("checkAuthorSubscriber received nil or empty-named board.")
		return
	}
	u := models.User()
	accounts := author.Subscribers(bd.Name)
	for _, account := range accounts {
		user := u.Find(account)
		if user.Enable {
			taskCker := cBase
			taskCker.Profile = user.Profile
			taskCker.board = bd.Name
			go checkAuthorSubscription(user, bd, taskCker)
		}
	}
}

func checkAuthorSubscription(user user.User, bd *board.Board, ckerBase Checker) { // Pass ckerBase by value
	if bd == nil || bd.Name == "" {
		log.Warn("checkAuthorSubscription received nil or empty-named board.")
		return
	}
	for _, sub := range user.Subscribes {
		if sub.Board == bd.Name {
			for _, author := range sub.Authors {
				taskCker := ckerBase
				taskCker.board = bd.Name
				go checkAuthor(author, bd, taskCker)
			}
		}
	}
}

func checkAuthor(author string, bd *board.Board, cker Checker) { // Pass cker by value
	if bd == nil || bd.Name == "" {
		log.Warnf("checkAuthor received nil or empty-named board for author: %s.", author)
		return
	}
	log.WithFields(log.Fields{
		"board": cker.board, 
		"author": author,
		"articlesToCheckCount": len(bd.NewArticles),
		"targetUserAccount": cker.Profile.Account,
	}).Debug("Checking author match for user.")

	authorArticles := make(article.Articles, 0)
	for _, newAtcl := range bd.NewArticles {
		if newAtcl == nil { continue } 
		if strings.EqualFold(newAtcl.Author, author) {
			articleCopy := *newAtcl 
			authorArticles = append(authorArticles, &articleCopy)
		}
	}
	if len(authorArticles) != 0 {
		taskCker := cker
		taskCker.author = author
		taskCker.articles = authorArticles
		taskCker.subType = "author"
		taskCker.word = author
		log.WithFields(log.Fields{
			"board": taskCker.board,
			"author": author,
			"matchedArticlesCount": len(authorArticles),
			"targetUserAccount": taskCker.Profile.Account,
			"action": "sending_to_cker_ch_for_author",
		}).Info("Author matched, sending notification task to cker.ch.")
		taskCker.ch <- taskCker
	}
}
