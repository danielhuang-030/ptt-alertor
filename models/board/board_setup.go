package board

import (
	"fmt"
	"os"

	log "github.com/Ptt-Alertor/logrus"
)

var (
	defaultBoardDriver Driver
	defaultBoardCacher Cacher
)

// InitBoardStorage initializes the default board driver and cacher based on environment variables.
// It should be called once at application startup.
func InitBoardStorage() {
	driverType := os.Getenv("BOARD_DRIVER_TYPE")
	if driverType == "" {
		driverType = "file" // Default to file
		log.Info("BOARD_DRIVER_TYPE not set, defaulting to 'file'")
	}

	cacherType := os.Getenv("BOARD_CACHER_TYPE")
	// Default to "redis" if empty or explicitly "redis"
	if cacherType == "" || cacherType == "redis" {
		if os.Getenv("REDIS_HOST_PORT") == "" {
			log.Warn("BOARD_CACHER_TYPE is 'redis' but REDIS_HOST_PORT is not set. Redis Cacher operations may fail if Redis is not available on default/localhost.")
		}
		// Assuming Redis struct implements Cacher and is accessible
		defaultBoardCacher = new(Redis)
		log.Info("Board Cacher initialized with: redis")
	} else {
		log.Fatalf("Unsupported BOARD_CACHER_TYPE: %s. Currently only 'redis' is supported.", cacherType)
	}

	log.Infof("Initializing Board Driver with type: %s", driverType)
	switch driverType {
	case "file":
		// The actual initialization of 'articlesDir' (path) for File driver
		// is handled within file.go's init() function.
		// Here, we just instantiate the File driver.
		defaultBoardDriver = new(File)
		// The log message in file.go's init() will confirm the path used.
		log.Info("Board Driver initialized with: file. Storage path is determined by BOARD_FILE_STORAGE_PATH (if set) or defaults to ./storage/board_articles/ (handled in file.go).")
	case "redis":
		if os.Getenv("REDIS_HOST_PORT") == "" {
			log.Warn("BOARD_DRIVER_TYPE is 'redis' but REDIS_HOST_PORT is not set. Redis Driver operations may fail if Redis is not available on default/localhost.")
		}
		// Assuming Redis struct implements Driver and is accessible
		defaultBoardDriver = new(Redis)
		log.Info("Board Driver initialized with: redis")
	case "dynamodb":
		// Ensure AWS SDK related environment variables (AWS_REGION, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY)
		// are set in the environment for DynamoDB to work.
		if os.Getenv("AWS_REGION") == "" {
			log.Warn("BOARD_DRIVER_TYPE is 'dynamodb' but AWS_REGION is not set. DynamoDB operations may fail if region is not configured globally for the SDK.")
		}
		// Assuming DynamoDB struct implements Driver and is accessible
		defaultBoardDriver = new(DynamoDB)
		log.Info("Board Driver initialized with: dynamodb")
	default:
		log.Fatalf("Unsupported BOARD_DRIVER_TYPE: %s. Supported types are 'file', 'redis', 'dynamodb'.", driverType)
	}

	if defaultBoardDriver == nil {
		// This case should ideally be prevented by the default case in the switch,
		// but as a safeguard:
		log.Fatal("Failed to initialize defaultBoardDriver: driver instance is nil after switch.")
	}
	if defaultBoardCacher == nil {
		// This case should ideally be prevented by the logic for cacherType,
		// but as a safeguard:
		log.Fatal("Failed to initialize defaultBoardCacher: cacher instance is nil.")
	}

	log.Info("Board storage initialization complete.")
}

// GetDefaultBoardDriver returns the initialized default board driver.
// It will cause a fatal error if InitBoardStorage has not been called successfully.
func GetDefaultBoardDriver() Driver {
	if defaultBoardDriver == nil {
		log.Fatal("Board Driver not initialized. Call InitBoardStorage first.")
	}
	return defaultBoardDriver
}

// GetDefaultBoardCacher returns the initialized default board cacher.
// It will cause a fatal error if InitBoardStorage has not been called successfully.
func GetDefaultBoardCacher() Cacher {
	if defaultBoardCacher == nil {
		log.Fatal("Board Cacher not initialized. Call InitBoardStorage first.")
	}
	return defaultBoardCacher
}
