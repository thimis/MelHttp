// Command dist cross-compiles melhttpd and melc for every supported platform
// and packages them with checksums:
//
//	go run ./tools/dist                 # version from git describe
//	go run ./tools/dist -version v0.1.0 -out dist
//
// A release version (vMAJOR.MINOR.PATCH) must match internal/version, so a
// tag and the version the binaries report cannot disagree.
package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	melversion "github.com/thimis/MelHttp/internal/version"
)

// moduleRoot is where packages are built from (tests point it at the repo root).
var moduleRoot = "."

// Targets are the release platforms.
var Targets = []string{
	"linux/amd64", "linux/arm64",
	"windows/amd64", "windows/arm64",
	"darwin/amd64", "darwin/arm64",
	"freebsd/amd64", "freebsd/arm64",
}

func main() {
	version := flag.String("version", "", "version string (default: git describe)")
	out := flag.String("out", "dist", "output directory")
	flag.Parse()
	if *version == "" {
		*version = gitVersion()
	}
	if err := checkVersion(*version); err != nil {
		fail(err)
	}
	// Build the browser VM assets that melhttpd embeds (-obfuscate, -playground).
	generate := exec.Command("go", "generate", "./internal/webvm")
	generate.Stdout, generate.Stderr = os.Stdout, os.Stderr
	if err := generate.Run(); err != nil {
		fail(fmt.Errorf("go generate: %w", err))
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		fail(err)
	}
	readme, err := os.ReadFile("README.md")
	if err != nil {
		fail(err)
	}
	var (
		mu       sync.Mutex
		sums     []string
		firstErr error
		wg       sync.WaitGroup
	)
	for _, t := range Targets {
		wg.Add(1)
		go func() {
			defer wg.Done()
			name, sum, err := release(t, *version, *out, readme)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", t, err)
				}
				return
			}
			sums = append(sums, fmt.Sprintf("%x  %s", sum, name))
			fmt.Println("built", name)
		}()
	}
	wg.Wait()
	if firstErr != nil {
		fail(firstErr)
	}
	sort.Strings(sums)
	if err := os.WriteFile(filepath.Join(*out, "SHA256SUMS"), []byte(strings.Join(sums, "\n")+"\n"), 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("%d archives and SHA256SUMS in %s (version %s)\n", len(sums), *out, *version)
}

// release builds both binaries for one target and archives them.
func release(target, version, out string, readme []byte) (string, [32]byte, error) {
	goos, goarch, _ := strings.Cut(target, "/")
	exe := ""
	if goos == "windows" {
		exe = ".exe"
	}
	files := map[string][]byte{"README.md": readme}
	for _, cmd := range []string{"melhttpd", "melc"} {
		bin, err := goBuild(goos, goarch, version, "./cmd/"+cmd)
		if err != nil {
			return "", [32]byte{}, err
		}
		files[cmd+exe] = bin
	}
	base := fmt.Sprintf("melhttp_%s_%s_%s", strings.TrimPrefix(version, "v"), goos, goarch)
	var buf bytes.Buffer
	var name string
	var err error
	if goos == "windows" {
		name, err = base+".zip", writeZip(&buf, base, files)
	} else {
		name, err = base+".tar.gz", writeTarGz(&buf, base, files)
	}
	if err != nil {
		return "", [32]byte{}, err
	}
	return name, sha256.Sum256(buf.Bytes()), os.WriteFile(filepath.Join(out, name), buf.Bytes(), 0o644)
}

func goBuild(goos, goarch, version, pkg string) ([]byte, error) {
	tmp, err := os.MkdirTemp("", "melhttp-dist-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "bin")
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+version, "-o", bin, pkg)
	cmd.Dir = moduleRoot
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("go build %s: %v\n%s", pkg, err, out)
	}
	return os.ReadFile(bin)
}

func sortedNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func writeTarGz(w io.Writer, dir string, files map[string][]byte) error {
	zw := gzip.NewWriter(w)
	tw := tar.NewWriter(zw)
	for _, n := range sortedNames(files) {
		mode := int64(0o644)
		if !strings.HasSuffix(n, ".md") {
			mode = 0o755
		}
		hdr := &tar.Header{Name: dir + "/" + n, Mode: mode, Size: int64(len(files[n])), ModTime: time.Unix(0, 0)}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(files[n]); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return zw.Close()
}

func writeZip(w io.Writer, dir string, files map[string][]byte) error {
	zw := zip.NewWriter(w)
	for _, n := range sortedNames(files) {
		f, err := zw.CreateHeader(&zip.FileHeader{Name: dir + "/" + n, Method: zip.Deflate, Modified: time.Unix(0, 0)})
		if err != nil {
			return err
		}
		if _, err := f.Write(files[n]); err != nil {
			return err
		}
	}
	return zw.Close()
}

var releaseVersion = regexp.MustCompile(`^v?\d+\.\d+\.\d+$`)

// checkVersion refuses a release version that differs from internal/version.
// Other versions (git describe output such as v0.1.0-3-gabc1234, or "dev")
// are development builds and pass.
func checkVersion(v string) error {
	if releaseVersion.MatchString(v) && strings.TrimPrefix(v, "v") != melversion.Number {
		return fmt.Errorf("version %s does not match internal/version (%s): update internal/version.Number, CHANGELOG.md and docs/install.md first", v, melversion.Number)
	}
	return nil
}

func gitVersion() string {
	out, err := exec.Command("git", "describe", "--tags", "--always", "--dirty").Output()
	if err != nil {
		return "dev"
	}
	return strings.TrimSpace(string(out))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "dist:", err)
	os.Exit(1)
}
