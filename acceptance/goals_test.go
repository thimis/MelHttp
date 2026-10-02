//go:build acceptance

package acceptance

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thimis/MelHttp/internal/crawl"
)

// ---------------------------------------------------------------------------
// G1 — the VM runs real Malbolge exactly like the reference interpreter.
// ---------------------------------------------------------------------------

func TestG1_VMRunsRealMalbolge(t *testing.T) {
	requireBinaries(t)
	t.Run("hello", func(t *testing.T) {
		out := melc(t, nil, "run", "testdata/programs/hello.mb")
		if string(out) != "Hello, world." {
			t.Fatalf("hello printed %q", out)
		}
	})
	t.Run("cat-eof-and-step-limit", func(t *testing.T) {
		out, _, err := melcErr([]byte("abc"), "run", "-steps", "200000", "testdata/programs/cat.mb")
		if err == nil {
			t.Fatal("non-terminating cat should stop with a step-limit error")
		}
		if !bytes.HasPrefix(out, []byte("abc\xa8\xa8")) {
			t.Fatalf("cat output starts %q, want abc then 0xA8 (EOF = 59048)", out[:min(len(out), 16)])
		}
	})
	t.Run("check", func(t *testing.T) {
		melc(t, nil, "check", "testdata/programs/hello.mb")
		bad := writeFile(t, t.TempDir(), "bad.mb", []byte("this is not malbolge"))
		if _, _, err := melcErr(nil, "check", bad); err == nil {
			t.Fatal("check accepted an invalid program")
		}
	})
	t.Run("reference-differential", func(t *testing.T) {
		if !have("docker") {
			t.Fatal("docker is required for the reference-interpreter differential test")
		}
		goTest(t, "-tags", "reference", "-run", "TestReference", "./internal/malbolge")
	})
}

// ---------------------------------------------------------------------------
// G2 — the generator converts ANY bytes into Malbolge that prints them back.
// ---------------------------------------------------------------------------

