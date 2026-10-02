package melcgi

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/textproto"
	"strings"
)

// MaxHeaderBlock is the largest header block a program may print, counting
// the terminating blank line.
const MaxHeaderBlock = 8 << 10

var (
	// ErrNoHeaderBlock means the output has no blank line ending a header block.
	ErrNoHeaderBlock = errors.New("melcgi: no blank line after response headers")
	// ErrHeaderTooLarge means no blank line appears within MaxHeaderBlock bytes.
	ErrHeaderTooLarge = errors.New("melcgi: response header block too large")
	// ErrBadHeader means a header line is malformed or a single-use header repeats.
	ErrBadHeader = errors.New("melcgi: malformed response header")
	// ErrForbiddenHeader means the program printed a header outside the allowlist.
	ErrForbiddenHeader = errors.New("melcgi: forbidden response header")
	// ErrBadStatus means the Status header is not a code in 200..599.
	ErrBadStatus = errors.New("melcgi: bad Status header")
	// ErrNoContentType means a non-redirect response has no Content-Type.
	ErrNoContentType = errors.New("melcgi: missing Content-Type")
)

// Response is a parsed program response.
type Response struct {
	Status int         // HTTP status code, 200..599
	Header http.Header // canonicalized; never contains Status
	Body   []byte      // everything after the blank line, byte-exact
}

// allowed lists the response headers (lower case) a program may set, besides
// X-* headers.
var allowed = map[string]bool{
	"content-type":                true,
	"status":                      true,
	"location":                    true,
	"cache-control":               true,
	"content-language":            true,
	"content-disposition":         true,
	"last-modified":               true,
	"expires":                     true,
	"link":                        true,
	"set-cookie":                  true,
	"vary":                        true,
	"access-control-allow-origin": true,
}

// single lists headers that may appear at most once.
var single = map[string]bool{
	"Status":       true,
	"Content-Type": true,
	"Location":     true,
}

// isAllowed reports whether a program may set the header name. X-* headers
// are allowed except X-Powered-By and X-Malbolge(-*), which the server owns.
func isAllowed(name string) bool {
	l := strings.ToLower(name)
	if allowed[l] {
		return true
	}
	if !strings.HasPrefix(l, "x-") || l == "x-powered-by" {
		return false
	}
	return l != "x-malbolge" && !strings.HasPrefix(l, "x-malbolge-")
}

// ParseResponse parses program output as a MelCGI response: header lines,
// a blank line, then the body. Lines end in LF or CRLF. The returned Body
// aliases out. Every error wraps one of the Err* sentinels.
func ParseResponse(out []byte) (*Response, error) {
	hdrEnd, bodyStart, err := splitHeader(out)
	if err != nil {
		return nil, err
	}
	res := &Response{Header: make(http.Header), Body: out[bodyStart:]}
	lines := bytes.SplitAfter(out[:hdrEnd], []byte("\n"))
	for i, raw := range lines[:len(lines)-1] { // last element is empty
		name, value, err := parseLine(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: line %d: %q", err, i+1, clip(raw))
		}
		if err := res.add(name, value); err != nil {
			return nil, fmt.Errorf("%w: line %d", err, i+1)
		}
	}
	_, hasLocation := res.Header["Location"]
	if res.Status == 0 {
		res.Status = http.StatusOK
		if hasLocation {
			res.Status = http.StatusFound
		}
	}
	if _, ok := res.Header["Content-Type"]; !ok && !hasLocation {
		return nil, ErrNoContentType
	}
	return res, nil
}

// splitHeader locates the blank line ending the header block. hdrEnd is the
// offset just past the last header line; bodyStart is just past the blank line.
func splitHeader(out []byte) (hdrEnd, bodyStart int, err error) {
	win := out
	if len(win) > MaxHeaderBlock {
		win = win[:MaxHeaderBlock]
	}
	for pos := 0; ; {
		i := bytes.IndexByte(win[pos:], '\n')
		if i < 0 {
			break
		}
		if line := win[pos : pos+i]; len(line) == 0 || (len(line) == 1 && line[0] == '\r') {
			return pos, pos + i + 1, nil
		}
		pos += i + 1
	}
	if len(out) > MaxHeaderBlock {
		return 0, 0, fmt.Errorf("%w: no blank line in the first %d bytes", ErrHeaderTooLarge, MaxHeaderBlock)
	}
	return 0, 0, ErrNoHeaderBlock
}

