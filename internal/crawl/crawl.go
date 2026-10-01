// Package crawl verifies that a running server serves every file of a source
// tree byte-for-byte, with the expected Content-Type. It backs the end-to-end
// "served == source" goal tests and the tools/crawl command.
package crawl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/thimis/MelHttp/internal/mimetype"
)

// Options controls a crawl.
type Options struct {
	// Client is used for requests; nil means a client with compression disabled.
	Client *http.Client
	// Concurrency is the number of parallel requests (default 8).
	Concurrency int
	// Skip reports whether a source-relative slash path should be ignored.
	Skip func(rel string) bool
}

// Mismatch describes one file that was not served correctly.
type Mismatch struct {
	Path   string
	Reason string
}

// Report summarizes a crawl.
type Report struct {
	Checked    int
	Bytes      int64
	Mismatches []Mismatch
}

// OK reports whether every file matched.
func (r Report) OK() bool { return len(r.Mismatches) == 0 }

func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "checked %d files (%d bytes), %d mismatches", r.Checked, r.Bytes, len(r.Mismatches))
	for _, m := range r.Mismatches {
		fmt.Fprintf(&b, "\n  %s: %s", m.Path, m.Reason)
	}
	return b.String()
}

// Files lists the slash-separated relative paths of all regular files under root,
// excluding dotfiles/dot-directories other than .well-known.
func Files(root string, skip func(string) bool) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") && name != ".well-known" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if skip != nil && skip(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() {
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// URLPath returns the escaped URL path for a source-relative file path.
func URLPath(rel string) string {
	return (&url.URL{Path: "/" + rel}).EscapedPath()
}

// Site requests every file under srcRoot from baseURL and compares bodies and
// Content-Types. It returns an error only for setup failures; per-file problems
// are reported as mismatches.
func Site(ctx context.Context, baseURL, srcRoot string, opt Options) (Report, error) {
	files, err := Files(srcRoot, opt.Skip)
	if err != nil {
		return Report{}, err
	}
	client := opt.Client
	if client == nil {
		client = &http.Client{Transport: &http.Transport{DisableCompression: true}}
	}
	n := opt.Concurrency
	if n <= 0 {
		n = 8
	}
	var (
		mu  sync.Mutex
		rep Report
		wg  sync.WaitGroup
	)
	jobs := make(chan string)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range jobs {
				size, reason := checkFile(ctx, client, baseURL, srcRoot, rel)
				mu.Lock()
				rep.Checked++
				rep.Bytes += size
				if reason != "" {
					rep.Mismatches = append(rep.Mismatches, Mismatch{rel, reason})
				}
				mu.Unlock()
			}
		}()
	}
	for _, f := range files {
		jobs <- f
	}
	close(jobs)
	wg.Wait()
	sort.Slice(rep.Mismatches, func(i, j int) bool { return rep.Mismatches[i].Path < rep.Mismatches[j].Path })
	return rep, nil
}

func checkFile(ctx context.Context, client *http.Client, baseURL, srcRoot, rel string) (int64, string) {
	want, err := os.ReadFile(filepath.Join(srcRoot, filepath.FromSlash(rel)))
	if err != nil {
		return 0, "read source: " + err.Error()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+URLPath(rel), nil)
	if err != nil {
		return 0, err.Error()
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "read body: " + err.Error()
	}
	if resp.StatusCode != http.StatusOK {
		return int64(len(got)), fmt.Sprintf("status %d", resp.StatusCode)
	}
	if ct, wantCT := resp.Header.Get("Content-Type"), mimetype.ByName(path.Base(rel)); ct != wantCT {
		return int64(len(got)), fmt.Sprintf("content-type %q, want %q", ct, wantCT)
	}
	if !bytes.Equal(got, want) {
		return int64(len(got)), fmt.Sprintf("body differs: got %d bytes sha256 %x, want %d bytes sha256 %x",
			len(got), sha256.Sum256(got), len(want), sha256.Sum256(want))
	}
	return int64(len(got)), ""
}
