package jobs

import (
	"context"
	"testing"
	"time"
)

func TestFollowUpWorkDoesNotPanic(t *testing.T) {
	s := NewScheduler(SchedulerConfig{Workers: 1, QueueSize: 8, ActiveTick: time.Second, IdlePoll: time.Second},
		func() bool { return true },
		func() []string { return nil },
	)
	s.SetRefresher(&countingRefresher{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	if !s.queue.TryEnqueue(WorkItem{Kind: WorkCheckPushSum, Board: "gossiping"}) {
		t.Fatal("enqueue pushsum")
	}
	if !s.queue.TryEnqueue(WorkItem{Kind: WorkCheckComment, Board: "gossiping"}) {
		// may reject duplicate board — use different boards
	}
	_ = s.queue.TryEnqueue(WorkItem{Kind: WorkCheckComment, Board: "lol"})
	time.Sleep(200 * time.Millisecond)
	s.Stop()
}
