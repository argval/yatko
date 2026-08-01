package search

import (
	"testing"

	"github.com/argval/yatko/github"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		q     string
		kind  Kind
		owner string
		repo  string
	}{
		{"bun", KindBare, "", ""},
		{"uv", KindBare, "", ""},
		{"react-native", KindBare, "", ""},
		{"oven-sh", KindBare, "", ""},
		{"setup-uv", KindBare, "", ""},
		{"hello world", KindBare, "", ""},
		{"oven-sh/bun", KindSlug, "oven-sh", "bun"},
		{"facebook/react-native", KindSlug, "facebook", "react-native"},
		{"astral-sh/uv", KindSlug, "astral-sh", "uv"},
		{"cli/cli", KindSlug, "cli", "cli"},
	}
	for _, tc := range cases {
		kind, owner, repo := Classify(tc.q)
		if kind != tc.kind || owner != tc.owner || repo != tc.repo {
			t.Fatalf("Classify(%q) = (%v,%q,%q), want (%v,%q,%q)",
				tc.q, kind, owner, repo, tc.kind, tc.owner, tc.repo)
		}
	}
}

func TestBareAPIQueries(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"bun", []string{"user:bun archived:false", "bun in:name archived:false"}},
		{"uv", []string{"user:uv archived:false", "uv in:name archived:false"}},
		{"react-native", []string{`user:react-native archived:false`, `"react-native" in:name archived:false`}},
		{"oven-sh", []string{`user:oven-sh archived:false`, `"oven-sh" in:name archived:false`}},
		{"setup-uv", []string{`user:setup-uv archived:false`, `"setup-uv" in:name archived:false`}},
		{"hello world", []string{`"hello world" in:name archived:false`}},
		{"user:cli", []string{"user:cli archived:false"}},
		{"", nil},
	}
	for _, tc := range cases {
		got := BareAPIQueries(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("BareAPIQueries(%q) len=%d want %d (%v vs %v)", tc.in, len(got), len(tc.want), got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("BareAPIQueries(%q)[%d] = %q, want %q", tc.in, i, got[i], tc.want[i])
			}
		}
	}
}

func TestSlugAPIQuery(t *testing.T) {
	cases := []struct {
		owner, repo, want string
	}{
		{"oven-sh", "bun", "user:oven-sh in:name bun archived:false"},
		{"facebook", "react-native", `user:facebook in:name "react-native" archived:false`},
		{"astral-sh", "setup-uv", `user:astral-sh in:name "setup-uv" archived:false`},
		{"astral-sh", "uv", "user:astral-sh in:name uv archived:false"},
		{"cli", "cli", "user:cli in:name cli archived:false"},
	}
	for _, tc := range cases {
		if got := SlugAPIQuery(tc.owner, tc.repo); got != tc.want {
			t.Fatalf("SlugAPIQuery(%q,%q) = %q, want %q", tc.owner, tc.repo, got, tc.want)
		}
	}
}

