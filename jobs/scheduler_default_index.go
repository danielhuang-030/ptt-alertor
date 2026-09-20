package jobs

import (
	"strings"

	"github.com/Ptt-Alertor/ptt-alertor/models"
)

// BuildSubIndexFromUsers scans users once (may use KEYS internally) and caches boards.
// Call Refresh on Idle poll only — not on every Active tick.
func BuildSubIndexFromUsers() *redisSubIndex {
	idx := NewRedisSubIndex(loadSubsFromUsers)
	idx.Refresh()
	return idx
}

func loadSubsFromUsers() (bool, []string) {
	seen := map[string]struct{}{}
	var boards []string
	for _, u := range models.User().All() {
		if u == nil || !u.Enable {
			continue
		}
		for _, sub := range u.Subscribes {
			name := strings.ToLower(strings.TrimSpace(sub.Board))
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			boards = append(boards, name)
		}
	}
	return len(boards) > 0, boards
}
