package apkindex

import "strings"

func PackageName(packageSpec string) string {
	if index := strings.IndexAny(packageSpec, "@<>=~!"); index >= 0 {
		return packageSpec[:index]
	}
	return packageSpec
}
