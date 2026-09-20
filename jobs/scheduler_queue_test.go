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
	if ok1 != EnqueueOK || ok2 != EnqueueDuplicate {
		t.Fatalf("first=%v second=%v want OK,Duplicate", ok1, ok2)
	}
}

func TestWorkQueueAllowsSameBoardDifferentKinds(t *testing.T) {
	q := NewWorkQueue(8)
	if q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "gossiping"}) != EnqueueOK {
		t.Fatal("refresh")
	}
	if q.TryEnqueue(WorkItem{Kind: WorkCheckPushSum, Board: "gossiping"}) != EnqueueOK {
		t.Fatal("pushsum should not collide with refresh on same board")
	}
	if q.TryEnqueue(WorkItem{Kind: WorkCheckComment, Board: "gossiping"}) != EnqueueOK {
		t.Fatal("comment should not collide with refresh/pushsum on same board")
	}
}

func TestWorkQueueBounded(t *testing.T) {
	q := NewWorkQueue(1)
	if q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "a"}) != EnqueueOK {
		t.Fatal("first should enqueue")
	}
	if q.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "b"}) != EnqueueFull {
		t.Fatal("second should be full")
	}
}

func TestWorkQueueMarkDoneAllowsRequeue(t *testing.T) {
	q := NewWorkQueue(4)
	item := WorkItem{Kind: WorkRefreshBoard, Board: "a"}
	if q.TryEnqueue(item) != EnqueueOK {
		t.Fatal("enqueue")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	got, ok := q.Pop(ctx)
	if !ok || got.Board != "a" {
		t.Fatalf("pop ok=%v item=%+v", ok, got)
	}
	q.MarkDone(got)
	if q.TryEnqueue(item) != EnqueueOK {
		t.Fatal("should enqueue after MarkDone")
	}
}
