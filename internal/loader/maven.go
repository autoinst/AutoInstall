// Package loader contains loader metadata helpers without installation or network dependencies.
package loader

import (
	"path/filepath"
	"strings"
)

const DefaultMavenBase = "https://repo1.maven.org/maven2/"

// MavenRelPathFromName preserves the legacy group:artifact:version path convention.
// Short names are returned unchanged for callers that already pass relative paths.
// This compatibility helper does not classify modern artifacts or validate paths.
func MavenRelPathFromName(name string) string {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return name
	}
	group := strings.ReplaceAll(parts[0], ".", "/")
	artifact, version := parts[1], parts[2]
	return filepath.ToSlash(filepath.Join(group, artifact, version, artifact+"-"+version+".jar"))
}