// parseLine splits one header line (with its LF or CRLF) into a valid token
// name and a value trimmed of surrounding spaces and tabs.
func parseLine(raw []byte) (name, value string, err error) {
	line := string(raw[:len(raw)-1]) // drop '\n'
	line = strings.TrimSuffix(line, "\r")
	if line[0] == ' ' || line[0] == '\t' {
		return "", "", fmt.Errorf("%w: obsolete line folding", ErrBadHeader)
	}
	name, value, ok := strings.Cut(line, ":")
	if !ok {
		return "", "", fmt.Errorf("%w: missing colon", ErrBadHeader)
	}
	if !validToken(name) {
		return "", "", fmt.Errorf("%w: invalid header name", ErrBadHeader)
	}
	value = strings.Trim(value, " \t")
	for i := 0; i < len(value); i++ {
		if isControl(value[i]) {
			return "", "", fmt.Errorf("%w: control byte %#02x in value", ErrBadHeader, value[i])
		}
	}
	return name, value, nil
}

// add records one header on res, enforcing the allowlist and the
// single-occurrence and non-empty rules.
func (res *Response) add(name, value string) error {
	key := textproto.CanonicalMIMEHeaderKey(name)
	if !isAllowed(key) {
		return fmt.Errorf("%w: %s", ErrForbiddenHeader, key)
	}
	if single[key] {
		if _, dup := res.Header[key]; dup || (key == "Status" && res.Status != 0) {
			return fmt.Errorf("%w: duplicate %s", ErrBadHeader, key)
		}
	}
	switch key {
	case "Status":
		code, err := parseStatus(value)
		if err != nil {
			return err
		}
		res.Status = code
		return nil
	case "Content-Type", "Location":
		if value == "" {
			return fmt.Errorf("%w: empty %s", ErrBadHeader, key)
		}
	}
	res.Header[key] = append(res.Header[key], value)
	return nil
}

// parseStatus parses "NNN" or "NNN reason" with NNN in 200..599.
func parseStatus(v string) (int, error) {
	if len(v) < 3 || (len(v) > 3 && v[3] != ' ') {
		return 0, fmt.Errorf("%w: %q", ErrBadStatus, clip([]byte(v)))
	}
	code := 0
	for i := 0; i < 3; i++ {
		c := v[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("%w: %q", ErrBadStatus, clip([]byte(v)))
		}
		code = code*10 + int(c-'0')
	}
	if code < 200 || code > 599 {
		return 0, fmt.Errorf("%w: %d out of range 200..599", ErrBadStatus, code)
	}
	return code, nil
}

// validToken reports whether s is a non-empty RFC 7230 token.
func validToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isTokenChar(s[i]) {
			return false
		}
	}
	return true
}

// isTokenChar reports whether c is an RFC 7230 tchar.
func isTokenChar(c byte) bool {
	switch {
	case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		return true
	}
	return strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0
}

// clip shortens b for use in error messages.
func clip(b []byte) []byte {
	const max = 64
	if len(b) > max {
		return b[:max]
	}
	return b
}

// RawResponse wraps output that is not parsed (raw programs): the whole
// output is the body, with status 200 and the given Content-Type.
func RawResponse(out []byte, contentType string) *Response {
	return &Response{
		Status: http.StatusOK,
		Header: http.Header{"Content-Type": {contentType}},
		Body:   out,
	}
}

// HeaderBlock returns "Content-Type: <contentType>\n\n", the minimal header
// block generated programs print before their body. contentType must be a
// valid header value (no control bytes).
func HeaderBlock(contentType string) []byte {
	return []byte("Content-Type: " + contentType + "\n\n")
}
