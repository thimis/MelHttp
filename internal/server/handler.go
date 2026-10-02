package server

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/obfs"
	"github.com/thimis/MelHttp/internal/webvm"
)

const sourcePrefix = "/_source/"

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	rec := &recorder{ResponseWriter: w, status: http.StatusOK}
	defer func() {
		if p := recover(); p != nil {
			s.cfg.Logger.Error("panic", "path", r.URL.Path, "panic", fmt.Sprint(p))
			if !rec.wrote {
				http.Error(rec, "internal server error", http.StatusInternalServerError)
			}
		}
		s.metrics.observe(rec.status, time.Since(start))
		s.cfg.Logger.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status,
			"bytes", rec.bytes, "cache", rec.Header().Get("X-Malbolge-Cache"), "dur", time.Since(start))
	}()
	h := rec.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	h.Set("X-Frame-Options", "SAMEORIGIN")
	h.Set("Content-Security-Policy", "frame-ancestors 'self'")
	if r.TLS != nil && s.cfg.HSTS > 0 {
		h.Set("Strict-Transport-Security", "max-age="+strconv.FormatInt(int64(s.cfg.HSTS.Seconds()), 10))
	}
	for k, v := range s.site.Headers {
		h.Set(k, v)
	}
	s.serve(rec, r)
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	if p == "/healthz" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		io.WriteString(w, "ok\n")
		return
	}
	if s.assets != nil && strings.HasPrefix(p, webvm.Prefix) {
		s.assets.ServeHTTP(w, r)
		return
	}
	if !validURLPath(p) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if s.cfg.ExposeSource && strings.HasPrefix(p, sourcePrefix) {
		s.serveSource(w, r, "/"+strings.TrimPrefix(p, sourcePrefix))
		return
	}
	e := s.resolve(p)
	if e == nil {
		if s.index.isDir(p) && s.resolve(p+"/") != nil {
			target := p + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		if s.SPA() && (r.Method == http.MethodGet || r.Method == http.MethodHead) && !strings.Contains(path.Base(p), ".") {
			e = s.index.lookup("/index.html")
		}
	}
	if e == nil {
		s.notFound(w, r)
		return
	}
	if !methodAllowed(e, r.Method) {
		if e.kind == kindStatic {
			w.Header().Set("Allow", "GET, HEAD")
		} else {
			w.Header().Set("Allow", "GET, HEAD, POST")
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if e.kind != kindStatic && !s.decodeRequestBody(w, r) {
		return
	}
	if e.kind != kindStatic && r.ContentLength > s.cfg.MaxBody {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBody)
	}
	res, hit, err := s.result(r.Context(), e, r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.write(w, r, e, res, hit, 0)
}

// resolve maps a URL path to an entry; "/dir/" means "/dir/index.html".
func (s *Server) resolve(p string) *entry {
	if strings.HasSuffix(p, "/") {
		p += "index.html"
	}
	return s.index.lookup(p)
}

func methodAllowed(e *entry, m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead:
		return true
	case http.MethodPost:
		return e.kind != kindStatic
	}
	return false
}

// write sends a result. status overrides the result's status when non-zero
// (used for the custom 404 page).
func (s *Server) write(w http.ResponseWriter, r *http.Request, e *entry, res *result, hit bool, status int) {
	h := w.Header()
	for k, v := range res.header {
		h[k] = v
	}
	if res.program {
		h.Set("X-Powered-By", "Malbolge")
		h.Set("X-Malbolge-Steps", strconv.FormatInt(res.steps, 10))
	}
	if res.wasm {
		h.Set("X-Powered-By", "WebAssembly (MelHttp WASI)")
	}
	if hit {
		h.Set("X-Malbolge-Cache", "hit")
		s.metrics.hits.Add(1)
	} else {
		h.Set("X-Malbolge-Cache", "miss")
		s.metrics.misses.Add(1)
	}
	if status == 0 {
		status = res.status
	}
	if s.cfg.Obfuscate {
		h.Add("Vary", obfs.AcceptHeader)
		if status == http.StatusOK && s.wantsProgram(r) {
			s.writeProgram(w, r, e.url, res)
			return
		}
	}
	cacheable := status == http.StatusOK && res.deterministic
	if h.Get("Cache-Control") == "" {
		switch {
		case !cacheable:
			h.Set("Cache-Control", "no-store")
		case s.immutable(e.url):
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			h.Set("Cache-Control", "no-cache")
		}
	}
	if !cacheable {
		h.Set("Content-Length", strconv.Itoa(len(res.body)))
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			w.Write(res.body)
		}
		return
	}
	body, etag := res.body, res.etag
	if res.gz != nil {
		h.Add("Vary", "Accept-Encoding")
		if r.Header.Get("Range") == "" && acceptsGzip(r.Header.Get("Accept-Encoding")) {
			body, etag = res.gz, strings.TrimSuffix(res.etag, `"`)+`-gz"`
			h.Set("Content-Encoding", "gzip")
		}
	}
	h.Set("ETag", etag)
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

func (s *Server) immutable(url string) bool {
	for _, re := range s.site.immutable {
		if re.MatchString(url) {
			return true
		}
	}
	return false
}

// acceptsGzip reports whether an Accept-Encoding value allows gzip.
func acceptsGzip(v string) bool {
	for _, part := range strings.Split(v, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") && strings.TrimSpace(name) != "*" {
			continue
		}
		params = strings.ReplaceAll(params, " ", "")
		if q, ok := strings.CutPrefix(params, "q="); ok {
			if f, err := strconv.ParseFloat(q, 64); err == nil && f == 0 {
				return false
			}
		}
		return true
	}
	return false
}

