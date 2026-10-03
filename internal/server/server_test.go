package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thimis/MelHttp/internal/gen"
	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/melcgi"
)

// writeProgram compiles a MelCGI program printing a header block and body.
func writeProgram(t testing.TB, root, rel, contentType string, body []byte) {
	t.Helper()
	compileTo(t, filepath.Join(root, filepath.FromSlash(rel)), append(melcgi.HeaderBlock(contentType), body...))
}

// compileTo compiles exactly out (no header block added) to path.
func compileTo(t testing.TB, path string, out []byte) {
	t.Helper()
	chunks, err := gen.Compile(out, gen.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err := mbfile.Write(path, chunks); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t testing.TB, root, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func golden(t testing.TB, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "programs", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func newTestServer(t testing.TB, root string, cfg Config) (*Server, *httptest.Server) {
	t.Helper()
	cfg.Root = root
	if cfg.Revalidate == 0 {
		cfg.Revalidate = -1 // check files on every request in tests
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s)
	t.Cleanup(func() { ts.Close(); s.Close() })
	return s, ts
}

type resp struct {
	status int
	header http.Header
	body   []byte
}

func do(t testing.TB, method, url string, body io.Reader, hdr ...string) resp {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	tr := &http.Transport{DisableCompression: true}
	res, err := (&http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return resp{res.StatusCode, res.Header, b}
}

func get(t testing.TB, url string, hdr ...string) resp {
	return do(t, http.MethodGet, url, nil, hdr...)
}

// site builds the standard test site.
func site(t testing.TB) (string, map[string][]byte) {
	root := t.TempDir()
	pages := map[string][]byte{
		"index.html":        []byte("<!doctype html><h1>Index</h1>"),
		"about.html":        []byte("<p>About Malbolge — ünïcødé ✓</p>"),
		"docs/index.html":   []byte("<p>docs index</p>"),
		"big.html":          bytes.Repeat([]byte("<p>Malbolge makes this page in many chunks.</p>\n"), 1500),
		"main-AB12CD34.js":  []byte("console.log('hashed asset')"),
		"404.html":          []byte("<h1>custom not found</h1>"),
		"data/feed.json":    []byte(`{"ok":true}`),
		".well-known/x.txt": []byte("well known"),
	}
	for rel, body := range pages {
		writeProgram(t, root, rel+".mb", mimetypeFor(rel), body)
	}
	writeFile(t, root, "static.css", []byte("body{color:red}"))
	writeFile(t, root, ".env", []byte("SECRET=1"))
	writeFile(t, root, ".git/config", []byte("secret"))
	writeFile(t, root, "echo.txt.raw.mb", golden(t, "cat-terminating.mb"))
	writeFile(t, root, "noheader.html.mb", golden(t, "hello.mb"))
	writeFile(t, root, "broken.html.mb", []byte("this is not malbolge"))
	writeFile(t, root, "cat.txt.raw.mb", golden(t, "cat.mb")) // never halts
	return root, pages
}

func mimetypeFor(rel string) string {
	switch filepath.Ext(rel) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	}
	return "text/plain; charset=utf-8"
}

func TestServesPrograms(t *testing.T) {
	root, pages := site(t)
	_, ts := newTestServer(t, root, Config{})
	for url, rel := range map[string]string{
		"/": "index.html", "/index.html": "index.html", "/about.html": "about.html", "/docs/": "docs/index.html",
		"/big.html": "big.html", "/data/feed.json": "data/feed.json", "/.well-known/x.txt": ".well-known/x.txt",
	} {
		r := get(t, ts.URL+url)
		if r.status != 200 || !bytes.Equal(r.body, pages[rel]) {
			t.Errorf("GET %s = %d (%d bytes), want %s", url, r.status, len(r.body), rel)
			continue
		}
		if r.header.Get("Content-Type") != mimetypeFor(rel) || r.header.Get("X-Powered-By") != "Malbolge" ||
			r.header.Get("X-Malbolge-Steps") == "" || r.header.Get("ETag") == "" {
			t.Errorf("GET %s headers: %v", url, r.header)
		}
	}
	r := get(t, ts.URL+"/static.css")
	if r.status != 200 || string(r.body) != "body{color:red}" || r.header.Get("Content-Type") != "text/css; charset=utf-8" ||
		r.header.Get("X-Powered-By") != "" {
		t.Errorf("static css: %d %q %v", r.status, r.body, r.header)
	}
}

func TestSecurityHeadersAndHealth(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{})
	r := get(t, ts.URL+"/")
	for k, v := range map[string]string{
		"X-Content-Type-Options": "nosniff", "X-Frame-Options": "SAMEORIGIN",
		"Referrer-Policy": "strict-origin-when-cross-origin", "Content-Security-Policy": "frame-ancestors 'self'",
	} {
		if r.header.Get(k) != v {
			t.Errorf("%s = %q, want %q", k, r.header.Get(k), v)
		}
	}
	if h := get(t, ts.URL+"/healthz"); h.status != 200 || string(h.body) != "ok\n" {
		t.Errorf("healthz: %d %q", h.status, h.body)
	}
}

func TestNotFoundAndForbiddenPaths(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{})
	for _, p := range []string{
		"/missing.html", "/index.html.mb", "/INDEX.HTML.MB", "/Index.Html", "/big.html.mb/000.mb", "/big.html.mb/",
		"/.env", "/.git/config", "/index.html::$DATA", "/index.html.", "/index.html%20", "/INDEX~1.HTM",
		"/echo.txt.raw.mb", "/melhttp.json", "/static.css/",
	} {
		r := get(t, ts.URL+p)
		if r.status != 404 && r.status != 400 {
			t.Errorf("GET %s = %d, want 404/400", p, r.status)
		}
		if bytes.Contains(r.body, []byte("SECRET")) {
			t.Errorf("GET %s leaked a secret", p)
		}
	}
	// Traversal attempts are rejected before any lookup.
	for _, p := range []string{"/../go.mod", "/%2e%2e/go.mod", "/..%2fgo.mod", "/a/../index.html", "/a\\b"} {
		req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
		req.URL.Opaque = p // send the raw path unmodified
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 400 && res.StatusCode != 404 {
			t.Errorf("raw path %s = %d", p, res.StatusCode)
		}
	}
}

func TestCustom404Page(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{})
	r := get(t, ts.URL+"/nope")
	if r.status != 404 || string(r.body) != "<h1>custom not found</h1>" || r.header.Get("Cache-Control") != "no-store" {
		t.Fatalf("custom 404: %d %q %v", r.status, r.body, r.header)
	}
}

