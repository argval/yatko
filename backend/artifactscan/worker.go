package artifactscan

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"github.com/argval/yatko/github"
)

var (
	ErrLeaseLost = errors.New("artifact scan lease was replaced by another worker")
	errNotDue    = errors.New("artifact scan job is no longer due")
)

// ProcessNext claims one due asset and persists its result or retry state.
// Callback errors describe operational failures unless they wrap github.ErrMalformedArtifact.
func (s *Store) ProcessNext(ctx context.Context, inspect func(context.Context, github.Asset) (github.ArtifactMetadata, error)) (bool, error) {
	if inspect == nil {
		return false, errors.New("artifact scan requires an inspector")
	}
	job, err := s.claim(ctx)
	if err != nil || job == nil {
		return false, err
	}
	if job.State == StateError {
		return true, nil
	}
	attemptCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
	metadata, inspectErr := inspect(attemptCtx, job.Asset)
	cancel()
	// Release a canceled worker's lease even though its run context has ended.
	saveCtx, saveCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer saveCancel()
	err = s.finish(saveCtx, job, metadata, inspectErr)
	return true, err
}

// Run processes assets serially until canceled; storage errors stop the worker for its caller.
func (s *Store) Run(ctx context.Context, inspect func(context.Context, github.Asset) (github.ArtifactMetadata, error)) error {
	for {
		worked, err := s.ProcessNext(ctx, inspect)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil && !errors.Is(err, ErrLeaseLost) {
			return err
		}
		if worked {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

func (s *Store) claim(ctx context.Context) (*Record, error) {
	keys, err := s.candidates(ctx, s.now())
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		record, err := s.mutate(ctx, key, func(existing *Record) (*Record, error) {
			if existing == nil {
				return nil, errNotDue
			}
			now := s.now().UTC()
			if due, ok := existing.due(); !ok || due.After(now) {
				return nil, errNotDue
			}
			existing.UpdatedAt = now
			existing.Metadata = nil
			if existing.Attempts >= maxAttempts {
				existing.State = StateError
				existing.Reason = "inspection worker lease expired after the final attempt"
				existing.LeaseUntil = time.Time{}
				existing.LeaseToken = ""
				existing.NextAttemptAt = time.Time{}
				return existing, nil
			}
			existing.State = StateRunning
			existing.Attempts++
			existing.LeaseToken = rand.Text()
			existing.LeaseUntil = now.Add(leaseDuration)
			existing.NextAttemptAt = time.Time{}
			return existing, nil
		})
		if errors.Is(err, errNotDue) {
			continue
		}
		return record, err
	}
	return nil, nil
}

func (s *Store) finish(ctx context.Context, job *Record, metadata github.ArtifactMetadata, inspectErr error) error {
	_, err := s.mutate(ctx, job.Key, func(existing *Record) (*Record, error) {
		if existing == nil || existing.State != StateRunning || existing.LeaseToken != job.LeaseToken {
			return nil, ErrLeaseLost
		}
		now := s.now().UTC()
		existing.UpdatedAt = now
		existing.LeaseUntil = time.Time{}
		existing.LeaseToken = ""
		if inspectErr != nil && !errors.Is(inspectErr, github.ErrMalformedArtifact) {
			existing.Metadata = nil
			existing.LastError = errorText(inspectErr)
			if existing.Attempts >= maxAttempts {
				existing.State = StateError
				existing.Reason = "inspection could not finish after three attempts: " + existing.LastError
				existing.NextAttemptAt = time.Time{}
			} else {
				existing.State = StateRetry
				existing.NextAttemptAt = now.Add(retryDelay * time.Duration(1<<(existing.Attempts-1)))
			}
			return existing, nil
		}
		if inspectErr != nil {
			metadata.Status = "failed"
			existing.Reason = errorText(inspectErr)
		} else {
			existing.Reason = metadataReason(metadata)
		}
		if metadata.Status == "" {
			metadata.Status = "unknown"
		}
		if metadata.Role == "" {
			metadata.Role = "unknown"
		}
		if existing.Reason != "" {
			metadata.Evidence = append(metadata.Evidence, github.ArtifactEvidence{
				Source: "deep-inspection", Field: "inspection.reason", Value: existing.Reason,
			})
		}
		existing.State = StateComplete
		existing.Metadata = &metadata
		existing.LastError = ""
		existing.NextAttemptAt = time.Time{}
		return existing, nil
	})
	return err
}

// metadataReason preserves the inspector's specific cause when a file cannot be classified.
func metadataReason(metadata github.ArtifactMetadata) string {
	if metadata.Status == "inspected" && len(metadata.Targets) > 0 {
		return ""
	}
	for _, evidence := range metadata.Evidence {
		field := strings.ToLower(evidence.Field)
		if evidence.Value != "" && (strings.Contains(field, "reason") || strings.Contains(field, "skipped") || strings.Contains(field, "error") || strings.Contains(field, "limit")) {
			return evidence.Value
		}
	}
	switch metadata.Status {
	case "unsupported":
		return "inspector cannot read this file's payload format"
	case "failed":
		return "inspector reported malformed file contents without a more specific cause"
	default:
		return "inspection found no conclusive application platform or CPU evidence"
	}
}
