// Package hygiene decides whether a file is safe to commit to the public repo.
// It is used by tools/precommit.
package hygiene

import (
	"bytes"
	"path"
	"regexp"
	"strings"
)

var forbiddenPath = regexp.MustCompile(`(?i)(^|/)(` +
	`\.claude/|claude(\.local)?\.md$|\.mcp\.json$|` + // Claude Code files
	`\.env$|\.env\.[^/]*$|` + // environment files
	`id_(rsa|dsa|ecdsa|ed25519)[^/]*$|` + // SSH keys
	`node_modules/|\.npmrc$|\.netrc$|credentials[^/]*\.json$|` +
	`secrets?/|certs/|autocert-cache/` +
	`)|\.(pem|key|p12|pfx|jks|keystore|kdbx|crt|cer|csr)$`)

var secretContent = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN ([A-Z0-9]+ )*PRIVATE KEY-----`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),           // AWS access key id
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,}\b`), // GitHub tokens
	regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_-]{20,}`),    // Anthropic API keys
	regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}`), // Slack tokens
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),      // Google API keys
	regexp.MustCompile(`(?i)\bnpm_[A-Za-z0-9]{36}\b`),    // npm tokens
}

// CheckPath returns a reason if a repo-relative slash path must not be committed.
func CheckPath(p string) string {
	p = strings.TrimPrefix(path.Clean(strings.ReplaceAll(p, "\\", "/")), "./")
	if strings.HasSuffix(p, ".env.example") {
		return ""
	}
	if forbiddenPath.MatchString(p) {
		return "forbidden file name (secret, key or local tooling file)"
	}
	return ""
}

// CheckContent returns a reason if file content looks like it contains a secret.
// Binary content (NUL bytes) is skipped.
func CheckContent(data []byte) string {
	if bytes.IndexByte(data, 0) >= 0 {
		return ""
	}
	for _, re := range secretContent {
		if re.Match(data) {
			return "content matches secret pattern " + re.String()
		}
	}
	return ""
}
