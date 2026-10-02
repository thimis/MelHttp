package build

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thimis/MelHttp/internal/malbolge"
	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/melcgi"
	"github.com/thimis/MelHttp/internal/server"
)

func write(t *testing.T, root, rel string, data []byte) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) (string, map[string][]byte) {
	src := t.TempDir()
	files := map[string][]byte{
		"index.html":               []byte("<!doctype html><h1>Built by melc</h1>"),
		"css/site.css":             []byte("h1{color:#c00}"),
		"img/pixel.png":            {0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xA9, 0xff},
		"docs/naïve café.txt":      []byte("unicode name ✓"),
		".well-known/security.txt": []byte("Contact: mailto:security@example.com\n"),
		"big.js":                   bytes.Repeat([]byte("console.log('chunk me');\n"), 3000),
	}
	for rel, data := range files {
		write(t, src, rel, data)
	}
	write(t, src, ".env", []byte("SECRET=1"))
	write(t, src, ".git/HEAD", []byte("ref"))
	hello, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "programs", "hello.mb"))
	write(t, src, "hello.txt.raw.mb", hello)
	return src, files
}

// served runs the program built for a source file and returns its response.
func served(t *testing.T, out, rel string) *melcgi.Response {
	t.Helper()
	set, err := mbfile.Load(filepath.Join(out, filepath.FromSlash(rel)+".mb"))
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := set.RunBytes(context.Background(), nil, malbolge.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := melcgi.ParseResponse(b)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestSiteBuildsEveryFile(t *testing.T) {
	src, files := fixture(t)
	out := filepath.Join(t.TempDir(), "site")
	st, err := Site(context.Background(), Options{Src: src, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if st.Files != 7 || st.Compiled != 6 || st.Copied != 1 || st.Programs < 8 {
		t.Fatalf("stats: %+v", st)
	}
	t.Log(st)
	for rel, data := range files {
		resp := served(t, out, rel)
		if !bytes.Equal(resp.Body, data) {
			t.Errorf("%s: body differs", rel)
		}
	}
	if ct := served(t, out, "css/site.css").Header.Get("Content-Type"); ct != "text/css; charset=utf-8" {
		t.Errorf("css content type %q", ct)
	}
	for _, name := range []string{".env.mb", ".git", Marker, manifestName, "melhttp.json", "hello.txt.raw.mb"} {
		_, err := os.Stat(filepath.Join(out, name))
		if (err == nil) != (name != ".env.mb" && name != ".git") {
			t.Errorf("%s: exists=%v", name, err == nil)
		}
	}
	if fi, _ := os.Stat(filepath.Join(out, "big.js.mb")); fi == nil || !fi.IsDir() {
		t.Error("big.js should be a chunk directory")
	}
}

func TestIncrementalRebuild(t *testing.T) {
	src, _ := fixture(t)
	out := filepath.Join(t.TempDir(), "site")
	if _, err := Site(context.Background(), Options{Src: src, Out: out}); err != nil {
		t.Fatal(err)
	}
	write(t, src, "css/site.css", []byte("h1{color:#00c}"))
	os.Remove(filepath.Join(src, "img", "pixel.png"))
	st, err := Site(context.Background(), Options{Src: src, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if st.Compiled != 1 || st.Reused != 4 {
		t.Fatalf("rebuild: %+v", st)
	}
	if string(served(t, out, "css/site.css").Body) != "h1{color:#00c}" {
		t.Error("changed file not rebuilt")
	}
	if _, err := os.Stat(filepath.Join(out, "img", "pixel.png.mb")); err == nil {
		t.Error("deleted source file still in output")
	}
	// A different seed changes the hash, so everything recompiles.
	st, _ = Site(context.Background(), Options{Src: src, Out: out, Seed: 5})
	if st.Compiled != 5 {
		t.Errorf("seeded rebuild compiled %d files", st.Compiled)
	}
}

func TestSafety(t *testing.T) {
	src, _ := fixture(t)
	// Never replace a directory melc did not create.
	victim := t.TempDir()
	write(t, victim, "precious.txt", []byte("keep me"))
	if _, err := Site(context.Background(), Options{Src: src, Out: victim}); err == nil {
		t.Fatal("replaced a foreign directory")
	}
	if b, _ := os.ReadFile(filepath.Join(victim, "precious.txt")); string(b) != "keep me" {
		t.Fatal("foreign directory damaged")
	}
	for _, out := range []string{src, filepath.Join(src, "out"), filepath.Dir(src)} {
		if _, err := Site(context.Background(), Options{Src: src, Out: out}); err == nil {
			t.Errorf("accepted output %s overlapping the source", out)
		}
	}
	if _, err := Site(context.Background(), Options{Src: filepath.Join(src, "index.html"), Out: t.TempDir() + "/x"}); err == nil {
		t.Error("accepted a file as source")
	}
}

func TestCaseCollision(t *testing.T) {
	src := t.TempDir()
	write(t, src, "a.html", []byte("a"))
	if err := os.WriteFile(filepath.Join(src, "A.html"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(src)
	if len(entries) < 2 {
		t.Skip("case-insensitive file system: collision cannot exist here")
	}
	if _, err := Site(context.Background(), Options{Src: src, Out: filepath.Join(t.TempDir(), "o")}); err == nil ||
		!strings.Contains(err.Error(), "case") {
		t.Fatalf("err = %v, want case collision", err)
	}
}

func TestBadHandWrittenProgram(t *testing.T) {
	src := t.TempDir()
	write(t, src, "bad.txt.raw.mb", []byte("not malbolge"))
	if _, err := Site(context.Background(), Options{Src: src, Out: filepath.Join(t.TempDir(), "o")}); err == nil {
		t.Fatal("accepted an invalid hand-written program")
	}
}

func TestSiteConfig(t *testing.T) {
	src, _ := fixture(t)
	write(t, src, "melhttp.json", []byte(`{"headers": {"X-Site": "mel"}}`))
	out := filepath.Join(t.TempDir(), "site")
	yes := true
	if _, err := Site(context.Background(), Options{Src: src, Out: out, SPA: &yes}); err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	data, _ := os.ReadFile(filepath.Join(out, "melhttp.json"))
	json.Unmarshal(data, &cfg)
	if cfg["spa"] != true || cfg["headers"].(map[string]any)["X-Site"] != "mel" {
		t.Fatalf("site config %s", data)
	}
	if _, err := os.Stat(filepath.Join(out, "melhttp.json.mb")); err == nil {
		t.Error("melhttp.json must not be compiled into a page")
	}
}

func TestDetect(t *testing.T) {
	for name, files := range map[string]map[string]string{
		"angular": {"angular.json": "{}"},
		"next":    {"package.json": `{"dependencies":{"next":"15","react":"19"}}`},
		"nuxt":    {"package.json": `{"devDependencies":{"nuxt":"3"}}`},
		"astro":   {"package.json": `{"dependencies":{"astro":"5"}}`},
		"gatsby":  {"package.json": `{"dependencies":{"gatsby":"5"}}`},
		"cra":     {"package.json": `{"dependencies":{"react-scripts":"5"}}`},
		"vite":    {"package.json": `{"devDependencies":{"vite":"7","vue":"3"}}`},
		"hugo":    {"hugo.toml": "title='x'"},
		"jekyll":  {"_config.yml": "x: 1", "Gemfile": "gem 'jekyll'"},
		"static":  {"index.html": "<p>"},
	} {
		dir := t.TempDir()
		for rel, data := range files {
			write(t, dir, rel, []byte(data))
		}
		if got := Detect(dir).Name; got != name {
			t.Errorf("Detect(%v) = %s, want %s", files, got, name)
		}
	}
}

func TestResolveAndOutputDir(t *testing.T) {
	for alias, want := range map[string]string{"react": "vite", "vue": "vite", "svelte": "vite", "nextjs": "next", "static": "static"} {
		if p, err := Resolve(alias, "."); err != nil || p.Name != want {
			t.Errorf("Resolve(%s) = %v %v", alias, p.Name, err)
		}
	}
	if _, err := Resolve("cobol", "."); err == nil {
		t.Error("accepted unknown preset")
	}
	proj := t.TempDir()
	write(t, proj, "angular.json", []byte(`{"projects":{"shop":{"architect":{"build":{"options":{"outputPath":"dist/shop"}}}}}}`))
	write(t, proj, "dist/shop/browser/index.html", []byte("x"))
	dir, err := Presets["angular"].OutputDir(proj)
	if err != nil || filepath.ToSlash(dir) != filepath.ToSlash(filepath.Join(proj, "dist", "shop", "browser")) {
		t.Fatalf("angular output %q %v", dir, err)
	}
	obj := t.TempDir()
	write(t, obj, "angular.json", []byte(`{"projects":{"x":{"architect":{"build":{"options":{"outputPath":{"base":"build/x"}}}}}}}`))
	write(t, obj, "build/x/browser/index.html", []byte("x"))
	if dir, err := Presets["angular"].OutputDir(obj); err != nil || !strings.HasSuffix(filepath.ToSlash(dir), "build/x/browser") {
		t.Fatalf("angular object outputPath: %q %v", dir, err)
	}
	if _, err := Presets["vite"].OutputDir(t.TempDir()); err == nil || !strings.Contains(err.Error(), "--run-build") {
		t.Errorf("missing output: %v", err)
	}
	if len(PresetNames()) < 15 || PresetNames()[0] != "auto" {
		t.Errorf("PresetNames = %v", PresetNames())
	}
}

func TestWASIPassThrough(t *testing.T) {
	src := t.TempDir()
	write(t, src, "index.html", []byte("<p>x</p>"))
	module := []byte("\x00asm\x01\x00\x00\x00")
	write(t, src, "api/hello.txt.wasi", module)
	out := filepath.Join(t.TempDir(), "o")
	st, err := Site(context.Background(), Options{Src: src, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(out, "api", "hello.txt.wasi"))
	if !bytes.Equal(got, module) || st.Copied != 1 {
		t.Fatalf("module not copied verbatim: %q %+v", got, st)
	}
	write(t, src, "bad.wasi", []byte("not wasm"))
	if _, err := Site(context.Background(), Options{Src: src, Out: out}); err == nil {
		t.Fatal("accepted a non-wasm .wasi file")
	}
}

// TestRebuildWhileServing: a site can be rebuilt while melhttpd serves it
// (on Windows replacing the directory would fail); changed pages update,
// deleted pages disappear, and unchanged outputs keep their timestamps so
// the server's cache stays warm.
func TestRebuildWhileServing(t *testing.T) {
	src, _ := fixture(t)
	out := filepath.Join(t.TempDir(), "site")
	if _, err := Site(context.Background(), Options{Src: src, Out: out}); err != nil {
		t.Fatal(err)
	}
	srv, err := server.New(server.Config{Root: out, Revalidate: -1})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	ts := httptest.NewServer(srv)
	defer ts.Close()
	body := func(p string) (int, string) {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, b := body("/css/site.css"); code != 200 || b != "h1{color:#c00}" {
		t.Fatalf("before: %d %q", code, b)
	}
	before, _ := os.Stat(filepath.Join(out, "index.html.mb"))
	time.Sleep(20 * time.Millisecond)
	write(t, src, "css/site.css", []byte("h1{color:#0a0}"))
	os.Remove(filepath.Join(src, "img", "pixel.png"))
	write(t, src, "new.txt", []byte("fresh"))
	st, err := Site(context.Background(), Options{Src: src, Out: out})
	if err != nil {
		t.Fatalf("rebuild while serving: %v", err)
	}
	if st.Compiled != 2 {
		t.Errorf("rebuild compiled %d files, want 2", st.Compiled)
	}
	if code, b := body("/css/site.css"); code != 200 || b != "h1{color:#0a0}" {
		t.Errorf("changed page: %d %q", code, b)
	}
	if code, _ := body("/img/pixel.png"); code != 404 {
		t.Errorf("deleted page still served: %d", code)
	}
	if code, b := body("/new.txt"); code != 200 || b != "fresh" {
		t.Errorf("new page: %d %q", code, b)
	}
	after, _ := os.Stat(filepath.Join(out, "index.html.mb"))
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("an unchanged output was rewritten")
	}
	if entries, _ := filepath.Glob(out + ".tmp-*"); len(entries) > 0 {
		t.Errorf("staging left behind: %v", entries)
	}
}
