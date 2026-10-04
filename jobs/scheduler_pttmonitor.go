package jobs

import (
	"net/http"
	"time"

	log "github.com/Ptt-Alertor/logrus"
)

// RunSchedulerPttMonitor polls PTT like the classic monitor but only Pause/Resume
// the scheduler — never launches Checker/PushSum/Comment Run loops.
func RunSchedulerPttMonitor(sched *Scheduler) {
	log.Info("Start Scheduler PTT Monitor")
	const (
		url      = "https://www.ptt.cc/bbs/index.html"
		duration = 1 * time.Minute
		retry    = 3
	)
	errorCounter := 0
	ticker := time.NewTicker(duration)
	defer ticker.Stop()
	client := &http.Client{Timeout: 15 * time.Second}
	for range ticker.C {
		resp, err := client.Get(url)
		if err != nil {
			log.WithError(err).Error("HTTP Get Error")
			// Treat transport errors as unhealthy toward Pause.
			if errorCounter < retry {
				log.Info("Ptt is dying")
			}
			if errorCounter == retry {
				log.Info("Ptt is Dead")
				sched.Pause()
			}
			errorCounter++
			continue
		}
		status := resp.StatusCode
		_ = resp.Body.Close()
		if status == http.StatusOK {
			log.Info("Ptt is alive")
			if errorCounter >= retry {
				log.Info("Ptt is back to life")
				sched.Resume()
			}
			errorCounter = 0
			continue
		}
		if errorCounter < retry {
			log.Info("Ptt is dying")
		}
		if errorCounter == retry {
			log.Info("Ptt is Dead")
			sched.Pause()
		}
		errorCounter++
	}
}
