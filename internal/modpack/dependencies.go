package modpack

import (
	"context"
	"github.com/autoinst/AutoInstall/internal/download"
)

// Dependencies belong to one preparation session, never to a package-global wait group.
type Dependencies struct {
	Download download.Client
	CFAPIKey string
	Classify func(context.Context, string, string)
	Log      func(...any)
}

func (d Dependencies) log(args ...any) {
	if d.Log != nil {
		d.Log(args...)
	}
}
