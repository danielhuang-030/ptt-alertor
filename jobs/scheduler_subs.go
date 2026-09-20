package jobs

import "sync"

// SubIndex answers whether any subscriptions exist and which boards are tracked.
type SubIndex interface {
	HasAny() bool
	SubscribedBoards() []string
}

type fakeSubIndex struct {
	any    bool
	boards []string
}

func (f *fakeSubIndex) HasAny() bool { return f.any }

func (f *fakeSubIndex) SubscribedBoards() []string {
	out := make([]string, len(f.boards))
	copy(out, f.boards)
	return out
}

// redisSubIndex caches a subscription snapshot. Refresh from Idle poll only — not every Active tick.
// loader must not use Redis KEYS on the hot path.
type redisSubIndex struct {
	mu     sync.Mutex
	any    bool
	boards []string
	loader func() (bool, []string)
}

// NewRedisSubIndex builds a SubIndex. Pass nil loader to start empty until Refresh.
func NewRedisSubIndex(loader func() (bool, []string)) *redisSubIndex {
	if loader == nil {
		loader = func() (bool, []string) { return false, nil }
	}
	return &redisSubIndex{loader: loader}
}

func (r *redisSubIndex) HasAny() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.any
}

func (r *redisSubIndex) SubscribedBoards() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.boards))
	copy(out, r.boards)
	return out
}

// Refresh reloads the cached subscription snapshot.
func (r *redisSubIndex) Refresh() {
	any, boards := r.loader()
	r.mu.Lock()
	r.any = any
	r.boards = boards
	r.mu.Unlock()
}
