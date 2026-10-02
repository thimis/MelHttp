package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	melversion "github.com/thimis/MelHttp/internal/version"
)

func init() { moduleRoot = filepath.Join("..", "..") }

func TestReleaseLinuxTarball(t *testing.T) {
	out := t.TempDir()
	name, sum, err := release("linux/amd64", "v0.0.0-test", out, []byte("# readme"))
	if err != nil {
		t.Fatal(err)
	}
	if name != "melhttp_0.0.0-test_linux_amd64.tar.gz" {
		t.Fatalf("name %q", name)
	}
	data, _ := os.ReadFile(filepath.Join(out, name))
	if sha256.Sum256(data) != sum {
		t.Fatal("checksum does not match the file")
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	files := map[string]*tar.Header{}
	contents := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		files[h.Name] = h
		contents[h.Name], _ = io.ReadAll(tr)
	}
	dir := "melhttp_0.0.0-test_linux_amd64/"
	for _, f := range []string{"melhttpd", "melc"} {
		h := files[dir+f]
		if h == nil || h.Mode&0o111 == 0 {
			t.Fatalf("%s missing or not executable: %+v", f, h)
		}
		if !bytes.HasPrefix(contents[dir+f], []byte("\x7fELF")) {
			t.Errorf("%s is not a Linux executable", f)
		}
	}
	if string(contents[dir+"README.md"]) != "# readme" {
		t.Error("README missing")
	}
}

func TestReleaseWindowsZip(t *testing.T) {
	out := t.TempDir()
	name, _, err := release("windows/arm64", "v1.2.3", out, []byte("r"))
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(filepath.Join(out, name))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	want := "melhttp_1.2.3_windows_arm64/README.md,melhttp_1.2.3_windows_arm64/melc.exe,melhttp_1.2.3_windows_arm64/melhttpd.exe"
	if strings.Join(names, ",") != want {
		t.Fatalf("zip contents %v", names)
	}
	rc, _ := zr.File[1].Open()
	head := make([]byte, 2)
	io.ReadFull(rc, head)
	rc.Close()
	if string(head) != "MZ" {
		t.Error("not a Windows executable")
	}
}

func TestArchivesAreReproducible(t *testing.T) {
	files := map[string][]byte{"b": []byte("2"), "a": []byte("1"), "README.md": []byte("r")}
	var t1, t2, z1, z2 bytes.Buffer
	writeTarGz(&t1, "d", files)
	writeTarGz(&t2, "d", files)
	writeZip(&z1, "d", files)
	writeZip(&z2, "d", files)
	if !bytes.Equal(t1.Bytes(), t2.Bytes()) || !bytes.Equal(z1.Bytes(), z2.Bytes()) {
		t.Fatal("archives differ between identical builds")
	}
}

func TestReleaseBadTarget(t *testing.T) {
	if _, _, err := release("plan9/notanarch", "v1", t.TempDir(), nil); err == nil {
		t.Fatal("built for a nonexistent platform")
	}
}

func TestCheckVersion(t *testing.T) {
	for v, ok := range map[string]bool{
		"v" + melversion.Number: true,
		melversion.Number:       true,
		"v99.0.0":               false,
		"0.0.1":                 false,
		"v0.1.0-3-gabc1234":     true, // development build between tags
		"abc1234-dirty":         true,
		"dev":                   true,
	} {
		if err := checkVersion(v); (err == nil) != ok {
			t.Errorf("checkVersion(%q) = %v", v, err)
		}
	}
}
