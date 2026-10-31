package pkg

import "github.com/autoinst/AutoInstall/internal/loader"

const defaultMavenBase = loader.DefaultMavenBase

// Keep existing loader call sites compatible while the pure Maven logic lives below pkg.
func mavenRelPathFromName(name string) string {
	return loader.MavenRelPathFromName(name)
}
