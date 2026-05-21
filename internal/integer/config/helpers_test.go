package config

import (
	"slices"
	"strings"
)

func pathHasEntry(path, want string) bool {
	return slices.Contains(strings.Split(path, ":"), want)
}

func findPath(paths []PathDef, want string) (PathDef, bool) {
	for _, p := range paths {
		if p.Path == want {
			return p, true
		}
	}
	return PathDef{}, false
}