func TestG2_GeneratorRoundTripsAnyBytes(t *testing.T) {
	requireBinaries(t)
	t.Run("completeness-proof", func(t *testing.T) {
		goTest(t, "-run", "TestGeneratorCompleteness", "./internal/gen")
	})
	dir := t.TempDir()
	all := make([]byte, 0, 512)
	for i := range 256 {
		all = append(all, byte(i))
	}
	for i := 255; i >= 0; i-- {
		all = append(all, byte(i))
	}
	random := make([]byte, 1<<20)
	rand.Read(random)
	inputs := map[string][]byte{
		"empty.bin":   {},
		"one.bin":     {0x00},
		"all256.bin":  all,
		"random.bin":  random,
		"hello.html":  []byte("<!doctype html><title>hi</title><p>Hello, Malbolge! — ünïcødé ✓</p>\r\n"),
		"newline.txt": []byte("line1\nline2\r\nline3\n"),
	}
	corpus, _ := filepath.Glob(filepath.Join(repoRoot, "testdata", "corpus", "*"))
	if len(corpus) == 0 {
		t.Error("testdata/corpus is empty; the real-world corpus must be present")
	}
	for _, p := range corpus {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		inputs["corpus-"+filepath.Base(p)] = b
	}
	for name, data := range inputs {
		t.Run(name, func(t *testing.T) {
			src := writeFile(t, dir, "in/"+name, data)
			dst := filepath.Join(dir, "out", name+".mb")
			melc(t, nil, "gen", "-raw", "-o", dst, src)
			got := melc(t, nil, "run", "-steps", "0", dst)
			if !bytes.Equal(got, data) {
				t.Fatalf("round trip differs: got %d bytes, want %d", len(got), len(data))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// G3 — MelCGI: programs see the request, bad responses become 502.
// ---------------------------------------------------------------------------

func TestG3_MelCGIContract(t *testing.T) {
	requireBinaries(t)
	site := t.TempDir()
	cat, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "programs", "cat-terminating.mb"))
	if err != nil {
		t.Fatal(err)
	}
	hello, err := os.ReadFile(filepath.Join(repoRoot, "testdata", "programs", "hello.mb"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, site, "echo.txt.raw.mb", cat)
	writeFile(t, site, "noheader.html.mb", hello) // prints no CGI header block → 502
	page := writeFile(t, t.TempDir(), "ok.html", []byte("<p>ok</p>"))
	melc(t, nil, "gen", "-o", filepath.Join(site, "ok.html.mb"), page)
	base := startServer(t, site)

	r := do(t, http.MethodGet, base+"/echo.txt?x=1&y=two", map[string]string{"X-Test": "yes"})
	if r.Status != 200 {
		t.Fatalf("echo status %d: %s", r.Status, r.Body)
	}
	for _, want := range []string{
		"GATEWAY_INTERFACE=MelCGI/1.0\n", "REQUEST_METHOD=GET\n", "SCRIPT_NAME=/echo.txt\n",
		"QUERY_STRING=x=1&y=two\n", "SERVER_PROTOCOL=HTTP/1.1\n", "HTTP_X_TEST=yes\n",
	} {
		if !bytes.Contains(r.Body, []byte(want)) {
			t.Errorf("echo body missing %q:\n%s", want, r.Body)
		}
	}
	if ct := r.Header.Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("echo content-type %q", ct)
	}

	req, _ := http.NewRequest(http.MethodPost, base+"/echo.txt", strings.NewReader("posted-body"))
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(body, []byte("REQUEST_METHOD=POST\n")) || !bytes.HasSuffix(body, []byte("\n\nposted-body")) ||
		!bytes.Contains(body, []byte("CONTENT_LENGTH=11\n")) {
		t.Errorf("POST echo body:\n%s", body)
	}

	if r := get(t, base+"/noheader.html"); r.Status != http.StatusBadGateway {
		t.Errorf("program without header block: status %d, want 502", r.Status)
	}
	if r := get(t, base+"/ok.html"); r.Status != 200 || string(r.Body) != "<p>ok</p>" {
		t.Errorf("generated page: %d %q", r.Status, r.Body)
	}
	t.Run("fuzz", func(t *testing.T) {
		goTest(t, "-run", "^$", "-fuzz", "FuzzParseResponse", "-fuzztime", "20s", "./internal/melcgi")
		goTest(t, "-run", "^$", "-fuzz", "FuzzEncodeRequest", "-fuzztime", "20s", "./internal/melcgi")
	})
}

// ---------------------------------------------------------------------------
// G4 — the server is correct and safe on a hand-made site.
// ---------------------------------------------------------------------------

func TestG4_ServerCorrectAndSafe(t *testing.T) {
	requireBinaries(t)
	site := t.TempDir()
	src := t.TempDir()
	html := []byte("<!doctype html><html><body><h1>Index</h1></body></html>")
	big := bytes.Repeat([]byte("<p>Malbolge makes this page in many chunks.</p>\n"), 1200) // ~58 KB
	melc(t, nil, "gen", "-o", filepath.Join(site, "index.html.mb"), writeFile(t, src, "index.html", html))
	melc(t, nil, "gen", "-o", filepath.Join(site, "big.html.mb"), writeFile(t, src, "big.html", big))
	writeFile(t, site, "plain.css", []byte("body{color:red}"))
	writeFile(t, site, ".env", []byte("SECRET=1"))
	writeFile(t, site, ".well-known/security.txt", []byte("Contact: mailto:x@example.com\n"))
	if fi, err := os.Stat(filepath.Join(site, "big.html.mb")); err != nil || !fi.IsDir() {
		t.Fatalf("a 58 KB page should be generated as a chunk directory (err=%v)", err)
	}
	base := startServer(t, site)

	r := get(t, base+"/")
	if r.Status != 200 || !bytes.Equal(r.Body, html) {
		t.Fatalf("GET / = %d %q", r.Status, r.Body)
	}
	if r.Header.Get("Content-Type") != "text/html; charset=utf-8" || r.Header.Get("X-Powered-By") != "Malbolge" ||
		r.Header.Get("X-Malbolge-Steps") == "" || r.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("GET / headers: %v", r.Header)
	}
	if r := get(t, base+"/big.html"); r.Status != 200 || !bytes.Equal(r.Body, big) {
		t.Errorf("chunked page: status %d, %d bytes (want %d)", r.Status, len(r.Body), len(big))
	}
	if r := get(t, base+"/plain.css"); r.Status != 200 || string(r.Body) != "body{color:red}" ||
		r.Header.Get("Content-Type") != "text/css; charset=utf-8" {
		t.Errorf("static css: %d %q %v", r.Status, r.Body, r.Header)
	}
	if r := get(t, base+"/.well-known/security.txt"); r.Status != 200 {
		t.Errorf(".well-known: status %d", r.Status)
	}
	for _, p := range []string{
		"/missing.html", "/index.html.mb", "/INDEX.HTML.MB", "/Index.Html.Mb", "/big.html.mb/000.mb",
		"/.env", "/../go.mod", "/%2e%2e/go.mod", "/..%2fgo.mod", "/index.html::$DATA", "/index.html.",
		"/index.html%20", "/INDEX~1.HTM",
	} {
		if r := get(t, base+p); r.Status != http.StatusNotFound && r.Status != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 404/400 (body %q)", p, r.Status, r.Body)
		}
	}
	if r := do(t, http.MethodPost, base+"/plain.css", nil); r.Status != http.StatusMethodNotAllowed {
		t.Errorf("POST static = %d, want 405", r.Status)
	}
	h := do(t, http.MethodHead, base+"/", nil)
	if h.Status != 200 || len(h.Body) != 0 || h.Header.Get("Content-Length") != fmt.Sprint(len(html)) {
		t.Errorf("HEAD / = %d len(body)=%d Content-Length=%q", h.Status, len(h.Body), h.Header.Get("Content-Length"))
	}
	etag := r.Header.Get("ETag")
	if etag == "" {
		t.Fatal("no ETag on GET /")
	}
	if r := do(t, http.MethodGet, base+"/", map[string]string{"If-None-Match": etag}); r.Status != http.StatusNotModified {
		t.Errorf("If-None-Match = %d, want 304", r.Status)
	}
	g := do(t, http.MethodGet, base+"/big.html", map[string]string{"Accept-Encoding": "gzip"})
	if g.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("no gzip for big html: %v", g.Header)
	} else {
		zr, err := gzip.NewReader(bytes.NewReader(g.Body))
		if err != nil {
			t.Fatal(err)
		}
		plain, _ := io.ReadAll(zr)
		if !bytes.Equal(plain, big) {
			t.Error("gzip body does not decompress to the page")
		}
	}
	if r := do(t, http.MethodGet, base+"/", map[string]string{"Range": "bytes=0-8"}); r.Status != http.StatusPartialContent ||
		string(r.Body) != string(html[:9]) {
		t.Errorf("Range = %d %q", r.Status, r.Body)
	}
}

