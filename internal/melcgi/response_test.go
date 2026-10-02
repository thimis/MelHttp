package melcgi

import (
	"bytes"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestParseResponse(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		status int
		header http.Header
		body   string
		err    error
	}{
		{
			name:   "LF",
			in:     "Content-Type: text/plain\n\nhello",
			status: 200,
			header: http.Header{"Content-Type": {"text/plain"}},
			body:   "hello",
		},
		{
			name:   "CRLF",
			in:     "Content-Type: text/plain\r\nX-A: 1\r\n\r\nhello\r\n",
			status: 200,
			header: http.Header{"Content-Type": {"text/plain"}, "X-A": {"1"}},
			body:   "hello\r\n",
		},
		{
			name:   "mixed line endings",
			in:     "Content-Type: text/plain\r\nX-A: 1\n\r\nbody",
			status: 200,
			header: http.Header{"Content-Type": {"text/plain"}, "X-A": {"1"}},
			body:   "body",
		},
		{
			name:   "empty body",
			in:     "Content-Type: text/plain\n\n",
			status: 200,
			header: http.Header{"Content-Type": {"text/plain"}},
			body:   "",
		},
		{
			name:   "status with reason",
			in:     "Status: 404 Not Found\nContent-Type: text/html\n\nnope",
			status: 404,
			header: http.Header{"Content-Type": {"text/html"}},
			body:   "nope",
		},
		{
			name:   "status code only",
			in:     "Status: 503\nContent-Type: text/html\n\n",
			status: 503,
			header: http.Header{"Content-Type": {"text/html"}},
		},
		{
			name:   "status bounds",
			in:     "status: 599 x\nContent-Type: a/b\n\n",
			status: 599,
			header: http.Header{"Content-Type": {"a/b"}},
		},
		{
			name:   "status 200 lowest",
			in:     "Status: 200\nContent-Type: a/b\n\n",
			status: 200,
			header: http.Header{"Content-Type": {"a/b"}},
		},
		{
			name:   "location implies 302",
			in:     "Location: /elsewhere\n\n",
			status: 302,
			header: http.Header{"Location": {"/elsewhere"}},
		},
		{
			name:   "location with status 301",
			in:     "Status: 301 Moved Permanently\nLocation: https://example.com/\n\n",
			status: 301,
			header: http.Header{"Location": {"https://example.com/"}},
		},
		{
			name:   "location with content type and body",
			in:     "Location: /x\nContent-Type: text/html\n\n<a href=/x>x</a>",
			status: 302,
			header: http.Header{"Location": {"/x"}, "Content-Type": {"text/html"}},
			body:   "<a href=/x>x</a>",
		},
		{
			name:   "case-insensitive names canonicalized, spacing trimmed",
			in:     "content-type:text/plain \t\ncache-control:\t no-cache\nx-custom-thing:  v a l  \n\n",
			status: 200,
			header: http.Header{"Content-Type": {"text/plain"}, "Cache-Control": {"no-cache"}, "X-Custom-Thing": {"v a l"}},
		},
		{
			name: "all allowed headers",
			in: "Content-Type: text/html\nCache-Control: max-age=60\nContent-Language: en\n" +
				"Content-Disposition: inline\nLast-Modified: Mon, 01 Jan 2024 00:00:00 GMT\n" +
				"Expires: 0\nLink: </a>; rel=preload\nLink: </b>; rel=preload\nSet-Cookie: a=1\nSet-Cookie: b=2\n" +
				"Vary: Accept\nAccess-Control-Allow-Origin: *\nX-Frame-Options: DENY\n\n",
			status: 200,
			header: http.Header{
				"Content-Type": {"text/html"}, "Cache-Control": {"max-age=60"}, "Content-Language": {"en"},
				"Content-Disposition": {"inline"}, "Last-Modified": {"Mon, 01 Jan 2024 00:00:00 GMT"},
				"Expires": {"0"}, "Link": {"</a>; rel=preload", "</b>; rel=preload"}, "Set-Cookie": {"a=1", "b=2"},
				"Vary": {"Accept"}, "Access-Control-Allow-Origin": {"*"}, "X-Frame-Options": {"DENY"},
			},
		},
		{
			name:   "empty value for ordinary header allowed",
			in:     "Content-Type: a/b\nX-Empty:\n\n",
			status: 200,
			header: http.Header{"Content-Type": {"a/b"}, "X-Empty": {""}},
		},
		{
			name:   "tab and high bytes allowed in value",
			in:     "Content-Type: a/b\nX-V: a\tb\xc3\xa9\n\n",
			status: 200,
			header: http.Header{"Content-Type": {"a/b"}, "X-V": {"a\tb\xc3\xa9"}},
		},
		{
			name:   "body preserved byte-exact",
			in:     "Content-Type: application/octet-stream\n\n\x00\x01\r\n\r\n\n\r\xff\xfeStatus: 500\n",
			status: 200,
			header: http.Header{"Content-Type": {"application/octet-stream"}},
			body:   "\x00\x01\r\n\r\n\n\r\xff\xfeStatus: 500\n",
		},

		// Errors.
		{name: "empty output", in: "", err: ErrNoHeaderBlock},
		{name: "no blank line", in: "Content-Type: text/plain\nhello", err: ErrNoHeaderBlock},
		{name: "no newline at all", in: "Hello, world.", err: ErrNoHeaderBlock},
		{name: "empty header block LF", in: "\nhello", err: ErrNoContentType},
		{name: "empty header block CRLF", in: "\r\nhello", err: ErrNoContentType},
		{name: "missing content type", in: "X-A: 1\n\nbody", err: ErrNoContentType},
		{name: "status without content type", in: "Status: 404\n\n", err: ErrNoContentType},
		{name: "obsolete folding space", in: "Content-Type: text/plain\n folded\n\n", err: ErrBadHeader},
		{name: "obsolete folding tab", in: "Content-Type: text/plain\n\tfolded\n\n", err: ErrBadHeader},
		{name: "leading space first line", in: " Content-Type: a/b\n\n", err: ErrBadHeader},
		{name: "no colon", in: "Content-Type text/plain\n\n", err: ErrBadHeader},
		{name: "empty name", in: ": x\nContent-Type: a/b\n\n", err: ErrBadHeader},
		{name: "space in name", in: "Content Type: a/b\n\n", err: ErrBadHeader},
		{name: "space before colon", in: "Content-Type : a/b\n\n", err: ErrBadHeader},
		{name: "bad token char", in: "X-(a): 1\nContent-Type: a/b\n\n", err: ErrBadHeader},
		{name: "non-ascii name", in: "X-\xc3\xa9: 1\nContent-Type: a/b\n\n", err: ErrBadHeader},
		{name: "lone CR in value", in: "Content-Type: a/b\rX-Evil: 1\n\n", err: ErrBadHeader},
		{name: "CR only line", in: "Content-Type: a/b\n\r\r\n\n", err: ErrBadHeader},
		{name: "NUL in value", in: "Content-Type: a/\x00b\n\n", err: ErrBadHeader},
		{name: "DEL in value", in: "Content-Type: a/b\x7f\n\n", err: ErrBadHeader},
		{name: "ESC in value", in: "Content-Type: a/b\nX-A: \x1b[31m\n\n", err: ErrBadHeader},
		{name: "empty content type", in: "Content-Type:\n\n", err: ErrBadHeader},
		{name: "empty location", in: "Location: \n\n", err: ErrBadHeader},
		{name: "duplicate status", in: "Status: 200\nStatus: 404\nContent-Type: a/b\n\n", err: ErrBadHeader},
		{name: "duplicate content type", in: "Content-Type: a/b\ncontent-type: c/d\n\n", err: ErrBadHeader},
		{name: "duplicate location", in: "Location: /a\nLocation: /b\n\n", err: ErrBadHeader},
		{name: "status 199", in: "Status: 199\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "status 600", in: "Status: 600\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "status abc", in: "Status: abc\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "status empty", in: "Status:\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "status two digits", in: "Status: 20\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "status four digits", in: "Status: 2000\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "status suffix", in: "Status: 200OK\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "status signed", in: "Status: +200\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "status tab reason", in: "Status: 200\tOK\nContent-Type: a/b\n\n", err: ErrBadStatus},
		{name: "X-Powered-By", in: "Content-Type: a/b\nX-Powered-By: PHP\n\n", err: ErrForbiddenHeader},
		{name: "X-Malbolge-*", in: "Content-Type: a/b\nX-Malbolge-Steps: 1\n\n", err: ErrForbiddenHeader},
		{name: "X-Malbolge", in: "Content-Type: a/b\nx-malbolge: 1\n\n", err: ErrForbiddenHeader},
	}
	for _, name := range []string{
		"Content-Length", "Transfer-Encoding", "Connection", "Content-Encoding", "Date", "Server",
		"ETag", "Set-Cookie2", "Strict-Transport-Security", "Keep-Alive", "Upgrade", "Trailer",
		"Content-Security-Policy", "Www-Authenticate", "Age", "Allow", "Retry-After", "Xfoo",
	} {
		tests = append(tests, struct {
			name   string
			in     string
			status int
			header http.Header
			body   string
			err    error
		}{name: "forbidden " + name, in: "Content-Type: a/b\n" + name + ": 1\n\n", err: ErrForbiddenHeader})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ParseResponse([]byte(tt.in))
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("err = %v, want %v", err, tt.err)
				}
				if res != nil {
					t.Fatalf("non-nil response with error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if res.Status != tt.status {
				t.Errorf("status = %d, want %d", res.Status, tt.status)
			}
			if !reflect.DeepEqual(res.Header, tt.header) {
				t.Errorf("header = %#v, want %#v", res.Header, tt.header)
			}
			if string(res.Body) != tt.body {
				t.Errorf("body = %q, want %q", res.Body, tt.body)
			}
		})
	}
}

