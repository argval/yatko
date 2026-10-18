package artifactscan

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/argval/yatko/github"
)

func TestOpenFromEnvRequiresExplicitDedicatedConfiguration(t *testing.T) {
	t.Setenv("ARTIFACT_SCAN_DIR", "")
	t.Setenv("ARTIFACT_SCAN_REDIS_URL", "")
	t.Setenv("REDIS_URL", "redis://unused.invalid:6379")
	t.Setenv("KV_URL", "redis://unused.invalid:6379")
	t.Setenv("UPSTASH_REDIS_URL", "redis://unused.invalid:6379")
	store, close, err := OpenFromEnv("inspector-1")
	if err != nil || store != nil {
		t.Fatalf("generic Redis enabled artifact scans: store=%v err=%v", store, err)
	}
	if err := close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARTIFACT_SCAN_DIR", t.TempDir())
	store, close, err = OpenFromEnv("inspector-1")
	if err != nil || store == nil || store.client != nil {
		t.Fatalf("local configuration: store=%v err=%v", store, err)
	}
	if err := close(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ARTIFACT_SCAN_REDIS_URL", "redis://unused.invalid:6379")
	if _, _, err := OpenFromEnv("inspector-1"); err == nil {
		t.Fatal("ambiguous local and Redis configuration accepted")
	}
	t.Setenv("ARTIFACT_SCAN_DIR", "")
	t.Setenv("ARTIFACT_SCAN_REDIS_URL", "not-a-redis-url")
	if _, _, err := OpenFromEnv("inspector-1"); err == nil {
		t.Fatal("invalid explicit Redis URL silently disabled scans")
	}
}

func TestOpenFromEnvRedisIsLazyAndHasBoundedTimeouts(t *testing.T) {
	server := miniredis.RunT(t)
	t.Setenv("ARTIFACT_SCAN_DIR", "")
	t.Setenv("ARTIFACT_SCAN_REDIS_URL", "redis://"+server.Addr())
	store, close, err := OpenFromEnv("inspector-1")
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	if server.CommandCount() != 0 {
		t.Fatal("configuration contacted Redis before a store operation")
	}
	options := store.client.Options()
	if options.DialTimeout > 300*time.Millisecond || options.ReadTimeout > 300*time.Millisecond ||
		options.WriteTimeout > 300*time.Millisecond || !options.ContextTimeoutEnabled || options.MaxRetries != 0 {
		t.Fatalf("unbounded Redis configuration: %+v", options)
	}
	if _, found, err := store.Lookup(context.Background(), releaseAsset()); err != nil || found {
		t.Fatalf("lazy Redis configuration unusable: found=%v err=%v", found, err)
	}
}

func TestLookupManyReturnsOnlyMatchingAssetIdentities(t *testing.T) {
	stores(t, func(t *testing.T, env *testStore) {
		ctx := context.Background()
		asset := releaseAsset()
		if _, err := env.first.Enqueue(ctx, asset); err != nil {
			t.Fatal(err)
		}
		if _, err := env.first.ProcessNext(ctx, completedInspector); err != nil {
			t.Fatal(err)
		}
		missing := asset
		missing.BrowserDownloadURL += "?new=1"
		records, err := env.second.LookupMany(ctx, []github.Asset{asset, missing})
		if err != nil || len(records) != 1 || !records[asset.BrowserDownloadURL].Ready() {
			t.Fatalf("batch evidence: %+v err=%v", records, err)
		}
		replaced := asset
		replaced.ID++
		records, err = env.second.LookupMany(ctx, []github.Asset{replaced})
		if err != nil || len(records) != 0 {
			t.Fatalf("batch reused replaced evidence: %+v err=%v", records, err)
		}
	})
}
