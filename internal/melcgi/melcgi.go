// Package melcgi implements MelCGI/1.0, the CGI-like contract between the
// HTTP server and a Malbolge program. A program reads a block of
// "NAME=value" meta-variable lines, a blank line and the request body on
// stdin, and prints a CGI response (header lines, a blank line, the body)
// on stdout. The full specification is docs/melcgi.md.
//
// Request encoding never trusts the client: every meta value is forced onto
// one line, hop-by-hop and spoofable headers are dropped. Response parsing
// never trusts the program: only an allowlist of headers is accepted, and
// anything malformed is an error the server turns into 502 Bad Gateway.
package melcgi

// GatewayInterface is the value of the GATEWAY_INTERFACE meta-variable.
const GatewayInterface = "MelCGI/1.0"
