package artifactscan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/argval/yatko/github"
	"github.com/redis/go-redis/v9"
)

type testStore struct {
	first, second *Store
	open          func(string) *Store
	clock         atomic.Int64
	redis         *miniredis.Miniredis
}

// stores exercises the same persisted behavior against real local files and isolated Redis.
func stores(t *testing.T, test func(*testing.T, *testStore)) {
	t.Helper()
	for _, backend := range []string{"local", "redis"} {
		t.Run(backend, func(t *testing.T) {
			env := &testStore{}
			env.clock.Store(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC).UnixNano())
			var constructor func(string) (*Store, error)
			if backend == "local" {
				dir := t.TempDir()
				constructor = func(version string) (*Store, error) { return NewLocal(dir, version) }
			} else {
				env.redis = miniredis.RunT(t)
				constructor = func(version string) (*Store, error) {
					client := redis.NewClient(&redis.Options{Addr: env.redis.Addr()})
					t.Cleanup(func() { _ = client.Close() })
					return NewRedis(client, version)
				}
			}
			env.open = func(version string) *Store {
				store, err := constructor(version)
				if err != nil {
					t.Fatal(err)
				}
				store.now = func() time.Time { return time.Unix(0, env.clock.Load()).UTC() }
				return store
			}
			env.first, env.second = env.open("inspector-1"), env.open("inspector-1")
			test(t, env)
		})
	}
}

func releaseAsset() github.Asset {
	return github.Asset{
		ID: 123, Name: "app.dmg", Size: 4096, UpdatedAt: "2026-10-17T09:00:00Z",
		BrowserDownloadURL: "https://github.com/example/app/releases/download/v1/app.dmg",
		Digest:             "sha256:abcd", State: "uploaded",
	}
}

func inspectedMetadata() github.ArtifactMetadata {
	return github.ArtifactMetadata{
		Status: "inspected", Role: "application",
		Targets: []github.ArtifactTarget{{Platform: "darwin", Arch: "arm64"}},
		Evidence: []github.ArtifactEvidence{
			{Source: "deep-inspection", Field: "file.sha256", Value: "abcdef"},
			{Source: "deep-inspection", Field: "extractor.tool", Value: "7zz"},
			{Source: "deep-inspection", Field: "binary.entry", Value: "App.app/Contents/MacOS/App"},
		},
	}
}

func completedInspector(context.Context, github.Asset) (github.ArtifactMetadata, error) {
	return inspectedMetadata(), nil
}

func lookup(t *testing.T, store *Store, asset github.Asset) Record {
	t.Helper()
	r, ok, err := store.Lookup(context.Background(), asset)
	if err != nil || !ok {
		t.Fatalf("Lookup: found=%v, err=%v", ok, err)
	}
	return r
}

func TestCompletedEvidenceSurvivesRestartAndGenericCacheExpiry(t *testing.T) {
	stores(t, func(t *testing.T, env *testStore) {
		ctx := context.Background()
		asset := releaseAsset()
		asset.Artifact = &github.ArtifactMetadata{Status: "inspected", Targets: []github.ArtifactTarget{{Platform: "wrong"}}}
		asset.ArchivePlatform = "wrong"
		if _, found, err := env.first.Lookup(ctx, asset); err != nil || found {
			t.Fatalf("empty Lookup: found=%v err=%v", found, err)
		}
		if worked, err := env.first.ProcessNext(ctx, completedInspector); worked || err != nil {
			t.Fatalf("Lookup created work: worked=%v err=%v", worked, err)
		}
		if _, err := env.first.Enqueue(ctx, asset); err != nil {
			t.Fatal(err)
		}
		worked, err := env.first.ProcessNext(ctx, func(_ context.Context, got github.Asset) (github.ArtifactMetadata, error) {
			if got.Artifact != nil || got.ArchivePlatform != "" {
				t.Error("caller enrichment reached full-file inspector")
			}
			return inspectedMetadata(), nil
		})
		if !worked || err != nil {
			t.Fatalf("ProcessNext: worked=%v err=%v", worked, err)
		}
		env.clock.Add(int64(48 * time.Hour))
		if env.redis != nil {
			env.redis.FastForward(48 * time.Hour)
		}
		restarted := env.open("inspector-1")
		r := lookup(t, restarted, asset)
		if !r.Ready() || !reflect.DeepEqual(*r.Metadata, inspectedMetadata()) || r.Attempts != 1 {
			t.Fatalf("completed evidence not retained: %+v", r)
		}
		if r.Asset.Artifact != nil || r.Asset.ArchivePlatform != "" {
			t.Fatal("caller enrichment persisted")
		}
		if worked, err := restarted.ProcessNext(ctx, completedInspector); worked || err != nil {
			t.Fatalf("completed job ran again: worked=%v err=%v", worked, err)
		}
	})
}

