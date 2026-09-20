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

const defaultBoardBackoff = 60 * time.Second

// Scheduler owns Idle/Active ticks and the work queue.
type Scheduler struct {
	cfg     SchedulerConfig
	queue   *WorkQueue
	hasSubs func() bool
	boards  func() []string

	mu            sync.Mutex
	state         SchedulerState
	refresher     BoardRefresher
	started       bool
	paused        bool
	runCtx        context.Context
	runCancel     context.CancelFunc
	wg            sync.WaitGroup
	lastEnqueued  map[string]time.Time
	nextAllowed   map[string]time.Time
	highSet       map[string]struct{}
	refreshOK     int
	refreshFail   int
	obsCounter    int
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
	high := make(map[string]struct{}, len(cfg.HighBoards))
	for _, b := range cfg.HighBoards {
		if b != "" {
			high[b] = struct{}{}
		}
	}
	return &Scheduler{
		cfg:          cfg,
		queue:        NewWorkQueue(cfg.QueueSize),
		hasSubs:      hasSubs,
		boards:       boards,
		state:        StateIdle,
		lastEnqueued: make(map[string]time.Time),
		nextAllowed:  make(map[string]time.Time),
		highSet:      high,
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

func (s *Scheduler) boardInterval(board string) time.Duration {
	if _, ok := s.highSet[board]; ok {
		if s.cfg.HighTick > 0 {
			return s.cfg.HighTick
		}
		return 5 * time.Second
	}
	if s.cfg.ActiveTick > 0 {
		return s.cfg.ActiveTick
	}
	return 5 * time.Second
}

// TickOnce advances Idle/Active and enqueues due board refreshes when Active.
func (s *Scheduler) TickOnce(now time.Time) {
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
		s.mu.Lock()
		if until, ok := s.nextAllowed[name]; ok && now.Before(until) {
			s.mu.Unlock()
			continue
		}
		last := s.lastEnqueued[name]
		interval := s.boardInterval(name)
		due := last.IsZero() || !last.Add(interval).After(now)
		if due {
			s.lastEnqueued[name] = now
		}
		s.mu.Unlock()
		if !due {
			continue
		}
		s.enqueueWork(WorkItem{Kind: WorkRefreshBoard, Board: name})
	}
}

func (s *Scheduler) noteRefreshFailure(board string, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshFail++
	backoff := defaultBoardBackoff
	s.nextAllowed[board] = now.Add(backoff)
}

func (s *Scheduler) noteRefreshSuccess(board string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refreshOK++
	delete(s.nextAllowed, board)
}

func (s *Scheduler) observabilitySnapshot() (state SchedulerState, qlen, boards, ok, fail int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.queue.Len(), len(s.boards()), s.refreshOK, s.refreshFail
}
