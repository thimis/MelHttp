// Package wasi runs WebAssembly (WASI preview 1) modules as MelCGI programs:
// the request arrives on stdin, the response leaves on stdout, exactly like
// a Malbolge program. Modules are sandboxed by wazero, a pure-Go runtime:
// no file system, no environment, no network, bounded memory and time.
package wasi

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// Magic is the first bytes of every WebAssembly binary.
const Magic = "\x00asm"

var (
	// ErrTimeout means the module ran past its deadline.
	ErrTimeout = errors.New("wasi: module exceeded its time limit")
	// ErrOutputLimit means the module wrote too much.
	ErrOutputLimit = errors.New("wasi: output limit exceeded")
)

// ExitError reports a non-zero exit status.
type ExitError struct {
	Code   uint32
	Stderr string
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("wasi: module exited with status %d: %s", e.Code, e.Stderr)
}

// Runner compiles and runs modules. It is safe for concurrent use.
type Runner struct {
	rt wazero.Runtime

	mu      sync.Mutex
	modules map[string]compiled // by key
}

type compiled struct {
	version string
	mod     wazero.CompiledModule
}

// New creates a runner; each module instance may use at most memoryMB MiB.
func New(ctx context.Context, memoryMB int) (*Runner, error) {
	if memoryMB <= 0 {
		memoryMB = 64
	}
	cfg := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(uint32(memoryMB) * 16) // 64 KiB pages
	rt := wazero.NewRuntimeWithConfig(ctx, cfg)
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		rt.Close(ctx)
		return nil, err
	}
	return &Runner{rt: rt, modules: map[string]compiled{}}, nil
}

// Close releases all compiled modules.
func (r *Runner) Close(ctx context.Context) error { return r.rt.Close(ctx) }

// compile returns the compiled module for key, recompiling when version changes.
func (r *Runner) compile(ctx context.Context, key, version string, load func() ([]byte, error)) (wazero.CompiledModule, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.modules[key]; ok && c.version == version {
		return c.mod, nil
	}
	code, err := load()
	if err != nil {
		return nil, err
	}
	mod, err := r.rt.CompileModule(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("wasi: %w", err)
	}
	if old, ok := r.modules[key]; ok {
		old.mod.Close(ctx)
	}
	r.modules[key] = compiled{version, mod}
	return mod, nil
}

// Run executes the module identified by key (recompiled whenever version
// changes; load supplies its bytes) with stdin and stdout. A module that
// exits non-zero returns *ExitError; output beyond maxOutput bytes fails
// with ErrOutputLimit.
func (r *Runner) Run(ctx context.Context, key, version string, load func() ([]byte, error),
	stdin io.Reader, stdout io.Writer, maxOutput int64) error {
	mod, err := r.compile(ctx, key, version, load)
	if err != nil {
		return err
	}
	if stdin == nil {
		stdin = eofReader{}
	}
	// Exceeding the output limit cancels the run, which terminates the module
	// even if it ignores write errors.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := &limitWriter{w: stdout, n: maxOutput, onExceed: cancel}
	stderr := &limitWriter{w: &stderrBuf{}, n: 4096}
	cfg := wazero.NewModuleConfig().
		WithName("").
		WithArgs(key).
		WithStdin(stdin).
		WithStdout(out).
		WithStderr(stderr).
		WithSysWalltime().
		WithSysNanotime().
		WithSysNanosleep().
		WithRandSource(rand.Reader)
	inst, err := r.rt.InstantiateModule(ctx, mod, cfg)
	if inst != nil {
		inst.Close(ctx)
	}
	if out.exceeded {
		return ErrOutputLimit
	}
	var exit *sys.ExitError
	if errors.As(err, &exit) {
		switch exit.ExitCode() {
		case 0:
			return nil
		case sys.ExitCodeDeadlineExceeded, sys.ExitCodeContextCanceled:
			return ErrTimeout
		}
		return &ExitError{Code: exit.ExitCode(), Stderr: stderr.w.(*stderrBuf).String()}
	}
	if err != nil && ctx.Err() != nil {
		return ErrTimeout
	}
	return err
}

type eofReader struct{}

func (eofReader) Read([]byte) (int, error) { return 0, io.EOF }

type limitWriter struct {
	w        io.Writer
	n        int64
	exceeded bool
	onExceed func()
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n > 0 && int64(len(p)) > l.n {
		l.exceeded = true
		if l.onExceed != nil {
			l.onExceed()
		}
		return 0, ErrOutputLimit
	}
	if l.n > 0 {
		l.n -= int64(len(p))
	}
	return l.w.Write(p)
}

type stderrBuf struct{ b []byte }

func (s *stderrBuf) Write(p []byte) (int, error) { s.b = append(s.b, p...); return len(p), nil }
func (s *stderrBuf) String() string              { return string(s.b) }