func TestMelCGIEcho(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{})
	r := get(t, ts.URL+"/echo.txt?x=1&y=two", "X-Test", "yes", "Cookie", "session=secret")
	if r.status != 200 || r.header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("echo: %d %v", r.status, r.header)
	}
	for _, want := range []string{
		"GATEWAY_INTERFACE=MelCGI/1.0\n", "REQUEST_METHOD=GET\n", "SCRIPT_NAME=/echo.txt\n",
		"QUERY_STRING=x=1&y=two\n", "HTTP_X_TEST=yes\n",
	} {
		if !bytes.Contains(r.body, []byte(want)) {
			t.Errorf("echo missing %q:\n%s", want, r.body)
		}
	}
	if bytes.Contains(r.body, []byte("secret")) {
		t.Error("cookies must not reach programs by default")
	}
	p := do(t, http.MethodPost, ts.URL+"/echo.txt", strings.NewReader("posted-body"), "Content-Type", "text/plain")
	if !bytes.HasSuffix(p.body, []byte("\n\nposted-body")) || !bytes.Contains(p.body, []byte("CONTENT_LENGTH=11\n")) {
		t.Errorf("POST echo:\n%s", p.body)
	}
	if r.header.Get("X-Malbolge-Cache") != "miss" || r.header.Get("Cache-Control") != "no-store" {
		t.Errorf("dynamic program headers: %v", r.header)
	}
	// Dynamic programs are never served from cache.
	r2 := get(t, ts.URL+"/echo.txt?second")
	if !bytes.Contains(r2.body, []byte("QUERY_STRING=second\n")) || r2.header.Get("X-Malbolge-Cache") != "miss" {
		t.Errorf("second echo: %s", r2.body)
	}
}

func TestProgramFailures(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{MaxSteps: 200_000, Timeout: 2 * time.Second})
	if r := get(t, ts.URL+"/noheader.html"); r.status != http.StatusBadGateway {
		t.Errorf("no header block: %d, want 502", r.status)
	}
	if r := get(t, ts.URL+"/broken.html"); r.status != http.StatusInternalServerError {
		t.Errorf("invalid program: %d, want 500", r.status)
	}
	if r := get(t, ts.URL+"/cat.txt"); r.status != http.StatusInternalServerError {
		t.Errorf("step limit: %d, want 500", r.status)
	}
	_, ts2 := newTestServer(t, root, Config{Timeout: 50 * time.Millisecond})
	if r := get(t, ts2.URL+"/cat.txt"); r.status != http.StatusServiceUnavailable {
		t.Errorf("timeout: %d, want 503", r.status)
	}
	big := do(t, http.MethodPost, ts.URL+"/echo.txt", bytes.NewReader(make([]byte, 2<<20)))
	if big.status != http.StatusRequestEntityTooLarge {
		t.Errorf("huge body: %d, want 413", big.status)
	}
}

