package hygiene

import "testing"

func TestCheckPath(t *testing.T) {
	bad := []string{
		".claude/settings.json", "CLAUDE.md", "sub/CLAUDE.local.md", ".mcp.json",
		".env", "app/.env.production", "id_rsa", "home/id_ed25519.pub", "server.key",
		"tls/cert.pem", "a.pfx", "web/node_modules/x/index.js", "secrets/db.txt",
		"certs/ca.crt", "autocert-cache/acme", ".npmrc", "credentials.json", `win\path\.env`,
	}
	good := []string{
		".env.example", "README.md", "docs/claude-notes-not.md", "go.mod",
		"internal/hygiene/hygiene.go", "testsites/classic/keyboard.html", "monkey.txt",
		"testdata/corpus/font.woff2", "environment.ts",
	}
	for _, p := range bad {
		if CheckPath(p) == "" {
			t.Errorf("CheckPath(%q) allowed a forbidden file", p)
		}
	}
	for _, p := range good {
		if r := CheckPath(p); r != "" {
			t.Errorf("CheckPath(%q) = %q, want allowed", p, r)
		}
	}
}

func TestCheckContent(t *testing.T) {
	// Built by concatenation so this file itself never contains a key header.
	const begin, pk = "-----BEGIN ", "PRIVATE KEY-----"
	bad := []string{
		begin + pk + "\nabc",
		"x " + begin + "RSA " + pk + " y",
		begin + "OPENSSH " + pk,
		"aws=AKIA" + "ABCDEFGHIJKLMNOP",
		"token: ghp_" + "abcdefghijklmnopqrstuvwxyz0123456789AB",
		"key=sk-ant-" + "api03-abcdefghijklmnopqrstuvwxyz",
	}
	for _, s := range bad {
		if CheckContent([]byte(s)) == "" {
			t.Errorf("CheckContent(%q) missed a secret", s)
		}
	}
	good := []string{"package main", "-----BEGIN CERTIFICATE-----", "PRIVATE KEY is a phrase", ""}
	for _, s := range good {
		if r := CheckContent([]byte(s)); r != "" {
			t.Errorf("CheckContent(%q) = %q", s, r)
		}
	}
	if CheckContent([]byte("\x00"+begin+pk)) != "" {
		t.Error("binary content should be skipped")
	}
}
