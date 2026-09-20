package jobs

import (
	"context"
	"time"

	log "github.com/Ptt-Alertor/logrus"
)

// BoardRefresher fetches and matches a board (injectable for tests).
type BoardRefresher interface {
	Refresh(board string) error
}

type defaultBoardRefresher struct{}

func (defaultBoardRefresher) Refresh(board string) error {
	return refreshBoardAndNotify(board)
}

// SetRefresher overrides the board refresh implementation (tests / wiring).
func (s *Scheduler) SetRefresher(r BoardRefresher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refresher = r
}

// Start runs worker pool and ticker loops until ctx cancel or Stop.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.started = true
	if s.refresher == nil {
		s.refresher = defaultBoardRefresher{}
	}
	s.runCtx, s.runCancel = context.WithCancel(ctx)
	workers := s.cfg.Workers
	if workers < 1 {
		workers = 1
	}
	s.wg.Add(workers + 1)
	runCtx := s.runCtx
	s.mu.Unlock()

	for i := 0; i < workers; i++ {
		go s.workerLoop(runCtx)
	}
	go s.tickLoop(runCtx)
}

// Stop cancels workers and waits for them to finish.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.started {
		s.mu.Unlock()
		return
	}
	cancel := s.runCancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.wg.Wait()
	s.mu.Lock()
	s.started = false
	s.mu.Unlock()
}

// Pause stops enqueueing new board refreshes (PTT outage).
func (s *Scheduler) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	log.Info("Scheduler paused")
}

// Resume allows enqueueing again.
func (s *Scheduler) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	log.Info("Scheduler resumed")
}

func (s *Scheduler) workerLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		item, ok := s.queue.Pop(ctx)
		if !ok {
			return
		}
		s.handleItem(item)
		s.queue.MarkDone(item.Board)
	}
}

func (s *Scheduler) handleItem(item WorkItem) {
	defer func() {
		if rec := recover(); rec != nil {
			log.WithField("board", item.Board).Errorf("scheduler work panic: %v", rec)
		}
	}()
	switch item.Kind {
	case WorkRefreshBoard:
		s.mu.Lock()
		r := s.refresher
		s.mu.Unlock()
		if r == nil {
			return
		}
		if err := r.Refresh(item.Board); err != nil {
			log.WithError(err).WithField("board", item.Board).Warn("board refresh failed")
		}
	case WorkCheckPushSum, WorkCheckComment:
		// Follow-ups: best-effort hooks; full wiring can extend without busy loops.
	}
}

func (s *Scheduler) tickLoop(ctx context.Context) {
	defer s.wg.Done()
	idle := s.cfg.IdlePoll
	active := s.cfg.ActiveTick
	if idle < time.Second {
		idle = 30 * time.Second
	}
	if active < time.Second {
		active = 5 * time.Second
	}
	ticker := time.NewTicker(active)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.mu.Lock()
			paused := s.paused
			s.mu.Unlock()
			if paused {
				continue
			}
			s.TickOnce(now)
			if s.State() == StateIdle && idle > active {
				ticker.Reset(idle)
			} else {
				ticker.Reset(active)
			}
		}
	}
}
