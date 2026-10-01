package crawl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, data := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFilesSkipsDotfilesButKeepsWellKnown(t *testing.T) {
	root := writeTree(t, map[string]string{
		"index.html":               "x",
		"a/b c/ünï.txt":            "y",
		".git/config":              "no",
		".env":                     "no",
		".well-known/security.txt": "z",
		"skipme/file.txt":          "no",
	})
	got, err := Files(root, func(rel string) bool { return rel == "skipme" })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{".well-known/security.txt", "a/b c/ünï.txt", "index.html"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("Files = %q, want %q", got, want)
	}
}

func TestURLPathEscapes(t *testing.T) {
	if got := URLPath("a/b c/ünï.txt"); got != "/a/b%20c/%C3%BCn%C3%AF.txt" {
		t.Fatalf("URLPath = %q", got)
	}
}

func TestSiteDetectsMismatches(t *testing.T) {
	root := writeTree(t, map[string]string{
		"good.html":   "<p>hi</p>",
		"bad.css":     "body{}",
		"missing.js":  "1",
		"wrongct.txt": "plain",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/good.html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte("<p>hi</p>"))
		case "/bad.css":
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
			w.Write([]byte("body{ }"))
		case "/wrongct.txt":
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("plain"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	rep, err := Site(context.Background(), srv.URL, root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Checked != 4 {
		t.Fatalf("Checked = %d, want 4", rep.Checked)
	}
	var bad []string
	for _, m := range rep.Mismatches {
		bad = append(bad, m.Path)
	}
	if strings.Join(bad, ",") != "bad.css,missing.js,wrongct.txt" {
		t.Fatalf("mismatches = %v\n%s", bad, rep)
	}
	if rep.OK() {
		t.Fatal("OK() = true with mismatches")
	}
}
