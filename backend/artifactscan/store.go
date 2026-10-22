// Package artifactscan keeps full-file inspection work and evidence outside HTTP requests.
package artifactscan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/argval/yatko/github"
	"github.com/redis/go-redis/v9"
)

const (
	StateQueued   = "queued"
	StateRunning  = "running"
	StateRetry    = "retry"
	StateComplete = "complete"
	StateError    = "error"

	leaseDuration  = 15 * time.Minute
	attemptTimeout = 10 * time.Minute
	retryDelay     = time.Minute
	maxAttempts    = 3
)

// Record separates observed file facts from queue failures and retry state.
type Record struct {
	Key              string                   `json:"key"`
	InspectorVersion string                   `json:"inspector_version"`
	Asset            github.Asset             `json:"asset"`
	State            string                   `json:"state"`
	Metadata         *github.ArtifactMetadata `json:"metadata,omitempty"`
	Reason           string                   `json:"reason,omitempty"`
	LastError        string                   `json:"last_error,omitempty"`
	Attempts         int                      `json:"attempts"`
	NextAttemptAt    time.Time                `json:"next_attempt_at,omitempty"`
	LeaseUntil       time.Time                `json:"lease_until,omitempty"`
	LeaseToken       string                   `json:"lease_token,omitempty"`
	UpdatedAt        time.Time                `json:"updated_at"`
}

// Ready reports whether metadata came from a completed inspection, including unknown results.
func (r Record) Ready() bool {
	return r.State == StateComplete && r.Metadata != nil
}

// Store uses either a local directory or an explicitly supplied Redis client.
// Evidence has no expiry; asset identity and inspector version invalidate it.
type Store struct {
	version string
	dir     string
	client  *redis.Client
	prefix  string
	now     func() time.Time
}

// NewRedis uses a dedicated namespace without contacting Redis or reading environment variables.
// The caller owns the client and must configure durable Redis persistence.
func NewRedis(client *redis.Client, inspectorVersion string) (*Store, error) {
	if client == nil || strings.TrimSpace(inspectorVersion) == "" {
		return nil, errors.New("artifact scan requires a Redis client and inspector version")
	}
	versionHash := sha256.Sum256([]byte(inspectorVersion))
	return &Store{
		version: inspectorVersion, client: client, now: time.Now,
		prefix: "artifactscan:v1:{" + hex.EncodeToString(versionHash[:]) + "}:",
	}, nil
}

// Lookup reads existing evidence or queue state without creating or claiming a job.
func (s *Store) Lookup(ctx context.Context, asset github.Asset) (Record, bool, error) {
	r, err := s.read(ctx, s.key(asset))
	if err != nil || r == nil {
		return Record{}, false, err
	}
	return *r, true, nil
}

// LookupMany reads a release's records with one Redis round-trip or local file reads.
// Results use the uploaded asset URL; absent assets are omitted.
func (s *Store) LookupMany(ctx context.Context, assets []github.Asset) (map[string]Record, error) {
	results := make(map[string]Record)
	if len(assets) == 0 {
		return results, nil
	}
	if s.client == nil {
		for _, asset := range assets {
			record, found, err := s.Lookup(ctx, asset)
			if err != nil {
				return results, err
			}
			if found {
				results[asset.BrowserDownloadURL] = record
			}
		}
		return results, nil
	}
	keys := make([]string, len(assets))
	for i, asset := range assets {
		keys[i] = s.prefix + "record:" + s.key(asset)
	}
	values, err := s.client.MGet(ctx, keys...).Result()
	if err != nil {
		return results, err
	}
	for i, value := range values {
		if value == nil {
			continue
		}
		data, ok := value.(string)
		if !ok {
			return results, errors.New("invalid Redis artifact scan record type")
		}
		record, err := decodeRecord([]byte(data), s.key(assets[i]), s.version)
		if err != nil {
			return results, err
		}
		results[assets[i].BrowserDownloadURL] = *record
	}
	return results, nil
}

// Enqueue saves a new asset once. Completed or exhausted jobs stay unchanged until identity changes.
func (s *Store) Enqueue(ctx context.Context, asset github.Asset) (Record, error) {
	asset.Artifact = nil
	asset.ArchivePlatform = ""
	key := s.key(asset)
	r, err := s.mutate(ctx, key, func(existing *Record) (*Record, error) {
		if existing != nil {
			return nil, nil
		}
		now := s.now().UTC()
		return &Record{
			Key: key, InspectorVersion: s.version, Asset: asset,
			State: StateQueued, NextAttemptAt: now, UpdatedAt: now,
		}, nil
	})
	if err != nil {
		return Record{}, err
	}
	return *r, nil
}