func TestAssetReplacementAndInspectorVersionInvalidateEvidence(t *testing.T) {
	stores(t, func(t *testing.T, env *testStore) {
		ctx := context.Background()
		asset := releaseAsset()
		original, err := env.first.Enqueue(ctx, asset)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := env.first.ProcessNext(ctx, completedInspector); err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct {
			name   string
			change func(*github.Asset)
		}{
			{"URL", func(a *github.Asset) { a.BrowserDownloadURL += "?new=1" }},
			{"ID", func(a *github.Asset) { a.ID++ }},
			{"name", func(a *github.Asset) { a.Name = "other.dmg" }},
			{"size", func(a *github.Asset) { a.Size++ }},
			{"updated", func(a *github.Asset) { a.UpdatedAt = "2026-10-18T09:00:00Z" }},
			{"digest", func(a *github.Asset) { a.Digest = "sha256:other" }},
		} {
			t.Run(test.name, func(t *testing.T) {
				changed := asset
				test.change(&changed)
				if _, found, err := env.second.Lookup(ctx, changed); found || err != nil {
					t.Fatalf("replacement reused evidence: found=%v err=%v", found, err)
				}
				r, err := env.second.Enqueue(ctx, changed)
				if err != nil || r.State != StateQueued || r.Key == original.Key {
					t.Fatalf("replacement not queued separately: %+v err=%v", r, err)
				}
			})
		}
		asset.DownloadCount++
		asset.Label = "Different display label"
		if !lookup(t, env.second, asset).Ready() {
			t.Fatal("changing counters or labels invalidated completed evidence")
		}
		nextVersion := env.open("inspector-2")
		if _, found, err := nextVersion.Lookup(ctx, asset); found || err != nil {
			t.Fatalf("new inspector reused old evidence: found=%v err=%v", found, err)
		}
		if r, err := nextVersion.Enqueue(ctx, asset); err != nil || r.Key == original.Key || r.State != StateQueued {
			t.Fatalf("new inspector not queued: %+v err=%v", r, err)
		}
	})
}

func TestDuplicateEnqueueAndConcurrentWorkers(t *testing.T) {
	stores(t, func(t *testing.T, env *testStore) {
		ctx := context.Background()
		asset := releaseAsset()
		var group sync.WaitGroup
		failures := make(chan error, 16)
		for i := range 16 {
			group.Add(1)
			go func() {
				defer group.Done()
				store := env.first
				if i%2 != 0 {
					store = env.second
				}
				_, err := store.Enqueue(ctx, asset)
				failures <- err
			}()
		}
		group.Wait()
		close(failures)
		for err := range failures {
			if err != nil {
				t.Fatal(err)
			}
		}
		started, release := make(chan struct{}), make(chan struct{})
		result := make(chan error, 1)
		go func() {
			worked, err := env.first.ProcessNext(ctx, func(context.Context, github.Asset) (github.ArtifactMetadata, error) {
				close(started)
				<-release
				return inspectedMetadata(), nil
			})
			if !worked && err == nil {
				err = errors.New("enqueued work was not claimed")
			}
			result <- err
		}()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("inspector did not start")
		}
		worked, err := env.second.ProcessNext(ctx, func(context.Context, github.Asset) (github.ArtifactMetadata, error) {
			return github.ArtifactMetadata{}, errors.New("second worker claimed an active lease")
		})
		close(release)
		if worked || err != nil {
			t.Errorf("duplicate job ran: worked=%v err=%v", worked, err)
		}
		if err := <-result; err != nil {
			t.Fatal(err)
		}
		if got := lookup(t, env.second, asset); !got.Ready() || got.Attempts != 1 {
			t.Fatalf("duplicate enqueue changed attempts: %+v", got)
		}
	})
}