func TestNormalizeQuery(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"  Clip ", "clip"},
		{"https://github.com/astral-sh/uv", "astral-sh/uv"},
		{"https://github.com/facebook/react-native", "facebook/react-native"},
		{"github.com/astral-sh/uv", "astral-sh/uv"},
		{"https://github.com/astral-sh/uv.git", "astral-sh/uv"},
		{"astral-sh/uv", "astral-sh/uv"},
		{"astral-sh/uv/", "astral-sh/uv"},
		{"react-native", "react-native"},
		{"oven-sh", "oven-sh"},
		// Owner URLs collapse to the same bare login as typing the owner.
		{"https://github.com/oven-sh", "oven-sh"},
		{"https://github.com/oven-sh/", "oven-sh"},
		{"github.com/Oven-Sh", "oven-sh"},
		{"https://github.com/cli", "cli"},
		// Legacy sentinel from older clients.
		{"owner:oven-sh", "oven-sh"},
	}
	for _, tc := range cases {
		if got := NormalizeQuery(tc.in); got != tc.want {
			t.Fatalf("NormalizeQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFilterItems(t *testing.T) {
	items := []github.SearchRepo{
		{Owner: "cli", Repo: "cli"},
		{Owner: "charmbracelet", Repo: "bubbletea"},
		{Owner: "clipper", Repo: "tool"},
	}

	got := FilterItems(items, "clip")
	if len(got) != 1 || got[0].Owner != "clipper" {
		t.Fatalf("expected only clipper/tool, got %+v", got)
	}

	got = FilterItems(items, "cli")
	if len(got) != 2 {
		t.Fatalf("expected cli/cli and clipper/tool, got %+v", got)
	}
}

func TestFilterItems_SlugAndBare(t *testing.T) {
	items := []github.SearchRepo{
		{Owner: "astral-sh", Repo: "uv"},
		{Owner: "astral-sh", Repo: "setup-uv"},
		{Owner: "facebook", Repo: "react-native"},
		{Owner: "other", Repo: "react-native"},
		{Owner: "oven-sh", Repo: "bun"},
	}

	got := FilterItems(items, "astral-sh/uv")
	if len(got) != 1 || got[0].Repo != "uv" {
		t.Fatalf("slug: %+v", got)
	}

	got = FilterItems(items, "astral-sh/u")
	if len(got) != 1 || got[0].Repo != "uv" {
		t.Fatalf("slug prefix must not keep setup-uv: %+v", got)
	}

	got = FilterItems(items, "astral-sh")
	if len(got) != 2 {
		t.Fatalf("bare owner should keep that owner's repos: %+v", got)
	}

	got = FilterItems(items, "react-native")
	if len(got) != 2 {
		t.Fatalf("bare repo name: %+v", got)
	}

	got = FilterItems(items, "oven-sh")
	if len(got) != 1 || got[0].Owner != "oven-sh" {
		t.Fatalf("bare org name should match owner: %+v", got)
	}
}

func TestRankSuggestions(t *testing.T) {
	items := []github.SearchRepo{
		{Owner: "RageLtd", Repo: "bun-test-utils", Stars: 3},
		{Owner: "someone", Repo: "bun", Stars: 100},
		{Owner: "oven-sh", Repo: "bun", Stars: 80000},
		{Owner: "oven-sh", Repo: "setup-bun", Stars: 500},
		{Owner: "facebook", Repo: "react-native", Stars: 100000},
		{Owner: "other", Repo: "react-native", Stars: 50},
	}

	got := RankSuggestions(items, "oven-sh")
	if got[0].Owner != "oven-sh" || got[0].Repo != "bun" {
		t.Fatalf("bare owner rank (owned by stars when no exact name): %+v", got[0])
	}
	if got[1].Owner != "oven-sh" || got[1].Repo != "setup-bun" {
		t.Fatalf("bare owner second owned repo: %+v", got[1])
	}

	got = RankSuggestions(items, "bun")
	if got[0].Repo != "bun" || got[0].Owner != "oven-sh" {
		t.Fatalf("bare exact repo-name before owned-by-q: %+v", got[0])
	}
	// Unrelated user "bun" should not outrank exact-name oven-sh/bun.
	itemsWithBunUser := append([]github.SearchRepo{
		{Owner: "Bun", Repo: "unpack", Stars: 10},
		{Owner: "Bun", Repo: "mail", Stars: 5},
	}, items...)
	got = RankSuggestions(itemsWithBunUser, "bun")
	if got[0].Owner != "oven-sh" || got[0].Repo != "bun" {
		t.Fatalf("exact name must beat user:bun noise: %+v", summarizeRank(got[:3]))
	}

	got = RankSuggestions(items, "react-native")
	if got[0].Owner != "facebook" || got[0].Repo != "react-native" {
		t.Fatalf("dashed repo-name should float exact name by stars: %+v", got[0])
	}

	got = RankSuggestions(items, "facebook/react-native")
	if got[0].Owner != "facebook" || got[0].Repo != "react-native" {
		t.Fatalf("slug exact: %+v", got[0])
	}
}

func summarizeRank(items []github.SearchRepo) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Owner+"/"+it.Repo)
	}
	return out
}

func TestMergeDedup(t *testing.T) {
	owned := []github.SearchRepo{
		{Owner: "oven-sh", Repo: "bun", Stars: 1},
	}
	named := []github.SearchRepo{
		{Owner: "oven-sh", Repo: "bun", Stars: 1},
		{Owner: "someone", Repo: "bun", Stars: 2},
	}
	got := mergeDedup(owned, named)
	if len(got) != 2 {
		t.Fatalf("expected 2 after dedup, got %+v", got)
	}
	if got[0].Owner != "oven-sh" || got[1].Owner != "someone" {
		t.Fatalf("owner leg first: %+v", got)
	}
}

func TestQuoteKeyword(t *testing.T) {
	if got := quoteKeyword("bun"); got != "bun" {
		t.Fatalf("got %q", got)
	}
	if got := quoteKeyword("react-native"); got != `"react-native"` {
		t.Fatalf("got %q", got)
	}
}
