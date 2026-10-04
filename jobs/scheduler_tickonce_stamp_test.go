package jobs

import (
	"context"
	"testing"
	"time"
)

func TestTickOnceDoesNotStampLastEnqueuedOnEnqueueFull(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		ActiveTick: 10 * time.Second,
		HighTick:   10 * time.Second,
		Workers:    1,
		QueueSize:  1,
		IdlePoll:   time.Second,
	}, func() bool { return true }, func() []string { return []string{"a", "b"} })

	t0 := time.Unix(3000, 0)
	s.TickOnce(t0)
	if s.QueueLen() != 1 {
		t.Fatalf("queue=%d want 1 (capacity)", s.QueueLen())
	}
	s.mu.Lock()
	lastA, okA := s.lastEnqueued["a"]
	lastB, okB := s.lastEnqueued["b"]
	s.mu.Unlock()
	// Exactly one board stamped (OK), the other hit Full and must NOT be stamped.
	stamped := 0
	if okA && !lastA.IsZero() {
		stamped++
	}
	if okB && !lastB.IsZero() {
		stamped++
	}
	if stamped != 1 {
		t.Fatalf("stamped=%d want 1 (only EnqueueOK); a=%v b=%v", stamped, lastA, lastB)
	}

	// Immediately: unstamped board should still be due and retry enqueue.
	// Drain the one queued item first so capacity frees, but leave lastEnqueued of stamped board.
	item, _ := s.queue.Pop(context.Background())
	s.queue.MarkDone(item)

	t1 := t0.Add(time.Second) // still within ActiveTick for stamped board
	s.TickOnce(t1)
	// Unstamped board must enqueue; stamped must not (interval not elapsed).
	if s.QueueLen() != 1 {
		t.Fatalf("after retry queue=%d want 1 (unstamped board)", s.QueueLen())
	}
}

func TestStartTriggersImmediateTickOnce(t *testing.T) {
	s := NewScheduler(SchedulerConfig{
		ActiveTick: time.Second,
		HighTick:   time.Second,
		Workers:    1,
		QueueSize:  8,
		IdlePoll:   time.Hour, // would starve without immediate tick
	}, func() bool { return true }, func() []string { return []string{"gossiping"} })
	ref := &countingRefresher{}
	s.SetRefresher(ref)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ref.n.Load() >= 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	s.Stop()
	if ref.n.Load() < 1 {
		t.Fatal("Start should immediate TickOnce and refresh without waiting IdlePoll")
	}
}
