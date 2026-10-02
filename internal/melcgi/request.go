package melcgi

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// Request holds the values encoded into a program's meta-variable block.
// Values are stored raw; Meta escapes them.
type Request struct {
	Method     string // REQUEST_METHOD
	Protocol   string // SERVER_PROTOCOL, e.g. "HTTP/1.1"
	ScriptName string // SCRIPT_NAME: URL path of the program
	PathInfo   string // PATH_INFO: path after SCRIPT_NAME, may be empty
	Query      string // QUERY_STRING: raw query, without '?'
	ServerName string // SERVER_NAME: host part of the Host header
	ServerPort string // SERVER_PORT
	RemoteAddr string // REMOTE_ADDR: client IP, without port

	ContentType   string // CONTENT_TYPE, omitted when empty
	ContentLength int64  // CONTENT_LENGTH, omitted unless > 0

	// Header holds request headers to pass as HTTP_* variables. Meta
	// filters it again, so it may contain headers that will be dropped.
	Header http.Header
}

// Options controls which request headers reach the program.
type Options struct {
	// AllowSensitiveHeaders passes Authorization, Cookie and
	// Proxy-Authorization through as HTTP_* variables.
	AllowSensitiveHeaders bool
}

// neverPass lists header names (as HTTP_* variable suffixes) that are never
// passed: Proxy (httpoxy), headers with their own variables, and hop-by-hop
// headers.
var neverPass = map[string]bool{
	"PROXY":             true,
	"CONTENT_TYPE":      true,
	"CONTENT_LENGTH":    true,
	"CONNECTION":        true,
	"UPGRADE":           true,
	"TE":                true,
	"TRAILER":           true,
	"TRANSFER_ENCODING": true,
	"KEEP_ALIVE":        true,
	"PROXY_CONNECTION":  true,
}

// sensitive lists headers passed only with Options.AllowSensitiveHeaders.
var sensitive = map[string]bool{
	"AUTHORIZATION":       true,
	"COOKIE":              true,
	"PROXY_AUTHORIZATION": true,
}

// FromHTTP builds a Request from r. scriptName and pathInfo come from the
// router. The Host is split into SERVER_NAME and SERVER_PORT (missing port:
// 80, or 443 over TLS); RemoteAddr keeps only the host. Header is a filtered
// copy of r.Header.
func FromHTTP(r *http.Request, scriptName, pathInfo string, opt Options) Request {
	host := r.Host
	if host == "" && r.URL != nil {
		host = r.URL.Host
	}
	name, port := splitHost(host)
	if port == "" {
		port = "80"
		if r.TLS != nil {
			port = "443"
		}
	}
	remote, _ := splitHost(r.RemoteAddr)
	var query string
	if r.URL != nil {
		query = r.URL.RawQuery
	}
	return Request{
		Method:        r.Method,
		Protocol:      r.Proto,
		ScriptName:    scriptName,
		PathInfo:      pathInfo,
		Query:         query,
		ServerName:    name,
		ServerPort:    port,
		RemoteAddr:    remote,
		ContentType:   r.Header.Get("Content-Type"),
		ContentLength: r.ContentLength,
		Header:        filterHeader(r.Header, opt),
	}
}

// splitHost splits "host:port", "[v6]:port", "host" or "[v6]" into host and
// port (empty if absent). Brackets are removed from IPv6 literals.
func splitHost(hostport string) (host, port string) {
	h, p, err := net.SplitHostPort(hostport)
	if err != nil {
		h, p = hostport, ""
	}
	if len(h) >= 2 && h[0] == '[' && h[len(h)-1] == ']' {
		h = h[1 : len(h)-1]
	}
	return h, p
}

// filterHeader returns a copy of h holding only the headers Meta passes.
func filterHeader(h http.Header, opt Options) http.Header {
	out := make(http.Header)
	for k, vs := range h {
		if _, ok := envName(k, opt); ok {
			out[k] = append([]string(nil), vs...)
		}
	}
	return out
}

// envName returns the HTTP_* variable name for a header, or false if the
// header must not be passed. Names must consist of ASCII letters, digits and
// '-'; in particular a '_' disqualifies a header, so that "X_Foo" cannot
// spoof "X-Foo".
func envName(header string, opt Options) (string, bool) {
	if header == "" {
		return "", false
	}
	b := make([]byte, len(header))
	for i := 0; i < len(header); i++ {
		c := header[i]
		switch {
		case c >= 'a' && c <= 'z':
			b[i] = c - 'a' + 'A'
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			b[i] = c
		case c == '-':
			b[i] = '_'
		default:
			return "", false
		}
	}
	name := string(b)
	if neverPass[name] || (sensitive[name] && !opt.AllowSensitiveHeaders) {
		return "", false
	}
	return "HTTP_" + name, true
}

// Meta encodes r as a meta-variable block, including the terminating blank
// line. Standard variables come first in a fixed order, then HTTP_*
// variables sorted by name. Every value is passed through escapeValue so
// each variable is exactly one line.
func (r Request) Meta(opt Options) []byte {
	var b bytes.Buffer
	put := func(name, value string) {
		b.WriteString(name)
		b.WriteByte('=')
		b.WriteString(escapeValue(value))
		b.WriteByte('\n')
	}
	put("GATEWAY_INTERFACE", GatewayInterface)
	put("SERVER_PROTOCOL", r.Protocol)
	put("REQUEST_METHOD", r.Method)
	put("SCRIPT_NAME", r.ScriptName)
	put("PATH_INFO", r.PathInfo)
	put("QUERY_STRING", r.Query)
	put("SERVER_NAME", r.ServerName)
	put("SERVER_PORT", r.ServerPort)
	put("REMOTE_ADDR", r.RemoteAddr)
	if r.ContentType != "" {
		put("CONTENT_TYPE", r.ContentType)
	}
	if r.ContentLength > 0 {
		put("CONTENT_LENGTH", strconv.FormatInt(r.ContentLength, 10))
	}
	vars := httpVars(r.Header, opt)
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		put(name, strings.Join(vars[name], ", "))
	}
	b.WriteByte('\n')
	return b.Bytes()
}

// httpVars maps HTTP_* variable names to header values. Header keys that
// differ only in case merge into one variable, in sorted key order.
func httpVars(h http.Header, opt Options) map[string][]string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	vars := make(map[string][]string)
	for _, k := range keys {
		if name, ok := envName(k, opt); ok {
			vars[name] = append(vars[name], h[k]...)
		}
	}
	return vars
}

// escapeValue percent-encodes control bytes (below 0x20 except tab, and
// 0x7f) so the value fits on one line. Nothing else is escaped: a literal
// '%' stays as is, since query strings already contain %XX sequences.
func escapeValue(s string) string {
	n := 0
	for i := 0; i < len(s); i++ {
		if isControl(s[i]) {
			n++
		}
	}
	if n == 0 {
		return s
	}
	const hex = "0123456789ABCDEF"
	b := make([]byte, 0, len(s)+2*n)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if isControl(c) {
			b = append(b, '%', hex[c>>4], hex[c&0xf])
		} else {
			b = append(b, c)
		}
	}
	return string(b)
}

// isControl reports whether c is a control byte other than tab.
func isControl(c byte) bool {
	return (c < 0x20 && c != '\t') || c == 0x7f
}

// Input returns the program's stdin: meta followed by body. The body is not
// read until meta is exhausted. A nil body is treated as empty.
func Input(meta []byte, body io.Reader) io.Reader {
	if body == nil {
		return bytes.NewReader(meta)
	}
	return io.MultiReader(bytes.NewReader(meta), body)
}
