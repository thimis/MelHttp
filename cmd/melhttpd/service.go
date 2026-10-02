package main

import (
	"path/filepath"
	"strings"
)

// serviceArgs prepares the flags stored in a Windows service definition: the
// -service and -service-name flags are dropped, and path flags become absolute
// (services start in C:\Windows\System32, not where you installed them from).
func serviceArgs(args []string) []string {
	pathFlags := map[string]bool{
		"root": true, "log-file": true, "tls-cert": true, "tls-key": true, "acme-cache": true, "acme-ca-root": true,
	}
	skip := map[string]bool{"service": true, "service-name": true}
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		name, value, hasValue := strings.Cut(strings.TrimLeft(a, "-"), "=")
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
			continue
		}
		takesValue := !hasValue && i+1 < len(args) && !isBoolFlag(name)
		if skip[name] {
			if takesValue {
				i++
			}
			continue
		}
		if pathFlags[name] {
			if !hasValue && i+1 < len(args) {
				i++
				value = args[i]
			}
			if abs, err := filepath.Abs(value); err == nil {
				value = abs
			}
			out = append(out, "-"+name+"="+value)
			continue
		}
		out = append(out, a)
	}
	return out
}

// isBoolFlag lists melhttpd's boolean flags (they take no separate value).
func isBoolFlag(name string) bool {
	switch name {
	case "spa", "expose-source", "no-cache", "warm", "allow-sensitive-headers", "healthcheck", "version",
		"tls-self-signed", "https-redirect", "obfuscate", "playground", "wasi":
		return true
	}
	return false
}
