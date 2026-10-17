// Package assetml predicts a release asset's OS, CPU architecture, and libc
// from its filename, using the char n-gram logistic model trained in ml/train.py.
package assetml

import (
	_ "embed"
	"encoding/json"
	"math"
	"strings"
	"sync"
	"unicode"
)

//go:embed model.json
var modelJSON []byte

// Prediction holds per-class probabilities for each head; each map sums to 1.
type Prediction struct {
	OS, Arch, Libc map[string]float64
}

type head struct {
	Classes   []string             `json:"classes"`
	Intercept []float64            `json:"intercept"`
	Weights   map[string][]float64 `json:"weights"`
}

type model struct {
	Version  int             `json:"version"`
	NgramMin int             `json:"ngram_min"`
	NgramMax int             `json:"ngram_max"`
	Heads    map[string]head `json:"heads"`
}

// loadModel decodes the embedded model once; a bad embed is a build bug, so it panics.
var loadModel = sync.OnceValue(func() model {
	var m model
	if err := json.Unmarshal(modelJSON, &m); err != nil {
		panic("assetml: bad embedded model.json: " + err.Error())
	}
	return m
})

// Predict scores a filename with every head of the embedded model.
func Predict(name string) Prediction {
	m := loadModel()
	grams := ngrams(name, m.NgramMin, m.NgramMax)
	return Prediction{
		OS:   m.Heads["os"].score(grams),
		Arch: m.Heads["arch"].score(grams),
		Libc: m.Heads["libc"].score(grams),
	}
}

// ngrams mirrors sklearn's char analyzer: lowercase, collapse whitespace runs
// of 2+ to one space, then collect distinct code-point n-grams of each length.
func ngrams(name string, lo, hi int) map[string]struct{} {
	r := collapseSpace([]rune(strings.ToLower(name)))
	out := make(map[string]struct{})
	for n := lo; n <= hi && n <= len(r); n++ {
		for i := 0; i+n <= len(r); i++ {
			out[string(r[i:i+n])] = struct{}{}
		}
	}
	return out
}

// collapseSpace replaces each run of 2+ whitespace runes (Python's \s\s+) with one space.
func collapseSpace(r []rune) []rune {
	out := make([]rune, 0, len(r))
	for i := 0; i < len(r); {
		j := i
		for j < len(r) && isSpace(r[j]) {
			j++
		}
		switch {
		case j-i >= 2:
			out = append(out, ' ')
			i = j
		default:
			out = append(out, r[i])
			i++
		}
	}
	return out
}

// isSpace matches Python's str regex \s, which also counts the 0x1c-0x1f separators.
func isSpace(r rune) bool {
	return unicode.IsSpace(r) || (r >= 0x1c && r <= 0x1f)
}

// score returns the softmax over intercept plus the weights of present n-grams.
func (h head) score(grams map[string]struct{}) map[string]float64 {
	z := append([]float64(nil), h.Intercept...)
	for g := range grams {
		for k, w := range h.Weights[g] {
			z[k] += w
		}
	}
	top := math.Inf(-1)
	for _, v := range z {
		top = max(top, v)
	}
	sum := 0.0
	for k, v := range z {
		z[k] = math.Exp(v - top)
		sum += z[k]
	}
	out := make(map[string]float64, len(z))
	for k, c := range h.Classes {
		out[c] = z[k] / sum
	}
	return out
}
