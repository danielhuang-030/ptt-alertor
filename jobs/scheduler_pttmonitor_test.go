package jobs

import (
	"testing"
	"time"
)

func TestSchedulerPauseResumeBlocksEnqueue(t *testing.T) {
	s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
		func() bool { return true },
		func() []string { return []string{"gossiping"} },
	)
	s.Pause()
	s.TickOnce(time.Now())
	if s.QueueLen() != 0 {
		t.Fatalf("paused scheduler should not enqueue, q=%d", s.QueueLen())
	}
	s.Resume()
	s.TickOnce(time.Now())
	if s.QueueLen() != 1 {
		t.Fatalf("after resume expect 1, q=%d", s.QueueLen())
	}
}
