//go:build acceptance && windows

package acceptance

import (
	"net/http"
	"os/exec"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// TestG15_WindowsService installs melhttpd as a real Windows service, starts
// it, checks it serves, then stops and removes it. Needs an elevated prompt
// (GitHub's Windows runners are elevated).
func TestG15_WindowsService(t *testing.T) {
	requireServer(t)
	if !windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("needs an elevated (Administrator) prompt to install a service")
	}
	site := t.TempDir()
	src := t.TempDir()
	melc(t, nil, "build", "-o", site, writeFileDir(t, src, "index.html", "<h1>from a Windows service</h1>"))
	addr := freeAddr(t)
	name := "melhttpd-g15-test"
	ctl := func(args ...string) {
		t.Helper()
		out, err := exec.Command(serverBin, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("melhttpd %v: %v\n%s", args, err, out)
		}
	}
	exec.Command(serverBin, "-service", "uninstall", "-service-name", name).Run() // leftovers
	ctl("-service", "install", "-service-name", name, "-root", site, "-addr", addr)
	t.Cleanup(func() { exec.Command(serverBin, "-service", "uninstall", "-service-name", name).Run() })
	ctl("-service", "start", "-service-name", name)
	var ok bool
	for i := 0; i < 100 && !ok; i++ {
		if resp, err := http.Get("http://" + addr + "/"); err == nil {
			ok = resp.StatusCode == 200 && resp.Header.Get("X-Powered-By") == "Malbolge"
			resp.Body.Close()
		}
		if !ok {
			time.Sleep(200 * time.Millisecond)
		}
	}
	if !ok {
		t.Fatal("the service never served the site")
	}
	ctl("-service", "stop", "-service-name", name)
	stopped := false
	for i := 0; i < 50 && !stopped; i++ {
		_, err := http.Get("http://" + addr + "/healthz")
		stopped = err != nil
		time.Sleep(200 * time.Millisecond)
	}
	if !stopped {
		t.Error("the service did not stop")
	}
	ctl("-service", "uninstall", "-service-name", name)
}

func writeFileDir(t *testing.T, dir, name, content string) string {
	writeFile(t, dir, name, []byte(content))
	return dir
}