// ---------------------------------------------------------------------------
// G5 — melc build + melhttpd serve every file of every simple test site exactly.
// ---------------------------------------------------------------------------

func TestG5_BuiltSitesServedByteIdentical(t *testing.T) {
	requireBinaries(t)
	for _, name := range []string{"hello", "classic", "cgi"} {
		t.Run(name, func(t *testing.T) {
			src := filepath.Join(repoRoot, "testsites", name)
			if _, err := os.Stat(src); err != nil {
				t.Fatalf("test site missing: %v", err)
			}
			out := filepath.Join(t.TempDir(), "site")
			melc(t, nil, "build", "-o", out, src)
			assertNoPlainContent(t, out)
			base := startServer(t, out)
			rep, err := crawl.Site(context.Background(), base, src, crawl.Options{Skip: isProgramPath})
			if err != nil {
				t.Fatal(err)
			}
			if rep.Checked == 0 || !rep.OK() {
				t.Fatal(rep)
			}
			t.Log(rep)
		})
	}
}

// isProgramPath skips hand-written Malbolge programs in a source tree: they are
// served by running them, not as content.
func isProgramPath(rel string) bool {
	return strings.HasSuffix(strings.ToLower(rel), ".mb") || strings.Contains(strings.ToLower(rel), ".mb/")
}

