package jobs

import (
	"testing"
	"time"
)

// TestActiveToIdleAfterSubIndexRefresh covers C2/I8: while Active, a SubIndex
// Refresh that clears subscriptions must let the next TickOnce return to Idle
// (main must Refresh on IdlePoll regardless of state so HasAny is not stale).
func TestActiveToIdleAfterSubIndexRefresh(t *testing.T) {
	idx := NewRedisSubIndex(func() (bool, []string) {
		return true, []string{"gossiping"}
	})
	idx.Refresh()
	s := NewScheduler(SchedulerConfig{ActiveTick: time.Second, IdlePoll: time.Second, Workers: 1, QueueSize: 8},
		idx.HasAny, idx.SubscribedBoards)
	s.TickOnce(time.Now())
	if s.State() != StateActive {
		t.Fatalf("state=%v want active", s.State())
	}

	// Simulate IdlePoll Refresh while still Active (unsubscribe last board).
	idx.loader = func() (bool, []string) { return false, nil }
	idx.Refresh()
	if idx.HasAny() {
		t.Fatal("index should report no subs after refresh")
	}

	s.TickOnce(time.Now())
	if s.State() != StateIdle {
		t.Fatalf("state=%v want idle after refresh cleared subs", s.State())
	}
	if len(idx.SubscribedBoards()) != 0 {
		t.Fatalf("boards=%v want empty", idx.SubscribedBoards())
	}
}
