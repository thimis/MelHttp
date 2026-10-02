package server

import (
	"io/fs"
	"path"
	"strings"
	"sync"
	"time"
)

type kind uint8

const (
	kindStatic  kind = iota // a plain file
	kindProgram             // a MelCGI program (.mb file or chunk directory)
	kindRaw                 // a program whose whole output is the body (.raw.mb)
	kindWASI                // a WebAssembly (WASI) MelCGI handler (.wasi)
	kindWASIRaw             // a WASI handler whose whole output is the body (.raw.wasi)
)

// entry is one servable URL.
type entry struct {
	url  string // "/about.html"
	file string // slash path under the root: "about.html.mb"
	kind kind
	dir  bool // a chunk directory
}

// index maps URL paths to entries. It is built from directory listings, so
// only names that really exist under the root, spelled exactly as on disk,
// can ever match; this rules out traversal, Windows alternate data streams,
// 8.3 short names and case games on every OS.
type index struct {
	mu       sync.RWMutex
	fsys     fs.FS
	entries  map[string]*entry
	dirs     map[string]bool // URL paths of directories ("/docs")
	scanned  time.Time
	minAge   time.Duration
	warnings []string
}

func newIndex(fsys fs.FS, minAge time.Duration) *index {
	ix := &index{fsys: fsys, minAge: minAge}
	ix.scan()
	return ix
}

// scan rebuilds the index from the file system.
func (ix *index) scan() {
	entries := map[string]*entry{}
	dirs := map[string]bool{"/": true}
	var warnings []string
	add := func(e *entry) {
		if old, ok := entries[e.url]; ok {
			// A program wins over a static file of the same URL.
			if old.kind != kindStatic || e.kind == kindStatic {
				warnings = append(warnings, "ignoring "+e.file+": "+old.file+" serves "+e.url)
				return
			}
			warnings = append(warnings, "ignoring "+old.file+": "+e.file+" serves "+e.url)
		}
		entries[e.url] = e
	}
	fs.WalkDir(ix.fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == "." {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") && !(name == ".well-known" && d.IsDir()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		isDir := d.IsDir()
		if !isDir && !d.Type().IsRegular() {
			return nil // symlinks and devices are never served
		}
		lower := strings.ToLower(name)
		switch {
		case strings.HasSuffix(lower, ".raw.wasi") && !isDir:
			add(&entry{url: "/" + p[:len(p)-len(".raw.wasi")], file: p, kind: kindWASIRaw})
		case strings.HasSuffix(lower, ".wasi") && !isDir:
			add(&entry{url: "/" + p[:len(p)-len(".wasi")], file: p, kind: kindWASI})
		case strings.HasSuffix(lower, ".raw.mb"):
			add(&entry{url: "/" + p[:len(p)-len(".raw.mb")], file: p, kind: kindRaw, dir: isDir})
		case strings.HasSuffix(lower, ".mb"):
			add(&entry{url: "/" + p[:len(p)-len(".mb")], file: p, kind: kindProgram, dir: isDir})
		case isDir:
			dirs["/"+p] = true
			return nil
		case p == SiteConfigFile:
		default:
			add(&entry{url: "/" + p, file: p, kind: kindStatic})
		}
		if isDir {
			return fs.SkipDir // a chunk directory's files are not URLs
		}
		return nil
	})
	ix.mu.Lock()
	ix.entries, ix.dirs, ix.warnings, ix.scanned = entries, dirs, warnings, time.Now()
	ix.mu.Unlock()
}

// lookup returns the entry for a URL path. A miss triggers a rescan (at most
// once per minAge), so new files appear without a restart.
func (ix *index) lookup(url string) *entry {
	ix.mu.RLock()
	e := ix.entries[url]
	stale := time.Since(ix.scanned) >= ix.minAge
	ix.mu.RUnlock()
	if e == nil && stale {
		ix.scan()
		ix.mu.RLock()
		e = ix.entries[url]
		ix.mu.RUnlock()
	}
	return e
}

func (ix *index) isDir(url string) bool {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.dirs[url]
}

// all returns every entry (for warm-up).
func (ix *index) all() []*entry {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	out := make([]*entry, 0, len(ix.entries))
	for _, e := range ix.entries {
		out = append(out, e)
	}
	return out
}

// validURLPath reports whether a decoded URL path is acceptable at all.
// Lookups are exact, so this is only defense in depth.
func validURLPath(p string) bool {
	if p == "" || p[0] != '/' || strings.ContainsAny(p, "\\\x00") {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] < 0x20 || p[i] == 0x7f {
			return false
		}
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." || seg == "." {
			return false
		}
	}
	return path.Clean(p) == p || strings.HasSuffix(p, "/") && path.Clean(p)+"/" == p
}
