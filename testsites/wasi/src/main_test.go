package main

import (
	"strings"
	"testing"
	"time"
)

func TestHandle(t *testing.T) {
	req := "REQUEST_METHOD=POST\nQUERY_STRING=x=<script>alert(1)</script>\nHTTP_X_TEST=yes\n\nmessage=<b>hi</b>"
	var out strings.Builder
	handle(strings.NewReader(req), &out, time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC))
	got := out.String()
	head, body, ok := strings.Cut(got, "\n\n")
	if !ok || head != "Content-Type: text/html; charset=utf-8" {
		t.Fatalf("header block %q", head)
	}
	for _, want := range []string{
		"<h1>Hello from WASI</h1>",
		"Server time: 2026-10-02T12:00:00Z",
		"<tr><td>REQUEST_METHOD</td><td>POST</td></tr>",
		"<tr><td>HTTP_X_TEST</td><td>yes</td></tr>",
		"x=&lt;script&gt;alert(1)&lt;/script&gt;", // request values are escaped: no XSS
		"<pre>message=&lt;b&gt;hi&lt;/b&gt;</pre>",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<script>") {
		t.Error("unescaped script tag in the page")
	}
}

func TestHandleEmptyRequest(t *testing.T) {
	var out strings.Builder
	handle(strings.NewReader(""), &out, time.Now())
	if !strings.HasPrefix(out.String(), "Content-Type: text/html") || strings.Contains(out.String(), "<h2>Body</h2>") {
		t.Fatalf("empty request: %q", out.String())
	}
}
