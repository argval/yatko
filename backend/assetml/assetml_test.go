package assetml

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// TestParity checks Go predictions against the probabilities ml/train.py wrote.
func TestParity(t *testing.T) {
	raw, err := os.ReadFile("testdata/parity.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name string
		OS   map[string]float64 `json:"os"`
		Arch map[string]float64 `json:"arch"`
		Libc map[string]float64 `json:"libc"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) == 0 {
		t.Fatal("no parity cases")
	}
	for _, c := range cases {
		got := Predict(c.Name)
		for head, pair := range map[string][2]map[string]float64{
			"os": {c.OS, got.OS}, "arch": {c.Arch, got.Arch}, "libc": {c.Libc, got.Libc},
		} {
			want, have := pair[0], pair[1]
			if len(want) != len(have) {
				t.Errorf("%q %s: classes %v, want %v", c.Name, head, have, want)
			}
			for cls, p := range want {
				if math.Abs(have[cls]-p) > 1e-6 {
					t.Errorf("%q %s[%s] = %.9f, want %.9f", c.Name, head, cls, have[cls], p)
				}
			}
		}
	}
}

// TestNgrams pins featurization to sklearn's char analyzer output for
// whitespace runs, single tabs, and non-ASCII runes (fixture from sklearn 1.9).
func TestNgrams(t *testing.T) {
	want := []string{"\tü", " b", " b\t", "a ", "a b", "b\t", "b\tü"}
	got := ngrams("A  \t B\tÜ", 2, 3)
	if len(got) != len(want) {
		t.Fatalf("got %d grams %v, want %v", len(got), got, want)
	}
	for _, g := range want {
		if _, ok := got[g]; !ok {
			t.Errorf("missing %q in %v", g, got)
		}
	}
}
