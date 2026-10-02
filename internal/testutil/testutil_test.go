package testutil

import (
	"fmt"
	"testing"
)

type fakeTB struct{ skipped, failed string }

func (f *fakeTB) Helper()                        {}
func (f *fakeTB) Skip(args ...any)               { f.skipped = fmt.Sprint(args...) }
func (f *fakeTB) Fatalf(format string, a ...any) { f.failed = fmt.Sprintf(format, a...) }

func TestSkipNormally(t *testing.T) {
	t.Setenv(StrictEnv, "")
	f := &fakeTB{}
	Skip(f, "docker not running")
	if f.skipped != "docker not running" || f.failed != "" {
		t.Fatalf("%+v", f)
	}
}

func TestFailInStrictMode(t *testing.T) {
	t.Setenv(StrictEnv, "1")
	f := &fakeTB{}
	Skip(f, "node not installed")
	if f.skipped != "" || f.failed == "" || !Strict() {
		t.Fatalf("%+v", f)
	}
}