// assertNoPlainContent checks the "everything Malbolge" rule: a built site
// contains only Malbolge programs (plus melhttp.json).
func assertNoPlainContent(t *testing.T, root string) {
	// Dotfiles (build marker, manifest) are never served.
	t.Helper()
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(filepath.Base(p), ".") {
			return nil
		}
		if rel != "melhttp.json" && !isProgramPath(rel) {
			t.Errorf("built site contains non-Malbolge file %s", rel)
		}
		return nil
	})
}

// ---------------------------------------------------------------------------
// G6 — the Angular Material showcase runs entirely from Malbolge.
// ---------------------------------------------------------------------------

func TestG6_AngularShowcase(t *testing.T) {
	requireBinaries(t)
	proj := filepath.Join(repoRoot, "testsites", "angular-showcase")
	out := buildFrontend(t, proj, "angular")
	start := time.Now()
	base := startServer(t, out, "-expose-source")
	t.Logf("startup incl. warm-up: %v", time.Since(start))
	dist := filepath.Join(proj, "dist", "angular-showcase", "browser")
	assertCrawl(t, base, dist)
	assertDeepLink(t, base, dist, "/dashboard", "/forms", "/theming", "/malbolge")
	if r := get(t, base+"/_source/index.html"); r.Status != 200 || !bytes.Contains(r.Body, []byte("(")) {
		t.Errorf("/_source/index.html = %d", r.Status)
	}
	runPlaywright(t, proj, base)
}

// ---------------------------------------------------------------------------
// G7 — warm cached serving is close to a plain Go static file server.
// ---------------------------------------------------------------------------

func TestG7_PerformanceNearStatic(t *testing.T) {
	requireBinaries(t)
	src, site := t.TempDir(), t.TempDir()
	page := bytes.Repeat([]byte("<div class=\"card\">Malbolge performance test</div>\n"), 1300) // ~64 KB
	writeFile(t, src, "page.html", page)
	melc(t, nil, "gen", "-o", filepath.Join(site, "page.html.mb"), filepath.Join(src, "page.html"))
	base := startServer(t, site)
	static := httptest.NewServer(http.FileServer(http.Dir(src)))
	defer static.Close()

	const n, workers = 3000, 16
	mel := throughput(t, base+"/page.html", n, workers, page)
	ref := throughput(t, static.URL+"/page.html", n, workers, page)
	ratio := mel / ref
	t.Logf("melhttpd %.0f req/s, http.FileServer %.0f req/s, ratio %.2f", mel, ref, ratio)
	if ratio < 0.5 {
		t.Errorf("cached Malbolge serving is %.2fx of static (want >= 0.5; target 0.7)", ratio)
	}
}