func TestMethods(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{})
	if r := do(t, http.MethodPost, ts.URL+"/static.css", nil); r.status != 405 || r.header.Get("Allow") != "GET, HEAD" {
		t.Errorf("POST static: %d %v", r.status, r.header)
	}
	if r := do(t, http.MethodDelete, ts.URL+"/", nil); r.status != 405 {
		t.Errorf("DELETE program: %d", r.status)
	}
	h := do(t, http.MethodHead, ts.URL+"/about.html", nil)
	if h.status != 200 || len(h.body) != 0 || h.header.Get("Content-Length") == "" {
		t.Errorf("HEAD: %d len=%d %v", h.status, len(h.body), h.header)
	}
}

func TestConditionalRangeAndGzip(t *testing.T) {
	root, pages := site(t)
	_, ts := newTestServer(t, root, Config{})
	r := get(t, ts.URL+"/big.html")
	etag := r.header.Get("ETag")
	if c := get(t, ts.URL+"/big.html", "If-None-Match", etag); c.status != 304 || len(c.body) != 0 {
		t.Errorf("If-None-Match: %d", c.status)
	}
	if rg := get(t, ts.URL+"/big.html", "Range", "bytes=0-9"); rg.status != 206 || !bytes.Equal(rg.body, pages["big.html"][:10]) {
		t.Errorf("Range: %d %q", rg.status, rg.body)
	}
	g := get(t, ts.URL+"/big.html", "Accept-Encoding", "br, gzip;q=0.8")
	if g.header.Get("Content-Encoding") != "gzip" || !strings.Contains(g.header.Get("Vary"), "Accept-Encoding") {
		t.Fatalf("gzip headers: %v", g.header)
	}
	zr, err := gzip.NewReader(bytes.NewReader(g.body))
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := io.ReadAll(zr)
	if !bytes.Equal(plain, pages["big.html"]) {
		t.Error("gzip body differs")
	}
	if gz := get(t, ts.URL+"/big.html", "Accept-Encoding", "gzip", "If-None-Match", g.header.Get("ETag")); gz.status != 304 {
		t.Errorf("gzip If-None-Match: %d", gz.status)
	}
	if no := get(t, ts.URL+"/big.html", "Accept-Encoding", "gzip;q=0"); no.header.Get("Content-Encoding") != "" {
		t.Error("gzip;q=0 must disable gzip")
	}
}

func TestCacheControl(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{})
	if r := get(t, ts.URL+"/main-AB12CD34.js"); r.header.Get("Cache-Control") != "public, max-age=31536000, immutable" {
		t.Errorf("hashed asset: %q", r.header.Get("Cache-Control"))
	}
	if r := get(t, ts.URL+"/about.html"); r.header.Get("Cache-Control") != "no-cache" {
		t.Errorf("page: %q", r.header.Get("Cache-Control"))
	}
}

func TestCachingAndSingleflight(t *testing.T) {
	root, _ := site(t)
	s, ts := newTestServer(t, root, Config{})
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := get(t, ts.URL+"/big.html"); r.status != 200 {
				t.Errorf("status %d", r.status)
			}
		}()
	}
	wg.Wait()
	if n := s.Runs(); n != 1 {
		t.Fatalf("20 concurrent requests ran the VM %d times, want 1", n)
	}
	if r := get(t, ts.URL+"/big.html"); r.header.Get("X-Malbolge-Cache") != "hit" {
		t.Error("expected a cache hit")
	}
	if s.Runs() != 1 {
		t.Fatal("cache hit ran the VM")
	}
}

func TestNoCache(t *testing.T) {
	root, _ := site(t)
	s, ts := newTestServer(t, root, Config{NoCache: true})
	for range 3 {
		get(t, ts.URL+"/about.html")
	}
	if s.Runs() != 3 {
		t.Fatalf("NoCache: %d runs, want 3", s.Runs())
	}
}

func TestHotReload(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{})
	get(t, ts.URL+"/about.html")
	time.Sleep(20 * time.Millisecond) // distinct mod time on coarse file systems
	writeProgram(t, root, "about.html.mb", "text/html; charset=utf-8", []byte("<p>changed</p>"))
	if r := get(t, ts.URL+"/about.html"); string(r.body) != "<p>changed</p>" {
		t.Fatalf("after change: %q", r.body)
	}
	// A brand-new page appears without a restart.
	writeProgram(t, root, "new.html.mb", "text/html; charset=utf-8", []byte("<p>new</p>"))
	if r := get(t, ts.URL+"/new.html"); r.status != 200 || string(r.body) != "<p>new</p>" {
		t.Fatalf("new page: %d %q", r.status, r.body)
	}
	// A page that grows into a chunk directory still reloads.
	big := bytes.Repeat([]byte("<i>grown</i>"), 6000)
	writeProgram(t, root, "about.html.mb", "text/html; charset=utf-8", big)
	if r := get(t, ts.URL+"/about.html"); !bytes.Equal(r.body, big) {
		t.Fatalf("grown page: %d bytes", len(r.body))
	}
}

