package server

import (
	"bytes"
	"context"
	"net/http"
	"strconv"
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

func TestTransportRequestBodies(t *testing.T) {
	root, _ := site(t)
	_, ts := newTestServer(t, root, Config{Obfuscate: true, MaxBody: 1000})
	enc, err := obfs.Encode([]byte("secret-ish form data ✓"), 5)
	if err != nil {
		t.Fatal(err)
	}
	r := do(t, http.MethodPost, ts.URL+"/echo.txt", bytes.NewReader(enc),
		obfs.RequestEncodingHeader, obfs.Program, "Content-Type", "text/plain")
	if r.status != 200 || r.header.Get(obfs.RequestDecodedHeader) != "1" ||
		!bytes.HasSuffix(r.body, []byte("\n\nsecret-ish form data ✓")) ||
		bytes.Contains(r.body, []byte("HTTP_X_MALBOLGE_CONTENT_ENCODING")) {
		t.Fatalf("decoded request: %d %v\n%s", r.status, r.header, r.body)
	}
	if !bytes.Contains(r.body, []byte("CONTENT_LENGTH="+strconv.Itoa(len("secret-ish form data ✓"))+"\n")) {
		t.Errorf("CONTENT_LENGTH should be the decoded size:\n%s", r.body)
	}
	if bad := do(t, http.MethodPost, ts.URL+"/echo.txt", strings.NewReader("not programs"), obfs.RequestEncodingHeader, obfs.Program); bad.status != 400 {
		t.Errorf("invalid container: %d", bad.status)
	}
	big, _ := obfs.Encode(bytes.Repeat([]byte("x"), 2000), 1)
	if r := do(t, http.MethodPost, ts.URL+"/echo.txt", bytes.NewReader(big), obfs.RequestEncodingHeader, obfs.Program); r.status != 413 {
		t.Errorf("decoded body over -max-body: %d", r.status)
	}
	// Without -obfuscate the header means nothing: the body passes through as is.
	_, plain := newTestServer(t, root, Config{})
	p := do(t, http.MethodPost, plain.URL+"/echo.txt", strings.NewReader("raw"), obfs.RequestEncodingHeader, obfs.Program)
	if !bytes.HasSuffix(p.body, []byte("\n\nraw")) {
		t.Errorf("disabled transport touched the body: %q", p.body)
	}
}
