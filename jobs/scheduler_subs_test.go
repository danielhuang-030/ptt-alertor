package jobs

import (
	"testing"
	"time"
)

func TestSchedulerUsesSubIndex(t *testing.T) {
	idx := &fakeSubIndex{any: true, boards: []string{"soft_job"}}
	s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
		idx.HasAny, idx.SubscribedBoards)
	s.TickOnce(time.Now())
	if s.QueueLen() != 1 {
		t.Fatalf("queue=%d", s.QueueLen())
	}
}

func TestRedisSubIndexRefresh(t *testing.T) {
	calls := 0
	idx := NewRedisSubIndex(func() (bool, []string) {
		calls++
		return true, []string{"gossiping"}
	})
	if idx.HasAny() {
		t.Fatal("should start empty")
	}
	idx.Refresh()
	if !idx.HasAny() || len(idx.SubscribedBoards()) != 1 {
		t.Fatalf("after refresh any=%v boards=%v", idx.HasAny(), idx.SubscribedBoards())
	}
	if calls != 1 {
		t.Fatalf("calls=%d", calls)
	}
}
