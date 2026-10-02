package server

import (
	"bytes"
	"compress/gzip"
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/thimis/MelHttp/internal/malbolge"
	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/melcgi"
	"github.com/thimis/MelHttp/internal/mimetype"
	"github.com/thimis/MelHttp/internal/wasi"
)

// result is a complete response produced by a program or a static file.
type result struct {
	status        int
	header        http.Header
	body          []byte
	gz            []byte // gzip-compressed body, if worthwhile
	etag          string // strong ETag of body
	steps         int64  // VM instructions (0 for static files)
	program       bool
	wasm          bool   // produced by a WASI handler
	deterministic bool   // the program never read its input
	version       string // signature of the files it came from
}

func (r *result) size() int64 { return int64(len(r.body)+len(r.gz)) + 512 }

// httpError carries the status a failed run should produce.
type httpError struct {
	status int
	err    error
}

func (e *httpError) Error() string { return e.err.Error() }
func (e *httpError) Unwrap() error { return e.err }

// version returns a signature of an entry's files (sizes and mod times).
func (s *Server) version(e *entry) (string, error) {
	var b bytes.Buffer
	stat := func(name string) error {
		fi, err := fs.Stat(s.fsys, name)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s:%d:%d;", name, fi.Size(), fi.ModTime().UnixNano())
		return nil
	}
	if err := stat(e.file); err != nil {
		return "", err
	}
	if e.dir {
		chunks, err := mbfile.Chunks(s.fsys, e.file)
		if err != nil {
			return "", err
		}
		for _, c := range chunks {
			if err := stat(c); err != nil {
				return "", err
			}
		}
	}
	return b.String(), nil
}

// produce runs a program (or reads a static file) for request r. r may be a
// synthetic request during warm-up.
func (s *Server) produce(ctx context.Context, e *entry, r *http.Request) (*result, error) {
	version, err := s.version(e)
	if err != nil {
		return nil, &httpError{http.StatusNotFound, err}
	}
	if e.kind == kindStatic {
		body, err := fs.ReadFile(s.fsys, e.file)
		if err != nil {
			return nil, &httpError{http.StatusNotFound, err}
		}
		h := http.Header{"Content-Type": {mimetype.ByName(e.url)}}
		return finish(&result{status: http.StatusOK, header: h, body: body, deterministic: true, version: version}), nil
	}
	if (e.kind == kindWASI || e.kind == kindWASIRaw) && s.wasi == nil {
		return nil, &httpError{http.StatusNotFound, errors.New("WASI handlers are disabled (-wasi)")}
	}

	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return nil, &httpError{http.StatusServiceUnavailable, ctx.Err()}
	}
	ctx, cancel := context.WithTimeout(ctx, s.cfg.Timeout)
	defer cancel()

	req := melcgi.FromHTTP(r, e.url, "", melcgi.Options{AllowSensitiveHeaders: s.cfg.AllowSensitiveHeaders})
	var body io.Reader
	if r.Body != nil && r.Body != http.NoBody {
		body = r.Body
	}
	in := melcgi.Input(req.Meta(melcgi.Options{AllowSensitiveHeaders: s.cfg.AllowSensitiveHeaders}), body)
	var out bytes.Buffer
	if e.kind == kindWASI || e.kind == kindWASIRaw {
		return s.produceWASI(ctx, e, version, in, &out)
	}
	set, err := mbfile.LoadFS(s.fsys, e.file)
	if err != nil {
		return nil, &httpError{http.StatusInternalServerError, err}
	}
	run, err := set.Run(ctx, in, &out, malbolge.Limits{MaxSteps: s.cfg.MaxSteps, MaxOutput: s.cfg.MaxOutput})
	s.runs.Add(1)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			status = http.StatusServiceUnavailable
		}
		return nil, &httpError{status, fmt.Errorf("%s: %w (after %d steps)", e.file, err, run.Steps)}
	}
	var resp *melcgi.Response
	if e.kind == kindRaw {
		resp = melcgi.RawResponse(out.Bytes(), mimetype.ByName(e.url))
	} else if resp, err = melcgi.ParseResponse(out.Bytes()); err != nil {
		return nil, &httpError{http.StatusBadGateway, fmt.Errorf("%s: %w", e.file, err)}
	}
	return finish(&result{
		status: resp.Status, header: resp.Header, body: resp.Body, steps: run.Steps,
		program: true, deterministic: !run.ReadInput, version: version,
	}), nil
}

// finish computes the ETag and, for compressible bodies, a gzip variant.
func finish(r *result) *result {
	sum := sha256.Sum256(r.body)
	r.etag = `"` + hex.EncodeToString(sum[:12]) + `"`
	if len(r.body) >= 1024 && mimetype.Compressible(r.header.Get("Content-Type")) {
		var b bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
		zw.Write(r.body)
		zw.Close()
		if b.Len() < len(r.body)*9/10 {
			r.gz = b.Bytes()
		}
	}
	return r
}

// cache holds results of deterministic programs and static files.
type cache struct {
	mu       sync.Mutex
	max      int64
	used     int64
	items    map[string]*list.Element // by URL
	lru      *list.List               // of *cacheItem, most recent first
	inflight map[string]*call
}

type cacheItem struct {
	url     string
	res     *result
	checked time.Time
}

type call struct {
	done chan struct{}
	res  *result
	err  error
}