func throughput(t *testing.T, url string, n, workers int, want []byte) float64 {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{DisableCompression: true, MaxIdleConnsPerHost: workers}}
	var next, bad atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for next.Add(1) <= int64(n) {
				resp, err := client.Get(url)
				if err != nil {
					bad.Add(1)
					continue
				}
				b, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				if !bytes.Equal(b, want) {
					bad.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	if bad.Load() > 0 {
		t.Fatalf("%s: %d bad responses", url, bad.Load())
	}
	return float64(n) / time.Since(start).Seconds()
}

// ---------------------------------------------------------------------------
// G8 — Docker image builds, runs healthy, serves the sites.
// ---------------------------------------------------------------------------

func TestG8_Docker(t *testing.T) {
	if os.Getenv("MELHTTP_DOCKER") != "1" {
		t.Skip("set MELHTTP_DOCKER=1 to run the Docker goal (builds images, takes minutes)")
	}
	if !have("docker") {
		t.Fatal("docker not found")
	}
	run(t, repoRoot, 30*time.Minute, "docker", "compose", "up", "--build", "--detach", "--wait")
	t.Cleanup(func() { run(t, repoRoot, 5*time.Minute, "docker", "compose", "down") })
	sites := map[string]string{"8081": "classic", "8083": "hello"}
	for port, name := range sites {
		base := "http://127.0.0.1:" + port
		rep, err := crawl.Site(context.Background(), base, filepath.Join(repoRoot, "testsites", name), crawl.Options{Skip: isProgramPath})
		if err != nil || !rep.OK() || rep.Checked == 0 {
			t.Errorf("%s in docker: %v %s", name, err, rep)
		}
	}
	dist := filepath.Join(repoRoot, "testsites", "angular-showcase", "dist", "angular-showcase", "browser")
	if _, err := os.Stat(dist); err == nil {
		assertDeepLink(t, "http://127.0.0.1:8080", dist, "/dashboard")
	} else if r := get(t, "http://127.0.0.1:8080/"); r.Status != 200 {
		t.Errorf("angular container GET / = %d", r.Status)
	}
}

// ---------------------------------------------------------------------------
// G9 — every OS/arch builds; the VM and generator compile to WebAssembly.
// ---------------------------------------------------------------------------

func TestG9_CrossPlatformBuilds(t *testing.T) {
	targets := []string{
		"linux/amd64", "linux/arm64", "windows/amd64", "windows/arm64",
		"darwin/amd64", "darwin/arm64", "freebsd/amd64", "freebsd/arm64",
	}
	for _, tg := range targets {
		t.Run(strings.ReplaceAll(tg, "/", "-"), func(t *testing.T) {
			goos, goarch, _ := strings.Cut(tg, "/")
			crossBuild(t, goos, goarch, "./cmd/melc", "./cmd/melhttpd")
		})
	}
	t.Run("js-wasm", func(t *testing.T) { crossBuild(t, "js", "wasm", "./internal/malbolge", "./internal/gen") })
	t.Run("wasip1", func(t *testing.T) { crossBuild(t, "wasip1", "wasm", "./internal/malbolge", "./internal/gen") })
	t.Run("dist-tool", func(t *testing.T) {
		if _, err := os.Stat(filepath.Join(repoRoot, "tools", "dist", "main.go")); err != nil {
			t.Fatal("tools/dist is missing")
		}
	})
}

func crossBuild(t *testing.T, goos, goarch string, pkgs ...string) {
	t.Helper()
	cmd := exec.Command("go", append([]string{"build", "-o", t.TempDir() + string(os.PathSeparator)}, pkgs...)...)
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("GOOS=%s GOARCH=%s go build %v: %v\n%s", goos, goarch, pkgs, err, out)
	}
}

// ---------------------------------------------------------------------------
// G10 — other frameworks (React, Vue) convert via presets.
// ---------------------------------------------------------------------------

func TestG10_FrameworkPresets(t *testing.T) {
	requireBinaries(t)
	for _, name := range []string{"react-vite", "vue-vite"} {
		t.Run(name, func(t *testing.T) {
			proj := filepath.Join(repoRoot, "testsites", name)
			out := buildFrontend(t, proj, "auto")
			base := startServer(t, out)
			dist := filepath.Join(proj, "dist")
			assertCrawl(t, base, dist)
			assertDeepLink(t, base, dist, "/about")
			runPlaywright(t, proj, base)
		})
	}
}

// ---------------------------------------------------------------------------
// G11 — the repository is safe to publish.
// ---------------------------------------------------------------------------

