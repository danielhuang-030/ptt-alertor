package jobs

import (
	"os"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
)

// RunOneshotJobsIfEnabled runs migration/cleanup jobs only when RUN_ONESHOT_JOBS=1.
func RunOneshotJobsIfEnabled() {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("RUN_ONESHOT_JOBS")))
	if v != "1" && v != "true" && v != "yes" {
		log.Info("Skipping oneshot jobs (set RUN_ONESHOT_JOBS=1 to enable)")
		return
	}
	log.Info("Running oneshot jobs")
	NewPushSumKeyReplacer().Run()
	NewMigrateBoard(map[string]string{}).Run()
	NewTop().Run()
	NewCacheCleaner().Run()
	NewGenerator().Run()
	NewFetcher().Run()
	NewMigrateDB().Run()
	NewCategoryCleaner().Run()
}
