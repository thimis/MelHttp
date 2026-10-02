package server

import (
	"bytes"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/obfs"
)

func TestInjectTransport(t *testing.T) {
	tag := transportScript
	for _, c := range []struct{ in, want string }{
		{"<html><head><title>x</title></head><body>hi</body></html>",
			"<html><head><title>x</title>" + tag + "</head><body>hi</body></html>"},
		{"<!DOCTYPE html><HTML><HEAD></HEAD><BODY></BODY>", "<!DOCTYPE html><HTML><HEAD>" + tag + "</HEAD><BODY></BODY>"},
		{"<head>\n</head \n>", "<head>\n" + tag + "</head \n>"},
		// No </head>: appended, never placed before the doctype.
		{"<!doctype html><h1>Index</h1>", "<!doctype html><h1>Index</h1>" + tag},
		{"", tag},
		// Byte offsets stay right after characters whose lower case is longer (İ).
		{"<title>İİİ</title></head>", "<title>İİİ</title>" + tag + "</head>"},
		// Already opted in: unchanged.
		{`<head><script src="/_melhttp/obfuscate.js"></script></head>`, `<head><script src="/_melhttp/obfuscate.js"></script></head>`},
	} {
		if got := string(injectTransport([]byte(c.in))); got != c.want {
			t.Errorf("injectTransport(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
}

func TestObfuscateInjectServesTaggedHTML(t *testing.T) {
	root, pages := site(t)
	_, ts := newTestServer(t, root, Config{ObfuscateInject: true})
	tag := []byte(transportScript)

	r := get(t, ts.URL+"/", "Accept-Encoding", "identity")
	if r.status != 200 || !bytes.Equal(r.body, append(append([]byte{}, pages["index.html"]...), tag...)) {
		t.Fatalf("index: %d %q", r.status, r.body)
	}
	// The ETag belongs to the injected page, and conditional requests work.
	if c := get(t, ts.URL+"/", "If-None-Match", r.header.Get("ETag")); c.status != 304 {
		t.Errorf("If-None-Match: %d", c.status)
	}
	// gzip variant carries the tag too (big.html is compressible).
	if g := get(t, ts.URL+"/big.html"); !bytes.HasSuffix(g.body, tag) || !bytes.HasPrefix(g.body, pages["big.html"]) {
		t.Errorf("big.html (gzip) is not the page plus the tag")
	}
	// The custom 404 page is HTML too.
	if nf := get(t, ts.URL+"/missing"); nf.status != 404 || !bytes.Contains(nf.body, tag) {
		t.Errorf("404: %d %q", nf.status, nf.body)
	}
	// Other types are untouched.
	for _, p := range []string{"main-AB12CD34.js", "data/feed.json"} {
		if g := get(t, ts.URL+"/"+p); !bytes.Equal(g.body, pages[p]) {
			t.Errorf("%s changed: %q", p, g.body)
		}
	}
	// -obfuscate-inject implies -obfuscate: the script it adds is served,
	// and pages travel as Malbolge on request.
	if js := get(t, ts.URL+"/_melhttp/obfuscate.js"); js.status != 200 || !strings.Contains(string(js.body), "serviceWorker") {
		t.Fatalf("obfuscate.js: %d", js.status)
	}
	if e := get(t, ts.URL+"/about.html", obfs.AcceptHeader, obfs.Program); e.header.Get(obfs.EncodingHeader) != obfs.Program {
		t.Errorf("transport is not on: %v", e.header)
	}
}

func TestNoInjectionByDefault(t *testing.T) {
	root, pages := site(t)
	for _, cfg := range []Config{{}, {Obfuscate: true}} {
		_, ts := newTestServer(t, root, cfg)
		if r := get(t, ts.URL+"/about.html"); !bytes.Equal(r.body, pages["about.html"]) {
			t.Errorf("%+v: page changed: %q", cfg, r.body)
		}
	}
}
