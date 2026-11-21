// Package version holds the release version, set at build time:
//
//	go build -ldflags "-X github.com/andriantam/ace-mate/internal/version.Version=27.0.0"
package version

// Version is a calendar version, YEAR.MAJOR.MINOR (see the README). The default matches the
// latest release in CHANGELOG.md.
var Version = "27.0.0"