func TestParseResponseErrorsAreDistinct(t *testing.T) {
	errs := []error{ErrNoHeaderBlock, ErrHeaderTooLarge, ErrBadHeader, ErrForbiddenHeader, ErrBadStatus, ErrNoContentType}
	for i, a := range errs {
		if a == nil || !strings.HasPrefix(a.Error(), "melcgi: ") {
			t.Errorf("error %d: %v", i, a)
		}
		for j, b := range errs {
			if i != j && errors.Is(a, b) {
				t.Errorf("%v is %v", a, b)
			}
		}
	}
}

func TestParseResponseErrorDetail(t *testing.T) {
	_, err := ParseResponse([]byte("Content-Type: a/b\nServer: x\n\n"))
	if err == nil || !strings.Contains(err.Error(), "Server") {
		t.Fatalf("error lacks header name: %v", err)
	}
}

// headerOfSize returns a header block of exactly n bytes (including the
// terminating blank line) consisting of one Content-Type and one X-Pad header.
func headerOfSize(n int) string {
	const pre = "Content-Type: a/b\nX-Pad: "
	const post = "\n\n"
	return pre + strings.Repeat("p", n-len(pre)-len(post)) + post
}

func TestParseResponseHeaderLimit(t *testing.T) {
	exact := headerOfSize(MaxHeaderBlock)
	if len(exact) != MaxHeaderBlock {
		t.Fatalf("helper: len %d", len(exact))
	}
	res, err := ParseResponse([]byte(exact + "body"))
	if err != nil || string(res.Body) != "body" {
		t.Fatalf("exact limit: %v", err)
	}

	over := headerOfSize(MaxHeaderBlock + 1)
	if _, err := ParseResponse([]byte(over + "body")); !errors.Is(err, ErrHeaderTooLarge) {
		t.Fatalf("over limit: err = %v", err)
	}
	// Over the limit with no blank line anywhere is also "too large".
	if _, err := ParseResponse([]byte(strings.Repeat("x", MaxHeaderBlock+1))); !errors.Is(err, ErrHeaderTooLarge) {
		t.Fatalf("long no-blank: err = %v", err)
	}
	// Exactly MaxHeaderBlock bytes and no blank line: nothing was cut off.
	if _, err := ParseResponse([]byte(strings.Repeat("x", MaxHeaderBlock))); !errors.Is(err, ErrNoHeaderBlock) {
		t.Fatalf("short no-blank: err = %v", err)
	}
	// CRLF terminator straddling the limit.
	crlf := strings.Replace(headerOfSize(MaxHeaderBlock), "\n\n", "\r\n\r\n", 1)
	crlf = crlf[:len(crlf)-4-2] + "\r\n\r\n" // keep exactly MaxHeaderBlock bytes
	if len(crlf) != MaxHeaderBlock {
		t.Fatalf("helper: len %d", len(crlf))
	}
	if _, err := ParseResponse([]byte(crlf)); err != nil {
		t.Fatalf("crlf exact: %v", err)
	}
	if _, err := ParseResponse([]byte("X" + crlf)); !errors.Is(err, ErrHeaderTooLarge) {
		t.Fatalf("crlf over: %v", err)
	}
}

