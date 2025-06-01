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

// Old cool-down globals removed

func init() {
	for _, name := range highBoardNames {
		standardizedName := strings.ToLower(strings.TrimSpace(name))
		// Ensure highBoardNames itself stores standardized names if used elsewhere,
		// though typically direct use of highBoards (which will have standardized names) is more common.
		// For this change, we focus on standardizing names in highBoards[*].Name
		bd := models.Board()
		bd.Name = standardizedName
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
				log.WithField("high_boards_count", len(highBoards)).Debug("Processing high priority boards.") // Simplified further
				checkBoards(highBoards, checkHighBoardDuration, nil)
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
		// Dynamically create set of high priority board names to skip them in normal processing
		highBoardNamesSet := make(map[string]struct{})
		envBoardHigh := os.Getenv("BOARD_HIGH")
		// log.WithField("BOARD_HIGH_env_val", envBoardHigh).Debug("Read BOARD_HIGH for dynamic skip set creation.") // Removed
		if envBoardHigh != "" {
			names := strings.Split(envBoardHigh, ",")
			for _, name := range names {
				standardizedName := strings.ToLower(strings.TrimSpace(name))
				if standardizedName != "" {
					highBoardNamesSet[standardizedName] = struct{}{}
					// log.WithField("board_added_to_skip_set", standardizedName).Debug("Dynamically added to highBoardNamesSet for normal board skipping") // Removed
				}
			}
		}
		if len(highBoardNamesSet) == 0 {
			log.Debug("Skip set (from BOARD_HIGH) for normal boards is EMPTY.") // Simplified
		} else {
			log.WithField("skip_set_count", len(highBoardNamesSet)).Debug("Skip set (from BOARD_HIGH) for normal boards populated.") // Simplified
		}

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
						log.Info("Switch to Slow Mode")
						duration = c.duration * 2
					} else {
						log.Info("Switch to Normal Mode")
						duration = c.duration
					}
					offPeak = op
				}
			default:
				allNormalBoards := models.Board().All() // Board.All() now returns standardized names
				// log.Debug("--- Normal Boards List Details (before passing to checkBoards) ---")
				log.WithField("normal_boards_count", len(allNormalBoards)).Debug("Processing normal boards.") // Simplified further
				// log.Debug("--- End of Normal Boards List Details ---")
				checkBoards(allNormalBoards, duration, highBoardNamesSet)
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
		case cker := <-c.ch:
			ckCh <- cker
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

func checkBoards(bds []*board.Board, duration time.Duration, skipNames map[string]struct{}) {
	// log.WithField("num_boards_received", len(bds)).Debug("checkBoards BEGIN") // Removed
	// if skipNames != nil { // Removed skipNames map content logging
	// 	log.Debug("--- skipNames Map Content (received by checkBoards) ---")
	// 	...
	// 	log.Debug("--- End of skipNames Map Content ---")
	// } else {
	// 	log.Debug("skipNames map is NIL")
	// }

	for _, bd := range bds {
		// SkipNames check
		if skipNames != nil {
			if _, shouldSkip := skipNames[bd.Name]; shouldSkip {
				// log.WithField("board", bd.Name).Debug("Skipping board as it was found in skipNames.") // This can be noisy if many are skipped. Retained for now.
				continue
			}
		}

		// log.WithFields(log.Fields{ // Commented out: "Board passed skipName check..."
		// 	"board_name": bd.Name,
		// 	"board_ptr": fmt.Sprintf("%p", bd),
		// }).Debug("Board passed skipName check, proceeding to start checkNewArticle goroutine.")

		time.Sleep(duration)
		go checkNewArticle(bd, boardCh)
	}
}

func checkNewArticle(bd *board.Board, boardCh chan *board.Board) {
	log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd)}).Debug("checkNewArticle BEGIN")
	bd.WithNewArticles()
	log.WithFields(log.Fields{"board_name": bd.Name, "new_articles_count": len(bd.NewArticles), "online_articles_count": len(bd.OnlineArticles)}).Debug("Articles status after WithNewArticles")
	if bd.NewArticles == nil && len(bd.OnlineArticles) > 0 {
		bd.Articles = bd.OnlineArticles
		log.WithField("board", bd.Name).Info("Created Articles")
		bd.Save()
	}
	if len(bd.NewArticles) != 0 {
		bd.Articles = bd.OnlineArticles
		log.WithField("board", bd.Name).Info("Updated Articles")
		if err := bd.Save(); err == nil {
			// log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd)}).Debug("Sending board to boardCh") // Commented out
			boardCh <- bd
		}
	}
}

func checkKeywordSubscriber(bd *board.Board, cker Checker) {
	// log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd), "account_to_check_keywords": cker.Profile.Account}).Debug("checkKeywordSubscriber BEGIN") // Commented out
	u := models.User()
	accounts := keyword.Subscribers(bd.Name)
	for _, account := range accounts {
		user := u.Find(account)
		if user.Enable {
			// Profile is now specific to this user for the subsequent subscription check
			localCker := cker
			localCker.Profile = user.Profile
			go checkKeywordSubscription(user, bd, localCker)
		}
	}
}