func newCache(max int64) *cache {
	return &cache{max: max, items: map[string]*list.Element{}, lru: list.New(), inflight: map[string]*call{}}
}

func (c *cache) get(url string) (*cacheItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[url]
	if !ok {
		return nil, false
	}
	c.lru.MoveToFront(el)
	return el.Value.(*cacheItem), true
}

func (c *cache) put(url string, res *result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeLocked(url)
	if res.size() > c.max/4 {
		return // never let one response evict most of the cache
	}
	c.items[url] = c.lru.PushFront(&cacheItem{url: url, res: res, checked: time.Now()})
	c.used += res.size()
	for c.used > c.max {
		c.removeLocked(c.lru.Back().Value.(*cacheItem).url)
	}
}

func (c *cache) remove(url string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.removeLocked(url)
}

func (c *cache) removeLocked(url string) {
	if el, ok := c.items[url]; ok {
		c.used -= el.Value.(*cacheItem).res.size()
		c.lru.Remove(el)
		delete(c.items, url)
	}
}

// result returns the response for e, from the cache when possible.
// Concurrent misses for the same URL share one run.
func (s *Server) result(ctx context.Context, e *entry, r *http.Request) (*result, bool, error) {
	if s.cfg.NoCache {
		res, err := s.produce(ctx, e, r)
		return res, false, err
	}
	if res := s.cached(e); res != nil {
		return res, true, nil
	}
	if s.dynamic(e) {
		res, err := s.produce(ctx, e, r)
		return res, false, err
	}
	s.cache.mu.Lock()
	if c, ok := s.cache.inflight[e.url]; ok {
		s.cache.mu.Unlock()
		select {
		case <-c.done:
			if c.err == nil && !c.res.deterministic {
				// That run read its request; this request needs its own run.
				res, err := s.produce(ctx, e, r)
				return res, false, err
			}
			return c.res, true, c.err
		case <-ctx.Done():
			return nil, false, &httpError{http.StatusServiceUnavailable, ctx.Err()}
		}
	}
	c := &call{done: make(chan struct{})}
	s.cache.inflight[e.url] = c
	s.cache.mu.Unlock()

	c.res, c.err = s.produce(ctx, e, r)
	if c.err == nil {
		if c.res.deterministic {
			s.cache.put(e.url, c.res)
		} else {
			s.markDynamic(e.url, c.res.version)
		}
	}
	s.cache.mu.Lock()
	delete(s.cache.inflight, e.url)
	s.cache.mu.Unlock()
	close(c.done)
	return c.res, false, c.err
}

// cached returns the cached result for e if it is still current: at most once
// per Revalidate interval its files are checked against the cached version.
func (s *Server) cached(e *entry) *result {
	it, ok := s.cache.get(e.url)
	if !ok {
		return nil
	}
	s.cache.mu.Lock()
	due := s.cfg.Revalidate < 0 || time.Since(it.checked) >= s.cfg.Revalidate
	if due {
		it.checked = time.Now()
	}
	s.cache.mu.Unlock()
	if due {
		if v, err := s.version(e); err != nil || v != it.res.version {
			s.cache.remove(e.url)
			return nil
		}
	}
	return it.res
}

// dynamic reports whether e is a program known to read its input (and so is
// run on every request), as long as its files have not changed since.
func (s *Server) dynamic(e *entry) bool {
	s.dynMu.Lock()
	v, ok := s.dyn[e.url]
	s.dynMu.Unlock()
	if !ok {
		return false
	}
	cur, err := s.version(e)
	return err == nil && cur == v
}

func (s *Server) markDynamic(url, version string) {
	s.dynMu.Lock()
	s.dyn[url] = version
	s.dynMu.Unlock()
}

// produceWASI runs a WebAssembly handler. Its responses are never cached:
// unlike a Malbolge program, a WASI module can read the clock and random
// numbers, so its output may change on every request.
func (s *Server) produceWASI(ctx context.Context, e *entry, version string, in io.Reader, out *bytes.Buffer) (*result, error) {
	err := s.wasi.Run(ctx, e.file, version, s.wasiLoader(e), in, out, s.cfg.MaxOutput)
	s.runs.Add(1)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, wasi.ErrTimeout) {
			status = http.StatusServiceUnavailable
		}
		return nil, &httpError{status, fmt.Errorf("%s: %w", e.file, err)}
	}
	var resp *melcgi.Response
	if e.kind == kindWASIRaw {
		resp = melcgi.RawResponse(out.Bytes(), mimetype.ByName(e.url))
	} else if resp, err = melcgi.ParseResponse(out.Bytes()); err != nil {
		return nil, &httpError{http.StatusBadGateway, fmt.Errorf("%s: %w", e.file, err)}
	}
	return finish(&result{status: resp.Status, header: resp.Header, body: resp.Body, wasm: true, version: version}), nil
}

func (s *Server) wasiLoader(e *entry) func() ([]byte, error) {
	return func() ([]byte, error) {
		code, err := fs.ReadFile(s.fsys, e.file)
		if err == nil && !bytes.HasPrefix(code, []byte(wasi.Magic)) {
			err = errors.New("not a WebAssembly module")
		}
		return code, err
	}
}

func (s *Server) precompileWASI(ctx context.Context, e *entry) error {
	version, err := s.version(e)
	if err != nil {
		return err
	}
	return s.wasi.Precompile(ctx, e.file, version, s.wasiLoader(e))
}
