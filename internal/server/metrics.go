package server

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"
)

// metrics are Prometheus-style counters, kept with atomics (no client
// library: stdlib only).
type metrics struct {
	requests  [6]atomic.Int64 // by status class: index 1..5 = 1xx..5xx
	durations [len(durationBuckets) + 1]atomic.Int64
	durSumNs  atomic.Int64
	vmSteps   atomic.Int64
	wasiRuns  atomic.Int64
	hits      atomic.Int64
	misses    atomic.Int64
	encoded   atomic.Int64 // transport responses
	failures  atomic.Int64 // program/handler errors (5xx from execution)
}

var durationBuckets = [...]float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

func (m *metrics) observe(status int, d time.Duration) {
	if c := status / 100; c >= 1 && c <= 5 {
		m.requests[c].Add(1)
	}
	m.durSumNs.Add(int64(d))
	sec := d.Seconds()
	for i, b := range durationBuckets {
		if sec <= b {
			m.durations[i].Add(1)
			return
		}
	}
	m.durations[len(durationBuckets)].Add(1)
}

// MetricsHandler serves metrics in the Prometheus text format. Mount it on a
// private listener (melhttpd -metrics-addr), not on the public site.
func (s *Server) MetricsHandler(version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		s.writeMetrics(w, version)
	})
}

func (s *Server) writeMetrics(w io.Writer, version string) {
	m := &s.metrics
	counter := func(name, help string, v int64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n", name, help, name, name, v)
	}
	gauge := func(name, help string, v int64) {
		fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n", name, help, name, name, v)
	}
	fmt.Fprintf(w, "# HELP melhttp_build_info Build information.\n# TYPE melhttp_build_info gauge\nmelhttp_build_info{version=%q} 1\n", version)

	fmt.Fprint(w, "# HELP melhttp_requests_total HTTP requests by status class.\n# TYPE melhttp_requests_total counter\n")
	for c := 1; c <= 5; c++ {
		fmt.Fprintf(w, "melhttp_requests_total{code=\"%dxx\"} %d\n", c, m.requests[c].Load())
	}
	fmt.Fprint(w, "# HELP melhttp_request_duration_seconds Time to answer requests.\n# TYPE melhttp_request_duration_seconds histogram\n")
	var cum int64
	for i, b := range durationBuckets {
		cum += m.durations[i].Load()
		fmt.Fprintf(w, "melhttp_request_duration_seconds_bucket{le=\"%g\"} %d\n", b, cum)
	}
	cum += m.durations[len(durationBuckets)].Load()
	fmt.Fprintf(w, "melhttp_request_duration_seconds_bucket{le=\"+Inf\"} %d\n", cum)
	fmt.Fprintf(w, "melhttp_request_duration_seconds_sum %g\n", time.Duration(m.durSumNs.Load()).Seconds())
	fmt.Fprintf(w, "melhttp_request_duration_seconds_count %d\n", cum)

	counter("melhttp_vm_runs_total", "Malbolge program executions.", s.runs.Load()-m.wasiRuns.Load())
	counter("melhttp_vm_steps_total", "Malbolge instructions executed.", m.vmSteps.Load())
	counter("melhttp_wasi_runs_total", "WebAssembly (WASI) handler executions.", m.wasiRuns.Load())
	counter("melhttp_cache_hits_total", "Responses served from the cache.", m.hits.Load())
	counter("melhttp_cache_misses_total", "Responses that had to be produced.", m.misses.Load())
	counter("melhttp_transport_responses_total", "Responses sent as Malbolge programs (-obfuscate).", m.encoded.Load())
	counter("melhttp_execution_errors_total", "Program or handler failures (timeouts, limits, invalid output).", m.failures.Load())
	s.cache.mu.Lock()
	entries, bytes := len(s.cache.items), s.cache.used
	s.cache.mu.Unlock()
	gauge("melhttp_cache_entries", "Responses in the cache.", int64(entries))
	gauge("melhttp_cache_bytes", "Approximate size of the cache.", bytes)
	gauge("melhttp_vm_inflight", "Programs running right now.", int64(len(s.sem)))
	gauge("melhttp_site_urls", "URLs in the site index.", int64(len(s.index.all())))
}
