// Package build compiles a whole website into a MelHttp site: every file
// becomes a verified Malbolge program (or chunk directory) that prints a
// MelCGI header block followed by the file's bytes.
package build

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thimis/MelHttp/internal/gen"
	"github.com/thimis/MelHttp/internal/mbfile"
	"github.com/thimis/MelHttp/internal/melcgi"
	"github.com/thimis/MelHttp/internal/mimetype"
	"github.com/thimis/MelHttp/internal/server"
	"github.com/thimis/MelHttp/internal/wasi"
)

// GeneratorVersion changes whenever generated code changes, invalidating the
// incremental-build manifest.
const GeneratorVersion = "gen-3"

// Marker marks a directory as melc output; only such directories are ever
// replaced by a build.
const Marker = ".melc-build"

const manifestName = ".melc-manifest.json"

// Options configures Site.
type Options struct {
	Src     string // directory to compile
	Out     string // output site directory
	SPA     *bool  // write "spa" into melhttp.json (nil: keep the source's setting or false)
	Seed    uint64
	Workers int // files compiled in parallel (default GOMAXPROCS)
	Log     io.Writer
}

// Stats describes a build.
type Stats struct {
	Files, Compiled, Reused, Copied int
	InBytes, OutBytes               int64
	Programs                        int
	Duration                        time.Duration
}

func (s Stats) String() string {
	ratio := 0.0
	if s.InBytes > 0 {
		ratio = float64(s.OutBytes) / float64(s.InBytes)
	}
	return fmt.Sprintf("%d files (%d compiled, %d unchanged, %d hand-written programs) → %d programs; %d → %d bytes (%.1f×) in %v",
		s.Files, s.Compiled, s.Reused, s.Copied, s.Programs, s.InBytes, s.OutBytes, ratio, s.Duration.Round(time.Millisecond))
}

type manifest struct {
	Generator string            `json:"generator"`
	Files     map[string]string `json:"files"` // source rel path → content hash
}

// Site compiles opt.Src into opt.Out.
func Site(ctx context.Context, opt Options) (Stats, error) {
	start := time.Now()
	var st Stats
	if opt.Workers <= 0 {
		opt.Workers = runtime.GOMAXPROCS(0)
	}
	if opt.Log == nil {
		opt.Log = io.Discard
	}
	src, err := filepath.Abs(opt.Src)
	if err != nil {
		return st, err
	}
	out, err := filepath.Abs(opt.Out)
	if err != nil {
		return st, err
	}
	if err := checkPaths(src, out); err != nil {
		return st, err
	}
	files, err := listFiles(src)
	if err != nil {
		return st, err
	}

	old := readManifest(out)
	tmp := fmt.Sprintf("%s.tmp-%d", out, os.Getpid())
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return st, err
	}
	defer os.RemoveAll(tmp)

	next := manifest{Generator: GeneratorVersion, Files: map[string]string{}}
	var mu sync.Mutex
	var firstErr error
	var compiled, reused, copied, programs atomic.Int64
	var inBytes, outBytes atomic.Int64
	jobs := make(chan string)
	var wg sync.WaitGroup
	for range opt.Workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for rel := range jobs {
				kind, hash, nIn, nOut, nProg, err := buildFile(ctx, src, out, tmp, rel, old, opt.Seed)
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", rel, err)
				}
				if hash != "" {
					next.Files[rel] = hash
				}
				mu.Unlock()
				switch kind {
				case "compiled":
					compiled.Add(1)
				case "reused":
					reused.Add(1)
				case "copied":
					copied.Add(1)
				}
				inBytes.Add(nIn)
				outBytes.Add(nOut)
				programs.Add(int64(nProg))
				if err == nil {
					fmt.Fprintf(opt.Log, "  %-8s %s\n", kind, rel)
				}
			}
		}()
	}
	for _, rel := range files {
		if ctx.Err() != nil {
			break
		}
		jobs <- rel
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return st, firstErr
	}
	if err := ctx.Err(); err != nil {
		return st, err
	}
	if err := writeSiteConfig(src, tmp, opt.SPA); err != nil {
		return st, err
	}
	if err := writeJSON(filepath.Join(tmp, manifestName), next); err != nil {
		return st, err
	}
	if err := os.WriteFile(filepath.Join(tmp, Marker), []byte("built by melc; this directory is replaced on rebuild\n"), 0o644); err != nil {
		return st, err
	}
	if err := publish(tmp, out, files); err != nil {
		return st, err
	}
	st = Stats{
		Files: len(files), Compiled: int(compiled.Load()), Reused: int(reused.Load()), Copied: int(copied.Load()),
		InBytes: inBytes.Load(), OutBytes: outBytes.Load(), Programs: int(programs.Load()), Duration: time.Since(start),
	}
	return st, nil
}

