package jobs

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestHighTickEnqueuesFasterThanActiveTick(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		ActiveTick: 10 * time.Second,
		HighTick:   5 * time.Second,
		HighBoards: []string{"gossiping"},
		Workers:    1,
		QueueSize:  8,
		IdlePoll:   time.Second,
	}, func() bool { return true }, func() []string { return []string{"gossiping", "lol"} })

	t0 := time.Unix(1000, 0)
	s.TickOnce(t0)
	if s.QueueLen() != 2 {
		t.Fatalf("initial queue=%d want 2", s.QueueLen())
	}
	// Drain pending so re-enqueue is allowed.
	for s.QueueLen() > 0 {
		item, _ := s.queue.Pop(t.Context())
		s.queue.MarkDone(item)
	}

	// 5s later: high board due, normal board not yet.
	s.TickOnce(t0.Add(5 * time.Second))
	if s.QueueLen() != 1 {
		t.Fatalf("after 5s queue=%d want 1 (high only)", s.QueueLen())
	}
	item, ok := s.queue.Pop(t.Context())
	if !ok || item.Board != "gossiping" {
		t.Fatalf("expected gossiping, got ok=%v item=%+v", ok, item)
	}
	s.queue.MarkDone(item)

	// 10s from start: lol due.
	s.TickOnce(t0.Add(10 * time.Second))
	found := map[string]bool{}
	for s.QueueLen() > 0 {
		it, _ := s.queue.Pop(t.Context())
		found[it.Board] = true
		s.queue.MarkDone(it)
	}
	if !found["lol"] {
		t.Fatalf("lol should enqueue after ActiveTick; found=%v", found)
	}
}

type failRefresher struct{}

func (failRefresher) Refresh(board string) error {
	return errors.New("boom")
}

func TestRefreshFailureAppliesBackoff(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		ActiveTick: time.Second,
		HighTick:   time.Second,
		Workers:    1,
		QueueSize:  8,
		IdlePoll:   time.Second,
	}, func() bool { return true }, func() []string { return []string{"gossiping"} })

	t0 := time.Unix(2000, 0)
	s.noteRefreshFailure("gossiping", t0)
	s.TickOnce(t0.Add(10 * time.Second))
	if s.QueueLen() != 0 {
		t.Fatalf("should skip during backoff, queue=%d", s.QueueLen())
	}
	s.TickOnce(t0.Add(defaultBoardBackoff + time.Second))
	if s.QueueLen() != 1 {
		t.Fatalf("should enqueue after backoff, queue=%d", s.QueueLen())
	}
}

func TestHighTickConfigIsFiveSecondsWhenActiveLonger(t *testing.T) {
	t.Setenv("SCHED_IDLE_POLL", "30s")
	t.Setenv("SCHED_ACTIVE_TICK", "10s")
	t.Setenv("SCHED_WORKERS", "")
	t.Setenv("SCHED_QUEUE_SIZE", "")
	t.Setenv("SCHED_LEGACY", "")
	t.Setenv("BOARD_HIGH", "gossiping")
	t.Setenv("SCHED_HIGH_TICK", "")
	cfg := LoadSchedulerConfig()
	if cfg.HighTick != 5*time.Second {
		t.Fatalf("HighTick=%v want 5s when ActiveTick=10s", cfg.HighTick)
	}
}

func TestRefreshErrorViaWorkerArmsBackoff(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		ActiveTick: time.Second,
		HighTick:   time.Second,
		Workers:    1,
		QueueSize:  8,
		IdlePoll:   time.Hour, // avoid ticker noise
	}, func() bool { return true }, func() []string { return []string{"gossiping"} })
	s.SetRefresher(failRefresher{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s.mu.Lock()
		fail := s.refreshFail
		s.mu.Unlock()
		if fail >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.Stop()

	s.mu.Lock()
	fail := s.refreshFail
	until, ok := s.nextAllowed["gossiping"]
	s.mu.Unlock()
	if fail < 1 {
		t.Fatal("refreshFail not incremented")
	}
	if !ok || until.IsZero() {
		t.Fatal("nextAllowed not armed after refresh error")
	}

	// Within backoff window: TickOnce must skip.
	s.TickOnce(time.Now())
	if s.QueueLen() != 0 {
		t.Fatalf("should skip during backoff after worker failure, queue=%d", s.QueueLen())
	}
}