func checkKeywordSubscription(user user.User, bd *board.Board, cker Checker) {
	// log.WithFields(log.Fields{"account": user.Profile.Account, "board_name": bd.Name}).Debug("checkKeywordSubscription BEGIN") // Commented out
	for _, sub := range user.Subscribes {
		standardizedSubBoard := strings.ToLower(strings.TrimSpace(sub.Board))
		// log.WithFields(log.Fields{"account": user.Profile.Account, "board_name": bd.Name, "original_sub_board": sub.Board, "standardized_sub_board": standardizedSubBoard, "sub_keywords": strings.Join(sub.Keywords, ",")}).Debug("Checking subscription rule") // Commented out
		if bd.Name == standardizedSubBoard { // bd.Name is already standardized
			cker.board = bd.Name // Use standardized board name
			for _, keyword := range sub.Keywords {
				standardizedKeyword := strings.ToLower(strings.TrimSpace(keyword))
				go checkKeyword(standardizedKeyword, bd, cker)
			}
		}
	}
}

func checkKeyword(standardizedKeyword string, bd *board.Board, cker Checker) {
	keywordArticles := make(article.Articles, 0)
	for _, newAtcl := range bd.NewArticles {
		// Assuming newAtcl.MatchKeyword can handle or expects a standardized (e.g., lowercase) keyword
		if newAtcl.MatchKeyword(standardizedKeyword) {
			newAtcl.Author = ""
			keywordArticles = append(keywordArticles, newAtcl)
		}
	}
	if len(keywordArticles) != 0 {
		cker.keyword = standardizedKeyword // Store standardized keyword
		cker.articles = keywordArticles
		cker.subType = "keyword"
		cker.word = standardizedKeyword // Store standardized word
		// log.WithFields(log.Fields{"board": cker.board, "keyword": cker.keyword, "sub_type": cker.subType, "word": cker.word, "articles_count": len(cker.articles), "profile_account": cker.Profile.Account, "discord_ch_id": cker.Profile.DiscordChannelID}).Debug("Preparing to send Checker via c.ch from checkKeyword") // Commented out
		cker.ch <- cker
	}
}

func checkAuthorSubscriber(bd *board.Board, cker Checker) {
	// log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd), "account_to_check_authors": cker.Profile.Account}).Debug("checkAuthorSubscriber BEGIN") // Commented out
	u := models.User()
	accounts := author.Subscribers(bd.Name)
	for _, account := range accounts {
		user := u.Find(account)
		if user.Enable {
			// Profile is now specific to this user for the subsequent subscription check
			localCker := cker
			localCker.Profile = user.Profile
			go checkAuthorSubscription(user, bd, localCker)
		}
	}
}

func checkAuthorSubscription(user user.User, bd *board.Board, cker Checker) {
	// log.WithFields(log.Fields{"account": user.Profile.Account, "board_name": bd.Name}).Debug("checkAuthorSubscription BEGIN") // Commented out
	for _, sub := range user.Subscribes {
		standardizedSubBoard := strings.ToLower(strings.TrimSpace(sub.Board))
		// log.WithFields(log.Fields{"account": user.Profile.Account, "board_name": bd.Name, "original_sub_board": sub.Board, "standardized_sub_board": standardizedSubBoard, "sub_authors": strings.Join(sub.Authors, ",")}).Debug("Checking subscription rule") // Commented out
		if bd.Name == standardizedSubBoard { // bd.Name is already standardized
			cker.board = bd.Name // Use standardized board name
			for _, author := range sub.Authors {
				standardizedAuthor := strings.ToLower(strings.TrimSpace(author))
				go checkAuthor(standardizedAuthor, bd, cker)
			}
		}
	}
}

func checkAuthor(standardizedAuthor string, bd *board.Board, cker Checker) {
	authorArticles := make(article.Articles, 0)
	for _, newAtcl := range bd.NewArticles {
		// strings.EqualFold is already case-insensitive. Passing standardizedAuthor maintains consistency.
		if strings.EqualFold(newAtcl.Author, standardizedAuthor) {
			authorArticles = append(authorArticles, newAtcl)
		}
	}
	if len(authorArticles) != 0 {
		cker.author = standardizedAuthor // Store standardized author
		cker.articles = authorArticles
		cker.subType = "author"
		cker.word = standardizedAuthor // Store standardized word
		// log.WithFields(log.Fields{"board": cker.board, "author": cker.author, "sub_type": cker.subType, "word": cker.word, "articles_count": len(cker.articles), "profile_account": cker.Profile.Account, "discord_ch_id": cker.Profile.DiscordChannelID}).Debug("Preparing to send Checker via c.ch from checkAuthor") // Commented out
		cker.ch <- cker
	}
}