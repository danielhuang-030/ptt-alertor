package jobs

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// SchedulerConfig holds env-driven settings for the central scheduler.
type SchedulerConfig struct {
	IdlePoll   time.Duration
	ActiveTick time.Duration
	Workers    int
	QueueSize  int
	HighBoards []string
	HighTick   time.Duration
	Legacy     bool
}

// LoadSchedulerConfig reads SCHED_* / BOARD_HIGH with safe defaults.
func LoadSchedulerConfig() SchedulerConfig {
	cfg := SchedulerConfig{
		IdlePoll:   30 * time.Second,
		ActiveTick: 5 * time.Second,
		Workers:    2,
		QueueSize:  256,
		Legacy:     false,
	}

	if v := strings.TrimSpace(os.Getenv("SCHED_IDLE_POLL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.IdlePoll = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("SCHED_ACTIVE_TICK")); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cfg.ActiveTick = d
		}
	}
	if v := strings.TrimSpace(os.Getenv("SCHED_WORKERS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Workers = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("SCHED_QUEUE_SIZE")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.QueueSize = n
		}
	}

	legacy := strings.ToLower(strings.TrimSpace(os.Getenv("SCHED_LEGACY")))
	cfg.Legacy = legacy == "1" || legacy == "true" || legacy == "yes"

	if v := strings.TrimSpace(os.Getenv("BOARD_HIGH")); v != "" {
		parts := strings.Split(v, ",")
		for _, p := range parts {
			name := strings.ToLower(strings.TrimSpace(p))
			if name != "" {
				cfg.HighBoards = append(cfg.HighBoards, name)
			}
		}
	}

	// HighTick defaults to 5s floor so high boards can tick faster than a longer ActiveTick.
	minHigh := 5 * time.Second
	cfg.HighTick = minHigh
	if cfg.ActiveTick > 0 && cfg.ActiveTick < minHigh {
		// Keep floor at 5s even if ActiveTick were misconfigured below it.
		cfg.HighTick = minHigh
	}

	return cfg
}
