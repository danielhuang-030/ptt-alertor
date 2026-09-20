package jobs

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type countingRefresher struct {
	n atomic.Int32
}

func (c *countingRefresher) Refresh(board string) error {
	c.n.Add(1)
	return nil
}

func TestSchedulerWorkerCallsRefresh(t *testing.T) {
	ref := &countingRefresher{}
	s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
		func() bool { return true },
		func() []string { return []string{"gossiping"} },
	)
	s.SetRefresher(ref)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)
	s.TickOnce(time.Now())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ref.n.Load() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if ref.n.Load() < 1 {
		t.Fatal("Refresh not called")
	}
	s.Stop()
	// after MarkDone, can enqueue again
	if !s.queue.TryEnqueue(WorkItem{Kind: WorkRefreshBoard, Board: "gossiping"}) {
		t.Fatal("should allow re-enqueue after done")
	}
}
