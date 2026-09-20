package jobs

import (
	"context"
	"sync"
)

// WorkKind identifies scheduler work item types.
type WorkKind int

const (
	WorkRefreshBoard WorkKind = iota
	WorkCheckPushSum
	WorkCheckComment
)

// WorkItem is one unit of scheduler work.
type WorkItem struct {
	Kind  WorkKind
	Board string
}

// WorkQueue is a bounded queue with at most one queued/in-flight item per board.
type WorkQueue struct {
	mu       sync.Mutex
	ch       chan WorkItem
	pending  map[string]struct{} // queued or in-flight
	inflight map[string]struct{}
}

// NewWorkQueue creates a queue with the given capacity.
func NewWorkQueue(size int) *WorkQueue {
	if size < 1 {
		size = 1
	}
	return &WorkQueue{
		ch:       make(chan WorkItem, size),
		pending:  make(map[string]struct{}),
		inflight: make(map[string]struct{}),
	}
}

// TryEnqueue adds an item if capacity and per-board rules allow.
func (q *WorkQueue) TryEnqueue(item WorkItem) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if item.Board == "" {
		return false
	}
	if _, ok := q.pending[item.Board]; ok {
		return false
	}
	select {
	case q.ch <- item:
		q.pending[item.Board] = struct{}{}
		return true
	default:
		return false
	}
}

// Pop waits for the next item or context cancel.
func (q *WorkQueue) Pop(ctx context.Context) (WorkItem, bool) {
	select {
	case <-ctx.Done():
		return WorkItem{}, false
	case item := <-q.ch:
		q.mu.Lock()
		q.inflight[item.Board] = struct{}{}
		// keep pending until MarkDone so duplicates stay rejected while in-flight
		q.mu.Unlock()
		return item, true
	}
}

// MarkDone clears board reservation after work finishes.
func (q *WorkQueue) MarkDone(board string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	delete(q.inflight, board)
	delete(q.pending, board)
}

// Len returns approximate queued items (not including in-flight already popped).
func (q *WorkQueue) Len() int {
	return len(q.ch)
}
