package mimetype

import "testing"

func TestByName(t *testing.T) {
	tests := []struct {
		name, want string
	}{
		{"index.html", "text/html; charset=utf-8"},
		{"INDEX.HTM", "text/html; charset=utf-8"},
		{"main-AB12CD34.js", "text/javascript; charset=utf-8"},
		{"chunk.mjs", "text/javascript; charset=utf-8"},
		{"styles.css", "text/css; charset=utf-8"},
		{"data.json", "application/json"},
		{"app.webmanifest", "application/manifest+json"},
		{"main.js.map", "application/json"},
		{"logo.svg", "image/svg+xml"},
		{"photo.PNG", "image/png"},
		{"photo.jpeg", "image/jpeg"},
		{"photo.jpg", "image/jpeg"},
		{"anim.gif", "image/gif"},
		{"pic.webp", "image/webp"},
		{"pic.avif", "image/avif"},
		{"favicon.ico", "image/x-icon"},
		{"font.woff2", "font/woff2"},
		{"font.woff", "font/woff"},
		{"font.ttf", "font/ttf"},
		{"font.otf", "font/otf"},
		{"app.wasm", "application/wasm"},
		{"readme.txt", "text/plain; charset=utf-8"},
		{"feed.xml", "application/xml"},
		{"doc.pdf", "application/pdf"},
		{"clip.mp4", "video/mp4"},
		{"song.mp3", "audio/mpeg"},
		{"noext", "application/octet-stream"},
		{"weird.zzz", "application/octet-stream"},
		{"dir.d/file", "application/octet-stream"},
	}
	for _, tt := range tests {
		if got := ByName(tt.name); got != tt.want {
			t.Errorf("ByName(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestCompressible(t *testing.T) {
	yes := []string{"text/html; charset=utf-8", "text/css; charset=utf-8", "application/json",
		"image/svg+xml", "application/wasm", "text/javascript; charset=utf-8", "application/manifest+json",
		"application/xml", "font/ttf", "font/otf", "image/x-icon"}
	no := []string{"image/png", "image/jpeg", "font/woff2", "font/woff", "video/mp4", "application/octet-stream",
		"image/webp", "image/avif", "audio/mpeg", "application/pdf"}
	for _, ct := range yes {
		if !Compressible(ct) {
			t.Errorf("Compressible(%q) = false, want true", ct)
		}
	}
	for _, ct := range no {
		if Compressible(ct) {
			t.Errorf("Compressible(%q) = true, want false", ct)
		}
	}
}
