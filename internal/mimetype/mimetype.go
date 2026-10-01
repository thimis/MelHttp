// Package mimetype maps file names to Content-Type values using a fixed,
// built-in table. It deliberately does not use mime.TypeByExtension, whose
// results vary by OS (Windows registry, /etc/mime.types).
package mimetype

import (
	"path"
	"strings"
)

// Default is returned for unknown extensions.
const Default = "application/octet-stream"

var byExt = map[string]string{
	".html":        "text/html; charset=utf-8",
	".htm":         "text/html; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".cjs":         "text/javascript; charset=utf-8",
	".txt":         "text/plain; charset=utf-8",
	".md":          "text/markdown; charset=utf-8",
	".csv":         "text/csv; charset=utf-8",
	".json":        "application/json",
	".map":         "application/json",
	".webmanifest": "application/manifest+json",
	".xml":         "application/xml",
	".rss":         "application/rss+xml",
	".atom":        "application/atom+xml",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".jpg":         "image/jpeg",
	".jpeg":        "image/jpeg",
	".gif":         "image/gif",
	".webp":        "image/webp",
	".avif":        "image/avif",
	".ico":         "image/x-icon",
	".bmp":         "image/bmp",
	".woff2":       "font/woff2",
	".woff":        "font/woff",
	".ttf":         "font/ttf",
	".otf":         "font/otf",
	".wasm":        "application/wasm",
	".pdf":         "application/pdf",
	".mp4":         "video/mp4",
	".webm":        "video/webm",
	".mp3":         "audio/mpeg",
	".ogg":         "audio/ogg",
	".wav":         "audio/wav",
	".zip":         "application/zip",
	".gz":          "application/gzip",
}

// ByName returns the Content-Type for a file name or URL path, based on its
// final extension (case-insensitive).
func ByName(name string) string {
	ext := strings.ToLower(path.Ext(name))
	if ext == "" || strings.Contains(ext, "/") {
		return Default
	}
	if ct, ok := byExt[ext]; ok {
		return ct
	}
	return Default
}

// Compressible reports whether a response of this Content-Type benefits from
// gzip. Already-compressed formats (images, woff/woff2, media, archives) do not.
func Compressible(contentType string) bool {
	mt := contentType
	if i := strings.IndexByte(mt, ';'); i >= 0 {
		mt = mt[:i]
	}
	mt = strings.TrimSpace(strings.ToLower(mt))
	switch {
	case strings.HasPrefix(mt, "text/"):
		return true
	case strings.HasSuffix(mt, "+json"), strings.HasSuffix(mt, "+xml"):
		return true
	}
	switch mt {
	case "application/json", "application/xml", "application/wasm", "application/javascript",
		"font/ttf", "font/otf", "image/x-icon", "image/bmp":
		return true
	}
	return false
}
