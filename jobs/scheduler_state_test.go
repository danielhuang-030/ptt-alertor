package jobs

import (
	"testing"
	"time"
)

func TestSchedulerIdleSkipsEnqueue(t *testing.T) {
	s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
		func() bool { return false },
		func() []string { return []string{"gossiping"} },
	)
	s.TickOnce(time.Now())
	if s.State() != StateIdle {
		t.Fatalf("state=%v", s.State())
	}
	if s.QueueLen() != 0 {
		t.Fatalf("queue=%d want 0", s.QueueLen())
	}
}

func TestSchedulerActiveEnqueuesBoards(t *testing.T) {
	s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
		func() bool { return true },
		func() []string { return []string{"gossiping", "lol"} },
	)
	s.TickOnce(time.Now())
	if s.State() != StateActive {
		t.Fatalf("state=%v", s.State())
	}
	if s.QueueLen() != 2 {
		t.Fatalf("queue=%d want 2", s.QueueLen())
	}
}

func TestSchedulerActiveToIdle(t *testing.T) {
	has := true
	s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
		func() bool { return has },
		func() []string { return []string{"gossiping"} },
	)
	s.TickOnce(time.Now())
	if s.State() != StateActive || s.QueueLen() != 1 {
		t.Fatalf("active setup state=%v q=%d", s.State(), s.QueueLen())
	}
	// drain queue reservation so we can observe no *new* enqueue; state still matters
	has = false
	s.TickOnce(time.Now())
	if s.State() != StateIdle {
		t.Fatalf("state=%v want idle", s.State())
	}
}
