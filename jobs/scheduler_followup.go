package jobs

import (
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
	"github.com/Ptt-Alertor/ptt-alertor/models/pushsum"
)

// Hooks for follow-up gating / execution (swappable in tests).
var boardHasPushSumFn = defaultBoardHasPushSum
var boardHasCommentTargetsFn = defaultBoardHasCommentTargets
var runPushSumBoardFn = checkPushSumBoard
var runCommentBoardFn = checkCommentBoard

func defaultBoardHasPushSum(board string) bool {
	name := strings.ToLower(strings.TrimSpace(board))
	if name == "" {
		return false
	}
	if pushsum.Exist(name) {
		return true
	}
	return len(pushsum.ListSubscribers(name)) > 0
}

func defaultBoardHasCommentTargets(board string) bool {
	name := strings.ToLower(strings.TrimSpace(board))
	if name == "" {
		return false
	}
	for _, code := range new(article.Articles).List() {
		a := models.Article().Find(code)
		if a.Code == "" || a.Board == "" {
			continue
		}
		if strings.EqualFold(a.Board, name) {
			return true
		}
	}
	return false
}

// checkPushSumBoard is a one-shot crawl+match for scheduler follow-ups (no Run loop).
func checkPushSumBoard(board string) {
	name := strings.ToLower(strings.TrimSpace(board))
	if name == "" {
		return
	}
	psc := *NewPushSumChecker()
	ba := BoardArticles{board: name}
	baCh := make(chan BoardArticles, 1)
	psc.crawlArticles(ba, baCh)
	result := <-baCh
	if len(result.articles) == 0 {
		return
	}
	psc.checkSubscribers(result)
}

// checkCommentBoard is a one-shot comment check for tracked articles on a board.
func checkCommentBoard(board string) {
	name := strings.ToLower(strings.TrimSpace(board))
	if name == "" {
		return
	}
	cc := *NewCommentChecker()
	for _, code := range new(article.Articles).List() {
		a := models.Article().Find(code)
		if a.Code == "" || a.Board == "" || !strings.EqualFold(a.Board, name) {
			continue
		}
		ach := make(chan article.Article, 1)
		cc.checkComments(code, ach)
		select {
		case art := <-ach:
			cc.Article = art
			cc.checkSubscribers()
		default:
		}
	}
}

func (s *Scheduler) maybeEnqueueFollowUps(board string) {
	if boardHasPushSumFn(board) {
		s.enqueueWork(WorkItem{Kind: WorkCheckPushSum, Board: board})
	}
	if boardHasCommentTargetsFn(board) {
		s.enqueueWork(WorkItem{Kind: WorkCheckComment, Board: board})
	}
}

// enqueueWork wraps TryEnqueue; I6 adds drop logging in a later pass via this helper.
func (s *Scheduler) enqueueWork(item WorkItem) EnqueueStatus {
	return s.queue.TryEnqueue(item)
}
