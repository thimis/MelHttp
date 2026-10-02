// Package testutil holds helpers shared by MelHttp's tests.
package testutil

import "os"

// StrictEnv is the environment variable that turns "this test needs a tool
// that is not installed" skips into failures. scripts/test.* -Full set it,
// so a full run either runs everything or says what is missing.
const StrictEnv = "MELHTTP_STRICT"

// TB is the part of testing.TB that Skip needs.
type TB interface {
	Helper()
	Skip(args ...any)
	Fatalf(format string, args ...any)
}

// Strict reports whether strict mode is on.
func Strict() bool { return os.Getenv(StrictEnv) == "1" }

// Skip skips the test because a prerequisite (git, node, Docker, …) is
// missing — or fails it in strict mode.
func Skip(t TB, reason string) {
	t.Helper()
	if Strict() {
		t.Fatalf("%s (required: %s=1 runs everything)", reason, StrictEnv)
		return
	}
	t.Skip(reason)
}
