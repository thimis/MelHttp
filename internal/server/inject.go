package server

import (
	"bytes"
	"mime"

	"github.com/thimis/MelHttp/internal/webvm"
)

// transportScript is the tag that opts a page in to the Malbolge transport.
const transportScript = `<script src="` + webvm.Prefix + `obfuscate.js"></script>`

// maybeInject adds the transport script to an HTML response when
// Config.ObfuscateInject is set, so any site can use the transport without
// editing its pages. It runs before the ETag and gzip variant are computed.
func (s *Server) maybeInject(r *result) {
	if !s.cfg.ObfuscateInject {
		return
	}
	if mt, _, _ := mime.ParseMediaType(r.header.Get("Content-Type")); mt != "text/html" {
		return
	}
	r.body = injectTransport(r.body)
}

// injectTransport returns an HTML document with the transport script added
// before </head>, or at the end when the document has no </head>. Pages that
// already include the script are returned unchanged.
func injectTransport(body []byte) []byte {
	if bytes.Contains(body, []byte(webvm.Prefix+"obfuscate.js")) {
		return body
	}
	out := make([]byte, 0, len(body)+len(transportScript))
	if i := indexASCIIFold(body, "</head"); i >= 0 {
		out = append(out, body[:i]...)
		out = append(out, transportScript...)
		return append(out, body[i:]...)
	}
	out = append(out, body...)
	return append(out, transportScript...)
}

// indexASCIIFold finds lower-case ASCII needle in b, ignoring ASCII case.
// (Unlike lower-casing the whole page, it never shifts byte offsets.)
func indexASCIIFold(b []byte, needle string) int {
	for i := 0; i+len(needle) <= len(b); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			c := b[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
