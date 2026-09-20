package jobs

import (
	"testing"
	"time"
)

func TestLoadSchedulerConfigDefaults(t *testing.T) {
	t.Setenv("SCHED_IDLE_POLL", "")
	t.Setenv("SCHED_ACTIVE_TICK", "")
	t.Setenv("SCHED_WORKERS", "")
	t.Setenv("SCHED_QUEUE_SIZE", "")
	t.Setenv("SCHED_LEGACY", "")
	t.Setenv("BOARD_HIGH", "")
	t.Setenv("SCHED_HIGH_TICK", "")
	cfg := LoadSchedulerConfig()
	if cfg.IdlePoll < 30*time.Second || cfg.IdlePoll > 60*time.Second {
		t.Fatalf("IdlePoll default out of range: %v", cfg.IdlePoll)
	}
	if cfg.ActiveTick < 5*time.Second || cfg.ActiveTick > 10*time.Second {
		t.Fatalf("ActiveTick default out of range: %v", cfg.ActiveTick)
	}
	if cfg.Workers < 2 || cfg.Workers > 4 {
		t.Fatalf("Workers default out of range: %d", cfg.Workers)
	}
	if cfg.Legacy {
		t.Fatal("Legacy should default false")
	}
}

func TestLoadSchedulerConfigOverrides(t *testing.T) {
	t.Setenv("SCHED_IDLE_POLL", "45s")
	t.Setenv("SCHED_ACTIVE_TICK", "8s")
	t.Setenv("SCHED_WORKERS", "3")
	t.Setenv("SCHED_QUEUE_SIZE", "128")
	t.Setenv("SCHED_LEGACY", "1")
	t.Setenv("BOARD_HIGH", " Gossiping , LoL ")
	t.Setenv("SCHED_HIGH_TICK", "")
	cfg := LoadSchedulerConfig()
	if cfg.IdlePoll != 45*time.Second {
		t.Fatalf("IdlePoll=%v", cfg.IdlePoll)
	}
	if cfg.ActiveTick != 8*time.Second {
		t.Fatalf("ActiveTick=%v", cfg.ActiveTick)
	}
	if cfg.Workers != 3 {
		t.Fatalf("Workers=%d", cfg.Workers)
	}
	if cfg.QueueSize != 128 {
		t.Fatalf("QueueSize=%d", cfg.QueueSize)
	}
	if !cfg.Legacy {
		t.Fatal("Legacy should be true")
	}
	if len(cfg.HighBoards) != 2 || cfg.HighBoards[0] != "gossiping" || cfg.HighBoards[1] != "lol" {
		t.Fatalf("HighBoards=%v", cfg.HighBoards)
	}
	if cfg.HighTick != 5*time.Second {
		t.Fatalf("HighTick=%v want 5s (faster than ActiveTick 8s)", cfg.HighTick)
	}
}

func TestLoadSchedulerConfigHighTickOverride(t *testing.T) {
	t.Setenv("SCHED_IDLE_POLL", "")
	t.Setenv("SCHED_ACTIVE_TICK", "10s")
	t.Setenv("SCHED_WORKERS", "")
	t.Setenv("SCHED_QUEUE_SIZE", "")
	t.Setenv("SCHED_LEGACY", "")
	t.Setenv("BOARD_HIGH", "gossiping")
	t.Setenv("SCHED_HIGH_TICK", "8s")
	cfg := LoadSchedulerConfig()
	if cfg.HighTick != 8*time.Second {
		t.Fatalf("HighTick=%v want 8s", cfg.HighTick)
	}
	t.Setenv("SCHED_HIGH_TICK", "2s") // below floor
	cfg = LoadSchedulerConfig()
	if cfg.HighTick != 5*time.Second {
		t.Fatalf("HighTick=%v want 5s floor", cfg.HighTick)
	}
}
