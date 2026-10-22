package github

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const harvestDir = "../../shared/picker/"

var manifestName = regexp.MustCompile(`^(latest.*\.yml|latest\.json|dist-manifest\.json)$`)

// TestSnapshotHarvestManifests records what production's ReleaseMetadata reads
// from publisher manifests for every harvested release that ships one, so the
// warm harvest eval sees the same evidence /dl does. It downloads only the
// manifests (and one release lookup each), so it runs only on request:
//
//	set -a; source ../.env; set +a; HARVEST_MANIFESTS=1 go test ./github -run TestSnapshotHarvestManifests -v
func TestSnapshotHarvestManifests(t *testing.T) {
	if os.Getenv("HARVEST_MANIFESTS") != "1" {
		t.Skip("set HARVEST_MANIFESTS=1 to refresh shared/picker/harvest_manifests.json")
	}
	f, err := os.Open(harvestDir + "harvest.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type release struct{ repo, tag string }
	var releases []release
	seen := map[release]bool{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r struct{ Repo, Tag, Name string }
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		k := release{r.Repo, r.Tag}
		if manifestName.MatchString(r.Name) && !seen[k] {
			seen[k] = true
			releases = append(releases, k)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}

	c := NewClient()
	out := map[string]map[string]ArtifactMetadata{}
	for _, r := range releases {
		owner, repo, _ := strings.Cut(r.repo, "/")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		rel, _, _, err := c.GetReleaseByTag(ctx, owner, repo, r.tag, "")
		if err != nil {
			cancel()
			t.Errorf("%s@%s: %v", r.repo, r.tag, err)
			continue
		}
		facts, err := c.ReleaseMetadata(ctx, rel.Assets)
		cancel()
		switch {
		case err != nil && len(facts) == 0:
			t.Errorf("%s@%s: %v", r.repo, r.tag, err)
		case err != nil:
			// Like /dl: one malformed manifest keeps the other manifests' facts.
			t.Logf("%s@%s: kept valid facts despite: %v", r.repo, r.tag, err)
		}
		if len(facts) > 0 {
			out[r.repo+"@"+r.tag] = facts
		}
	}
	if t.Failed() {
		return // Preserve the last complete snapshot after a partial refresh.
	}
	data, err := json.MarshalIndent(out, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(harvestDir+"harvest_manifests.json", append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote manifest facts for %d of %d releases", len(out), len(releases))
}

// TestReinspectHarvest re-runs InspectArtifact on harvested assets whose name
// ends with HARVEST_REINSPECT (e.g. ".msi") and rewrites their rows in
// harvest.jsonl, so labels improve with the inspector. Network: range reads only.
//
//	HARVEST_REINSPECT=.msi go test ./github -run TestReinspectHarvest -v -timeout 30m
func TestReinspectHarvest(t *testing.T) {
	suffix := os.Getenv("HARVEST_REINSPECT")
	if suffix == "" {
		t.Skip("set HARVEST_REINSPECT=<suffix> to re-inspect harvested assets")
	}
	data, err := os.ReadFile(harvestDir + "harvest.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	c := NewClient()
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	before, after := map[string]int{}, map[string]int{}
	for i, line := range lines {
		var row map[string]any
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		name, _ := row["name"].(string)
		if !strings.HasSuffix(strings.ToLower(name), suffix) {
			continue
		}
		original, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		repo, tag := row["repo"].(string), row["tag"].(string)
		size := int64(row["size"].(float64))
		a := Asset{Name: name, Size: size, State: "uploaded",
			BrowserDownloadURL: "https://github.com/" + repo + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(name)}
		m, err := c.InspectArtifact(context.Background(), a)
		if err != nil {
			t.Errorf("%s %s: %v", repo, name, err)
			continue // A transport or parse failure must not erase earlier evidence.
		}
		before[row["status"].(string)]++
		after[m.Status]++
		row["status"], row["role"] = m.Status, m.Role
		row["targets_partial"], row["launcher_arch"] = m.TargetsPartial, m.LauncherArch
		delete(row, "targets")
		delete(row, "error")
		if len(m.Targets) > 0 {
			row["targets"] = m.Targets
		}
		if err != nil {
			row["error"] = err.Error()
		}
		if m.Status != "inspected" {
			var why []string
			for _, e := range m.Evidence {
				why = append(why, e.Value)
			}
			t.Logf("%s %s: %s %v %v", repo, name, m.Status, why, err)
		}
		out, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(original, out) {
			lines[i] = string(out)
		}
	}
	if err := os.WriteFile(harvestDir+"harvest.jsonl", []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("status before %v, after %v", before, after)
}