// key excludes changing counters and caller enrichment, but includes every replacement signal.
func (s *Store) key(asset github.Asset) string {
	identity := struct {
		Version string
		URL     string
		ID      int64
		Name    string
		Size    int64
		Updated string
		Digest  string
	}{s.version, asset.BrowserDownloadURL, asset.ID, asset.Name, asset.Size, asset.UpdatedAt, asset.Digest}
	data, _ := json.Marshal(identity)
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// due keeps active leases in the queue so another worker can recover them after a crash.
func (r *Record) due() (time.Time, bool) {
	switch r.State {
	case StateQueued, StateRetry:
		return r.NextAttemptAt, true
	case StateRunning:
		return r.LeaseUntil, true
	default:
		return time.Time{}, false
	}
}

func (s *Store) read(ctx context.Context, key string) (*Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var data []byte
	var err error
	if s.client != nil {
		data, err = s.client.Get(ctx, s.prefix+"record:"+key).Bytes()
		if errors.Is(err, redis.Nil) {
			return nil, nil
		}
	} else {
		data, err = s.readLocal(key)
	}
	if err != nil || data == nil {
		return nil, err
	}
	return decodeRecord(data, key, s.version)
}

// decodeRecord detects corrupt or misplaced files before they can become download evidence.
func decodeRecord(data []byte, key, version string) (*Record, error) {
	var r Record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decode artifact scan record: %w", err)
	}
	if r.Key != key || r.InspectorVersion != version {
		return nil, errors.New("artifact scan record identity does not match its key")
	}
	return &r, nil
}

func (s *Store) candidates(ctx context.Context, now time.Time) ([]string, error) {
	if s.client != nil {
		return s.client.ZRangeByScore(ctx, s.prefix+"due", &redis.ZRangeBy{
			Min: "-inf", Max: fmt.Sprint(now.UnixMilli()), Count: 32,
		}).Result()
	}
	records, err := s.localCandidates(ctx, now)
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		a, _ := records[i].due()
		b, _ := records[j].due()
		if a.Equal(b) {
			return records[i].Key < records[j].Key
		}
		return a.Before(b)
	})
	keys := make([]string, 0, min(len(records), 32))
	for _, record := range records[:min(len(records), 32)] {
		keys = append(keys, record.Key)
	}
	return keys, nil
}

// mutate applies a change with one record lock or Redis WATCH transaction.
// Returning nil from change leaves the previous record untouched.
func (s *Store) mutate(ctx context.Context, key string, change func(*Record) (*Record, error)) (*Record, error) {
	if s.client == nil {
		return s.mutateLocal(ctx, key, change)
	}
	var result *Record
	for range 16 {
		err := s.client.Watch(ctx, func(tx *redis.Tx) error {
			data, err := tx.Get(ctx, s.prefix+"record:"+key).Bytes()
			var existing *Record
			if err == nil {
				existing, err = decodeRecord(data, key, s.version)
			}
			if err != nil && !errors.Is(err, redis.Nil) {
				return err
			}
			next, err := change(existing)
			if err != nil {
				return err
			}
			if next == nil {
				result = existing
				return nil
			}
			encoded, err := json.Marshal(next)
			if err != nil {
				return err
			}
			_, err = tx.TxPipelined(ctx, func(p redis.Pipeliner) error {
				p.Set(ctx, s.prefix+"record:"+key, encoded, 0)
				if due, ok := next.due(); ok {
					p.ZAdd(ctx, s.prefix+"due", redis.Z{Score: float64(due.UnixMilli()), Member: key})
				} else {
					p.ZRem(ctx, s.prefix+"due", key)
				}
				return nil
			})
			if err == nil {
				result = next
			}
			return err
		}, s.prefix+"record:"+key)
		if !errors.Is(err, redis.TxFailedErr) {
			return result, err
		}
	}
	return nil, errors.New("artifact scan record stayed busy after 16 transaction attempts")
}

// errorText omits request URLs because redirects may include signed download credentials.
func errorText(err error) string {
	var requestError *url.Error
	if errors.As(err, &requestError) {
		return requestError.Err.Error()
	}
	return err.Error()
}
