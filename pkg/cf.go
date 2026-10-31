package pkg

import (
	"os"
	"strings"

	"github.com/autoinst/AutoInstall/internal/modpack"
)

// Retained for legacy private builds; public releases never embed credentials.
var cfapiKey string

func curseForgeAPIKey() string {
	if key, ok := os.LookupEnv("CF_API_KEY"); ok {
		return strings.TrimSpace(key)
	}
	return cfapiKey
}

type CurseForgeManifest = modpack.CurseForgeManifest
type CFError = modpack.CFError