// fail turns a production error into a response, logging the details.
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	var he *httpError
	if errors.As(err, &he) {
		status = he.status
	}
	var mbe *http.MaxBytesError
	if errors.As(err, &mbe) {
		status = http.StatusRequestEntityTooLarge
	}
	if status == http.StatusNotFound {
		s.notFound(w, r)
		return
	}
	s.metrics.failures.Add(1)
	s.cfg.Logger.Error("program failed", "path", r.URL.Path, "status", status, "error", err)
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, http.StatusText(status), status)
}

// notFound serves /404.html (a program or file) with status 404 if the site
// has one.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	if e := s.index.lookup("/404.html"); e != nil && r.URL.Path != "/404.html" {
		get := r.Clone(r.Context())
		get.Method, get.Body, get.ContentLength = http.MethodGet, http.NoBody, 0
		if res, hit, err := s.result(r.Context(), e, get); err == nil {
			s.write(w, r, e, res, hit, http.StatusNotFound)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "404 page not found", http.StatusNotFound)
}

// serveSource serves the Malbolge source of the program behind a URL path,
// for curious visitors (enabled with Config.ExposeSource).
func (s *Server) serveSource(w http.ResponseWriter, r *http.Request, target string) {
	e := s.resolve(target)
	if e == nil || (e.kind != kindProgram && e.kind != kindRaw) || !validURLPath(target) {
		http.Error(w, "no Malbolge program at "+target, http.StatusNotFound)
		return
	}
	files := []string{e.file}
	if e.dir {
		var err error
		if files, err = mbfile.Chunks(s.fsys, e.file); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	var b bytes.Buffer
	for _, f := range files {
		data, err := fs.ReadFile(s.fsys, f)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		b.Write(data)
	}
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Malbolge-Programs", strconv.Itoa(len(files)))
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b.Bytes()))
}

// recorder captures the status and size for the access log.
type recorder struct {
	http.ResponseWriter
	status int
	bytes  int64
	wrote  bool
}

func (r *recorder) WriteHeader(code int) {
	if !r.wrote {
		r.status, r.wrote = code, true
	}
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.wrote = true
	n, err := r.ResponseWriter.Write(b)
	r.bytes += int64(n)
	return n, err
}