func TestExpiredLeaseRecoversAndRejectsStaleWorker(t *testing.T) {
	stores(t, func(t *testing.T, env *testStore) {
		ctx := context.Background()
		asset := releaseAsset()
		if _, err := env.first.Enqueue(ctx, asset); err != nil {
			t.Fatal(err)
		}
		abandoned, err := env.first.claim(ctx)
		if err != nil || abandoned == nil || abandoned.State != StateRunning {
			t.Fatalf("claim: %+v err=%v", abandoned, err)
		}
		if work, err := env.second.claim(ctx); err != nil || work != nil {
			t.Fatalf("active lease stolen: %+v err=%v", work, err)
		}
		env.clock.Add(int64(leaseDuration + time.Second))
		restarted := env.open("inspector-1")
		recovered, err := restarted.claim(ctx)
		if err != nil || recovered == nil || recovered.Attempts != 2 || recovered.LeaseToken == abandoned.LeaseToken {
			t.Fatalf("expired lease not recovered: %+v err=%v", recovered, err)
		}
		if err := env.first.finish(ctx, abandoned, inspectedMetadata(), nil); !errors.Is(err, ErrLeaseLost) {
			t.Fatalf("stale worker replaced current result: %v", err)
		}
		if err := restarted.finish(ctx, recovered, inspectedMetadata(), nil); err != nil {
			t.Fatal(err)
		}
		if !lookup(t, env.second, asset).Ready() {
			t.Fatal("recovered result not shared")
		}
	})
}

func TestTransportFailureRetriesWithoutArtifactProof(t *testing.T) {
	stores(t, func(t *testing.T, env *testStore) {
		ctx := context.Background()
		asset := releaseAsset()
		if _, err := env.first.Enqueue(ctx, asset); err != nil {
			t.Fatal(err)
		}
		inspect := func(context.Context, github.Asset) (github.ArtifactMetadata, error) {
			return github.ArtifactMetadata{Status: "failed", Targets: []github.ArtifactTarget{{Platform: "wrong"}}},
				&url.Error{Op: "Get", URL: "https://example.invalid/a?token=private", Err: io.ErrUnexpectedEOF}
		}
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			worked, err := env.first.ProcessNext(ctx, inspect)
			if !worked || err != nil {
				t.Fatalf("attempt %d: worked=%v err=%v", attempt, worked, err)
			}
			r := lookup(t, env.second, asset)
			if r.Ready() || r.Metadata != nil || r.Attempts != attempt || r.LastError != io.ErrUnexpectedEOF.Error() {
				t.Fatalf("transport failure became artifact evidence: %+v", r)
			}
			if strings.Contains(r.LastError, "private") {
				t.Fatal("signed request credentials persisted")
			}
			if attempt < maxAttempts {
				if r.State != StateRetry || !r.NextAttemptAt.Equal(env.first.now().Add(retryDelay*time.Duration(1<<(attempt-1)))) {
					t.Fatalf("retry backoff missing: %+v", r)
				}
				if worked, err := env.second.ProcessNext(ctx, inspect); worked || err != nil {
					t.Fatalf("retry ignored backoff: worked=%v err=%v", worked, err)
				}
				env.clock.Store(r.NextAttemptAt.Add(time.Second).UnixNano())
			} else if r.State != StateError || r.Reason == "" {
				t.Fatalf("retry did not terminate with operational reason: %+v", r)
			}
		}
		if _, err := env.second.Enqueue(ctx, asset); err != nil {
			t.Fatal(err)
		}
		if worked, err := env.second.ProcessNext(ctx, inspect); worked || err != nil {
			t.Fatalf("bounded retry restarted on enqueue: worked=%v err=%v", worked, err)
		}
	})
}

