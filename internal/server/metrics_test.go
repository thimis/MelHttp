package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestMetrics(t *testing.T) {
	root, _ := site(t)
	s, ts := newTestServer(t, root, Config{MaxSteps: 100_000})
	get(t, ts.URL+"/about.html") // miss
	get(t, ts.URL+"/about.html") // hit
	get(t, ts.URL+"/missing")    // 404 (custom page: a hit or miss of /404.html)
	get(t, ts.URL+"/cat.txt")    // step limit → 500
	mts := httptest.NewServer(s.MetricsHandler("test-1"))
	defer mts.Close()
	res, err := http.Get(mts.URL)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	text := string(body)
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Errorf("content type %q", res.Header.Get("Content-Type"))
	}
	for _, want := range []string{
		`melhttp_build_info{version="test-1"} 1`,
		`melhttp_requests_total{code="2xx"} 2`,
		`melhttp_requests_total{code="4xx"} 1`,
		`melhttp_requests_total{code="5xx"} 1`,
		`melhttp_request_duration_seconds_count 4`,
		`melhttp_request_duration_seconds_bucket{le="+Inf"} 4`,
		`melhttp_execution_errors_total 1`,
		`melhttp_wasi_runs_total 0`,
	} {
		if !strings.Contains(text, want+"\n") {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if !regexp.MustCompile(`(?m)^melhttp_cache_hits_total [1-9]`).MatchString(text) ||
		!regexp.MustCompile(`(?m)^melhttp_vm_steps_total [1-9]\d*$`).MatchString(text) ||
		!regexp.MustCompile(`(?m)^melhttp_cache_entries [1-9]`).MatchString(text) {
		t.Errorf("counters not moving:\n%s", text)
	}
	// Every sample line is "name{labels} value" or "name value"; every metric has HELP and TYPE.
	sample := regexp.MustCompile(`^[a-z_]+(\{[^}]*\})? [0-9.e+-]+$`)
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if !strings.HasPrefix(line, "#") && !sample.MatchString(line) {
			t.Errorf("bad sample line %q", line)
		}
	}
	if strings.Count(text, "# TYPE ") != strings.Count(text, "# HELP ") {
		t.Error("HELP/TYPE mismatch")
	}
}