// checkPaths refuses outputs that would clobber the source or a directory
// that melc did not create.
func checkPaths(src, out string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", src)
	}
	if out == src || within(out, src) || within(src, out) {
		return fmt.Errorf("output %s must not be inside the source %s (or contain it)", out, src)
	}
	if entries, err := os.ReadDir(out); err == nil && len(entries) > 0 {
		if _, err := os.Stat(filepath.Join(out, Marker)); err != nil {
			return fmt.Errorf("refusing to replace %s: it exists and was not created by melc build", out)
		}
	}
	return nil
}

func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}

// listFiles returns the slash paths of the files to build, skipping dotfiles
// (except .well-known), symlinks and the site config. Names differing only in
// case are an error: they would collide on Windows and macOS.
func listFiles(src string) ([]string, error) {
	var files []string
	lower := map[string]string{}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == src {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") && !(name == ".well-known" && d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if strings.HasSuffix(strings.ToLower(name), ".mb") {
				files = append(files, rel) // a hand-written chunk directory
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || rel == server.SiteConfigFile {
			return nil
		}
		key := strings.ToLower(rel)
		if prev, ok := lower[key]; ok {
			return fmt.Errorf("%s and %s differ only in case; they would collide on Windows and macOS", prev, rel)
		}
		lower[key] = rel
		files = append(files, rel)
		return nil
	})
	sort.Strings(files)
	return files, err
}

// buildFile compiles (or reuses, or copies) one source file into tmp.
func buildFile(ctx context.Context, src, out, tmp, rel string, old manifest, seed uint64) (kind, hash string, nIn, nOut int64, nProg int, err error) {
	srcPath := filepath.Join(src, filepath.FromSlash(rel))
	if strings.HasSuffix(strings.ToLower(rel), ".wasi") {
		// A WebAssembly handler: check it is one and copy it as is.
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return "", "", 0, 0, 0, err
		}
		if !bytes.HasPrefix(data, []byte(wasi.Magic)) {
			return "", "", 0, 0, 0, errors.New("not a WebAssembly module")
		}
		n, err := copyTree(srcPath, filepath.Join(tmp, filepath.FromSlash(rel)))
		return "copied", "", n, n, 0, err
	}
	if strings.HasSuffix(strings.ToLower(rel), ".mb") {
		// A hand-written program: validate it and copy it as is.
		set, err := mbfile.Load(srcPath)
		if err != nil {
			return "", "", 0, 0, 0, err
		}
		n, err := copyTree(srcPath, filepath.Join(tmp, filepath.FromSlash(rel)))
		return "copied", "", n, n, len(set.Programs), err
	}
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	sum := sha256.Sum256(append([]byte(fmt.Sprintf("%s|%d|", GeneratorVersion, seed)), data...))
	hash = hex.EncodeToString(sum[:])
	dst := filepath.Join(tmp, filepath.FromSlash(rel)+".mb")
	prev := filepath.Join(out, filepath.FromSlash(rel)+".mb")
	if old.Generator == GeneratorVersion && old.Files[rel] == hash {
		// Unchanged: leave the existing output alone (a running server keeps
		// its cached response, since the files' times do not change).
		if set, err := mbfile.Load(prev); err == nil {
			return "reused", hash, int64(len(data)), treeSize(prev), len(set.Programs), nil
		}
	}
	program := append(melcgi.HeaderBlock(mimetype.ByName(rel)), data...)
	chunks, err := gen.Compile(program, gen.Options{Seed: seed})
	if err != nil {
		return "", "", 0, 0, 0, err
	}
	if err := gen.Verify(ctx, chunks, program); err != nil {
		return "", "", 0, 0, 0, fmt.Errorf("self-check failed (please report this bug): %w", err)
	}
	if err := mbfile.Write(dst, chunks); err != nil {
		return "", "", 0, 0, 0, err
	}
	for _, c := range chunks {
		nOut += int64(len(c))
	}
	return "compiled", hash, int64(len(data)), nOut, len(chunks), nil
}

