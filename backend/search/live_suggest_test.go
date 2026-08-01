//go:build live

package search

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
)

// Run with: LIVE_SEARCH=1 go test ./search -tags live -run TestLiveSuggest -v -count=1
func TestLiveSuggest(t *testing.T) {
	if os.Getenv("LIVE_SEARCH") != "1" {
		t.Skip("set LIVE_SEARCH=1 to hit GitHub")
	}

	gh := github.NewClient()
	c := cache.New()
	ac := NewAutocomplete(gh, c)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	type check struct {
		name string
		raw  string // before NormalizeQuery
		want func(t *testing.T, items []github.SearchRepo)
	}

	containsSlug := func(items []github.SearchRepo, owner, repo string) bool {
		for _, it := range items {
			if strings.EqualFold(it.Owner, owner) && strings.EqualFold(it.Repo, repo) {
				return true
			}
		}
		return false
	}
	allOwnedBy := func(items []github.SearchRepo, owner string) bool {
		if len(items) == 0 {
			return false
		}
		for _, it := range items {
			if !strings.EqualFold(it.Owner, owner) {
				return false
			}
		}
		return true
	}
	firstOwnedBy := func(items []github.SearchRepo, owner string) bool {
		return len(items) > 0 && strings.EqualFold(items[0].Owner, owner)
	}
	hasExactRepoName := func(items []github.SearchRepo, name string) bool {
		for _, it := range items {
			if strings.EqualFold(it.Repo, name) {
				return true
			}
		}
		return false
	}

	cases := []check{
		{
			name: "bare_owner_oven-sh",
			raw:  "oven-sh",
			want: func(t *testing.T, items []github.SearchRepo) {
				if !firstOwnedBy(items, "oven-sh") {
					t.Fatalf("expected oven-sh repos first, got %+v", summarize(items))
				}
				if !containsSlug(items, "oven-sh", "bun") {
					t.Fatalf("expected oven-sh/bun among hits, got %+v", summarize(items))
				}
			},
		},
		{
			name: "owner_url_same_as_bare",
			raw:  "https://github.com/oven-sh",
			want: func(t *testing.T, items []github.SearchRepo) {
				if !containsSlug(items, "oven-sh", "bun") {
					t.Fatalf("owner URL should surface bun, got %+v", summarize(items))
				}
			},
		},
		{
			name: "bare_repo_react-native",
			raw:  "react-native",
			want: func(t *testing.T, items []github.SearchRepo) {
				if !hasExactRepoName(items, "react-native") {
					t.Fatalf("expected a react-native repo, got %+v", summarize(items))
				}
				// GitHub moved facebook/react-native → react/react-native.
				if !containsSlug(items, "react", "react-native") && !containsSlug(items, "facebook", "react-native") {
					t.Fatalf("expected react/react-native (or facebook/), got %+v", summarize(items))
				}
				if len(items) == 0 || !strings.EqualFold(items[0].Repo, "react-native") {
					t.Fatalf("exact name should rank first, got %+v", summarize(items))
				}
			},
		},
		{
			name: "bare_repo_uv",
			raw:  "uv",
			want: func(t *testing.T, items []github.SearchRepo) {
				if !containsSlug(items, "astral-sh", "uv") {
					t.Fatalf("expected astral-sh/uv, got %+v", summarize(items))
				}
				if len(items) == 0 || !strings.EqualFold(items[0].Owner, "astral-sh") || !strings.EqualFold(items[0].Repo, "uv") {
					t.Fatalf("astral-sh/uv should rank first, got %+v", summarize(items))
				}
			},
		},
		{
			name: "slug_astral-sh_uv",
			raw:  "astral-sh/uv",
			want: func(t *testing.T, items []github.SearchRepo) {
				if len(items) == 0 || !strings.EqualFold(items[0].Owner, "astral-sh") || !strings.EqualFold(items[0].Repo, "uv") {
					t.Fatalf("slug should put exact first, got %+v", summarize(items))
				}
				if !allOwnedBy(items, "astral-sh") {
					t.Fatalf("slug results should stay under astral-sh, got %+v", summarize(items))
				}
			},
		},
		{
			name: "slug_dashed_repo",
			raw:  "facebook/react-native",
			want: func(t *testing.T, items []github.SearchRepo) {
				if len(items) == 0 {
					t.Fatal("expected hits")
				}
				// Search may return react/react-native; GetRepo ensure may add facebook/.
				if !containsSlug(items, "react", "react-native") && !containsSlug(items, "facebook", "react-native") {
					t.Fatalf("expected react-native under react or facebook, got %+v", summarize(items))
				}
			},
		},
		{
			name: "repo_url",
			raw:  "https://github.com/cli/cli",
			want: func(t *testing.T, items []github.SearchRepo) {
				if !containsSlug(items, "cli", "cli") {
					t.Fatalf("expected cli/cli, got %+v", summarize(items))
				}
			},
		},
		{
			name: "slug_dashed_owner",
			raw:  "oven-sh/bun",
			want: func(t *testing.T, items []github.SearchRepo) {
				if len(items) == 0 || !strings.EqualFold(items[0].Owner, "oven-sh") || !strings.EqualFold(items[0].Repo, "bun") {
					t.Fatalf("expected oven-sh/bun first, got %+v", summarize(items))
				}
			},
		},
		{
			name: "bare_bun",
			raw:  "bun",
			want: func(t *testing.T, items []github.SearchRepo) {
				if !containsSlug(items, "oven-sh", "bun") {
					t.Fatalf("expected oven-sh/bun for bare bun, got %+v", summarize(items))
				}
				if len(items) == 0 || !strings.EqualFold(items[0].Owner, "oven-sh") || !strings.EqualFold(items[0].Repo, "bun") {
					t.Fatalf("oven-sh/bun should rank first over user Bun noise, got %+v", summarize(items))
				}
			},
		},
		{
			name: "legacy_owner_sentinel",
			raw:  "owner:oven-sh",
			want: func(t *testing.T, items []github.SearchRepo) {
				if !containsSlug(items, "oven-sh", "bun") {
					t.Fatalf("legacy sentinel should normalize to bare owner, got %+v", summarize(items))
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q := NormalizeQuery(tc.raw)
			t.Logf("raw=%q normalized=%q queries=%v", tc.raw, q, queryDebug(q))
			items, err := ac.Suggest(ctx, q)
			if err != nil {
				t.Fatalf("Suggest(%q): %v", q, err)
			}
			t.Logf("hits=%v", summarize(items))
			tc.want(t, items)
		})
	}
}

func queryDebug(q string) []string {
	kind, owner, repo := Classify(q)
	if kind == KindSlug {
		return []string{SlugAPIQuery(owner, repo)}
	}
	return BareAPIQueries(q)
}

func summarize(items []github.SearchRepo) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Owner+"/"+it.Repo)
	}
	return out
}
