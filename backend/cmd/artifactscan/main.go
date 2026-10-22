// Command artifactscan runs full-file inspection separately from the web server.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/argval/yatko/artifactscan"
	"github.com/argval/yatko/github"
)

type scanCase struct {
	Repo  string       `json:"repo"`
	Tag   string       `json:"tag"`
	Asset github.Asset `json:"asset"`
}

type scanResult struct {
	scanCase
	Inspector string                  `json:"inspector"`
	Metadata  github.ArtifactMetadata `json:"metadata"`
	Error     string                  `json:"error,omitempty"`
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run keeps batch verification and durable queue operation in one small executable.
func run(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: artifactscan worker [-once] | enqueue -cases PATH | inspect -cases PATH -out PATH [-files DIR]")
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	casesPath := fs.String("cases", "", "JSON release-asset inventory")
	files := fs.String("files", "", "existing URL-hashed files (inspect only)")
	out := fs.String("out", "", "inspection JSON report (inspect only)")
	workers := fs.Int("workers", 2, "parallel inspections, from 1 to 4 (inspect only)")
	once := fs.Bool("once", false, "drain currently due jobs and exit (worker only)")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	d := &github.DeepInspector{
		SevenZip: os.Getenv("ARTIFACT_SCAN_7ZIP"), Inno: os.Getenv("ARTIFACT_SCAN_INNO"),
		DownloadDir: os.Getenv("ARTIFACT_SCAN_DOWNLOAD_DIR"),
	}
	if d.Inno == "" {
		d.Inno = "inno-inspect"
	}
	if args[0] == "inspect" || args[0] == "enqueue" {
		if *casesPath == "" {
			return errors.New("-cases is required")
		}
		data, err := os.ReadFile(*casesPath)
		if err != nil {
			return err
		}
		var cases []scanCase
		if err := json.Unmarshal(data, &cases); err != nil {
			return err
		}
		if len(cases) == 0 {
			return errors.New("inventory has no assets")
		}
		if args[0] == "inspect" {
			if *out == "" || *workers < 1 || *workers > 4 {
				return errors.New("inspect requires -out and between 1 and 4 workers")
			}
			return inspectCases(ctx, d, cases, *files, *out, *workers)
		}
		s, closeStore, err := configuredStore()
		if err != nil {
			return err
		}
		defer closeStore()
		for _, c := range cases {
			if _, err := s.Enqueue(ctx, c.Asset); err != nil {
				return err
			}
		}
		fmt.Fprintf(os.Stderr, "Queued %d assets using inspector %s\n", len(cases), github.DeepInspectorVersion)
		return nil
	}
	if args[0] != "worker" {
		return errors.New("unknown command; use worker, enqueue, or inspect")
	}
	s, closeStore, err := configuredStore()
	if err != nil {
		return err
	}
	defer closeStore()
	if *once {
		for {
			worked, err := s.ProcessNext(ctx, d.Inspect)
			if err != nil || !worked {
				return err
			}
		}
	}
	return s.Run(ctx, d.Inspect)
}

func configuredStore() (*artifactscan.Store, func() error, error) {
	s, closeStore, err := artifactscan.OpenFromEnv(github.DeepInspectorVersion)
	if err == nil && s == nil {
		return nil, closeStore, errors.New("set ARTIFACT_SCAN_DIR or ARTIFACT_SCAN_REDIS_URL explicitly")
	}
	return s, closeStore, err
}

// inspectCases saves each completed result atomically, so an interrupted batch is reviewable.
func inspectCases(ctx context.Context, d *github.DeepInspector, cases []scanCase, files, out string, workers int) error {
	jobs := make(chan scanCase)
	results := make(chan scanResult)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for c := range jobs {
				var m github.ArtifactMetadata
				var err error
				if files != "" {
					m, err = d.InspectFile(ctx, c.Asset, filepath.Join(files, github.DeepDownloadName(c.Asset)))
				} else {
					m, err = d.Inspect(ctx, c.Asset)
				}
				r := scanResult{scanCase: c, Inspector: github.DeepInspectorVersion, Metadata: m}
				if err != nil {
					r.Error = err.Error()
				}
				results <- r
			}
		})
	}
	go func() {
		defer close(jobs)
		for _, c := range cases {
			select {
			case jobs <- c:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	completed := make(map[string]scanResult)
	var saveErr error
	failures := 0
	for r := range results {
		completed[r.Asset.BrowserDownloadURL] = r
		if r.Error != "" {
			failures++
		}
		fmt.Fprintf(os.Stderr, "%d/%d %s %s %v %s\n", len(completed), len(cases), r.Asset.Name, r.Metadata.Status, r.Metadata.Targets, r.Error)
		ordered := make([]scanResult, 0, len(completed))
		for _, c := range cases {
			if result, ok := completed[c.Asset.BrowserDownloadURL]; ok {
				ordered = append(ordered, result)
			}
		}
		if err := saveReport(out, ordered); err != nil {
			saveErr = err
		}
	}
	if saveErr != nil {
		return saveErr
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if failures > 0 {
		return fmt.Errorf("%d inspections failed; details saved in report", failures)
	}
	return nil
}

func saveReport(filename string, results []scanResult) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(filename), ".inspection-report-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filename)
}
