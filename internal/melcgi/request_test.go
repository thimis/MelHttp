package melcgi

import (
	"bytes"
	"crypto/tls"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

// baseRequest is a fully populated Request without headers.
func baseRequest() Request {
	return Request{
		Method:     "GET",
		Protocol:   "HTTP/1.1",
		ScriptName: "/echo.txt",
		PathInfo:   "",
		Query:      "x=1&y=two",
		ServerName: "example.com",
		ServerPort: "8080",
		RemoteAddr: "127.0.0.1",
	}
}

const baseMeta = "GATEWAY_INTERFACE=MelCGI/1.0\n" +
	"SERVER_PROTOCOL=HTTP/1.1\n" +
	"REQUEST_METHOD=GET\n" +
	"SCRIPT_NAME=/echo.txt\n" +
	"PATH_INFO=\n" +
	"QUERY_STRING=x=1&y=two\n" +
	"SERVER_NAME=example.com\n" +
	"SERVER_PORT=8080\n" +
	"REMOTE_ADDR=127.0.0.1\n"

func TestMetaContractExample(t *testing.T) {
	r := baseRequest()
	r.ContentType = "text/plain"
	r.ContentLength = 11
	r.Header = http.Header{"Accept": {"*/*"}, "X-Test": {"yes"}}
	want := baseMeta +
		"CONTENT_TYPE=text/plain\n" +
		"CONTENT_LENGTH=11\n" +
		"HTTP_ACCEPT=*/*\n" +
		"HTTP_X_TEST=yes\n" +
		"\n"
	if got := string(r.Meta(Options{})); got != want {
		t.Fatalf("Meta mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestMeta(t *testing.T) {
	tests := []struct {
		name   string
		mod    func(*Request)
		opt    Options
		want   string   // exact HTTP_*/CONTENT_* tail after baseMeta, before the blank line
		absent []string // substrings that must not appear
	}{
		{
			name: "no headers no body",
			mod:  func(r *Request) {},
			want: "",
		},
		{
			name: "zero and negative content length omitted",
			mod:  func(r *Request) { r.ContentLength = -1 },
			want: "",
		},
		{
			name: "content type without length",
			mod:  func(r *Request) { r.ContentType = "application/json" },
			want: "CONTENT_TYPE=application/json\n",
		},
		{
			name: "content length without type",
			mod:  func(r *Request) { r.ContentLength = 5 },
			want: "CONTENT_LENGTH=5\n",
		},
		{
			name: "http headers sorted by variable name",
			mod: func(r *Request) {
				r.Header = http.Header{
					"X-Zeta":          {"z"},
					"Accept-Language": {"en"},
					"Accept":          {"*/*"},
					"User-Agent":      {"ua"},
				}
			},
			want: "HTTP_ACCEPT=*/*\nHTTP_ACCEPT_LANGUAGE=en\nHTTP_USER_AGENT=ua\nHTTP_X_ZETA=z\n",
		},
		{
			name: "multiple values joined",
			mod:  func(r *Request) { r.Header = http.Header{"Accept": {"text/html", "*/*"}} },
			want: "HTTP_ACCEPT=text/html, */*\n",
		},
		{
			name: "non-canonical keys merge into one variable",
			mod:  func(r *Request) { r.Header = http.Header{"X-Test": {"a"}, "x-test": {"b"}} },
			want: "HTTP_X_TEST=a, b\n",
		},
		{
			name: "digits allowed in names",
			mod:  func(r *Request) { r.Header = http.Header{"X-B3-Traceid": {"1"}} },
			want: "HTTP_X_B3_TRACEID=1\n",
		},
		{
			name: "httpoxy Proxy header dropped",
			mod: func(r *Request) {
				r.Header = http.Header{"Proxy": {"http://evil:8080"}, "proxy": {"http://evil"}}
			},
			want:   "",
			absent: []string{"HTTP_PROXY", "evil"},
		},
		{
			name: "hop-by-hop and body headers dropped",
			mod: func(r *Request) {
				r.Header = http.Header{
					"Content-Type":      {"text/plain"},
					"Content-Length":    {"3"},
					"Connection":        {"close"},
					"Upgrade":           {"h2c"},
					"Te":                {"trailers"},
					"Trailer":           {"X"},
					"Transfer-Encoding": {"chunked"},
					"Keep-Alive":        {"300"},
					"Proxy-Connection":  {"keep-alive"},
					"X-Ok":              {"1"},
				}
			},
			want: "HTTP_X_OK=1\n",
		},
		{
			name: "sensitive headers dropped by default",
			mod: func(r *Request) {
				r.Header = http.Header{
					"Authorization":       {"Basic Zm9v"},
					"Cookie":              {"s=1"},
					"Proxy-Authorization": {"Basic YmFy"},
				}
			},
			want:   "",
			absent: []string{"HTTP_AUTHORIZATION", "HTTP_COOKIE", "HTTP_PROXY_AUTHORIZATION"},
		},
		{
			name: "sensitive headers passed when allowed",
			mod: func(r *Request) {
				r.Header = http.Header{
					"Authorization":       {"Basic Zm9v"},
					"Cookie":              {"s=1"},
					"Proxy-Authorization": {"Basic YmFy"},
					"Proxy":               {"still-no"},
				}
			},
			opt:  Options{AllowSensitiveHeaders: true},
			want: "HTTP_AUTHORIZATION=Basic Zm9v\nHTTP_COOKIE=s=1\nHTTP_PROXY_AUTHORIZATION=Basic YmFy\n",
		},
		{
			name: "underscore names skipped",
			mod: func(r *Request) {
				r.Header = http.Header{"X_Test": {"spoof"}, "X-Test": {"real"}, "Content_Type": {"x"}}
			},
			want:   "HTTP_X_TEST=real\n",
			absent: []string{"spoof"},
		},
		{
			name: "names outside letters digits dash skipped",
			mod: func(r *Request) {
				r.Header = http.Header{"X.Dot": {"a"}, "X Space": {"b"}, "": {"c"}, "X=Eq": {"d"}, "X\nNl": {"e"}}
			},
			want: "",
		},
		{
			name: "control bytes escaped in header value",
			mod:  func(r *Request) { r.Header = http.Header{"X-Test": {"a\r\nEVIL=1"}} },
			want: "HTTP_X_TEST=a%0D%0AEVIL=1\n",
		},
		{
			name: "tab kept, DEL and NUL escaped, percent and high bytes kept",
			mod:  func(r *Request) { r.Header = http.Header{"X-Test": {"a\tb\x7fc\x00d%41\xff"}} },
			want: "HTTP_X_TEST=a\tb%7Fc%00d%41\xff\n",
		},
		{
			name: "content type escaped",
			mod:  func(r *Request) { r.ContentType = "text/plain\nHTTP_X=1" },
			want: "CONTENT_TYPE=text/plain%0AHTTP_X=1\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := baseRequest()
			tt.mod(&r)
			got := string(r.Meta(tt.opt))
			want := baseMeta + tt.want + "\n"
			if got != want {
				t.Fatalf("Meta mismatch\n got: %q\nwant: %q", got, want)
			}
			for _, s := range tt.absent {
				if strings.Contains(got, s) {
					t.Errorf("Meta contains %q", s)
				}
			}
		})
	}
}

func TestMetaEscapesStandardVariables(t *testing.T) {
	r := Request{
		Method:     "GET\r\nEVIL=1",
		Protocol:   "HTTP/1.1\x00",
		ScriptName: "/a\nb",
		PathInfo:   "/p\rq",
		Query:      "q=%0A\r\nEVIL=1",
		ServerName: "h\x1b",
		ServerPort: "80\n",
		RemoteAddr: "1.2.3.4\x7f",
	}
	want := "GATEWAY_INTERFACE=MelCGI/1.0\n" +
		"SERVER_PROTOCOL=HTTP/1.1%00\n" +
		"REQUEST_METHOD=GET%0D%0AEVIL=1\n" +
		"SCRIPT_NAME=/a%0Ab\n" +
		"PATH_INFO=/p%0Dq\n" +
		"QUERY_STRING=q=%0A%0D%0AEVIL=1\n" +
		"SERVER_NAME=h%1B\n" +
		"SERVER_PORT=80%0A\n" +
		"REMOTE_ADDR=1.2.3.4%7F\n" +
		"\n"
	if got := string(r.Meta(Options{})); got != want {
		t.Fatalf("Meta mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestMetaDoesNotMutateHeader(t *testing.T) {
	r := baseRequest()
	r.Header = http.Header{"Accept": {"a", "b"}, "Proxy": {"x"}}
	r.Meta(Options{})
	if len(r.Header) != 2 || len(r.Header["Accept"]) != 2 {
		t.Fatalf("header mutated: %v", r.Header)
	}
}

func newHTTPRequest(t *testing.T, method, target, body string) *http.Request {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	r, err := http.NewRequest(method, target, rd)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFromHTTP(t *testing.T) {
	tests := []struct {
		name string
		mk   func(t *testing.T) *http.Request
		opt  Options
		want Request
		hdr  http.Header
	}{
		{
			name: "host with port",
			mk: func(t *testing.T) *http.Request {
				r := newHTTPRequest(t, "POST", "http://example.com:8080/echo.txt/extra?x=1&y=two", "hello world")
				r.Header.Set("Content-Type", "text/plain")
				r.Header.Set("X-Test", "yes")
				r.RemoteAddr = "127.0.0.1:54321"
				return r
			},
			want: Request{Method: "POST", Protocol: "HTTP/1.1", ScriptName: "/echo.txt", PathInfo: "/extra",
				Query: "x=1&y=two", ServerName: "example.com", ServerPort: "8080", RemoteAddr: "127.0.0.1",
				ContentType: "text/plain", ContentLength: 11},
			hdr: http.Header{"X-Test": {"yes"}},
		},
		{
			name: "http host without port defaults to 80",
			mk: func(t *testing.T) *http.Request {
				r := newHTTPRequest(t, "GET", "http://example.com/echo.txt", "")
				r.RemoteAddr = "10.0.0.1:1"
				return r
			},
			want: Request{Method: "GET", Protocol: "HTTP/1.1", ScriptName: "/echo.txt", PathInfo: "/extra",
				ServerName: "example.com", ServerPort: "80", RemoteAddr: "10.0.0.1"},
			hdr: http.Header{},
		},
		{
			name: "tls host without port defaults to 443",
			mk: func(t *testing.T) *http.Request {
				r := newHTTPRequest(t, "GET", "https://example.com/echo.txt", "")
				r.TLS = &tls.ConnectionState{}
				r.RemoteAddr = "10.0.0.1:1"
				return r
			},
			want: Request{Method: "GET", Protocol: "HTTP/1.1", ScriptName: "/echo.txt", PathInfo: "/extra",
				ServerName: "example.com", ServerPort: "443", RemoteAddr: "10.0.0.1"},
			hdr: http.Header{},
		},
		{
			name: "ipv6 host and remote addr",
			mk: func(t *testing.T) *http.Request {
				r := newHTTPRequest(t, "GET", "http://[::1]:9000/echo.txt", "")
				r.RemoteAddr = "[::1]:1234"
				return r
			},
			want: Request{Method: "GET", Protocol: "HTTP/1.1", ScriptName: "/echo.txt", PathInfo: "/extra",
				ServerName: "::1", ServerPort: "9000", RemoteAddr: "::1"},
			hdr: http.Header{},
		},
		{
			name: "bracketed ipv6 host without port",
			mk: func(t *testing.T) *http.Request {
				r := newHTTPRequest(t, "GET", "http://[::1]/echo.txt", "")
				r.RemoteAddr = "pipe"
				return r
			},
			want: Request{Method: "GET", Protocol: "HTTP/1.1", ScriptName: "/echo.txt", PathInfo: "/extra",
				ServerName: "::1", ServerPort: "80", RemoteAddr: "pipe"},
			hdr: http.Header{},
		},
		{
			name: "host with empty port",
			mk: func(t *testing.T) *http.Request {
				r := newHTTPRequest(t, "GET", "http://example.com/echo.txt", "")
				r.Host = "example.com:"
				return r
			},
			want: Request{Method: "GET", Protocol: "HTTP/1.1", ScriptName: "/echo.txt", PathInfo: "/extra",
				ServerName: "example.com", ServerPort: "80"},
			hdr: http.Header{},
		},
		{
			name: "falls back to URL host when Host is empty",
			mk: func(t *testing.T) *http.Request {
				r := &http.Request{Method: "GET", Proto: "HTTP/1.0",
					URL: &url.URL{Host: "u.example:81", Path: "/echo.txt"}, Header: http.Header{}}
				return r
			},
			want: Request{Method: "GET", Protocol: "HTTP/1.0", ScriptName: "/echo.txt", PathInfo: "/extra",
				ServerName: "u.example", ServerPort: "81"},
			hdr: http.Header{},
		},
		{
			name: "headers filtered with options",
			mk: func(t *testing.T) *http.Request {
				r := newHTTPRequest(t, "GET", "http://example.com/echo.txt", "")
				r.Header.Set("Cookie", "s=1")
				r.Header.Set("Proxy", "evil")
				r.Header["X_Spoof"] = []string{"no"}
				r.Header.Add("Accept", "a")
				r.Header.Add("Accept", "b")
				return r
			},
			opt: Options{AllowSensitiveHeaders: true},
			want: Request{Method: "GET", Protocol: "HTTP/1.1", ScriptName: "/echo.txt", PathInfo: "/extra",
				ServerName: "example.com", ServerPort: "80"},
			hdr: http.Header{"Cookie": {"s=1"}, "Accept": {"a", "b"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hr := tt.mk(t)
			got := FromHTTP(hr, "/echo.txt", "/extra", tt.opt)
			hdr := got.Header
			got.Header = nil
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("FromHTTP\n got: %+v\nwant: %+v", got, tt.want)
			}
			if len(hdr) != len(tt.hdr) {
				t.Fatalf("header: got %v want %v", hdr, tt.hdr)
			}
			for k, v := range tt.hdr {
				if strings.Join(hdr[k], "|") != strings.Join(v, "|") {
					t.Fatalf("header %s: got %v want %v", k, hdr[k], v)
				}
			}
		})
	}
}

func TestFromHTTPHeaderIsCopy(t *testing.T) {
	hr := newHTTPRequest(t, "GET", "http://example.com/x", "")
	hr.Header.Set("Accept", "a")
	got := FromHTTP(hr, "/x", "", Options{})
	hr.Header.Set("Accept", "changed")
	hr.Header["Accept"][0] = "changed"
	if got.Header.Get("Accept") != "a" {
		t.Fatalf("FromHTTP shares header storage with the http.Request")
	}
}

func TestFromHTTPMetaEndToEnd(t *testing.T) {
	hr := newHTTPRequest(t, "GET", "http://example.com:8080/echo.txt?x=1&y=two", "")
	hr.Header.Set("Accept", "*/*")
	hr.Header.Set("X-Test", "yes")
	hr.RemoteAddr = "127.0.0.1:5555"
	got := string(FromHTTP(hr, "/echo.txt", "", Options{}).Meta(Options{}))
	want := baseMeta + "HTTP_ACCEPT=*/*\nHTTP_X_TEST=yes\n\n"
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}

func TestFromHTTPInjectionStaysOnOneLine(t *testing.T) {
	hr := &http.Request{
		Method: "GET", Proto: "HTTP/1.1", Host: "example.com",
		URL:        &url.URL{Path: "/echo.txt", RawQuery: "a=1\r\nEVIL=1"},
		Header:     http.Header{"X-Test": {"v\r\nEVIL=1"}},
		RemoteAddr: "127.0.0.1:1",
	}
	meta := string(FromHTTP(hr, "/echo.txt", "", Options{}).Meta(Options{}))
	for _, line := range strings.Split(strings.TrimSuffix(meta, "\n\n"), "\n") {
		if strings.HasPrefix(line, "EVIL") {
			t.Fatalf("injected variable on its own line: %q", meta)
		}
	}
	if !strings.Contains(meta, "QUERY_STRING=a=1%0D%0AEVIL=1\n") || !strings.Contains(meta, "HTTP_X_TEST=v%0D%0AEVIL=1\n") {
		t.Fatalf("unexpected meta %q", meta)
	}
}

// trackingReader records whether it was read from.
type trackingReader struct {
	r    io.Reader
	read bool
}

func (t *trackingReader) Read(p []byte) (int, error) {
	t.read = true
	return t.r.Read(p)
}

func TestInputLazy(t *testing.T) {
	meta := baseRequest().Meta(Options{})
	body := &trackingReader{r: strings.NewReader("BODY")}
	in := Input(meta, body)

	got := make([]byte, len(meta))
	if _, err := io.ReadFull(in, got); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, meta) {
		t.Fatalf("meta: got %q", got)
	}
	if body.read {
		t.Fatal("body read while only meta was consumed")
	}
	rest, err := io.ReadAll(in)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "BODY" || !body.read {
		t.Fatalf("body: got %q read=%v", rest, body.read)
	}
}

func TestInputNilBody(t *testing.T) {
	meta := []byte("A=1\n\n")
	got, err := io.ReadAll(Input(meta, nil))
	if err != nil || string(got) != "A=1\n\n" {
		t.Fatalf("got %q %v", got, err)
	}
}
