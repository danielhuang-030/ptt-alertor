package jobs

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
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
var highBoards []*board.Board
var highBoardNames = strings.Split(os.Getenv("BOARD_HIGH"), ",")

var (
	boardProcessingMutex      = &sync.Mutex{}
	boardsCurrentlyProcessing = make(map[string]bool)
)

func init() {
	for _, name := range highBoardNames {
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
	ch       chan Checker
	duration time.Duration
}

// NewChecker gets a Checker instance
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

// Run is main in Job
func (c Checker) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// step 1: check boards which one has new articles
	// check high boards
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

	// check off peak
	offPeakCh := make(chan bool)
	go c.checkOffPeak(ctx, offPeakCh)

	// check normal boards, slow when off peak
	go func() {
		var offPeak bool
		duration := c.duration
		
		// Prepare the list of normal boards by excluding highBoards
		allBoards := models.Board().All()
		highBoardNameSet := make(map[string]bool)
		for _, hb := range highBoards {
			if hb != nil { // Add nil check for safety
				highBoardNameSet[hb.Name] = true
			}
		}

		// Initialize normalBoards as an empty slice of *board.Board
		normalBoards := make([]*board.Board, 0) 
		for _, b := range allBoards {
			if b != nil { // Add nil check for safety
				if _, isHigh := highBoardNameSet[b.Name]; !isHigh {
					normalBoards = append(normalBoards, b)
				}
			}
		}
		// Log the counts after filtering
		log.Infof("Initialized board checks: %d high-frequency boards, %d normal-frequency boards to be checked by this goroutine.", len(highBoards), len(normalBoards))

		for { // Inner loop starts here
			select {
			case <-ctx.Done():
				for len(offPeakCh) > 0 {
					<-offPeakCh
				}
				return
			case op := <-offPeakCh:
				if offPeak != op {
					if op {
						// Corrected log message for clarity
						log.Info("Switch to Slow Mode for normal boards")
						duration = c.duration * 2
					} else {
						log.Info("Switch to Normal Mode for normal boards")
						duration = c.duration
					}
					offPeak = op
				}
			default:
				// Now call checkBoards with the filtered normalBoards list
				checkBoards(normalBoards, duration) 
			}
		}
	}()

	// main
	for {
		select {
		//step 2: check user who subscribes board
		case bd := <-boardCh:
			go checkKeywordSubscriber(bd, c)
			go checkAuthorSubscriber(bd, c)
		//step 3: send notification
		case receivedCker := <-c.ch:
			log.WithFields(log.Fields{
				"targetUserAccount": receivedCker.Profile.Account,
				"board": receivedCker.board,
				"type": receivedCker.subType,
				"word": receivedCker.word,
				"articleCount": len(receivedCker.articles),
				"action": "relaying_task_to_ckCh",
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

	// Defer unlocking in all return paths
	defer func() {
		boardProcessingMutex.Lock()
		delete(boardsCurrentlyProcessing, bd.Name)
		boardProcessingMutex.Unlock()
		log.WithField("board", bd.Name).Debug("Finished processing board, lock released.")
	}()

	log.WithFields(log.Fields{"board": bd.Name, "source": "checkNewArticle"}).Debug("Preparing to call bd.WithNewArticles()")
	bd.WithNewArticles() // This populates bd.NewArticles and bd.OnlineArticles
	log.WithFields(log.Fields{"board": bd.Name, "newArticlesCount": len(bd.NewArticles), "source": "checkNewArticle"}).Debug("Returned from bd.WithNewArticles()")

	// This logic seems to handle two cases:
	// 1. Board just added, no saved articles, so OnlineArticles are "created" (but not "new" by comparison)
	// 2. Board has updates, NewArticles has content.
	// The original log "Created Articles" might be confusing if it means "newly fetched and saved online articles".
	// The log "Updated Articles" is for when bd.NewArticles is non-empty.

	// Case 1: No previously saved articles for this board, and online articles were fetched.
	// These are not "new" by comparison to a previous state, but they are the first fetch.
	// The original code had `if bd.NewArticles == nil && len(bd.OnlineArticles) > 0`.
	// `bd.NewArticles` would be nil if `savedArticles` was empty in `newArticles` func.
	if bd.NewArticles == nil && len(bd.OnlineArticles) > 0 {
		bd.Articles = bd.OnlineArticles // Set all fetched online articles as the current articles for the board
		log.WithField("board", bd.Name).Info("Board has no prior saved articles, saving current online articles.")
		// We should save here, so the next check considers these "saved".
		if err := bd.Save(); err != nil {
			log.WithError(err).WithField("board", bd.Name).Error("Failed to save initial articles for board.")
			// If save fails, we might not want to send to boardCh, or handle differently.
                // For now, let's keep original logic of not sending to boardCh in this specific sub-case.
		}
            // Original code did NOT send to boardCh here.
            // This implies that first fetchPopulating OnlineArticles but not NewArticles (by comparison)
            // should not trigger immediate keyword/author checks. This seems reasonable.
	}

	// Case 2: New articles found by comparison.
	if len(bd.NewArticles) > 0 {
		// The board's main article list (for saving) should reflect the latest online state
		bd.Articles = bd.OnlineArticles 
		log.WithField("board", bd.Name).Info("Updated Articles (new articles found by comparison).") // Clarified log
		if err := bd.Save(); err == nil { // Save the new state (all online articles)
			log.WithFields(log.Fields{"board": bd.Name, "newArticlesCount": len(bd.NewArticles), "action": "sending_to_boardCh"}).Info("Board has new articles, sending to boardCh for processing.")
			boardCh <- bd // Send the board (which includes NewArticles) for further checks
		} else {
                log.WithError(err).WithField("board", bd.Name).Error("Failed to save board after finding new articles.")
            }
	}
}

func checkKeywordSubscriber(bd *board.Board, cker Checker) {
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
	log.WithFields(log.Fields{
		"board": cker.board, // cker.board should be set before calling checkKeyword
		"keyword": keyword,
		"articlesToCheckCount": len(bd.NewArticles), 
		"targetUserAccount": cker.Profile.Account, // Log the specific user account being checked for
	}).Debug("Checking keyword match for user.")
	keywordArticles := make(article.Articles, 0)
	for _, newAtcl := range bd.NewArticles {
		if newAtcl.MatchKeyword(keyword) {
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
			"board": cker.board,
			"keyword": keyword,
			"matchedArticlesCount": len(keywordArticles),
			"targetUserAccount": cker.Profile.Account,
			"action": "sending_to_cker_ch_for_keyword",
		}).Info("Keyword matched, sending notification task to cker.ch.")
		cker.ch <- cker
	}
}

func checkAuthorSubscriber(bd *board.Board, cker Checker) {
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
	log.WithFields(log.Fields{
		"board": cker.board, // cker.board should be set before calling checkAuthor
		"author": author,
		"articlesToCheckCount": len(bd.NewArticles),
		"targetUserAccount": cker.Profile.Account,
	}).Debug("Checking author match for user.")
	authorArticles := make(article.Articles, 0)
	for _, newAtcl := range bd.NewArticles {
		if strings.EqualFold(newAtcl.Author, author) {
			authorArticles = append(authorArticles, newAtcl)
		}
	}
	if len(authorArticles) != 0 {
		cker.author = author
		cker.articles = authorArticles
		cker.subType = "author"
		cker.word = author
		log.WithFields(log.Fields{
			"board": cker.board,
			"author": author,
			"matchedArticlesCount": len(authorArticles),
			"targetUserAccount": cker.Profile.Account,
			"action": "sending_to_cker_ch_for_author",
		}).Info("Author matched, sending notification task to cker.ch.")
		cker.ch <- cker
	}
}
