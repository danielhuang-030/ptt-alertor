package jobs

import (
	"strings"
	"sync"

	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/models/article"
)

// CommentTargetIndex caches article codes with comment subscriptions, keyed by board.
// Refresh may KEYS/SCAN once per IdlePoll; HasBoard / CodesForBoard are memory-only
// and must be used on the Active follow-up hot path (never Articles.List there).
type CommentTargetIndex struct {
	mu      sync.Mutex
	byBoard map[string][]string
	loader  func() map[string][]string
}

// NewCommentTargetIndex builds an index. Nil loader uses Redis Articles.List + Find.
func NewCommentTargetIndex(loader func() map[string][]string) *CommentTargetIndex {
	if loader == nil {
		loader = loadCommentTargetsFromArticles
	}
	return &CommentTargetIndex{
		byBoard: make(map[string][]string),
		loader:  loader,
	}
}

func loadCommentTargetsFromArticles() map[string][]string {
	byBoard := make(map[string][]string)
	for _, code := range new(article.Articles).List() {
		a := models.Article().Find(code)
		if a.Code == "" || a.Board == "" {
			continue
		}
		board := strings.ToLower(strings.TrimSpace(a.Board))
		byBoard[board] = append(byBoard[board], code)
	}
	return byBoard
}

// Refresh reloads the in-memory board→codes map (IdlePoll cadence only).
func (c *CommentTargetIndex) Refresh() {
	next := c.loader()
	if next == nil {
		next = make(map[string][]string)
	}
	c.mu.Lock()
	c.byBoard = next
	c.mu.Unlock()
}

// HasBoard reports whether any tracked comment article belongs to board.
func (c *CommentTargetIndex) HasBoard(board string) bool {
	name := strings.ToLower(strings.TrimSpace(board))
	if name == "" {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.byBoard[name]) > 0
}

// CodesForBoard returns a copy of tracked article codes for board.
func (c *CommentTargetIndex) CodesForBoard(board string) []string {
	name := strings.ToLower(strings.TrimSpace(board))
	if name == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	src := c.byBoard[name]
	out := make([]string, len(src))
	copy(out, src)
	return out
}

// Package-level index used by the scheduler comment follow-up path.
var commentTargetIndex = NewCommentTargetIndex(nil)

// CommentTargets returns the shared comment-target index (for IdlePoll Refresh in main).
func CommentTargets() *CommentTargetIndex {
	return commentTargetIndex
}

// SetCommentTargetIndex replaces the shared index (tests).
func SetCommentTargetIndex(idx *CommentTargetIndex) {
	if idx == nil {
		idx = NewCommentTargetIndex(nil)
	}
	commentTargetIndex = idx
}
