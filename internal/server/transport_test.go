package server

import (
	"bytes"
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/malbolge"
	"github.com/thimis/MelHttp/internal/obfs"
	"github.com/thimis/MelHttp/internal/webvm"
)

func TestTransportEncoding(t *testing.T) {
	if err := webvm.Available(); err != nil {
		t.Fatal(err)
	}
	root, pages := site(t)
	_, ts := newTestServer(t, root, Config{Obfuscate: true, ObfuscateVariants: 3})

	plain := get(t, ts.URL+"/about.html")
	if !strings.Contains(plain.header.Get("Vary"), obfs.AcceptHeader) || plain.header.Get(obfs.EncodingHeader) != "" {
		t.Fatalf("plain response headers: %v", plain.header)
	}
	seen := map[string]bool{}
	for range 12 {
		r := get(t, ts.URL+"/about.html", obfs.AcceptHeader, obfs.Program)
		if r.status != 200 || r.header.Get(obfs.EncodingHeader) != obfs.Program || r.header.Get("Content-Type") != obfs.MediaType ||
			r.header.Get(obfs.ContentTypeHeader) != "text/html; charset=utf-8" || r.header.Get("Cache-Control") != "no-store" {
			t.Fatalf("encoded response: %d %v", r.status, r.header)
		}
		if bytes.Contains(r.body, []byte("About")) {
			t.Fatal("the page text is visible in the encoded body")
		}
		if _, err := malbolge.Load(bytes.SplitN(r.body, []byte("\n\n"), 2)[0]); err != nil {
			t.Fatalf("body is not Malbolge: %v", err)
		}
		dec, err := obfs.Decode(context.Background(), r.body, 0)
		if err != nil || !bytes.Equal(dec, pages["about.html"]) {
			t.Fatalf("decoded %q, %v", dec, err)
		}
		seen[string(r.body)] = true
	}
	if len(seen) < 2 {
		t.Errorf("12 requests produced %d distinct encodings, want variation", len(seen))
	}
	// Binary, chunked, gzip and HEAD.
	r := get(t, ts.URL+"/big.html", obfs.AcceptHeader, obfs.Program, "Accept-Encoding", "gzip")
	if r.header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("no gzip: %v", r.header)
	}
	if h := do(t, http.MethodHead, ts.URL+"/big.html", nil, obfs.AcceptHeader, obfs.Program); h.status != 200 || len(h.body) != 0 {
		t.Errorf("HEAD: %d", h.status)
	}
	// Dynamic programs and errors.
	echo := get(t, ts.URL+"/echo.txt?q=1", obfs.AcceptHeader, obfs.Program)
	dec, err := obfs.Decode(context.Background(), echo.body, 0)
	if err != nil || !bytes.Contains(dec, []byte("QUERY_STRING=q=1")) {
		t.Errorf("dynamic: %v %q", err, dec)
	}
	if nf := get(t, ts.URL+"/missing", obfs.AcceptHeader, obfs.Program); nf.status != 404 || nf.header.Get(obfs.EncodingHeader) != "" {
		t.Errorf("404 must not be encoded: %d %v", nf.status, nf.header)
	}
	// Assets.
	if sw := get(t, ts.URL+"/_melhttp/sw.js"); sw.status != 200 || sw.header.Get("Service-Worker-Allowed") != "/" {
		t.Errorf("sw.js: %d", sw.status)
	}
	if pg := get(t, ts.URL+"/_melhttp/playground.html"); pg.status != 404 {
		t.Errorf("playground must be off: %d", pg.status)
	}
}

func TestTransportOffByDefault(t *testing.T) {
	root, pages := site(t)
	_, ts := newTestServer(t, root, Config{})
	r := get(t, ts.URL+"/about.html", obfs.AcceptHeader, obfs.Program)
	if r.header.Get(obfs.EncodingHeader) != "" || !bytes.Equal(r.body, pages["about.html"]) {
		t.Fatal("transport encoding must be opt-in")
	}
	if r := get(t, ts.URL+"/_melhttp/sw.js"); r.status != 404 {
		t.Errorf("assets served while disabled: %d", r.status)
	}
	_, pg := newTestServer(t, root, Config{Playground: true})
	if r := get(t, pg.URL+"/_melhttp/"); r.status != 200 || !strings.Contains(string(r.body), "Malbolge Playground") {
		t.Errorf("playground: %d", r.status)
	}
}
