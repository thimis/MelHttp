// Command crawl checks a running MelHttp server against a source tree, or
// load-tests a URL.
//
//	go run ./tools/crawl -base http://localhost:8080 -src testsites/classic
//	go run ./tools/crawl -load http://localhost:8080/ -n 20000 -c 32
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thimis/MelHttp/internal/crawl"
)

func main() {
	base := flag.String("base", "http://localhost:8080", "server base URL")
	src := flag.String("src", "", "source tree whose files must be served byte-for-byte")
	load := flag.String("load", "", "load-test this URL instead of crawling")
	n := flag.Int("n", 10000, "load test: number of requests")
	c := flag.Int("c", 32, "load test / crawl: concurrency")
	gzip := flag.Bool("gzip", false, "load test: send Accept-Encoding: gzip")
	flag.Parse()

	switch {
	case *load != "":
		os.Exit(loadTest(*load, *n, *c, *gzip))
	case *src != "":
		skip := func(rel string) bool { // hand-written programs are not content
			l := strings.ToLower(rel)
			return strings.HasSuffix(l, ".mb") || strings.Contains(l, ".mb/") || rel == "melhttp.json"
		}
		rep, err := crawl.Site(context.Background(), *base, *src, crawl.Options{Concurrency: *c, Skip: skip})
		if err != nil {
			fmt.Fprintln(os.Stderr, "crawl:", err)
			os.Exit(2)
		}
		fmt.Println(rep)
		if !rep.OK() || rep.Checked == 0 {
			os.Exit(1)
		}
	default:
		flag.Usage()
		os.Exit(2)
	}
}

func loadTest(url string, n, workers int, gz bool) int {
	tr := &http.Transport{MaxIdleConnsPerHost: workers, DisableCompression: true}
	client := &http.Client{Transport: tr, Timeout: 60 * time.Second}
	lat := make([]time.Duration, n)
	var next, failed atomic.Int64
	var bytes atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				i := next.Add(1) - 1
				if i >= int64(n) {
					return
				}
				req, _ := http.NewRequest(http.MethodGet, url, nil)
				if gz {
					req.Header.Set("Accept-Encoding", "gzip")
				}
				t0 := time.Now()
				resp, err := client.Do(req)
				if err != nil {
					failed.Add(1)
					lat[i] = time.Since(t0)
					continue
				}
				m, _ := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				lat[i] = time.Since(t0)
				bytes.Add(m)
				if resp.StatusCode != http.StatusOK {
					failed.Add(1)
				}
			}
		}()
	}
	wg.Wait()
	total := time.Since(start)
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	pct := func(p float64) time.Duration { return lat[min(len(lat)-1, int(p*float64(len(lat))))] }
	fmt.Printf("%d requests, %d workers, %v: %.0f req/s, %.1f MB/s\n", n, workers, total.Round(time.Millisecond),
		float64(n)/total.Seconds(), float64(bytes.Load())/total.Seconds()/1e6)
	fmt.Printf("latency p50 %v  p90 %v  p99 %v  max %v; failed %d\n", pct(.50), pct(.90), pct(.99), lat[len(lat)-1], failed.Load())
	if failed.Load() > 0 {
		return 1
	}
	return 0
}
