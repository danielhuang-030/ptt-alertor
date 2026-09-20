package jobs

import (
	"strings"

	log "github.com/Ptt-Alertor/logrus"

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
	return commentTargetIndex.HasBoard(board)
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
// Uses CommentTargetIndex (memory) — must not KEYS on every follow-up.
func checkCommentBoard(board string) {
	name := strings.ToLower(strings.TrimSpace(board))
	if name == "" {
		return
	}
	cc := *NewCommentChecker()
	for _, code := range commentTargetIndex.CodesForBoard(name) {
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

// enqueueWork wraps TryEnqueue and logs drops (Debug for duplicate, Warn for full).
func (s *Scheduler) enqueueWork(item WorkItem) EnqueueStatus {
	st := s.queue.TryEnqueue(item)
	switch st {
	case EnqueueDuplicate:
		log.WithFields(log.Fields{"board": item.Board, "kind": item.Kind}).Debug("work queue drop: duplicate")
	case EnqueueFull:
		log.WithFields(log.Fields{"board": item.Board, "kind": item.Kind}).Warn("work queue drop: full")
	case EnqueueInvalid:
		log.WithFields(log.Fields{"board": item.Board, "kind": item.Kind}).Debug("work queue drop: invalid")
	}
	return st
}