func TestDirectoryRedirect(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{})
	r := get(t, ts.URL+"/docs?x=1")
	if r.status != 301 || r.header.Get("Location") != "/docs/?x=1" {
		t.Fatalf("dir redirect: %d %q", r.status, r.header.Get("Location"))
	}
}

func TestSPAFallback(t *testing.T) {
	root, pages := site(t)
	_, ts := newTestServer(t, root, Config{SPA: true})
	for _, p := range []string{"/dashboard", "/forms/step/2"} {
		if r := get(t, ts.URL+p); r.status != 200 || !bytes.Equal(r.body, pages["index.html"]) {
			t.Errorf("SPA %s: %d", p, r.status)
		}
	}
	if r := get(t, ts.URL+"/missing.js"); r.status != 404 {
		t.Errorf("SPA must not catch files with extensions: %d", r.status)
	}
	if r := do(t, http.MethodPost, ts.URL+"/dashboard", nil); r.status != 404 {
		t.Errorf("SPA fallback is for GET/HEAD only: %d", r.status)
	}
	// melhttp.json can enable SPA mode too.
	root2, _ := site(t)
	writeFile(t, root2, SiteConfigFile, []byte(`{"spa": true, "headers": {"X-Site": "mel"}}`))
	_, ts2 := newTestServer(t, root2, Config{})
	if r := get(t, ts2.URL+"/dashboard"); r.status != 200 || r.header.Get("X-Site") != "mel" {
		t.Errorf("site config: %d %v", r.status, r.header)
	}
}

func TestSiteConfigErrors(t *testing.T) {
	for _, cfg := range []string{`{`, `{"immutable": ["("]}`, `{"headers": {"Bad Name": "x"}}`} {
		root := t.TempDir()
		writeFile(t, root, SiteConfigFile, []byte(cfg))
		if _, err := New(Config{Root: root}); err == nil {
			t.Errorf("accepted bad site config %s", cfg)
		}
	}
	if _, err := New(Config{Root: filepath.Join(t.TempDir(), "missing")}); err == nil {
		t.Error("accepted a missing root")
	}
}

func TestExposeSource(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{ExposeSource: true})
	r := get(t, ts.URL+"/_source/about.html")
	want, _ := os.ReadFile(filepath.Join(root, "about.html.mb"))
	if r.status != 200 || !bytes.Equal(r.body, want) || r.header.Get("Content-Type") != "text/plain; charset=utf-8" {
		t.Fatalf("source: %d", r.status)
	}
	if r := get(t, ts.URL+"/_source/big.html"); r.status != 200 || r.header.Get("X-Malbolge-Programs") == "1" {
		t.Errorf("chunked source: %d %v", r.status, r.header.Get("X-Malbolge-Programs"))
	}
	for _, p := range []string{"/_source/static.css", "/_source/missing", "/_source/../.env"} {
		if r := get(t, ts.URL+p); r.status != 404 && r.status != 400 {
			t.Errorf("%s = %d", p, r.status)
		}
	}
	_, off := newTestServer(t, root, Config{})
	if r := get(t, off.URL+"/_source/about.html"); r.status != 404 {
		t.Errorf("source must be off by default: %d", r.status)
	}
}

func TestWarm(t *testing.T) {
	root, _ := site(t)
	s, ts := newTestServer(t, root, Config{MaxSteps: 1_000_000})
	st := s.Warm(context.Background())
	if st.Cached != 8 || st.Dynamic != 1 || st.Failed != 3 {
		t.Fatalf("warm stats %+v", st)
	}
	before := s.Runs()
	if r := get(t, ts.URL+"/big.html"); r.header.Get("X-Malbolge-Cache") != "hit" {
		t.Fatal("warm-up did not cache big.html")
	}
	if s.Runs() != before {
		t.Fatal("request after warm-up ran the VM")
	}
}

func TestAcceptsGzip(t *testing.T) {
	for v, want := range map[string]bool{
		"gzip": true, "deflate, gzip": true, "gzip;q=0": false, "gzip; q=0.5": true, "*": true, "br": false, "": false,
		"GZIP": true, "identity": false,
	} {
		if acceptsGzip(v) != want {
			t.Errorf("acceptsGzip(%q) = %v", v, !want)
		}
	}
}

func TestValidURLPath(t *testing.T) {
	for p, want := range map[string]bool{
		"/": true, "/a.html": true, "/dir/": true, "/a/b/c.js": true, "": false, "a": false, "/../x": false,
		"/a/./b": false, "//a": false, "/a\\b": false, "/a\x00": false, "/a\nb": false, "/a/..": false,
	} {
		if validURLPath(p) != want {
			t.Errorf("validURLPath(%q) = %v", p, !want)
		}
	}
}
