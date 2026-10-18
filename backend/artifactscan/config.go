package artifactscan

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// OpenFromEnv enables scans only for an explicit local directory or dedicated Redis URL.
// It never pings Redis or falls back to the application's generic cache settings.
func OpenFromEnv(inspectorVersion string) (*Store, func() error, error) {
	dir := strings.TrimSpace(os.Getenv("ARTIFACT_SCAN_DIR"))
	redisURL := strings.TrimSpace(os.Getenv("ARTIFACT_SCAN_REDIS_URL"))
	close := func() error { return nil }
	if dir != "" && redisURL != "" {
		return nil, close, errors.New("configure only one of ARTIFACT_SCAN_DIR or ARTIFACT_SCAN_REDIS_URL")
	}
	if dir != "" {
		store, err := NewLocal(dir, inspectorVersion)
		return store, close, err
	}
	if redisURL == "" {
		return nil, close, nil
	}
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, close, errors.New("invalid ARTIFACT_SCAN_REDIS_URL")
	}
	options.DialTimeout = 300 * time.Millisecond
	options.ReadTimeout = 300 * time.Millisecond
	options.WriteTimeout = 300 * time.Millisecond
	options.ContextTimeoutEnabled = true
	options.MaxRetries = -1
	client := redis.NewClient(options)
	store, err := NewRedis(client, inspectorVersion)
	if err != nil {
		_ = client.Close()
		return nil, close, err
	}
	return store, client.Close, nil
}
