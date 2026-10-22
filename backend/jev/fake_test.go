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

// fakeReply answers one request. For a Choice: the chosen option and its
// probability (the rest spread evenly). For Describe: attrs overrides the
// filename-token answers. status/latency apply to both kinds.
type fakeReply struct {
	choice  string
	p       float64
	attrs   func(file string) (os, cpu string, main float64)
	tamper  func(answers map[string]any) // optional: corrupt the answer
	status  int
	latency time.Duration
}

type fakeServer struct {
	describes, choices atomic.Int32
	mu                 sync.Mutex
	lastChoice         wireRequest
}

func (f *fakeServer) calls() int32 { return f.describes.Load() + f.choices.Load() }

// lastRequest is the last Choice request.
func (f *fakeServer) lastRequest() wireRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastChoice
}

// tokenAttrs reads OS, CPU and role from filename tokens, like a careful Jev.
func tokenAttrs(file string) (os, cpu string, main float64) {
	n := strings.ToLower(file)
	has := func(subs ...string) bool {
		for _, s := range subs {
			if strings.Contains(n, s) {
				return true
			}
		}
		return false
	}
	switch {
	case has("android", ".apk"):
		os = osAndroid
	case has("darwin", "mac", ".dmg", ".pkg"):
		os = osMac
	case has("windows", "win", ".exe", ".msi"):
		os = osWindows
	case has("linux", ".deb", ".rpm", ".appimage"):
		os = osLinux
	default:
		os = osNotStated
	}
	switch {
	case has("universal"):
		cpu = cpuUniversal
	case has("arm64", "aarch64"):
		cpu = cpuARM64
	case has("x64", "amd64", "x86_64"):
		cpu = cpuAMD64
	case has("386", "i686"):
		cpu = cpuX86
	default:
		cpu = cpuNotStated
	}
	main = 0.95
	if has("cli", "src", "debug") {
		main = 0.05
	}
	return os, cpu, main
}

// fakeJev serves Describe and Choice requests from reply, which sees the
// decoded request so it can answer by option name.
func fakeJev(t *testing.T, reply func(t *testing.T, r wireRequest) fakeReply) (*Client, *fakeServer) {
	t.Helper()
	fs := &fakeServer{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var r wireRequest
		if err := json.NewDecoder(req.Body).Decode(&r); err != nil {
			t.Error(err)
		}
		_, isChoice := r.Questions["choice"]
		if isChoice {
			fs.choices.Add(1)
			fs.mu.Lock()
			fs.lastChoice = r
			fs.mu.Unlock()
		} else {
			fs.describes.Add(1)
		}
		rep := reply(t, r)
		time.Sleep(rep.latency)
		if rep.status != 0 && rep.status != http.StatusOK {
			http.Error(w, "boom", rep.status)
			return
		}
		answers := map[string]any{}
		if isChoice {
			options := r.options(t)
			choice := rep.choice
			if choice == "" {
				choice = options[0]
			}
			probs := map[string]float64{}
			for _, o := range options {
				probs[o] = (1 - rep.p) / float64(len(options)-1)
			}
			probs[choice] = rep.p
			answers["choice"] = map[string]any{"type": "choice", "choice": choice, "probabilities": probs, "confidence": rep.p}
		} else {
			attrs := rep.attrs
			if attrs == nil {
				attrs = tokenAttrs
			}
			// Every answer lists all options (zero unless given), and the choice is the argmax.
			fullAnswer := func(criteria orderedCriteria, given map[string]float64) map[string]any {
				probs, choice := map[string]float64{}, ""
				for _, o := range criteria {
					probs[o.name] = given[o.name]
					if choice == "" || probs[o.name] > probs[choice] {
						choice = o.name
					}
				}
				return map[string]any{"type": "choice", "choice": choice, "probabilities": probs}
			}
			for id, q := range r.Questions {
				var in struct{ File string }
				json.Unmarshal(q.Instructions, &in)
				os, cpu, main := attrs(in.File)
				switch {
				case strings.HasPrefix(id, "os_"):
					answers[id] = fullAnswer(osCriteria, map[string]float64{os: 1})
				case strings.HasPrefix(id, "cpu_"):
					answers[id] = fullAnswer(cpuCriteria, map[string]float64{cpu: 1})
				default:
					answers[id] = fullAnswer(roleCriteria, map[string]float64{roleMain: main, "a companion tool": 1 - main})
				}
			}
		}
		if rep.tamper != nil {
			rep.tamper(answers)
		}
		json.NewEncoder(w).Encode(map[string]any{"model": "jev-test", "answers": answers, "usage": map[string]int{"input_tokens": 10}})
	}))
	t.Cleanup(srv.Close)
	return &Client{apiKey: "k", endpoint: srv.URL, model: "jev-test", http: srv.Client()}, fs
}

// picks returns a reply choosing name with probability p.
func picks(name string, p float64) func(*testing.T, wireRequest) fakeReply {
	return func(*testing.T, wireRequest) fakeReply { return fakeReply{choice: name, p: p} }
}

// containing chooses the first offered option containing sub.
func containing(sub string, p float64) func(*testing.T, wireRequest) fakeReply {
	return func(t *testing.T, r wireRequest) fakeReply {
		if _, ok := r.Questions["choice"]; !ok {
			return fakeReply{}
		}
		for _, o := range r.options(t) {
			if strings.Contains(o, sub) {
				return fakeReply{choice: o, p: p}
			}
		}
		return fakeReply{choice: InsufficientOption, p: p}
	}
}
