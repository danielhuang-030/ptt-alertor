package jobs

import (
	"strings"

	log "github.com/Ptt-Alertor/logrus"

	"github.com/Ptt-Alertor/ptt-alertor/models"
)

// refreshBoardAndNotify fetches a board and runs keyword/author matching into enqueueCheck.
// Fetch failures (rate-limit or RSS+HTML both down) return error so the scheduler can backoff.
func refreshBoardAndNotify(boardName string) error {
	name := strings.ToLower(strings.TrimSpace(boardName))
	if name == "" {
		return nil
	}
	bd := models.Board()
	bd.Name = name
	if err := bd.WithNewArticlesErr(); err != nil {
		return err
	}
	if bd.NewArticles == nil && len(bd.OnlineArticles) > 0 {
		bd.Articles = bd.OnlineArticles
		log.WithField("board", bd.Name).Info("Created Articles")
		if err := bd.Save(); err != nil {
			return err
		}
	}
	if len(bd.NewArticles) != 0 {
		bd.Articles = bd.OnlineArticles
		log.WithField("board", bd.Name).Info("Updated Articles")
		if err := bd.Save(); err != nil {
			return err
		}
		cker := NewChecker().Self()
		checkKeywordSubscriber(bd, cker)
		checkAuthorSubscriber(bd, cker)
	}
	return nil
}
