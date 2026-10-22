// Package release provides the release notes shipped with the application.
package release

import "embed"

//go:embed RELEASE-*.md
var notes embed.FS

// Read returns the notes for the exact application version, without a network request.
func Read(version string) (string, error) {
	data, err := notes.ReadFile("RELEASE-" + version + ".md")
	return string(data), err
}