// copyTree copies a file or a directory of files and returns the bytes copied.
func copyTree(from, to string) (int64, error) {
	fi, err := os.Stat(from)
	if err != nil {
		return 0, err
	}
	if !fi.IsDir() {
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			return 0, err
		}
		data, err := os.ReadFile(from)
		if err != nil {
			return 0, err
		}
		return int64(len(data)), os.WriteFile(to, data, 0o644)
	}
	var total int64
	entries, err := os.ReadDir(from)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		n, err := copyTree(filepath.Join(from, e.Name()), filepath.Join(to, e.Name()))
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func readManifest(out string) manifest {
	var m manifest
	if data, err := os.ReadFile(filepath.Join(out, manifestName)); err == nil {
		json.Unmarshal(data, &m)
	}
	return m
}

// writeSiteConfig writes melhttp.json: the source's own file if it has one,
// with "spa" set when the caller decided it.
func writeSiteConfig(src, tmp string, spa *bool) error {
	cfg := map[string]any{}
	if data, err := os.ReadFile(filepath.Join(src, server.SiteConfigFile)); err == nil {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("%s: %w", server.SiteConfigFile, err)
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if spa != nil {
		cfg["spa"] = *spa
	}
	return writeJSON(filepath.Join(tmp, server.SiteConfigFile), cfg)
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// outputPath is where the output for a source file lives in the site.
func outputPath(rel string) string {
	l := strings.ToLower(rel)
	if strings.HasSuffix(l, ".mb") || strings.HasSuffix(l, ".wasi") {
		return rel // hand-written programs and WASI handlers are copied as is
	}
	return rel + ".mb"
}

// publish moves the staged build in tmp into out. A new out is created with
// a single rename. An existing out is updated in place — staged outputs
// replace their old versions, outputs of deleted sources are removed, and
// unchanged outputs are left untouched — so a server can keep serving the
// directory while it is rebuilt (replacing the directory itself would fail
// on Windows and leave a server on Linux serving the old copy).
func publish(tmp, out string, files []string) error {
	if entries, err := os.ReadDir(out); err != nil || len(entries) == 0 {
		os.Remove(out)
		return os.Rename(tmp, out)
	}
	keep := map[string]bool{server.SiteConfigFile: true, manifestName: true, Marker: true}
	parents := map[string]bool{}
	for _, rel := range files {
		p := outputPath(rel)
		keep[p] = true
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			parents[d] = true
		}
	}
	for p := range keep {
		staged := filepath.Join(tmp, filepath.FromSlash(p))
		if _, err := os.Lstat(staged); err != nil {
			continue // unchanged: nothing staged
		}
		dst := filepath.Join(out, filepath.FromSlash(p))
		if err := os.RemoveAll(dst); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := os.Rename(staged, dst); err != nil {
			return err
		}
	}
	// Remove outputs whose sources are gone.
	return filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == out {
			return err
		}
		rel, _ := filepath.Rel(out, p)
		rel = filepath.ToSlash(rel)
		switch {
		case keep[rel]:
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		case d.IsDir() && parents[rel]:
			return nil
		}
		if err := os.RemoveAll(p); err != nil {
			return err
		}
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
}

func treeSize(p string) int64 {
	var n int64
	filepath.WalkDir(p, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				n += fi.Size()
			}
		}
		return nil
	})
	return n
}
