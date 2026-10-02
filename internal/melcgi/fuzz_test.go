package melcgi

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// serialize writes res back out as a MelCGI response.
func serialize(res *Response) []byte {
	var b bytes.Buffer
	b.WriteString("Status: " + strconv.Itoa(res.Status) + "\n")
	keys := make([]string, 0, len(res.Header))
	for k := range res.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range res.Header[k] {
			b.WriteString(k + ": " + v + "\n")
		}
	}
	b.WriteString("\n")
	b.Write(res.Body)
	return b.Bytes()
}

func FuzzParseResponse(f *testing.F) {
	for _, s := range []string{
		"Content-Type: text/plain\n\nhello",
		"Content-Type: text/plain\r\n\r\nhello\r\n",
		"Status: 404 Not Found\r\nContent-Type: text/html\n\nx",
		"Location: /x\n\n",
		"Status: 301\nLocation: /x\nSet-Cookie: a=1\nSet-Cookie: b=2\n\n",
		"\n\n",
		"X-A: 1\n\n",
		"Content-Type: a\n b\n\n",
		"Content-Type: a\rb\n\n",
		"content-type:a/b \t\nx-y:\t\n\n\x00\xff",
		"Status: 600\nContent-Type: a/b\n\n",
		"Content-Length: 1\nContent-Type: a/b\n\n",
		"Hello, world.",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, out []byte) {
		res, err := ParseResponse(out)
		if err != nil {
			if res != nil {
				t.Fatal("non-nil response with error")
			}
			return
		}
		if res.Status < 200 || res.Status > 599 {
			t.Fatalf("status %d", res.Status)
		}
		if res.Header == nil {
			t.Fatal("nil header")
		}
		if _, ok := res.Header["Status"]; ok {
			t.Fatal("Status stored in header")
		}
		for k, vs := range res.Header {
			if !validToken(k) {
				t.Fatalf("bad name %q", k)
			}
			for _, v := range vs {
				if strings.ContainsAny(v, "\r\n") {
					t.Fatalf("header %s value %q contains CR/LF", k, v)
				}
			}
		}
		if !bytes.HasSuffix(out, res.Body) {
			t.Fatal("body is not a suffix of the input")
		}
		again, err := ParseResponse(serialize(res))
		if err != nil {
			t.Fatalf("re-parse of %q: %v", serialize(res), err)
		}
		if again.Status != res.Status || !reflect.DeepEqual(again.Header, res.Header) || !bytes.Equal(again.Body, res.Body) {
			t.Fatalf("round trip mismatch:\n first: %+v\nsecond: %+v", res, again)
		}
	})
}

// metaLine is the shape of every meta-variable line. Digits may appear in
// HTTP_* names derived from header names such as X-B3-Traceid.
var metaLine = regexp.MustCompile(`^[A-Z][A-Z0-9_]*=[^\n]*$`)

// checkMeta asserts the structural invariants of a meta block.
func checkMeta(t *testing.T, meta []byte) {
	t.Helper()
	s := string(meta)
	if !strings.HasSuffix(s, "\n\n") {
		t.Fatalf("meta does not end with a blank line: %q", s)
	}
	lines := strings.Split(strings.TrimSuffix(s, "\n\n"), "\n")
	for _, line := range lines {
		if !metaLine.MatchString(line) {
			t.Fatalf("bad meta line %q in %q", line, s)
		}
		for i := 0; i < len(line); i++ {
			if c := line[i]; (c < 0x20 && c != '\t') || c == 0x7f {
				t.Fatalf("control byte %#x in line %q", c, line)
			}
		}
	}
	if lines[0] != "GATEWAY_INTERFACE=MelCGI/1.0" {
		t.Fatalf("first line %q", lines[0])
	}
}

func FuzzEncodeRequest(f *testing.F) {
	f.Add("GET", "/echo.txt", "x=1&y=two", "X-Test", "yes", []byte("hello"))
	f.Add("POST", "/a\r\nB=1", "q=\n", "X_Spoof", "v\r\nEVIL=1", []byte{})
	f.Add("", "", "", "", "", []byte(nil))
	f.Add("GET", "/", "", "Proxy", "http://evil", []byte("x"))
	f.Add("GET", "/", "", "Cookie", "a=b", []byte("x"))
	f.Add("\x00", "\x7f", "%0A", "X-\x01", "\t\x1f", []byte("\n\n"))
	f.Fuzz(func(t *testing.T, method, path, query, headerName, headerValue string, body []byte) {
		for _, opt := range []Options{{}, {AllowSensitiveHeaders: true}} {
			r := Request{
				Method: method, Protocol: "HTTP/1.1", ScriptName: path, PathInfo: path, Query: query,
				ServerName: headerValue, ServerPort: query, RemoteAddr: method,
				ContentType:   headerValue,
				ContentLength: int64(len(body)),
				Header:        http.Header{headerName: {headerValue, headerValue}, "Accept": {headerValue}},
			}
			meta := r.Meta(opt)
			checkMeta(t, meta)

			hr := &http.Request{
				Method: method, Proto: "HTTP/1.1", Host: headerValue,
				URL:        &url.URL{Path: path, RawQuery: query},
				Header:     http.Header{headerName: {headerValue}},
				RemoteAddr: headerValue,
			}
			checkMeta(t, FromHTTP(hr, path, query, opt).Meta(opt))

			in, err := io.ReadAll(Input(meta, bytes.NewReader(body)))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(in, append(append([]byte{}, meta...), body...)) {
				t.Fatal("Input is not meta followed by body")
			}
		}
	})
}
