package server

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Server serves one site directory.
type Server struct {
	cfg   Config
	root  *os.Root
	fsys  fs.FS
	site  siteConfig
	index *index
	cache *cache
	sem   chan struct{}
	runs  atomic.Int64 // VM runs, for tests and metrics

	dynMu sync.Mutex
	dyn   map[string]string // URL → version of programs that read input
}

// New opens the site at cfg.Root.
func New(cfg Config) (*Server, error) {
	cfg = cfg.withDefaults()
	root, err := os.OpenRoot(cfg.Root)
	if err != nil {
		return nil, fmt.Errorf("site root: %w", err)
	}
	s, err := newServer(cfg, root, root.FS())
	if err != nil {
		root.Close()
		return nil, err
	}
	return s, nil
}

func newServer(cfg Config, root *os.Root, fsys fs.FS) (*Server, error) {
	site, err := loadSiteConfig(fsys)
	if err != nil {
		return nil, err
	}
	minAge := time.Second
	if cfg.Revalidate < 0 {
		minAge = 0
	}
	s := &Server{
		cfg:   cfg,
		root:  root,
		fsys:  fsys,
		site:  site,
		index: newIndex(fsys, minAge),
		cache: newCache(cfg.CacheBytes),
		sem:   make(chan struct{}, cfg.MaxConcurrent),
		dyn:   map[string]string{},
	}
	for _, w := range s.index.warnings {
		cfg.Logger.Warn("site", "warning", w)
	}
	return s, nil
}

// Close releases the site root.
func (s *Server) Close() error {
	if s.root != nil {
		return s.root.Close()
	}
	return nil
}

// SPA reports whether unknown extension-less paths fall back to /index.html.
func (s *Server) SPA() bool { return s.cfg.SPA || s.site.SPA }

// Runs returns how many times a program has been executed.
func (s *Server) Runs() int64 { return s.runs.Load() }

// WarmStats summarizes a warm-up.
type WarmStats struct {
	Programs, Cached, Dynamic, Failed int
	Steps                             int64
	Bytes                             int64
	Duration                          time.Duration
}

// Warm executes every program once and caches the deterministic ones, so the
// first visitor does not wait for the VM.
func (s *Server) Warm(ctx context.Context) WarmStats {
	start := time.Now()
	var st WarmStats
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, e := range s.index.all() {
		if e.kind == kindStatic {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, _ := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost"+e.url, nil)
			r.RemoteAddr = "127.0.0.1:0"
			res, _, err := s.result(ctx, e, r)
			mu.Lock()
			defer mu.Unlock()
			st.Programs++
			switch {
			case err != nil:
				st.Failed++
				s.cfg.Logger.Warn("warm-up", "file", e.file, "error", err)
			case res.deterministic:
				st.Cached++
				st.Steps += res.steps
				st.Bytes += int64(len(res.body))
			default:
				st.Dynamic++
			}
		}()
	}
	wg.Wait()
	st.Duration = time.Since(start)
	return st
}
