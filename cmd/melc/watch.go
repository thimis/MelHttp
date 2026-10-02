package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/thimis/MelHttp/internal/build"
	"github.com/thimis/MelHttp/internal/server"
)

func cmdWatch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("melc watch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("o", "site", "output site `directory`")
	preset := fs.String("preset", "auto", "framework `name` (see melc build)")
	spa := fs.String("spa", "", "single-page app fallback: true or false (default: from the preset)")
	serve := fs.String("serve", "", "also serve the site on this `address` (e.g. :8080), reloading on every change")
	interval := fs.Duration("interval", 500*time.Millisecond, "how often to check for changes")
	fs.Usage = func() {
		fmt.Fprintln(stderr, "usage: melc watch [-o out-dir] [--preset name] [-serve :8080] <project-or-site-dir>")
		fmt.Fprintln(stderr, "Rebuilds whenever a file changes. For frameworks, run their own watcher (e.g. ng build --watch) alongside.")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fs.Usage()
		return exitUsage
	}
	project := fs.Arg(0)
	p, err := build.Resolve(*preset, project)
	if err != nil {
		fmt.Fprintln(stderr, "melc:", err)
		return exitUsage
	}
	opt := build.Options{Out: *out}
	switch *spa {
	case "":
		if p.Name != "static" {
			opt.SPA = &p.SPA
		}
	case "true", "false":
		v := *spa == "true"
		opt.SPA = &v
	default:
		fmt.Fprintln(stderr, "melc: --spa must be true or false")
		return exitUsage
	}

	var httpSrv *http.Server
	defer func() {
		if httpSrv != nil {
			httpSrv.Close()
		}
	}()
	var last [32]byte
	waiting := false
	for {
		src, err := p.OutputDir(project)
		if err != nil {
			if !waiting {
				fmt.Fprintf(stderr, "melc: waiting for build output (%v)\n", err)
				waiting = true
			}
		} else if snap, err := snapshot(src); err == nil && snap != last {
			waiting = false
			opt.Src = src
			st, err := build.Site(ctx, opt)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return exitOK
				}
				fmt.Fprintf(stderr, "melc: %s build failed: %v\n", time.Now().Format("15:04:05"), err)
			} else {
				last = snap
				fmt.Fprintf(stdout, "melc: %s %s\n", time.Now().Format("15:04:05"), st)
				if *serve != "" && httpSrv == nil {
					if httpSrv, err = startDevServer(*out, *serve, stdout, stderr); err != nil {
						fmt.Fprintln(stderr, "melc:", err)
						return exitError
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return exitOK
		case <-time.After(*interval):
		}
	}
}

// snapshot fingerprints a tree by names, sizes and modification times.
func snapshot(root string) ([32]byte, error) {
	h := sha256.New()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") && p != root && d.Name() != ".well-known" {
			return filepath.SkipDir
		}
		if fi, err := d.Info(); err == nil && !d.IsDir() {
			fmt.Fprintf(h, "%s|%d|%d\n", p, fi.Size(), fi.ModTime().UnixNano())
		}
		return nil
	})
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum, err
}

// startDevServer serves the site with immediate reloads (no warm-up, files
// re-checked on every request).
func startDevServer(root, addr string, stdout, stderr io.Writer) (*http.Server, error) {
	srv, err := server.New(server.Config{
		Root: root, Revalidate: -1,
		Logger: slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})
	if err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		srv.Close()
		return nil, err
	}
	hs := &http.Server{Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		hs.Serve(ln)
		srv.Close()
	}()
	fmt.Fprintf(stdout, "melc: serving %s on http://%s (Ctrl-C to stop)\n", root, ln.Addr())
	return hs, nil
}
