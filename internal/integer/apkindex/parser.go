// Package apkindex parses the Wolfi APKINDEX format and discovers
// available package versions.
package apkindex

import (
	"bufio"
	"io"
	"strings"
)

// Package represents a single package entry from an APKINDEX file.
type Package struct {
	Name    string // P: field
	Version string // V: field (full apk version string, e.g. "22.16.0-r0")
}

// Parse reads an APKINDEX text stream and returns all package entries.
// The APKINDEX format is a sequence of stanzas separated by blank lines.
// Each stanza has lines of the form "KEY:VALUE". Only P: (name) and
// V: (version) fields are extracted.
func Parse(r io.Reader) ([]Package, error) {
	var packages []Package
	var current Package

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			// Blank line ends the current stanza.
			if current.Name != "" {
				packages = append(packages, current)
				current = Package{}
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch key {
		case "P":
			current.Name = value
		case "V":
			current.Version = value
		}
	}
	// Capture any trailing stanza without a trailing blank line.
	if current.Name != "" {
		packages = append(packages, current)
	}
	return packages, scanner.Err()
}
