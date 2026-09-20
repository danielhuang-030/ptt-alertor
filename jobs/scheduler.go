package jobs

import (
	"context"
	"sync"
	"time"
)

// SchedulerState is Idle or Active.
type SchedulerState int

const (
	StateIdle SchedulerState = iota
	StateActive
)

// Scheduler owns Idle/Active ticks and the work queue.
type Scheduler struct {
	cfg     SchedulerConfig
	queue   *WorkQueue
	hasSubs func() bool
	boards  func() []string

	mu         sync.Mutex
	state      SchedulerState
	refresher  BoardRefresher
	started    bool
	paused     bool
	runCtx     context.Context
	runCancel  context.CancelFunc
	wg         sync.WaitGroup
}

// NewScheduler builds a scheduler with injectable subscription probes.
func NewScheduler(cfg SchedulerConfig, hasSubs func() bool, boards func() []string) *Scheduler {
	if cfg.QueueSize < 1 {
		cfg.QueueSize = 1
	}
	if hasSubs == nil {
		hasSubs = func() bool { return false }
	}
	if boards == nil {
		boards = func() []string { return nil }
	}
	return &Scheduler{
		cfg:     cfg,
		queue:   NewWorkQueue(cfg.QueueSize),
		hasSubs: hasSubs,
		boards:  boards,
		state:   StateIdle,
	}
}

// State returns the current Idle/Active state.
func (s *Scheduler) State() SchedulerState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// QueueLen returns queued work count.
func (s *Scheduler) QueueLen() int {
	return s.queue.Len()
}

// TickOnce advances Idle/Active and enqueues board refreshes when Active.
func (s *Scheduler) TickOnce(now time.Time) {
	_ = now
	s.mu.Lock()
	paused := s.paused
	s.mu.Unlock()
	if paused {
		return
	}
	if !s.hasSubs() {
		s.mu.Lock()
		s.state = StateIdle
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	s.state = StateActive
	s.mu.Unlock()
	for _, b := range s.boards() {
		name := b
		if name == "" {
			continue
		}
		s.enqueueWork(WorkItem{Kind: WorkRefreshBoard, Board: name})
	}
}
