package jobs

import (
	"context"
	"testing"
	"time"
)

func TestWorkQueueRejectsDuplicateBoard(t *testing.T) {
	q := NewWorkQueue(8)
	ok1 := q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "gossiping"})
	ok2 := q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "gossiping"})
	if !ok1 || ok2 {
		t.Fatalf("first=%v second=%v want true,false", ok1, ok2)
	}
}

func TestWorkQueueBounded(t *testing.T) {
	q := NewWorkQueue(1)
	if !q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "a"}) {
		t.Fatal("first should enqueue")
	}
	if q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "b"}) {
		t.Fatal("second should fail when full")
	}
}

func TestWorkQueueMarkDoneAllowsRequeue(t *testing.T) {
	q := NewWorkQueue(4)
	if !q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "a"}) {
		t.Fatal("enqueue")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	item, ok := q.Pop(ctx)
	if !ok || item.Board != "a" {
		t.Fatalf("pop ok=%v item=%+v", ok, item)
	}
	q.MarkDone("a")
	if !q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "a"}) {
		t.Fatal("should enqueue after MarkDone")
	}
}