func TestG11_RepoHygiene(t *testing.T) {
	out := run(t, repoRoot, time.Minute, "git", "ls-files")
	forbidden := regexp.MustCompile(`(^|/)(\.claude/|CLAUDE(\.local)?\.md$|\.mcp\.json$|\.env$|\.env\.|node_modules/|dist/|id_(rsa|ed25519|ecdsa|dsa))|\.(pem|key|p12|pfx|jks|kdbx)$`)
	for _, f := range strings.Split(strings.TrimSpace(out), "\n") {
		if f != "" && forbidden.MatchString(f) && !strings.HasSuffix(f, ".env.example") {
			t.Errorf("tracked file must not be published: %s", f)
		}
	}
	grep := exec.Command("git", "grep", "-l", "-I", "-E", "-----BEGIN ([A-Z]+ )?PRIVATE KEY-----")
	grep.Dir = repoRoot
	if b, _ := grep.Output(); len(bytes.TrimSpace(b)) > 0 {
		t.Errorf("tracked files contain private keys:\n%s", b)
	}
	readme, err := os.ReadFile(filepath.Join(repoRoot, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	links := regexp.MustCompile(`\]\(((?:docs/|\./docs/)[^)#\s]+)`).FindAllSubmatch(readme, -1)
	if len(links) == 0 {
		t.Error("README has no links into docs/")
	}
	for _, l := range links {
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(string(l[1])))); err != nil {
			t.Errorf("README links to missing %s", l[1])
		}
	}
}

// ---------------------------------------------------------------------------
// shared helpers for the goals above
// ---------------------------------------------------------------------------

func goTest(t *testing.T, args ...string) {
	t.Helper()
	run(t, repoRoot, 20*time.Minute, "go", append([]string{"test"}, args...)...)
}

func run(t *testing.T, dir string, timeout time.Duration, name string, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
	return string(out)
}

// buildFrontend builds a Node-based test site and converts it with melc.
func buildFrontend(t *testing.T, proj, preset string) string {
	t.Helper()
	if !have("npm") {
		t.Fatal("npm is required for frontend test sites")
	}
	if _, err := os.Stat(filepath.Join(proj, "package.json")); err != nil {
		t.Fatalf("test site missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "node_modules")); err != nil {
		run(t, proj, 15*time.Minute, npmCmd(), "ci", "--no-audit", "--no-fund")
	}
	out := filepath.Join(t.TempDir(), "site")
	melc(t, nil, "build", "--preset", preset, "--run-build", "-o", out, proj)
	assertNoPlainContent(t, out)
	return out
}

func assertCrawl(t *testing.T, base, dist string) {
	t.Helper()
	rep, err := crawl.Site(context.Background(), base, dist, crawl.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked == 0 || !rep.OK() {
		t.Fatal(rep)
	}
	t.Log(rep)
}

func assertDeepLink(t *testing.T, base, dist string, paths ...string) {
	t.Helper()
	index, err := os.ReadFile(filepath.Join(dist, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		r := get(t, base+p)
		if r.Status != 200 || !bytes.Equal(r.Body, index) {
			t.Errorf("deep link %s = %d (%d bytes), want index.html", p, r.Status, len(r.Body))
		}
	}
	if r := get(t, base+"/no-such-file.js"); r.Status != http.StatusNotFound {
		t.Errorf("missing asset with extension = %d, want 404 (no SPA fallback for files)", r.Status)
	}
}

func runPlaywright(t *testing.T, proj, base string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, npxCmd(), "playwright", "test", "--reporter=line")
	cmd.Dir = proj
	cmd.Env = append(os.Environ(), "BASE_URL="+base)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("playwright: %v\n%s", err, out)
	}
}

func npmCmd() string {
	if isWindows() {
		return "npm.cmd"
	}
	return "npm"
}

func npxCmd() string {
	if isWindows() {
		return "npx.cmd"
	}
	return "npx"
}

func isWindows() bool { return os.PathSeparator == '\\' }
