package connections

import (
	"os"
	"time"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/garyburd/redigo/redis"
)

var pool = newPool()

func newPool() *redis.Pool {
	return &redis.Pool{
		MaxIdle:     3,
		IdleTimeout: 300 * time.Second,
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", os.Getenv("REDIS_ENDPOINT")+":"+os.Getenv("REDIS_PORT"),
				redisDialOptions(os.Getenv("REDIS_PASSWORD"))...)
			if err != nil {
				log.Fatal(err)
			}
			return conn, err
		},
	}
}

// redisDialOptions adds AUTH only when a password is configured,
// so local setups without requirepass keep working.
func redisDialOptions(password string) []redis.DialOption {
	if password == "" {
		return nil
	}
	return []redis.DialOption{redis.DialPassword(password)}
}

// Redis get redis connection
func Redis() redis.Conn {
	return pool.Get()
}
