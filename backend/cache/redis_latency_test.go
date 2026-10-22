package cache

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/alicebob/miniredis/v2/server"
)

// A stalled SET on an established TLS connection must honor the total budget, including retries.
func TestStoreCachedRedisDeadlineKeepsLocalValue(t *testing.T) {
	certServer := httptest.NewTLSServer(http.NotFoundHandler())
	defer certServer.Close()
	mr := miniredis.NewMiniRedis()
	if err := mr.StartTLS(certServer.TLS.Clone()); err != nil {
		t.Fatal(err)
	}
	defer mr.Close()
	t.Setenv("REDIS_URL", "rediss://"+mr.Addr()+"?read_timeout=1s&write_timeout=1s&max_retries=10&min_retry_backoff=100ms&max_retry_backoff=100ms")
	t.Setenv("KV_URL", "")
	t.Setenv("UPSTASH_REDIS_URL", "")
	c := New()
	defer c.client.Close()
	roots := x509.NewCertPool()
	roots.AddCert(certServer.Certificate())
	c.client.Options().TLSConfig.RootCAs = roots
	if err := c.client.Ping(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	blocked, release := make(chan struct{}), make(chan struct{})
	defer close(release)
	mr.Server().SetPreHook(func(_ *server.Peer, cmd string, _ ...string) bool {
		if strings.EqualFold(cmd, "set") {
			select {
			case blocked <- struct{}{}:
			case <-release:
				return true
			}
			<-release
			return true
		}
		return false
	})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	begin := time.Now()
	done := make(chan error, 1)
	go func() { done <- StoreCached(ctx, c, "offer", "retained") }()
	select {
	case <-blocked:
	case <-time.After(time.Second):
		t.Fatal("SET did not reach the local Redis fixture")
	}
	if value, ok := GetCached[string](context.Background(), c, "offer"); !ok || value != "retained" {
		t.Fatalf("local value missing while SET is blocked: %q %v", value, ok)
	}
	if err := <-done; err == nil {
		t.Error("stalled SET unexpectedly succeeded")
	}
	if elapsed := time.Since(begin); elapsed > 500*time.Millisecond {
		t.Errorf("Redis exceeded caller deadline: %s", elapsed)
	}
	if value, ok := GetCached[string](context.Background(), c, "offer"); !ok || value != "retained" {
		t.Fatalf("local value lost after timeout: %q %v", value, ok)
	}
}
