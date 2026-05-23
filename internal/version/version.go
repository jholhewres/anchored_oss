// Package version holds the build-time server version.
package version

// Version is set via:
//
//	-ldflags "-X github.com/jholhewres/anchored_oss/internal/version.Version=v1.2.3"
//
// at build time. Defaults to "dev" for local development.
var Version = "dev"
