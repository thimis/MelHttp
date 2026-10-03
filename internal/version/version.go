// Package version holds the MelHttp release number, the single place it is
// set. Release builds (tools/dist, the Docker image) stamp the exact version
// into the binaries; a plain "go build" reports Number plus "-dev".
//
// MelHttp is in alpha while the version is 0.x: flags, the site layout and
// the MelCGI contract may still change between releases.
package version

// Number is the current release, without the "v" prefix of its git tag.
const Number = "0.1.0"

// Tag is the git tag of the current release.
const Tag = "v" + Number

// Dev is the version a plain "go build" reports.
const Dev = Number + "-dev"
