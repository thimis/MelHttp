// Package webvm serves MelHttp's browser assets under /_melhttp/: the
// Malbolge VM compiled to WebAssembly, the transport service worker, and the
// playground. melhttp.wasm and wasm_exec.js are generated, not committed:
//
//	go generate ./internal/webvm
package webvm

//go:generate go run ./gen

import (
	"bytes"
	"compress/gzip"
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"sync"
	"time"
)

//go:embed assets
var assets embed.FS

// Prefix is the URL path the assets are served under.
const Prefix = "/_melhttp/"

// ErrMissing means the generated assets were not built into this binary.
var ErrMissing = errors.New("browser VM assets are missing: run `go generate ./internal/webvm` before building")

// Available reports whether the generated assets are present.
func Available() error {
	for _, f := range []string{"assets/melhttp.wasm", "assets/wasm_exec.js"} {
		if _, err := fs.Stat(assets, f); err != nil {
			return ErrMissing
		}
	}
	return nil
}

// Handler serves the assets. The obfuscation files are served when
// transport is set, the playground when playground is set.
func Handler(transport, playground bool) http.Handler {
	allowed := map[string]bool{"melhttp.wasm": true, "wasm_exec.js": true}
	if transport {
		allowed["sw.js"], allowed["obfuscate.js"] = true, true
	}
	if playground {
		allowed["playground.html"] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, Prefix)
		if name == "" && playground {
			name = "playground.html"
		}
		if !allowed[name] {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		data, err := fs.ReadFile(assets, "assets/"+name)
		if err != nil {
			http.Error(w, ErrMissing.Error(), http.StatusServiceUnavailable)
			return
		}
		h := w.Header()
		switch {
		case strings.HasSuffix(name, ".wasm"):
			h.Set("Content-Type", "application/wasm")
		case strings.HasSuffix(name, ".js"):
			h.Set("Content-Type", "text/javascript; charset=utf-8")
		default:
			h.Set("Content-Type", "text/html; charset=utf-8")
		}
		h.Set("Cache-Control", "no-cache")
		h.Add("Vary", "Accept-Encoding")
		if name == "sw.js" {
			h.Set("Service-Worker-Allowed", "/")
		}
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && r.Header.Get("Range") == "" {
			data = gzipped(name, data)
			h.Set("Content-Encoding", "gzip")
		}
		http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(data))
	})
}

var gzCache sync.Map // asset name → gzip bytes

func gzipped(name string, data []byte) []byte {
	if v, ok := gzCache.Load(name); ok {
		return v.([]byte)
	}
	var b bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
	zw.Write(data)
	zw.Close()
	gzCache.Store(name, b.Bytes())
	return b.Bytes()
}
