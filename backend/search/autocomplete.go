// Package search owns homepage repo autocomplete: cache lookup while the user
// types. Handlers stay wire-only.
//
// Query contract (two shapes only):
//
//	slug — owner/repo (or a github.com/owner/repo URL)
//	       → user:owner in:name <quoted-repo-if-needed> + GetRepo ensure
//	bare — everything else (typed token, or pasted github.com/owner URL
//	       normalized to the bare login)
//	       → dual Search: user:<q> (owner's repos) + "<q>" in:name (name matches)
//	       → merge, then rank: exact repo name → owned-by-q → rest by stars
//
// Dashes are normal in both owner logins and repo names; bare never guesses
// which one the user meant — it does both. Always archived:false; stars sort
// at the API.
package search

import (
	"context"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/argval/yatko/cache"
	"github.com/argval/yatko/github"
)

// githubRepoURLRe extracts owner/repo from a pasted GitHub URL so autocomplete
// searches the slug instead of the full URL as free text (which matches
// descriptions that merely mention the link).
var githubRepoURLRe = regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?github\.com/([a-zA-Z0-9._-]+)/([a-zA-Z0-9._-]+)`)

// githubOwnerURLRe matches a pasted profile/org URL with no repo segment.
var githubOwnerURLRe = regexp.MustCompile(`(?i)^(?:https?://)?(?:www\.)?github\.com/([a-zA-Z0-9._-]+)/?$`)

// loginRe is a permissive GitHub login shape. Used to decide whether a bare
// token can meaningfully hit user:<q>; invalid shapes skip the owner leg.
var loginRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9._-]{0,37}[a-z0-9])?$`)

const (
	// MinQueryLen is the shortest query we will look up.
	MinQueryLen = 2
	// MaxQueryLen rejects oversized autocomplete queries.
	MaxQueryLen = 100
	// maxSuggestions caps the dropdown after merge + rank.
	maxSuggestions = 8
)

// Kind is the normalized shape of an autocomplete query.
type Kind int

const (
	KindBare Kind = iota
	KindSlug
)

// Autocomplete looks up GitHub repos for a typing prefix.
type Autocomplete struct {
	gh    *github.Client
	cache *cache.Cache
}

func NewAutocomplete(gh *github.Client, c *cache.Cache) *Autocomplete {
	return &Autocomplete{gh: gh, cache: c}
}

// Suggest returns repos matching q. Results are cached by the normalized key.
// Slug queries also ensure the exact repo via GetRepo.
func (a *Autocomplete) Suggest(ctx context.Context, q string) ([]github.SearchRepo, error) {
	key := cache.SearchKey(q)
	kind, owner, repo := Classify(q)

	items, err := cache.FetchCachedWithTTL(ctx, a.cache, key, cache.SearchSoftTTL, func(ctx context.Context, etag string) ([]github.SearchRepo, string, bool, error) {
		if kind == KindSlug {
			return a.gh.SearchRepositories(ctx, SlugAPIQuery(owner, repo), etag)
		}
		// Bare dual-fetch has no single ETag; store without conditional refresh.
		merged, err := a.searchBare(ctx, q)
		return merged, "", false, err
	})
	if err != nil {
		return nil, err
	}

	if kind == KindSlug {
		items = a.ensureExactRepo(ctx, items, owner, repo)
	}
	return trimSuggestions(RankSuggestions(items, q)), nil
}

// searchBare runs owner-browse and keyword search in parallel, then dedupes.
func (a *Autocomplete) searchBare(ctx context.Context, q string) ([]github.SearchRepo, error) {
	queries := BareAPIQueries(q)
	if len(queries) == 0 {
		return nil, nil
	}
	if len(queries) == 1 {
		items, _, _, err := a.gh.SearchRepositories(ctx, queries[0], "")
		return items, err
	}

	var (
		wg       sync.WaitGroup
		owned    []github.SearchRepo
		named    []github.SearchRepo
		ownedErr error
		namedErr error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		owned, _, _, ownedErr = a.gh.SearchRepositories(ctx, queries[0], "")
	}()
	go func() {
		defer wg.Done()
		named, _, _, namedErr = a.gh.SearchRepositories(ctx, queries[1], "")
	}()
	wg.Wait()

	if ownedErr != nil && namedErr != nil {
		return nil, ownedErr
	}
	if ownedErr != nil {
		return named, nil
	}
	if namedErr != nil {
		return owned, nil
	}
	return mergeDedup(owned, named), nil
}

// mergeDedup keeps owner-leg hits first, then keyword hits not already present.
func mergeDedup(owned, named []github.SearchRepo) []github.SearchRepo {
	seen := make(map[string]struct{}, len(owned)+len(named))
	out := make([]github.SearchRepo, 0, len(owned)+len(named))
	add := func(it github.SearchRepo) {
		key := strings.ToLower(it.Owner) + "/" + strings.ToLower(it.Repo)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, it)
	}
	for _, it := range owned {
		add(it)
	}
	for _, it := range named {
		add(it)
	}
	return out
}

func trimSuggestions(items []github.SearchRepo) []github.SearchRepo {
	if len(items) <= maxSuggestions {
		return items
	}
	return items[:maxSuggestions]
}

// ensureExactRepo prepends GET /repos/{owner}/{repo} when Search omitted it.
func (a *Autocomplete) ensureExactRepo(ctx context.Context, items []github.SearchRepo, owner, repo string) []github.SearchRepo {
	for _, it := range items {
		if strings.EqualFold(it.Owner, owner) && strings.EqualFold(it.Repo, repo) {
			return items
		}
	}
	r, _, _, err := a.gh.GetRepo(ctx, owner, repo, "")
	if err != nil || r == nil || r.Archived || r.Owner.Login == "" || r.Name == "" {
		return items
	}
	exact := github.SearchRepo{
		Owner:       r.Owner.Login,
		Repo:        r.Name,
		Description: r.Description,
		Stars:       r.Stars,
		AvatarURL:   r.Owner.AvatarURL,
	}
	return append([]github.SearchRepo{exact}, items...)
}

func splitSlug(q string) (owner, repo string, ok bool) {
	owner, repo, cut := strings.Cut(q, "/")
	if !cut || owner == "" || repo == "" || strings.Contains(repo, "/") {
		return "", "", false
	}
	return owner, repo, true
}

// Classify maps a normalized query to slug or bare.
func Classify(q string) (Kind, string, string) {
	if owner, repo, ok := splitSlug(q); ok {
		return KindSlug, owner, repo
	}
	return KindBare, "", ""
}

// FilterItems keeps repos that still match a longer typed query. Rules match
// Classify so optimistic/prefix filtering cannot disagree with ranking.
func FilterItems(items []github.SearchRepo, q string) []github.SearchRepo {
	out := make([]github.SearchRepo, 0, len(items))
	kind, owner, repo := Classify(q)
	if kind == KindSlug {
		owner = strings.ToLower(owner)
		repo = strings.ToLower(repo)
		for _, item := range items {
			if strings.ToLower(item.Owner) != owner {
				continue
			}
			if strings.HasPrefix(strings.ToLower(item.Repo), repo) {
				out = append(out, item)
			}
		}
		return out
	}
	for _, item := range items {
		slug := strings.ToLower(item.Owner + "/" + item.Repo)
		if strings.Contains(slug, q) ||
			strings.Contains(strings.ToLower(item.Repo), q) ||
			strings.Contains(strings.ToLower(item.Owner), q) {
			out = append(out, item)
		}
	}
	return out
}

// quoteKeyword wraps q so GitHub will not interpret -, :, etc. as operators.
func quoteKeyword(q string) string {
	if q == "" {
		return q
	}
	if !strings.ContainsAny(q, " -:\"") {
		return q
	}
	return `"` + strings.ReplaceAll(q, `"`, `\"`) + `"`
}

func withArchived(base string) string {
	if base == "" || strings.Contains(base, "archived:") {
		return base
	}
	return base + " archived:false"
}

// SlugAPIQuery is the GitHub Search q= for an owner/repo slug.
func SlugAPIQuery(owner, repo string) string {
	return withArchived("user:" + owner + " in:name " + quoteKeyword(repo))
}

// BareAPIQueries returns the GitHub Search q= values for a bare token.
// Login-shaped tokens get owner browse first, then keyword; otherwise keyword only.
func BareAPIQueries(q string) []string {
	if q == "" {
		return nil
	}
	if strings.Contains(q, ":") {
		// Power-user qualifier input (e.g. user:cli) — pass through as-is.
		return []string{withArchived(q)}
	}
	// in:name so description-only hits (lists that mention the word) stay out.
	keyword := quoteKeyword(q) + " in:name"
	if loginRe.MatchString(q) {
		return []string{
			withArchived("user:" + q),
			withArchived(keyword),
		}
	}
	return []string{withArchived(keyword)}
}

func sortByStarsDesc(items []github.SearchRepo) {
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Stars > items[j].Stars
	})
}

// RankSuggestions orders hits:
//
//	slug — exact owner/repo first, then rest by stars
//	bare — exact repo name, then owned-by-q, then rest by stars
//
// Exact name before owned so a famous repo (oven-sh/bun) is not buried under an
// unrelated user:<q> that filled the page (user Bun's repos for bare "bun").
func RankSuggestions(items []github.SearchRepo, q string) []github.SearchRepo {
	if len(items) == 0 {
		return items
	}

	kind, owner, repo := Classify(q)
	if kind == KindSlug {
		exact := make([]github.SearchRepo, 0, 1)
		rest := make([]github.SearchRepo, 0, len(items))
		for _, it := range items {
			if strings.EqualFold(it.Owner, owner) && strings.EqualFold(it.Repo, repo) {
				exact = append(exact, it)
			} else {
				rest = append(rest, it)
			}
		}
		sortByStarsDesc(rest)
		return append(exact, rest...)
	}

	// Bare: exact repo-name match, then owned-by-q, then rest.
	if !loginRe.MatchString(q) {
		out := append([]github.SearchRepo(nil), items...)
		sortByStarsDesc(out)
		return out
	}
	named := make([]github.SearchRepo, 0, len(items))
	owned := make([]github.SearchRepo, 0, len(items))
	rest := make([]github.SearchRepo, 0, len(items))
	for _, it := range items {
		switch {
		case strings.EqualFold(it.Repo, q):
			named = append(named, it)
		case strings.EqualFold(it.Owner, q):
			owned = append(owned, it)
		default:
			rest = append(rest, it)
		}
	}
	sortByStarsDesc(named)
	sortByStarsDesc(owned)
	sortByStarsDesc(rest)
	out := make([]github.SearchRepo, 0, len(items))
	out = append(out, named...)
	out = append(out, owned...)
	out = append(out, rest...)
	return out
}

// NormalizeQuery trims and lowercases for stable cache keys.
// Repo URLs → owner/repo. Owner-only URLs → bare login (same as typing the owner).
func NormalizeQuery(q string) string {
	q = strings.TrimSpace(q)
	if m := githubRepoURLRe.FindStringSubmatch(q); len(m) == 3 {
		repo := strings.TrimSuffix(strings.ToLower(m[2]), ".git")
		if repo != "" {
			return strings.ToLower(m[1]) + "/" + repo
		}
	}
	if m := githubOwnerURLRe.FindStringSubmatch(q); len(m) == 2 {
		owner := strings.ToLower(m[1])
		if owner != "" {
			return owner
		}
	}
	q = strings.ToLower(q)
	q = strings.TrimSuffix(q, "/")
	// Legacy sentinel from older clients → bare login.
	if owner, ok := strings.CutPrefix(q, "owner:"); ok && loginRe.MatchString(owner) {
		return owner
	}
	return q
}
