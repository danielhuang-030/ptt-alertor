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
				log.WithField("high_boards_count", len(highBoards)).Debug("Processing high priority boards")
				log.Debug("--- High Priority Boards Details ---")
				for i, bd := range highBoards {
					originalName := bd.Name // Assuming bd.Name is already standardized as per previous attempt
					standardizedName := strings.ToLower(strings.TrimSpace(originalName)) // Re-standardize for absolute certainty in log
					log.WithFields(log.Fields{
						"index":             i,
						"board_name_as_is":  originalName,
						"board_name_q":      fmt.Sprintf("%q", originalName),
						"standardized_name": standardizedName,
						"standardized_name_q": fmt.Sprintf("%q", standardizedName),
						"board_ptr":         fmt.Sprintf("%p", bd),
					}).Debug("High priority board detail")
				}
				log.Debug("--- End of High Priority Boards Details ---")
				checkBoards(highBoards, checkHighBoardDuration, nil) // No skipNames for high priority
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
		// Create set of high priority board names to skip them in normal processing
		highBoardNamesSet := make(map[string]struct{})
		for _, bd := range highBoards { // highBoards[*].Name is already standardized
			highBoardNamesSet[bd.Name] = struct{}{}
		}
		log.Debug("--- High Priority Board Names Set (for skipping) ---")
		i := 0
		for name := range highBoardNamesSet {
			log.WithFields(log.Fields{
				"index": i,
				"name_in_set_as_is": name,
				"name_in_set_q":     fmt.Sprintf("%q", name),
			}).Debug("Name in highBoardNamesSet")
			i++
		}
		if i == 0 {
			log.Debug("highBoardNamesSet is EMPTY")
		}
		log.Debug("--- End of High Priority Board Names Set ---")

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
				log.WithField("normal_boards_count", len(allNormalBoards)).Debug("Processing normal boards")
				log.Debug("--- Normal Boards List Details (before passing to checkBoards) ---")
				for i, bd := range allNormalBoards {
					originalName := bd.Name // Assuming bd.Name is already standardized by Board.All()
					standardizedName := strings.ToLower(strings.TrimSpace(originalName)) // Re-standardize for log
					log.WithFields(log.Fields{
						"index":             i,
						"board_name_as_is":  originalName,
						"board_name_q":      fmt.Sprintf("%q", originalName),
						"standardized_name": standardizedName,
						"standardized_name_q": fmt.Sprintf("%q", standardizedName),
						"board_ptr":         fmt.Sprintf("%p", bd),
					}).Debug("Normal board detail (from models.Board().All())")
				}
				log.Debug("--- End of Normal Boards List Details ---")
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
	log.WithField("num_boards_received", len(bds)).Debug("checkBoards BEGIN")
	if skipNames != nil {
		log.Debug("--- skipNames Map Content (received by checkBoards) ---")
		i := 0
		for name := range skipNames {
			log.WithFields(log.Fields{
				"index": i,
				"name_in_skipmap_as_is": name,
				"name_in_skipmap_q":     fmt.Sprintf("%q", name),
			}).Debug("Name in skipNames map")
			i++
		}
		if i == 0 {
			log.Debug("skipNames map is EMPTY")
		}
		log.Debug("--- End of skipNames Map Content ---")
	} else {
		log.Debug("skipNames map is NIL")
	}

	for _, bd := range bds {
		originalNameLoop := bd.Name // Assuming bd.Name is already standardized
		standardizedNameLoop := strings.ToLower(strings.TrimSpace(originalNameLoop)) // Re-standardize for log
		log.WithFields(log.Fields{
			"board_name_as_is":  originalNameLoop,
			"board_name_q":      fmt.Sprintf("%q", originalNameLoop),
			"standardized_name": standardizedNameLoop, // This is what should be used for lookup
			"standardized_name_q": fmt.Sprintf("%q", standardizedNameLoop),
			"board_ptr":         fmt.Sprintf("%p", bd),
		}).Debug("Board in checkBoards loop (before skip check)")

		// bd.Name is assumed to be standardized (lowercase, trimmed) by its creator (Board.All() or init())
		if skipNames != nil {
			// standardizedNameLoop is the name we should check in skipNames
			_, shouldSkip := skipNames[standardizedNameLoop] // Use the definitely standardized name for lookup
			log.WithFields(log.Fields{
				"board_name_checked": standardizedNameLoop,
				"board_name_checked_q": fmt.Sprintf("%q", standardizedNameLoop),
				"found_in_skipNames": shouldSkip,
			}).Debug("skipNames check result")
			if shouldSkip {
				log.WithField("board", standardizedNameLoop).Debug("Skipping board as it was found in skipNames.") // Original log
				continue
			}
		}
		log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd)}).Debug("Starting checkNewArticle goroutine")
		time.Sleep(duration)
		go checkNewArticle(bd, boardCh)
	}
}

func checkNewArticle(bd *board.Board, boardCh chan *board.Board) {
	log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd)}).Info("checkNewArticle BEGIN")
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
			log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd)}).Debug("Sending board to boardCh")
			boardCh <- bd
		}
	}
}

func checkKeywordSubscriber(bd *board.Board, cker Checker) {
	// Note: cker.Profile.Account might be empty here if this is the first time for this cker instance
	log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd), "account_to_check_keywords": cker.Profile.Account}).Debug("checkKeywordSubscriber BEGIN")
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
	log.WithFields(log.Fields{"account": user.Profile.Account, "board_name": bd.Name}).Debug("checkKeywordSubscription BEGIN")
	for _, sub := range user.Subscribes {
		standardizedSubBoard := strings.ToLower(strings.TrimSpace(sub.Board))
		log.WithFields(log.Fields{"account": user.Profile.Account, "board_name": bd.Name, "original_sub_board": sub.Board, "standardized_sub_board": standardizedSubBoard, "sub_keywords": strings.Join(sub.Keywords, ",")}).Debug("Checking subscription rule")
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
		log.WithFields(log.Fields{"board": cker.board, "keyword": cker.keyword, "sub_type": cker.subType, "word": cker.word, "articles_count": len(cker.articles), "profile_account": cker.Profile.Account, "discord_ch_id": cker.Profile.DiscordChannelID}).Debug("Preparing to send Checker via c.ch from checkKeyword")
		cker.ch <- cker
	}
}

func checkAuthorSubscriber(bd *board.Board, cker Checker) {
	// Note: cker.Profile.Account might be empty here
	log.WithFields(log.Fields{"board_name": bd.Name, "board_ptr": fmt.Sprintf("%p", bd), "account_to_check_authors": cker.Profile.Account}).Debug("checkAuthorSubscriber BEGIN")
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
	log.WithFields(log.Fields{"account": user.Profile.Account, "board_name": bd.Name}).Debug("checkAuthorSubscription BEGIN")
	for _, sub := range user.Subscribes {
		standardizedSubBoard := strings.ToLower(strings.TrimSpace(sub.Board))
		log.WithFields(log.Fields{"account": user.Profile.Account, "board_name": bd.Name, "original_sub_board": sub.Board, "standardized_sub_board": standardizedSubBoard, "sub_authors": strings.Join(sub.Authors, ",")}).Debug("Checking subscription rule")
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
		log.WithFields(log.Fields{"board": cker.board, "author": cker.author, "sub_type": cker.subType, "word": cker.word, "articles_count": len(cker.articles), "profile_account": cker.Profile.Account, "discord_ch_id": cker.Profile.DiscordChannelID}).Debug("Preparing to send Checker via c.ch from checkAuthor")
		cker.ch <- cker
	}
}