package pkg

import (
	"github.com/autoinst/AutoInstall/core"
	"github.com/autoinst/AutoInstall/internal/modpack"
)

// Compatibility facade; the adapter implementation has no core/pkg dependency.
type PackInfo = modpack.PackInfo

func DiscoverPack(root, input string) (*PackInfo, error) {
	return modpack.DiscoverPack(root, input, modpack.Dependencies{
		Download: core.DownloadClient(core.HTTPClient), CFAPIKey: curseForgeAPIKey(),
		Classify: runInstalledModFilterAt, Log: core.Log,
	})
}
