package jobs

import (
	"context"
	"fmt"
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

func pendingKey(item WorkItem) string {
	return fmt.Sprintf("%d:%s", item.Kind, item.Board)
}

// WorkQueue is a bounded queue with at most one queued/in-flight item per (Kind, Board).
type WorkQueue struct {
	mu       sync.Mutex
	ch       chan WorkItem
	pending  map[string]struct{} // queued or in-flight; key = kind:board
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

// EnqueueStatus distinguishes why TryEnqueue failed (for logging).
type EnqueueStatus int

const (
	EnqueueOK EnqueueStatus = iota
	EnqueueDuplicate
	EnqueueFull
	EnqueueInvalid
)

// TryEnqueue adds an item if capacity and per-(kind,board) rules allow.
func (q *WorkQueue) TryEnqueue(item WorkItem) EnqueueStatus {
	q.mu.Lock()
	defer q.mu.Unlock()
	if item.Board == "" {
		return EnqueueInvalid
	}
	key := pendingKey(item)
	if _, ok := q.pending[key]; ok {
		return EnqueueDuplicate
	}
	select {
	case q.ch <- item:
		q.pending[key] = struct{}{}
		return EnqueueOK
	default:
		return EnqueueFull
	}
}

// Pop waits for the next item or context cancel.
func (q *WorkQueue) Pop(ctx context.Context) (WorkItem, bool) {
	select {
	case <-ctx.Done():
		return WorkItem{}, false
	case item := <-q.ch:
		q.mu.Lock()
		q.inflight[pendingKey(item)] = struct{}{}
		// keep pending until MarkDone so duplicates stay rejected while in-flight
		q.mu.Unlock()
		return item, true
	}
}

// MarkDone clears (kind,board) reservation after work finishes.
func (q *WorkQueue) MarkDone(item WorkItem) {
	q.mu.Lock()
	defer q.mu.Unlock()
	key := pendingKey(item)
	delete(q.inflight, key)
	delete(q.pending, key)
}

// Len returns approximate queued items (not including in-flight already popped).
func (q *WorkQueue) Len() int {
	return len(q.ch)
}
