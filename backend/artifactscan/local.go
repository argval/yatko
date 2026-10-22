package artifactscan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// NewLocal saves records under a private, versioned directory with cross-process file locking.
func NewLocal(dir, inspectorVersion string) (*Store, error) {
	if strings.TrimSpace(dir) == "" || strings.TrimSpace(inspectorVersion) == "" {
		return nil, errors.New("artifact scan requires a directory and inspector version")
	}
	versionHash := sha256.Sum256([]byte(inspectorVersion))
	dir = filepath.Join(dir, "v1", hex.EncodeToString(versionHash[:]))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create artifact scan directory: %w", err)
	}
	return &Store{version: inspectorVersion, dir: dir, now: time.Now}, nil
}

func (s *Store) readLocal(key string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(s.dir, key+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

func (s *Store) localCandidates(ctx context.Context, now time.Time) ([]*Record, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var records []*Record
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		record, err := s.read(ctx, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		if record != nil {
			if due, ok := record.due(); ok && !due.After(now) {
				records = append(records, record)
			}
		}
	}
	return records, nil
}

func (s *Store) mutateLocal(ctx context.Context, key string, change func(*Record) (*Record, error)) (*Record, error) {
	lock, err := os.OpenFile(filepath.Join(s.dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	existing, err := s.read(ctx, key)
	if err != nil {
		return nil, err
	}
	next, err := change(existing)
	if err != nil {
		return nil, err
	}
	if next == nil {
		return existing, nil
	}
	if err := s.writeLocal(next); err != nil {
		return nil, err
	}
	return next, nil
}

// writeLocal makes the new record durable before replacing its previous complete file.
func (s *Store) writeLocal(record *Record) error {
	f, err := os.CreateTemp(s.dir, ".record-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := json.NewEncoder(f).Encode(record); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(s.dir, record.Key+".json")); err != nil {
		return err
	}
	dir, err := os.Open(s.dir)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