func TestUnknownUnsupportedAndMalformedResultsAreRetained(t *testing.T) {
	for _, test := range []struct {
		name     string
		metadata github.ArtifactMetadata
		err      error
		reason   string
	}{
		{"unsupported", github.ArtifactMetadata{Status: "unsupported", Evidence: []github.ArtifactEvidence{{Source: "deep-inspection", Field: "inspection.skipped", Value: "unsupported Inno payload version 7.1"}}}, nil, "unsupported Inno payload version 7.1"},
		{"unknown", github.ArtifactMetadata{Status: "unknown"}, nil, "no conclusive"},
		{"no-targets", github.ArtifactMetadata{Status: "inspected"}, nil, "no conclusive"},
		{"malformed", github.ArtifactMetadata{}, fmt.Errorf("%w: invalid archive header", github.ErrMalformedArtifact), "invalid archive header"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stores(t, func(t *testing.T, env *testStore) {
				ctx, asset := context.Background(), releaseAsset()
				if _, err := env.first.Enqueue(ctx, asset); err != nil {
					t.Fatal(err)
				}
				worked, err := env.first.ProcessNext(ctx, func(context.Context, github.Asset) (github.ArtifactMetadata, error) {
					return test.metadata, test.err
				})
				if !worked || err != nil {
					t.Fatalf("ProcessNext: worked=%v err=%v", worked, err)
				}
				r := lookup(t, env.second, asset)
				if !r.Ready() || !strings.Contains(r.Reason, test.reason) {
					t.Fatalf("terminal reason lost: %+v", r)
				}
				foundReason := false
				for _, evidence := range r.Metadata.Evidence {
					foundReason = foundReason || evidence.Source == "deep-inspection" && evidence.Field == "inspection.reason" && strings.Contains(evidence.Value, test.reason)
				}
				if !foundReason {
					t.Fatalf("metadata missing reason evidence: %+v", r.Metadata)
				}
				if worked, err := env.second.ProcessNext(ctx, completedInspector); worked || err != nil {
					t.Fatalf("terminal unknown retried: worked=%v err=%v", worked, err)
				}
			})
		})
	}
}

func TestCancellationSavesRetryWithoutWaitingForLease(t *testing.T) {
	stores(t, func(t *testing.T, env *testStore) {
		ctx, cancel := context.WithCancel(context.Background())
		asset := releaseAsset()
		if _, err := env.first.Enqueue(ctx, asset); err != nil {
			t.Fatal(err)
		}
		worked, err := env.first.ProcessNext(ctx, func(context.Context, github.Asset) (github.ArtifactMetadata, error) {
			cancel()
			return github.ArtifactMetadata{}, context.Canceled
		})
		if !worked || err != nil {
			t.Fatalf("canceled result not saved: worked=%v err=%v", worked, err)
		}
		if r := lookup(t, env.second, asset); r.State != StateRetry || r.LeaseToken != "" || !r.LeaseUntil.IsZero() || r.Metadata != nil {
			t.Fatalf("canceled worker kept lease or artifact proof: %+v", r)
		}
		if err := env.first.Run(ctx, completedInspector); err != nil {
			t.Fatalf("canceled Run: %v", err)
		}
	})
}

func TestRepeatedWorkerCrashesExhaustLeaseAttempts(t *testing.T) {
	stores(t, func(t *testing.T, env *testStore) {
		ctx, asset := context.Background(), releaseAsset()
		if _, err := env.first.Enqueue(ctx, asset); err != nil {
			t.Fatal(err)
		}
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			job, err := env.open("inspector-1").claim(ctx)
			if err != nil || job == nil || job.State != StateRunning || job.Attempts != attempt {
				t.Fatalf("claim %d: %+v err=%v", attempt, job, err)
			}
			env.clock.Add(int64(leaseDuration + time.Second))
		}
		worked, err := env.second.ProcessNext(ctx, func(context.Context, github.Asset) (github.ArtifactMetadata, error) {
			t.Error("exhausted crash attempts reached inspector")
			return inspectedMetadata(), nil
		})
		if !worked || err != nil {
			t.Fatalf("exhaustion not saved: worked=%v err=%v", worked, err)
		}
		if r := lookup(t, env.second, asset); r.State != StateError || r.Ready() || r.Metadata != nil || !strings.Contains(r.Reason, "lease expired") {
			t.Fatalf("crashes became proof or stayed queued: %+v", r)
		}
	})
}