func TestParseResponseBodyAliasesOutput(t *testing.T) {
	out := []byte("Content-Type: a/b\n\nbody")
	res, err := ParseResponse(out)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(out, res.Body) {
		t.Fatal("body is not a suffix of the output")
	}
}

func TestHeaderBlockRoundTrip(t *testing.T) {
	types := []string{
		"text/html; charset=utf-8", "text/plain", "application/json", "image/png",
		"application/octet-stream", "text/css; charset=utf-8", "image/svg+xml",
		"multipart/form-data; boundary=\"--x y\"", "font/woff2", "text/plain;charset=\xc3\xa9",
	}
	for _, ct := range types {
		hb := HeaderBlock(ct)
		if want := "Content-Type: " + ct + "\n\n"; string(hb) != want {
			t.Fatalf("HeaderBlock(%q) = %q", ct, hb)
		}
		for _, body := range []string{"", "x", "\n\nStatus: 500\n\n", "\x00\xff"} {
			res, err := ParseResponse(append(hb, body...))
			if err != nil {
				t.Fatalf("%q: %v", ct, err)
			}
			if res.Status != 200 || res.Header.Get("Content-Type") != ct || len(res.Header) != 1 || string(res.Body) != body {
				t.Fatalf("%q: got %+v", ct, res)
			}
		}
	}
}

func TestRawResponse(t *testing.T) {
	out := []byte("Status: 500\n\nnot parsed\x00")
	res := RawResponse(out, "text/plain")
	if res.Status != 200 {
		t.Errorf("status %d", res.Status)
	}
	if !reflect.DeepEqual(res.Header, http.Header{"Content-Type": {"text/plain"}}) {
		t.Errorf("header %v", res.Header)
	}
	if !bytes.Equal(res.Body, out) {
		t.Errorf("body %q", res.Body)
	}
	if res := RawResponse(nil, "a/b"); len(res.Body) != 0 || res.Status != 200 {
		t.Errorf("nil output: body %#v", res.Body)
	}
}
