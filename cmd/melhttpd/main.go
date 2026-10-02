// Command melhttpd serves a site whose pages are Malbolge programs.
//
//	melc build ./my-site -o ./site
//	melhttpd -root ./site -addr :8080
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/thimis/MelHttp/internal/config"
	"github.com/thimis/MelHttp/internal/server"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr, nil)
	stop()
	os.Exit(code)
}

// run is main without process globals. If ready is non-nil it receives the
// listening address once the server accepts connections.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer, ready func(net.Addr)) int {
	cfg, err := config.Parse(args, getenv, stderr)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		fmt.Fprintln(stderr, "melhttpd:", err)
		return 2
	}
	if cfg.Version {
		fmt.Fprintln(stdout, "melhttpd", version)
		return 0
	}
	if cfg.Healthcheck {
		return healthcheck(ctx, cfg.HealthcheckURL(), stderr)
	}

	var handler slog.Handler = slog.NewTextHandler(stderr, nil)
	if cfg.LogFormat == "json" {
		handler = slog.NewJSONHandler(stderr, nil)
	}
	log := slog.New(handler)
	cfg.Server.Logger = log

	srv, err := server.New(cfg.Server)
	if err != nil {
		log.Error("cannot open site", "root", cfg.Server.Root, "error", err)
		return 1
	}
	defer srv.Close()
	if cfg.Server.Warm {
		st := srv.Warm(ctx)
		log.Info("warm-up", "programs", st.Programs, "cached", st.Cached, "dynamic", st.Dynamic,
			"failed", st.Failed, "steps", st.Steps, "bytes", st.Bytes, "duration", st.Duration.Round(time.Millisecond))
	}

	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		log.Error("cannot listen", "addr", cfg.Addr, "error", err)
		return 1
	}
	hs := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      cfg.Server.Timeout + 60*time.Second, // longer than any program run
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(handler, slog.LevelWarn),
	}
	log.Info("melhttpd listening", "version", version, "addr", ln.Addr().String(), "root", cfg.Server.Root,
		"spa", srv.SPA(), "cache", !cfg.Server.NoCache)
	if ready != nil {
		ready(ln.Addr())
	}
	errc := make(chan error, 1)
	go func() { errc <- hs.Serve(ln) }()
	select {
	case err := <-errc:
		log.Error("server stopped", "error", err)
		return 1
	case <-ctx.Done():
	}
	log.Info("shutting down")
	sctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := hs.Shutdown(sctx); err != nil {
		log.Error("shutdown", "error", err)
		return 1
	}
	return 0
}

// healthcheck probes a running server, for container health checks (the
// distroless image has no shell or curl).
func healthcheck(ctx context.Context, url string, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintln(stderr, "unhealthy:", err)
		return 1
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(stderr, "unhealthy: status", resp.StatusCode)
		return 1
	}
	return 0
}
