package jev

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// wireRequest is the decoded request body as Jev would receive it.
type wireRequest struct {
	Model     string                     `json:"model"`
	State     map[string]json.RawMessage `json:"state"`
	Questions map[string]struct {
		Type         string          `json:"type"`
		Instructions json.RawMessage `json:"instructions"`
		Criteria     json.RawMessage `json:"criteria"`
	} `json:"questions"`
}

// options returns the Choice option keys in wire order.
func (r wireRequest) options(t *testing.T) []string {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(r.Questions["choice"].Criteria))
	var keys []string
	if _, err := dec.Token(); err != nil {
		t.Fatal(err)
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, tok.(string))
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			t.Fatal(err)
		}
	}
	return keys
}

// fakeReply answers one direct decision: the chosen option and its
// probability (the rest is spread evenly), and each file's "runs" Noul.
type fakeReply struct {
	choice  string
	p       float64
	runs    func(file string) float64
	tamper  func(answers map[string]any) // optional: corrupt the answer
	status  int
	latency time.Duration
}

type fakeServer struct {
	calls atomic.Int32
	mu    sync.Mutex
	last  wireRequest
}

func (f *fakeServer) lastRequest() wireRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

// fakeJev serves the direct schema (decide.go) from reply, which sees the
// decoded request so it can answer by option name.
func fakeJev(t *testing.T, reply func(t *testing.T, r wireRequest) fakeReply) (*Client, *fakeServer) {
	t.Helper()
	fs := &fakeServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		fs.calls.Add(1)
		var r wireRequest
		if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
			t.Error(err)
		}
		fs.mu.Lock()
		fs.last = r
		fs.mu.Unlock()
		rep := reply(t, r)
		time.Sleep(rep.latency)
		if rep.status != 0 && rep.status != http.StatusOK {
			http.Error(w, "boom", rep.status)
			return
		}
		options := r.options(t)
		probs := map[string]float64{}
		for _, o := range options {
			probs[o] = (1 - rep.p) / float64(len(options)-1)
		}
		probs[rep.choice] = rep.p
		answers := map[string]any{"choice": map[string]any{"type": "choice", "choice": rep.choice, "probabilities": probs, "confidence": rep.p}}
		for id, q := range r.Questions {
			if q.Type != "noul" {
				continue
			}
			var in struct {
				File struct{ Name string } `json:"file"`
			}
			json.Unmarshal(q.Instructions, &in)
			p := 0.95
			if rep.runs != nil {
				p = rep.runs(in.File.Name)
			}
			answers[id] = map[string]any{"type": "noul", "noul": p}
		}
		if rep.tamper != nil {
			rep.tamper(answers)
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "jev-test", "answers": answers, "usage": map[string]int{"input_tokens": 10}})
	}))
	t.Cleanup(srv.Close)
	return &Client{apiKey: "k", endpoint: srv.URL, model: "jev-test", http: srv.Client()}, fs
}

// picks returns a reply picking name with probability p; every file runs.
func picks(name string, p float64) func(*testing.T, wireRequest) fakeReply {
	return func(*testing.T, wireRequest) fakeReply { return fakeReply{choice: name, p: p} }
}

// containing picks the first offered option containing sub.
func containing(sub string, p float64) func(*testing.T, wireRequest) fakeReply {
	return func(t *testing.T, r wireRequest) fakeReply {
		for _, o := range r.options(t) {
			if strings.Contains(o, sub) {
				return fakeReply{choice: o, p: p}
			}
		}
		return fakeReply{choice: NoCompatibleOption, p: p}
	}
}
