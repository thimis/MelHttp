package webvm

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thimis/MelHttp/internal/obfs"
	"github.com/thimis/MelHttp/internal/testutil"
)

func requireAssets(t *testing.T) {
	t.Helper()
	if err := Available(); err != nil {
		t.Fatal(err) // CI runs go generate first; locally: go generate ./internal/webvm
	}
}

func TestHandler(t *testing.T) {
	requireAssets(t)
	ts := httptest.NewServer(Handler(true, false))
	defer ts.Close()
	for path, want := range map[string]string{
		"/_melhttp/melhttp.wasm": "application/wasm", "/_melhttp/sw.js": "text/javascript; charset=utf-8",
		"/_melhttp/obfuscate.js": "text/javascript; charset=utf-8", "/_melhttp/wasm_exec.js": "text/javascript; charset=utf-8",
	} {
		res, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 || res.Header.Get("Content-Type") != want {
			t.Errorf("%s: %d %q", path, res.StatusCode, res.Header.Get("Content-Type"))
		}
		if path == "/_melhttp/sw.js" && res.Header.Get("Service-Worker-Allowed") != "/" {
			t.Error("sw.js needs Service-Worker-Allowed: /")
		}
	}
	for _, p := range []string{"/_melhttp/playground.html", "/_melhttp/", "/_melhttp/../webvm.go", "/_melhttp/nope.js"} {
		if res, _ := http.Get(ts.URL + p); res.StatusCode != 404 {
			t.Errorf("%s = %d, want 404", p, res.StatusCode)
		}
	}
	// gzip for clients that accept it (the wasm shrinks a lot).
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/_melhttp/melhttp.wasm", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	res, err := (&http.Transport{DisableCompression: true}).RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	gz, _ := io.ReadAll(res.Body)
	res.Body.Close()
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil || res.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("gzip: %v %v", err, res.Header)
	}
	plain, _ := io.ReadAll(zr)
	want, _ := assets.ReadFile("assets/melhttp.wasm")
	if !bytes.Equal(plain, want) {
		t.Fatal("gzip body differs")
	}
	t.Logf("melhttp.wasm: %d bytes, %d gzipped", len(want), len(gz))

	pg := httptest.NewServer(Handler(false, true))
	defer pg.Close()
	for p, code := range map[string]int{"/_melhttp/": 200, "/_melhttp/playground.html": 200, "/_melhttp/sw.js": 404} {
		if res, _ := http.Get(pg.URL + p); res.StatusCode != code {
			t.Errorf("playground-only %s = %d, want %d", p, res.StatusCode, code)
		}
	}
}

// TestWasmInNode runs the real WebAssembly build under Node.js: it must run
// Malbolge and decode the transport encoding exactly like the native code.
func TestWasmInNode(t *testing.T) {
	requireAssets(t)
	node, err := exec.LookPath("node")
	if err != nil {
		testutil.Skip(t, "node not installed")
	}
	dir := t.TempDir()
	body := []byte("decoded by Go-in-WebAssembly in Node ✓ \x00\xa9\xff")
	container, err := obfs.Encode(body, 42)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "container.mb"), container, 0o644)
	os.WriteFile(filepath.Join(dir, "expected.bin"), body, 0o644)
	script := `
const fs = require("fs"), path = require("path");
require(path.resolve(process.argv[2], "wasm_exec.js"));
const go = new Go();
WebAssembly.instantiate(fs.readFileSync(path.resolve(process.argv[2], "melhttp.wasm")), go.importObject).then(({ instance }) => {
  go.run(instance);
  const hello = melhttpRun("(=<` + "`" + `#9]~6ZY327Uv4-QsqpMn&+Ij\"'E%e{Ab~w=_:]Kw%o44Uqp0/Q?xNvL:` + "`" + `H%c#DD2^WV>gY;dts76qKJImZkj", new Uint8Array(0), 1000);
  const out = Buffer.from(hello.output).toString();
  if (out !== "Hello, world." || !hello.halted) throw new Error("run: " + out + " " + hello.error);
  const d = melhttpDecode(new Uint8Array(fs.readFileSync(path.join(process.argv[3], "container.mb"))));
  if (d.error) throw new Error(d.error);
  if (!Buffer.from(d.data).equals(fs.readFileSync(path.join(process.argv[3], "expected.bin")))) throw new Error("decode differs");
  const c = melhttpCompile(new TextEncoder().encode("hi"), 0);
  const r = melhttpRun(c.programs, null, 1000000);
  if (Buffer.from(r.output).toString() !== "hi") throw new Error("compile+run: " + Buffer.from(r.output));
  console.log("ok " + melhttpVersion);
  process.exit(0);
});
`
	os.WriteFile(filepath.Join(dir, "check.js"), []byte(script), 0o644)
	abs, _ := filepath.Abs("assets")
	out, err := exec.Command(node, filepath.Join(dir, "check.js"), abs, dir).CombinedOutput()
	if err != nil || !strings.HasPrefix(string(out), "ok") {
		t.Fatalf("node: %v\n%s", err, out)
	}
}
