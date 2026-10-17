// Command harvest builds training data for the asset-name model. It finds
// popular repos, fetches each latest release, and records what Yatko's
// artifact inspector proves from file contents (ELF/PE/Mach-O headers,
// archive listings, .deb control data). Those observations are the labels.
//
//	cd backend && set -a && source ../.env && set +a
//	go run ./cmd/harvest -repos 500 -out ../ml/data/harvest.jsonl
//
// The output is append-only JSONL, so an interrupted run resumes where it stopped.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/argval/yatko/github"
	"github.com/argval/yatko/picker"
)

// queries mix topics and languages so the data covers many naming styles.
var queries = []string{
	"topic:cli stars:>300",
	"topic:desktop-app stars:>200",
	"topic:electron stars:>500",
	"topic:tauri stars:>200",
	"topic:terminal stars:>300",
	"topic:macos stars:>500",
	"topic:windows stars:>500",
	"topic:linux stars:>1000",
	"topic:android stars:>1000",
	"topic:developer-tools stars:>500",
	"topic:self-hosted stars:>1000",
	"language:rust stars:>1000",
	"language:go stars:>1000",
	"language:c++ stars:>2000",
	"language:zig stars:>300",
	"language:swift stars:>1000",
	"language:kotlin stars:>1000",
	"language:c# stars:>1000",
}

// Row is one release asset and what inspection proved about it.
type Row struct {
	Repo      string                  `json:"repo"`
	Tag       string                  `json:"tag"`
	Name      string                  `json:"name"`
	Size      int64                   `json:"size"`
	Status    string                  `json:"status"` // inspected | unsupported | failed
	Role      string                  `json:"role,omitempty"`
	Targets   []github.ArtifactTarget `json:"targets,omitempty"`
	Conflicts []string                `json:"conflicts,omitempty"`
	Error     string                  `json:"error,omitempty"`
}

func main() {
	nRepos := flag.Int("repos", 500, "repos with release assets to harvest")
	out := flag.String("out", "../ml/data/harvest.jsonl", "output JSONL (appended)")
	workers := flag.Int("workers", 6, "repos processed in parallel")
	maxAssets := flag.Int("max-assets", 40, "assets inspected per release")
	corpus := flag.String("exclude-corpus", "../shared/picker/corpus.json", "eval corpus; its repos are never harvested")
	flag.Parse()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN is required: search allows 10 req/min and REST 60 req/hr without it")
	}
	skip, err := excludedRepos(*corpus, *out)
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	candidates, err := searchCandidates(ctx, token, skip)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("candidates=%d already_done_or_excluded=%d", len(candidates), len(skip))

	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	var mu sync.Mutex
	enc := json.NewEncoder(f)

	gh := github.NewClient()
	var done, rows, inspected atomic.Int64
	jobs := make(chan string)
	var wg sync.WaitGroup
	for range *workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for repo := range jobs {
				if done.Load() >= int64(*nRepos) {
					continue
				}
				got := harvestRepo(ctx, gh, repo, *maxAssets)
				if len(got) == 0 {
					continue
				}
				mu.Lock()
				for _, r := range got {
					if err := enc.Encode(r); err != nil {
						log.Fatal(err)
					}
					if r.Status == "inspected" {
						inspected.Add(1)
					}
				}
				mu.Unlock()
				rows.Add(int64(len(got)))
				if n := done.Add(1); n%25 == 0 {
					log.Printf("repos=%d rows=%d inspected=%d", n, rows.Load(), inspected.Load())
				}
			}
		}()
	}
	for _, repo := range candidates {
		if done.Load() >= int64(*nRepos) {
			break
		}
		jobs <- repo
	}
	close(jobs)
	wg.Wait()
	log.Printf("finished repos=%d rows=%d inspected=%d out=%s", done.Load(), rows.Load(), inspected.Load(), *out)
}

// harvestRepo inspects the latest release's assets. Skipped files (source,
// signatures) are still written as rows: they are negatives for the role label.
func harvestRepo(ctx context.Context, gh *github.Client, repo string, maxAssets int) []Row {
	owner, name, _ := strings.Cut(repo, "/")
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	rel, _, _, err := gh.GetLatestRelease(rctx, owner, name, "")
	cancel()
	if err != nil || rel == nil || len(rel.Assets) == 0 {
		return nil
	}
	var rows []Row
	for i, a := range rel.Assets {
		if i >= maxAssets {
			break
		}
		if a.State != "" && a.State != "uploaded" {
			continue
		}
		row := Row{Repo: repo, Tag: rel.TagName, Name: a.Name, Size: a.Size}
		m, err := gh.InspectArtifact(ctx, a)
		row.Status, row.Role, row.Targets, row.Conflicts = m.Status, m.Role, m.Targets, m.Conflicts
		if err != nil {
			row.Error = err.Error()
		}
		rows = append(rows, row)
	}
	return rows
}

// excludedRepos returns the eval corpus repos plus repos already in the
// output file, so the model is never trained on the releases it is scored on.
func excludedRepos(corpusPath, outPath string) (map[string]bool, error) {
	skip := map[string]bool{}
	releases, err := picker.LoadCorpusFile(corpusPath)
	if err != nil {
		return nil, fmt.Errorf("load corpus: %w", err)
	}
	for _, r := range releases {
		skip[strings.ToLower(r.Repo)] = true
	}
	f, err := os.Open(outPath)
	if errors.Is(err, os.ErrNotExist) {
		return skip, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r Row
		if json.Unmarshal(sc.Bytes(), &r) == nil {
			skip[strings.ToLower(r.Repo)] = true
		}
	}
	return skip, sc.Err()
}

// searchCandidates interleaves results from every query (round robin) so no
// single topic dominates. Search allows 30 req/min with a token; we pace at 1 per 2.5s.
func searchCandidates(ctx context.Context, token string, skip map[string]bool) ([]string, error) {
	const pages = 3
	perQuery := make([][]string, len(queries))
	for qi, q := range queries {
		for page := 1; page <= pages; page++ {
			names, err := searchPage(ctx, token, q, page)
			if err != nil {
				return nil, fmt.Errorf("search %q page %d: %w", q, page, err)
			}
			perQuery[qi] = append(perQuery[qi], names...)
			time.Sleep(2500 * time.Millisecond)
			if len(names) < 100 {
				break
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	for i := 0; ; i++ {
		added := false
		for _, list := range perQuery {
			if i >= len(list) {
				continue
			}
			added = true
			key := strings.ToLower(list[i])
			if !seen[key] && !skip[key] {
				seen[key] = true
				out = append(out, list[i])
			}
		}
		if !added {
			return out, nil
		}
	}
}

func searchPage(ctx context.Context, token, query string, page int) ([]string, error) {
	u := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&sort=stars&order=desc&per_page=100&page=%d",
		url.QueryEscape(query+" archived:false"), page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Yatko-harvest/1.0")
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var body struct {
		Items []struct {
			FullName string `json:"full_name"`
		} `json:"items"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, err
	}
	names := make([]string, len(body.Items))
	for i, it := range body.Items {
		names[i] = it.FullName
	}
	return names, nil
}
